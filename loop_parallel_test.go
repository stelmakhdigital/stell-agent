package agent_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stelmakhdigital/stell-ai"
	"github.com/stelmakhdigital/stell-ai/provider"
	"github.com/stelmakhdigital/stell-ai/provider/mock"
	"github.com/stelmakhdigital/stell-agent"
	"github.com/stelmakhdigital/stell-agent/session"
	"github.com/stelmakhdigital/stell-agent/tools"
)

type slowTool struct {
	name string
	n    *atomic.Int32
	max  *atomic.Int32
}

func (t *slowTool) Def() ai.ToolDef {
	return ai.ToolDef{Name: t.name, Description: "slow", Parameters: map[string]any{"type": "object"}}
}

func (t *slowTool) Call(ctx context.Context, env *tools.Env, args map[string]any) (tools.Result, error) {
	cur := t.n.Add(1)
	for {
		prev := t.max.Load()
		if cur <= prev || t.max.CompareAndSwap(prev, cur) {
			break
		}
	}
	time.Sleep(40 * time.Millisecond)
	t.n.Add(-1)
	return tools.Result{Content: "ok:" + t.name}, nil
}

func TestLoopParallelTools(t *testing.T) {
	reg := provider.NewRegistry()
	mp := mock.New("faux")
	mp.Append(
		ai.ChatEvent{Type: ai.EventToolCall, ToolCall: &ai.ToolCall{ID: "1", Name: "a", Args: map[string]any{}}},
		ai.ChatEvent{Type: ai.EventToolCall, ToolCall: &ai.ToolCall{ID: "2", Name: "b", Args: map[string]any{}}},
		ai.ChatEvent{Type: ai.EventDone, StopReason: "toolUse"},
	)
	mp.Append(
		ai.ChatEvent{Type: ai.EventToken, Token: "done"},
		ai.ChatEvent{Type: ai.EventDone, StopReason: "completed"},
	)
	reg.Register(ai.ModelConfig{Name: "faux", Provider: "mock", Model: "faux"}, mp)

	var concurrent, peak atomic.Int32
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	_ = rt.Register(&slowTool{name: "a", n: &concurrent, max: &peak})
	_ = rt.Register(&slowTool{name: "b", n: &concurrent, max: &peak})

	sess := session.NewManager(t.TempDir())
	loop := &agent.Loop{
		Registry:      reg,
		Tools:         rt,
		Sessions:      sess,
		ModelName:     "faux",
		ModelID:       "faux",
		ToolExecution: agent.ToolExecutionParallel,
	}
	ch := make(chan agent.Event, 64)
	go loop.Run(context.Background(), "run tools", ch)
	for ev := range ch {
		if ev.Type == agent.EventError {
			t.Fatal(ev.Err)
		}
	}
	if peak.Load() < 2 {
		t.Fatalf("expected parallel peak>=2, got %d", peak.Load())
	}
}

func TestExecuteToolCallsSequential(t *testing.T) {
	var concurrent, peak atomic.Int32
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	_ = rt.Register(&slowTool{name: "a", n: &concurrent, max: &peak})
	_ = rt.Register(&slowTool{name: "b", n: &concurrent, max: &peak})
	out := agent.ExecuteToolCalls(context.Background(), rt, []ai.ToolCall{
		{ID: "1", Name: "a", Args: map[string]any{}},
		{ID: "2", Name: "b", Args: map[string]any{}},
	}, agent.ToolExecutionSequential, nil)
	if len(out) != 2 || out[0].Err != nil || out[1].Err != nil {
		t.Fatalf("%+v", out)
	}
	if peak.Load() != 1 {
		t.Fatalf("sequential peak=%d", peak.Load())
	}
}
