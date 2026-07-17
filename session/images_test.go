package session

import (
	"strings"
	"testing"

	"github.com/stelmakhdigital/ai"
)

func TestStripImagesFromActiveBranch(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _ = m.AppendMessage(ai.Message{
		Role:    ai.RoleUser,
		Content: "what is on screen",
		Images:  []ai.ImageContent{{Type: "image", Data: "abc", MimeType: "image/png"}},
	})

	if n := m.StripImagesFromActiveBranch(); n != 1 {
		t.Fatalf("stripped %d, want 1", n)
	}
	msgs := m.BuildMessages()
	if len(msgs) != 1 {
		t.Fatalf("messages: %d", len(msgs))
	}
	if len(msgs[0].Images) != 0 {
		t.Fatal("expected images removed")
	}
	if !strings.Contains(msgs[0].Content, "[image 1:") {
		t.Fatalf("expected placeholder, got %q", msgs[0].Content)
	}
}
