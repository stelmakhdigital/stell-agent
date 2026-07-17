package hooks

import (
	"context"
	"fmt"
	"testing"
)

func TestBusEmitOrderAndMutation(t *testing.T) {
	b := NewBus()
	var order []string
	b.On(ToolCall, func(_ context.Context, _ *Ctx, ev *Event) error {
		order = append(order, "first")
		ev.Args = map[string]any{"path": "rewritten.go"}
		return nil
	})
	b.On(ToolCall, func(_ context.Context, _ *Ctx, ev *Event) error {
		order = append(order, "second")
		if ev.Args["path"] != "rewritten.go" {
			t.Errorf("second handler must see first handler's mutation, got %v", ev.Args)
		}
		ev.Block = true
		return nil
	})
	b.OnAny(func(_ context.Context, _ *Ctx, ev *Event) error {
		order = append(order, "any")
		return nil
	})

	ev := &Event{Name: ToolCall, Args: map[string]any{"path": "a.go"}}
	if err := b.Emit(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if !ev.Block {
		t.Fatal("expected Block to be set")
	}
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "any" {
		t.Fatalf("wrong handler order: %v", order)
	}
}

func TestBusContinuesPastErrors(t *testing.T) {
	b := NewBus()
	ran := false
	b.On(Input, func(_ context.Context, _ *Ctx, _ *Event) error {
		return fmt.Errorf("boom")
	})
	b.On(Input, func(_ context.Context, _ *Ctx, ev *Event) error {
		ran = true
		ev.Text = "changed"
		return nil
	})
	ev, err := b.EmitNamed(context.Background(), Input, "s1", map[string]any{"text": "hi"})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected first error, got %v", err)
	}
	if !ran {
		t.Fatal("second handler must run despite first error")
	}
	if ev.Text != "changed" {
		t.Fatalf("Text = %q", ev.Text)
	}
}

func TestBusHasSubscriber(t *testing.T) {
	b := NewBus()
	if b.HasSubscriber(MessageUpdate) {
		t.Fatal("empty bus must have no subscribers")
	}
	b.On(MessageUpdate, func(_ context.Context, _ *Ctx, _ *Event) error { return nil })
	if !b.HasSubscriber(MessageUpdate) {
		t.Fatal("expected named subscriber")
	}
	b.RegisterInterest(func(name string) bool { return name == UserBash })
	if !b.HasSubscriber(UserBash) {
		t.Fatal("expected dynamic interest subscriber")
	}
	if b.HasSubscriber(ToolResult) {
		t.Fatal("tool_result must have no subscribers")
	}
}

func TestBusNilSafe(t *testing.T) {
	var b *Bus
	if err := b.Emit(context.Background(), &Event{Name: SessionStart}); err != nil {
		t.Fatal(err)
	}
	if b.HasSubscriber(SessionStart) {
		t.Fatal("nil bus has no subscribers")
	}
}

func TestAppendSystemText(t *testing.T) {
	ev := &Event{}
	ev.AppendSystemText("a")
	ev.AppendSystemText("  ")
	ev.AppendSystemText("b")
	if ev.AppendSystem != "a\n\nb" {
		t.Fatalf("AppendSystem = %q", ev.AppendSystem)
	}
}

func TestCtxUnavailableCapabilities(t *testing.T) {
	var c *Ctx
	if _, err := c.Exec(context.Background(), "ls"); err == nil {
		t.Fatal("expected error from nil ctx")
	}
	c = &Ctx{}
	if err := c.SendUserMessage(context.Background(), "hi", "steer"); err == nil {
		t.Fatal("expected error for unset capability")
	}
	if _, err := c.AppendEntry("x"); err == nil {
		t.Fatal("expected error for unset capability")
	}
	if _, _, err := c.UIInput(context.Background(), "m", "", ""); err == nil {
		t.Fatal("expected error for unset capability")
	}
}

func TestIsProviderHook(t *testing.T) {
	for _, name := range []string{BeforeProviderHeaders, BeforeProviderRequest, AfterProviderResponse} {
		if !IsProviderHook(name) {
			t.Fatalf("%s must be a provider hook", name)
		}
	}
	if IsProviderHook(ToolCall) {
		t.Fatal("tool_call is not a provider hook")
	}
}
