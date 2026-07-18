// Package proxy маршрутизирует вызовы LLM через HTTP SSE.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/stelmakhdigital/stell-ai"
	"github.com/stelmakhdigital/stell-agent"
)

// Options настраивают StreamProxy.
type Options struct {
	ProxyURL   string
	AuthToken  string
	HTTPClient *http.Client
}

// StreamProxy возвращает StreamFn, который POST-ит на {proxyURL}/api/stream и преобразует
// SSE-события прокси → ai.ChatEvent.
func StreamProxy(opts Options) agent.StreamFn {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(strings.TrimSpace(opts.ProxyURL), "/")
	token := strings.TrimSpace(opts.AuthToken)
	return func(ctx context.Context, req ai.ChatRequest) (<-chan ai.ChatEvent, error) {
		if base == "" {
			return nil, fmt.Errorf("proxy: empty proxy URL")
		}
		body, err := json.Marshal(proxyRequest{
			Model:    req.Model,
			Messages: req.Messages,
			Tools:    req.Tools,
			Options: proxyStreamOpts{
				MaxTokens:     req.MaxTokens,
				SessionID:     req.SessionID,
				Reasoning:     req.ThinkingLevel,
				ThinkingBudget: req.ThinkingBudget,
			},
		})
		if err != nil {
			return nil, err
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/stream", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if token != "" {
			httpReq.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			msg := fmt.Sprintf("Proxy error: %d %s", resp.StatusCode, resp.Status)
			var errBody struct {
				Error string `json:"error"`
			}
			if json.NewDecoder(resp.Body).Decode(&errBody) == nil && errBody.Error != "" {
				msg = "Proxy error: " + errBody.Error
			}
			return nil, fmt.Errorf("%s", msg)
		}
		ch := make(chan ai.ChatEvent, 32)
		go func() {
			defer close(ch)
			defer resp.Body.Close()
			if err := readProxySSE(ctx, resp.Body, ch); err != nil {
				select {
				case ch <- ai.ChatEvent{Type: ai.EventError, Err: err}:
				case <-ctx.Done():
				}
			}
		}()
		return ch, nil
	}
}

type proxyRequest struct {
	Model    string           `json:"model"`
	Messages []ai.Message     `json:"messages"`
	Tools    []ai.ToolDef     `json:"tools,omitempty"`
	Options  proxyStreamOpts  `json:"options"`
}

type proxyStreamOpts struct {
	MaxTokens      int    `json:"maxTokens,omitempty"`
	SessionID      string `json:"sessionId,omitempty"`
	Reasoning      string `json:"reasoning,omitempty"`
	ThinkingBudget int    `json:"thinkingBudget,omitempty"`
}

type proxyEvent struct {
	Type         string          `json:"type"`
	ContentIndex int             `json:"contentIndex"`
	Delta        string          `json:"delta"`
	ID           string          `json:"id"`
	ToolName     string          `json:"toolName"`
	Reason       string          `json:"reason"`
	ErrorMessage string          `json:"errorMessage"`
	Usage        *ai.Usage       `json:"usage"`
	Raw          json.RawMessage `json:"-"`
}

func readProxySSE(ctx context.Context, r io.Reader, ch chan<- ai.ChatEvent) error {
	sc := bufio.NewScanner(r)
	// Большие JSON-дельты вызовов инструментов.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	type toolAcc struct {
		id, name, json string
		index          int
	}
	tools := map[int]*toolAcc{}

	send := func(ev ai.ChatEvent) bool {
		select {
		case <-ctx.Done():
			return false
		case ch <- ev:
			return true
		}
	}

	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(line[6:])
		if data == "" || data == "[DONE]" {
			continue
		}
		var pe proxyEvent
		if err := json.Unmarshal([]byte(data), &pe); err != nil {
			continue
		}
		switch pe.Type {
		case "start", "text_start", "text_end", "thinking_start", "thinking_end":
			// no-op для провода stell ChatEvent.
		case "text_delta":
			if pe.Delta != "" && !send(ai.ChatEvent{Type: ai.EventToken, Token: pe.Delta}) {
				return ctx.Err()
			}
		case "thinking_delta":
			if pe.Delta != "" && !send(ai.ChatEvent{Type: ai.EventThinking, Token: pe.Delta}) {
				return ctx.Err()
			}
		case "toolcall_start":
			tools[pe.ContentIndex] = &toolAcc{id: pe.ID, name: pe.ToolName, index: pe.ContentIndex}
			if !send(ai.ChatEvent{
				Type:          ai.EventToolCallDelta,
				ToolCallIndex: pe.ContentIndex,
				ToolCallID:    pe.ID,
				ToolCallName:  pe.ToolName,
			}) {
				return ctx.Err()
			}
		case "toolcall_delta":
			acc := tools[pe.ContentIndex]
			if acc == nil {
				acc = &toolAcc{index: pe.ContentIndex}
				tools[pe.ContentIndex] = acc
			}
			acc.json += pe.Delta
			if !send(ai.ChatEvent{
				Type:          ai.EventToolCallDelta,
				ToolCallDelta: pe.Delta,
				ToolCallIndex: pe.ContentIndex,
				ToolCallID:    acc.id,
				ToolCallName:  acc.name,
			}) {
				return ctx.Err()
			}
		case "toolcall_end":
			acc := tools[pe.ContentIndex]
			if acc == nil {
				continue
			}
			args := map[string]any{}
			if acc.json != "" {
				_ = json.Unmarshal([]byte(acc.json), &args)
			}
			tc := &ai.ToolCall{ID: acc.id, Name: acc.name, Args: args}
			if !send(ai.ChatEvent{Type: ai.EventToolCall, ToolCall: tc}) {
				return ctx.Err()
			}
		case "done":
			reason := mapStopReason(pe.Reason)
			if !send(ai.ChatEvent{Type: ai.EventDone, StopReason: reason, Usage: pe.Usage}) {
				return ctx.Err()
			}
			return nil
		case "error":
			msg := pe.ErrorMessage
			if msg == "" {
				msg = "proxy error"
			}
			if !send(ai.ChatEvent{Type: ai.EventError, Err: fmt.Errorf("%s", msg)}) {
				return ctx.Err()
			}
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// Поток завершился без done — синтезируем completed.
	_ = send(ai.ChatEvent{Type: ai.EventDone, StopReason: "completed"})
	return nil
}

func mapStopReason(r string) string {
	switch r {
	case "stop", "":
		return "completed"
	case "toolUse":
		return "toolUse"
	case "length":
		return "length"
	default:
		return r
	}
}
