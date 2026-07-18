package agent_test

import (
	"context"
	"testing"

	"github.com/stelmakhdigital/stell-ai"
	"github.com/stelmakhdigital/stell-ai/provider"
	"github.com/stelmakhdigital/stell-ai/provider/faux"
	"github.com/stelmakhdigital/stell-ai/provider/mock"
	"github.com/stelmakhdigital/stell-agent"
	"github.com/stelmakhdigital/stell-agent/session"
	"github.com/stelmakhdigital/stell-agent/tools"
)

type termTool struct{ name string }

func (t termTool) Def() ai.ToolDef {
	return ai.ToolDef{Name: t.name, Description: "t", Parameters: map[string]any{"type": "object"}}
}

func (t termTool) Call(ctx context.Context, env *tools.Env, args map[string]any) (tools.Result, error) {
	return tools.Result{Content: "ok", Terminate: true}, nil
}

func TestLoopTerminateBatch(t *testing.T) {
	reg := provider.NewRegistry()
	mp := mock.New("m")
	mp.Append(
		mock.Call("1", "term", map[string]any{}),
		mock.Done(1, 1),
	)
	// Second LLM call must NOT happen.
	mp.Append(mock.Token("should-not"), mock.Done(1, 1))
	reg.Register(ai.ModelConfig{Name: "m", Provider: "mock", Model: "m"}, mp)

	sess := session.NewManager(t.TempDir())
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	_ = rt.Register(termTool{name: "term"})
	loop := &agent.Loop{
		Registry: reg, Tools: rt, Sessions: sess, ModelName: "m", ModelID: "m",
	}
	ch := make(chan agent.Event, 32)
	go loop.Run(context.Background(), "hi", ch)
	var llmish int
	for ev := range ch {
		if ev.Type == agent.EventToolResult {
			llmish++
		}
		if ev.Type == agent.EventError {
			t.Fatal(ev.Err)
		}
	}
	if mp.Requests() != nil && len(mp.Requests()) != 1 {
		t.Fatalf("llm calls=%d want 1", len(mp.Requests()))
	}
	if llmish != 1 {
		t.Fatalf("tool results=%d", llmish)
	}
}

func TestShouldStopAfterTurnBlocksFollowUp(t *testing.T) {
	reg := provider.NewRegistry()
	fp := faux.New("m")
	fp.ChunkSize = 0
	fp.SetResponses(
		[]ai.ChatEvent{faux.Call("1", "term", map[string]any{}), faux.Done(1, 1)},
		[]ai.ChatEvent{faux.Token("b"), faux.Done(1, 1)}, // must not run
	)
	reg.Register(ai.ModelConfig{Name: "m", Provider: "faux", Model: "m"}, fp)

	sess := session.NewManager(t.TempDir())
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	_ = rt.Register(termTool{name: "term"})
	fuPolls := 0
	loop := &agent.Loop{
		Registry: reg, Tools: rt, Sessions: sess, ModelName: "m", ModelID: "m",
		ShouldStopAfterTurn: func(ctx context.Context, iter int, turn agent.AssistantTurn, outcomes []agent.ToolCallOutcome) (bool, error) {
			return true, nil
		},
		FollowUpMessage: func() (ai.Message, bool) {
			fuPolls++
			return ai.Message{Role: ai.RoleUser, Content: "follow"}, true
		},
	}
	ch := make(chan agent.Event, 32)
	go loop.Run(context.Background(), "hi", ch)
	for ev := range ch {
		if ev.Type == agent.EventError {
			t.Fatal(ev.Err)
		}
	}
	if fuPolls != 0 {
		t.Fatalf("follow-up polls=%d want 0", fuPolls)
	}
	if len(fp.Requests()) != 1 {
		t.Fatalf("llm calls=%d", len(fp.Requests()))
	}
}

func TestLoopFollowUpInsideFaux(t *testing.T) {
	reg := provider.NewRegistry()
	fp := faux.New("m")
	fp.ChunkSize = 0
	fp.SetResponses(
		[]ai.ChatEvent{faux.Token("a"), faux.Done(1, 1)},
		[]ai.ChatEvent{faux.Token("b"), faux.Done(1, 1)},
	)
	reg.Register(ai.ModelConfig{Name: "m", Provider: "faux", Model: "m"}, fp)

	sess := session.NewManager(t.TempDir())
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	fu := 0
	loop := &agent.Loop{
		Registry: reg, Tools: rt, Sessions: sess, ModelName: "m", ModelID: "m",
		FollowUpMessage: func() (ai.Message, bool) {
			if fu > 0 {
				return ai.Message{}, false
			}
			fu++
			return ai.Message{Role: ai.RoleUser, Content: "follow"}, true
		},
	}
	ch := make(chan agent.Event, 32)
	go loop.Run(context.Background(), "hi", ch)
	var tok string
	for ev := range ch {
		if ev.Type == agent.EventToken {
			tok += ev.Token
		}
	}
	if tok != "ab" {
		t.Fatalf("tok=%q", tok)
	}
}

func TestLoopRunContinue(t *testing.T) {
	reg := provider.NewRegistry()
	mp := mock.New("m")
	mp.Append(mock.Token("cont"), mock.Done(1, 1))
	reg.Register(ai.ModelConfig{Name: "m", Provider: "mock", Model: "m"}, mp)

	sess := session.NewManager(t.TempDir())
	_, _ = sess.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "already"})
	_, _ = sess.AppendMessage(ai.Message{Role: ai.RoleAssistant, Content: "prev"})
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	loop := &agent.Loop{
		Registry: reg, Tools: rt, Sessions: sess, ModelName: "m", ModelID: "m",
	}
	ch := make(chan agent.Event, 32)
	go loop.RunContinue(context.Background(), ch)
	var tok string
	for ev := range ch {
		if ev.Type == agent.EventToken {
			tok += ev.Token
		}
	}
	if tok != "cont" {
		t.Fatalf("tok=%q", tok)
	}
}

func TestShouldTerminateToolBatch(t *testing.T) {
	if agent.ShouldTerminateToolBatch(nil) {
		t.Fatal("empty")
	}
	ok := agent.ShouldTerminateToolBatch([]agent.ToolCallOutcome{
		{Terminate: true},
		{Result: tools.Result{Terminate: true}},
	})
	if !ok {
		t.Fatal("want true")
	}
	if agent.ShouldTerminateToolBatch([]agent.ToolCallOutcome{{Terminate: true}, {}}) {
		t.Fatal("partial")
	}
}
