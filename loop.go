package agent

import (
	"context"
	"fmt"

	"github.com/stelmakhdigital/stell-ai"
	"github.com/stelmakhdigital/stell-ai/provider"
	"github.com/stelmakhdigital/stell-agent/session"
	"github.com/stelmakhdigital/stell-agent/tools"
)

// Loop — низкоуровневый исполнитель хода агента.
// Оркестрация продукта (расширения, discovery, сохранение настроек) остаётся
// в coding-agent и заполняет эти поля перед вызовом Run.
type Loop struct {
	Registry  *provider.Registry
	Tools     *tools.Runtime
	Sessions  *session.Manager
	ModelName string
	ModelID   string

	BuildSystem      func(ctx context.Context) string
	ConvertMessages  func(messages []ai.Message) []ai.Message
	TransformContext func(ctx context.Context, messages []ai.Message) []ai.Message
	MaxIterations    int
	SteerFn          func() (message string, ok bool)
	SteerMessage     SteerMessage
	ToolExecution    ToolExecutionMode

	BeforeToolCall func(ctx context.Context, call ai.ToolCall) (block bool, args map[string]any)
	AfterToolCall  func(ctx context.Context, call ai.ToolCall, outcome ToolCallOutcome) ToolCallOutcome
	PrepareChat    func(ctx context.Context, iter int, base ai.ChatRequest) ai.ChatRequest
	OnToolProgress func(call ai.ToolCall, partial string)

	StreamFn           StreamFn
	ProcessStream      ProcessStreamFn
	PrepareNextTurn    PrepareNextTurn
	AfterAssistantTurn AfterAssistantTurn
	ShouldStopAfterTurn ShouldStopAfterTurn
	FollowUpMessage    FollowUpMessage
	AfterTools         AfterTools
	AfterChatError     func(ctx context.Context, err error) (retry bool, notice string)

	// SkipUserAppend — вызывающий уже добавил user-сообщение.
	SkipUserAppend bool

	// SuppressMaxIterationsDone подавляет EventDone при достижении лимита итераций,
	// чтобы вызывающий мог выполнить финальный ход (продуктовый путь).
	SuppressMaxIterationsDone bool

	// LastStopReason устанавливается при выходе из цикла (для продуктового postamble).
	LastStopReason string

	// pendingDone* — при выходе inner через terminate (done откладывается до follow-up).
	pendingDoneUsage *ai.Usage
	pendingDoneSkip  bool
}

// Run выполняет один user-prompt через LLM и инструменты, пока модель не перестанет
// вызывать инструменты или не будет достигнут MaxIterations. События отправляются
// в ch; ch закрывается при возврате Run.
func (l *Loop) Run(ctx context.Context, prompt string, ch chan<- Event) {
	defer close(ch)
	l.run(ctx, prompt, ch)
}

// RunContinue продолжает цикл без нового user-сообщения.
// Последнее сообщение сессии не должно быть assistant, ожидающим инструменты —
// обычно это tool result.
func (l *Loop) RunContinue(ctx context.Context, ch chan<- Event) {
	defer close(ch)
	if err := l.ContinuePrepared(ctx, ch); err != nil {
		l.finish(ch, "error", err)
	}
}

// ContinuePrepared — как RunContinue, но не закрывает ch.
func (l *Loop) ContinuePrepared(ctx context.Context, ch chan<- Event) error {
	l.SkipUserAppend = true
	if l.Sessions != nil {
		if msgs := l.Sessions.BuildMessages(); len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			if last.Role == ai.RoleAssistant && len(last.ToolCalls) > 0 {
				return fmt.Errorf("loop: RunContinue requires tool results after pending tool calls")
			}
		}
	}
	l.run(ctx, "", ch)
	return nil
}

// RunPrepared — как Run, но не закрывает ch (канал принадлежит вызывающему).
func (l *Loop) RunPrepared(ctx context.Context, prompt string, ch chan<- Event) {
	l.run(ctx, prompt, ch)
}

