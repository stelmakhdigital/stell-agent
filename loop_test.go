package agent_test

import (
	"context"
	"testing"

	"github.com/stelmakhdigital/stell-ai"
	"github.com/stelmakhdigital/stell-ai/provider"
	"github.com/stelmakhdigital/stell-ai/provider/mock"
	"github.com/stelmakhdigital/stell-agent"
	"github.com/stelmakhdigital/stell-agent/session"
	"github.com/stelmakhdigital/stell-agent/tools"
)

func TestLoopRunFauxNoTools(t *testing.T) {
	_ = mock.New // ensure package linked
	reg := provider.NewRegistry()
	mp := mock.New("faux")
	mp.Append(
		ai.ChatEvent{Type: ai.EventToken, Token: "hello"},
		ai.ChatEvent{Type: ai.EventDone, StopReason: "completed"},
	)
	reg.Register(ai.ModelConfig{Name: "faux", Provider: "mock", Model: "faux"}, mp)

	sess := session.NewManager(t.TempDir())
	rt := tools.NewRuntime(tools.Env{Workspace: t.TempDir()})
	loop := &agent.Loop{
		Registry:  reg,
		Tools:     rt,
		Sessions:  sess,
		ModelName: "faux",
		ModelID:   "faux",
	}
	ch := make(chan agent.Event, 32)
	go loop.Run(context.Background(), "hi", ch)
	var tokens string
	var done bool
	for ev := range ch {
		switch ev.Type {
		case agent.EventToken:
			tokens += ev.Token
		case agent.EventDone:
			done = true
		case agent.EventError:
			t.Fatalf("error: %v", ev.Err)
		}
	}
	if tokens != "hello" || !done {
		t.Fatalf("tokens=%q done=%v", tokens, done)
	}
}
