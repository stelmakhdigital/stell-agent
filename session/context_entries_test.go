package session

import (
	"testing"

	"github.com/stelmakhdigital/ai"
)

func TestBuildContextEntries(t *testing.T) {
	m := NewManager("/tmp/ws")
	u1, _ := m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "hi"})
	_, _ = m.AppendMessage(ai.Message{Role: ai.RoleAssistant, Content: "hello"})
	if err := m.ForkAt(u1); err != nil {
		t.Fatal(err)
	}
	leaf := m.LeafID()
	_, _ = m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "forked"})
	if leaf == "" {
		t.Fatal("expected leaf id")
	}
	entries := m.BuildContextEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries on branch, got %d", len(entries))
	}
	if entries[0].Content != "hi" || entries[1].Content != "forked" {
		t.Fatalf("unexpected branch messages: %+v", entries)
	}
}

func TestAppendBashEntry(t *testing.T) {
	m := NewManager("/tmp/ws")
	id, err := m.AppendBashEntry("echo hi", "hi\n", BashEntryMeta{})
	if err != nil || id == "" {
		t.Fatalf("AppendBashEntry: id=%q err=%v", id, err)
	}
	entries := m.BuildContextEntries()
	if len(entries) != 1 || entries[0].Role != ai.RoleUser {
		t.Fatalf("expected bash as user message, got %+v", entries)
	}
}

func TestAppendBashEntryExcludedFromContext(t *testing.T) {
	m := NewManager("/tmp/ws")
	_, err := m.AppendBashEntry("echo hi", "hi\n", BashEntryMeta{ExcludeFromContext: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.BuildContextEntries()) != 0 {
		t.Fatalf("excluded bash must not appear in context")
	}
}