func (l *Loop) run(ctx context.Context, prompt string, ch chan<- Event) {
	l.LastStopReason = ""
	l.pendingDoneUsage = nil
	l.pendingDoneSkip = false
	if l.Sessions == nil || l.Registry == nil {
		l.finish(ch, "error", fmt.Errorf("loop: missing session or registry"))
		return
	}
	if !l.SkipUserAppend && prompt != "" {
		user := ai.Message{Role: ai.RoleUser, Content: prompt}
		if _, err := l.Sessions.AppendMessage(user); err != nil {
			l.finish(ch, "error", err)
			return
		}
		ch <- Event{Type: EventMessage, Message: user}
	}

	prov, _, ok := l.Registry.Get(l.ModelName)
	if !ok {
		l.finish(ch, "error", fmt.Errorf("model %q not registered", l.ModelName))
		return
	}

	system := ""
	if l.BuildSystem != nil {
		system = l.BuildSystem(ctx)
	}
	mode := l.ToolExecution
	if mode == "" {
		mode = ToolExecutionParallel
	}
	max := l.MaxIterations

	for {
		exitOuter := l.runInner(ctx, prov, system, mode, max, ch)
		if exitOuter {
			return
		}
		if _, ok := l.drainFollowUp(ch); ok {
			l.pendingDoneUsage = nil
			l.pendingDoneSkip = false
			continue
		}
		// Inner завершился через terminate без follow-up — отправляем отложенный done.
		if l.LastStopReason == "" {
			l.LastStopReason = "completed"
		}
		if !l.pendingDoneSkip {
			ch <- Event{Type: EventDone, Usage: l.pendingDoneUsage, StopReason: l.LastStopReason}
		}
		return
	}
}

