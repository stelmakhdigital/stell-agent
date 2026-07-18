package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stelmakhdigital/stell-ai"
)

func TestWireRoundTripToolResultAndThinking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	m := NewManager(dir)
	m.Header.ParentSession = "/tmp/parent.jsonl"
	if _, err := m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendMessage(ai.Message{
		Role: ai.RoleAssistant,
		Blocks: []ai.ContentBlock{
			{Type: ai.BlockTypeText, Text: "ok"},
			{Type: ai.BlockTypeToolCall, ToolCall: &ai.ToolCall{ID: "c1", Name: "bash", Args: map[string]any{"command": "true"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendMessage(ai.Message{
		Role: ai.RoleTool, Content: "done", ToolCallID: "c1", ToolName: "bash",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendThinkingLevelChange("high"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"role":"toolResult"`)) {
		t.Fatalf("expected toolResult in wire, got:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(`"parentSession"`)) {
		t.Fatalf("expected parentSession in header:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(`"thinking_level_change"`)) {
		t.Fatalf("expected thinking_level_change:\n%s", raw)
	}

	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Header.Version != FormatVersion {
		t.Fatalf("version=%d", loaded.Header.Version)
	}
	msgs := loaded.BuildMessages()
	var sawTool bool
	for _, msg := range msgs {
		if ai.IsToolRole(msg.Role) {
			sawTool = true
			if msg.Role != ai.RoleTool {
				t.Fatalf("normalized role=%q", msg.Role)
			}
		}
	}
	if !sawTool {
		t.Fatal("expected toolResult in messages")
	}
}

func TestDualReaderLegacyToolRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")
	line0, _ := json.Marshal(Header{Type: "session", Version: 4, ID: "x", Timestamp: "t", CWD: dir})
	pid := "aaaaaaaa"
	entry := map[string]any{
		"type": "message", "id": "bbbbbbbb", "parentId": nil, "timestamp": "t",
		"message": map[string]any{"role": "tool", "content": "out", "toolCallId": "c1"},
	}
	line1, _ := json.Marshal(entry)
	if err := os.WriteFile(path, append(append(line0, '\n'), append(line1, '\n')...), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = pid
	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].Message == nil {
		t.Fatal("expected message")
	}
	ai.NormalizeMessage(loaded.Entries[0].Message)
	if loaded.Entries[0].Message.Role != ai.RoleTool {
		t.Fatalf("role=%q", loaded.Entries[0].Message.Role)
	}
}