// runInner возвращает true, если внешний цикл должен завершиться (done/error/cancel).
func (l *Loop) runInner(ctx context.Context, prov ai.Provider, system string, mode ToolExecutionMode, max int, ch chan<- Event) bool {
	for iter := 0; max <= 0 || iter < max; iter++ {
		if ctx.Err() != nil {
			l.LastStopReason = "cancelled"
			ch <- Event{Type: EventDone, StopReason: "cancelled"}
			return true
		}

		if l.PrepareNextTurn != nil {
			if err := l.PrepareNextTurn(ctx, iter); err != nil {
				l.finish(ch, "error", err)
				return true
			}
		}

		msgs := l.Sessions.BuildMessages()
		if l.TransformContext != nil {
			msgs = l.TransformContext(ctx, msgs)
		}
		if l.ConvertMessages != nil {
			msgs = l.ConvertMessages(msgs)
		} else {
			msgs = ConvertToLlm(msgs)
		}
		if system != "" {
			msgs = append([]ai.Message{{Role: ai.RoleSystem, Content: system}}, msgs...)
		}
		model := l.ModelID
		if model == "" {
			model = l.ModelName
		}
		var toolDefs []ai.ToolDef
		if l.Tools != nil {
			toolDefs = l.Tools.Defs()
		}
		req := ai.ChatRequest{Model: model, Messages: msgs, Tools: toolDefs}
		if l.PrepareChat != nil {
			req = l.PrepareChat(ctx, iter, req)
		}

		stream, err := l.chat(ctx, prov, req)
		if err != nil {
			if l.AfterChatError != nil {
				if retry, notice := l.AfterChatError(ctx, err); retry {
					if notice != "" {
						ch <- Event{Type: EventNotice, Notice: notice}
					}
					continue
				}
			}
			l.finish(ch, "error", err)
			return true
		}

		turn, err := l.pumpStream(ctx, stream, ch)
		if err != nil {
			l.LastStopReason = "error"
			return true
		}
		if ctx.Err() != nil {
			l.LastStopReason = "cancelled"
			ch <- Event{Type: EventDone, StopReason: "cancelled"}
			return true
		}

		if !turn.SkipAppend {
			if _, err := l.Sessions.AppendMessage(turn.Message); err != nil {
				l.finish(ch, "error", err)
				return true
			}
			ch <- Event{Type: EventMessage, Message: turn.Message}
		}

		action := TurnExecuteTools
		doneReason := turn.StopReason
		if doneReason == "" {
			doneReason = "completed"
		}
		skipDone := false
		if l.AfterAssistantTurn != nil {
			var aerr error
			action, doneReason, skipDone, aerr = l.AfterAssistantTurn(ctx, iter, turn)
			if aerr != nil {
				l.finish(ch, "error", aerr)
				return true
			}
		} else if len(turn.ToolCalls) == 0 {
			action = TurnDone
		}

		switch action {
		case TurnAbort:
			l.LastStopReason = "error"
			return true
		case TurnDone:
			l.LastStopReason = doneReason
			l.pendingDoneUsage = turn.Usage
			l.pendingDoneSkip = skipDone
			return false // даём обработать follow-up до финального done
		case TurnContinue:
			continue
		case TurnRejectTools:
			if err := l.rejectToolCalls(ch, turn.ToolCalls, "Tool call was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments."); err != nil {
				l.finish(ch, "error", err)
				return true
			}
			continue
		case TurnExecuteTools:
			// продолжаем выполнение ниже
		}

		if len(turn.ToolCalls) == 0 || l.Tools == nil {
			l.LastStopReason = doneReason
			l.pendingDoneUsage = turn.Usage
			l.pendingDoneSkip = skipDone
			return false
		}

		calls := make([]ai.ToolCall, 0, len(turn.ToolCalls))
		for _, call := range turn.ToolCalls {
			if l.BeforeToolCall != nil {
				block, args := l.BeforeToolCall(ctx, call)
				if block {
					tr := &ToolResult{CallID: call.ID, Name: call.Name, Content: "blocked by extension hook"}
					toolMsg := ai.Message{Role: ai.RoleTool, Content: tr.Content, ToolCallID: call.ID, ToolName: call.Name}
					_, _ = l.Sessions.AppendMessage(toolMsg)
					ch <- Event{Type: EventToolResult, ToolResult: tr}
					continue
				}
				if args != nil {
					call.Args = args
				}
			}
			calls = append(calls, call)
		}
		outcomes := ExecuteToolCalls(ctx, l.Tools, calls, mode, l.OnToolProgress)
		for i, oc := range outcomes {
			if l.AfterToolCall != nil {
				oc = l.AfterToolCall(ctx, oc.Call, oc)
			}
			if oc.Result.Terminate {
				oc.Terminate = true
			}
			outcomes[i] = oc
			tr := &ToolResult{CallID: oc.Call.ID, Name: oc.Call.Name, Content: oc.Result.Content, FullOutputPath: oc.Result.FullOutputPath, Truncated: oc.Result.Truncated}
			if oc.Err != nil {
				tr.Error = oc.Err.Error()
			}
			content := tr.Content
			if tr.Error != "" {
				content = tr.Error
			}
			toolMsg := ai.Message{Role: ai.RoleTool, Content: content, ToolCallID: oc.Call.ID, ToolName: oc.Call.Name}
			if _, err := l.Sessions.AppendMessage(toolMsg); err != nil {
				l.finish(ch, "error", err)
				return true
			}
			ch <- Event{Type: EventToolResult, ToolResult: tr}
		}

		if l.AfterTools != nil {
			cont, err := l.AfterTools(ctx, iter, outcomes)
			if err != nil {
				l.finish(ch, "error", err)
				return true
			}
			if !cont {
				l.LastStopReason = "error"
				return true
			}
		}

		terminateBatch := ShouldTerminateToolBatch(outcomes)
		steered := l.injectSteer(ch)

		if l.ShouldStopAfterTurn != nil {
			stop, err := l.ShouldStopAfterTurn(ctx, iter, turn, outcomes)
			if err != nil {
				l.finish(ch, "error", err)
				return true
			}
			if stop {
				l.LastStopReason = doneReason
				if !skipDone {
					ch <- Event{Type: EventDone, Usage: turn.Usage, StopReason: doneReason}
				}
				return true
			}
		}

		if steered {
			continue
		}
		if terminateBatch {
			// Завершаем inner-цикл; outer ещё может подхватить follow-up до отправки done.
			l.LastStopReason = doneReason
			l.pendingDoneUsage = turn.Usage
			l.pendingDoneSkip = skipDone
			return false
		}
	}
	l.LastStopReason = "max_iterations"
	if !l.SuppressMaxIterationsDone {
		ch <- Event{Type: EventDone, StopReason: "max_iterations"}
	}
	return true
}

func (l *Loop) drainFollowUp(ch chan<- Event) (ai.Message, bool) {
	if l.FollowUpMessage == nil {
		return ai.Message{}, false
	}
	msg, ok := l.FollowUpMessage()
	if !ok {
		return ai.Message{}, false
	}
	if _, err := l.Sessions.AppendMessage(msg); err != nil {
		return ai.Message{}, false
	}
	ch <- Event{Type: EventMessage, Message: msg}
	return msg, true
}

func (l *Loop) chat(ctx context.Context, prov ai.Provider, req ai.ChatRequest) (<-chan ai.ChatEvent, error) {
	if l.StreamFn != nil {
		return l.StreamFn(ctx, req)
	}
	return prov.Chat(ctx, req)
}

func (l *Loop) pumpStream(ctx context.Context, stream <-chan ai.ChatEvent, ch chan<- Event) (AssistantTurn, error) {
	if l.ProcessStream != nil {
		return l.ProcessStream(ctx, stream, ch)
	}
	ch <- Event{Type: EventMessageStart}
	var assistant ai.Message
	assistant.Role = ai.RoleAssistant
	stopReason := "completed"
	var usage *ai.Usage
	for ev := range stream {
		switch ev.Type {
		case ai.EventToken:
			assistant.Content += ev.Token
			ch <- Event{Type: EventToken, Token: ev.Token}
			ch <- Event{Type: EventMessageUpdate, Token: ev.Token}
		case ai.EventThinking:
			ch <- Event{Type: EventThinkingToken, Thinking: ev.Token}
		case ai.EventToolCall:
			if ev.ToolCall != nil {
				assistant.ToolCalls = append(assistant.ToolCalls, *ev.ToolCall)
				ch <- Event{Type: EventToolCall, ToolCall: ev.ToolCall}
			}
		case ai.EventToolCallDelta:
			ch <- Event{Type: EventToolCallDelta, Token: ev.ToolCallDelta, ToolCall: &ai.ToolCall{ID: ev.ToolCallID, Name: ev.ToolCallName}}
		case ai.EventDone:
			if ev.StopReason != "" {
				stopReason = ev.StopReason
			}
			usage = ev.Usage
		case ai.EventError:
			ch <- Event{Type: EventError, Err: ev.Err}
			ch <- Event{Type: EventDone, StopReason: "error"}
			return AssistantTurn{}, ev.Err
		}
	}
	return AssistantTurn{
		Message:    assistant,
		ToolCalls:  assistant.ToolCalls,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

func (l *Loop) rejectToolCalls(ch chan<- Event, calls []ai.ToolCall, _ string) error {
	for _, call := range calls {
		tr := &ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			Error:  fmt.Sprintf("Tool call %q was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.", call.Name),
		}
		ch <- Event{Type: EventToolResult, ToolResult: tr}
		toolMsg := ai.Message{Role: ai.RoleTool, Content: tr.Error, ToolCallID: call.ID, ToolName: call.Name}
		if _, err := l.Sessions.AppendMessage(toolMsg); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loop) injectSteer(ch chan<- Event) bool {
	if l.SteerMessage != nil {
		if msg, ok := l.SteerMessage(); ok {
			if _, err := l.Sessions.AppendMessage(msg); err == nil {
				ch <- Event{Type: EventMessage, Message: msg}
				return true
			}
		}
		return false
	}
	if l.SteerFn != nil {
		if msg, ok := l.SteerFn(); ok && msg != "" {
			steer := ai.Message{Role: ai.RoleUser, Content: msg}
			if _, err := l.Sessions.AppendMessage(steer); err == nil {
				ch <- Event{Type: EventMessage, Message: steer}
				return true
			}
		}
	}
	return false
}

func (l *Loop) finish(ch chan<- Event, stop string, err error) {
	l.LastStopReason = stop
	if err != nil {
		ch <- Event{Type: EventError, Err: err}
	}
	ch <- Event{Type: EventDone, StopReason: stop}
}
