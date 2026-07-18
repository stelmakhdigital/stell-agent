package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stelmakhdigital/stell-ai"
)

func TestBranchWithSummary(t *testing.T) {
	m := NewManager(t.TempDir())
	id1, _ := m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "a"})
	_, _ = m.AppendMessage(ai.Message{Role: ai.RoleAssistant, Content: "b"})
	summaryID, err := m.BranchWithSummary(id1, "explored option A", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if summaryID == "" {
		t.Fatal("expected summary entry id")
	}
	branch := m.ActiveBranch()
	if len(branch) != 2 {
		t.Fatalf("branch len=%d", len(branch))
	}
	if branch[1].Type != "branch_summary" || branch[1].Summary != "explored option A" || branch[1].FromID != id1 {
		t.Fatalf("summary entry=%+v", branch[1])
	}
	msgs := m.BuildMessages()
	if len(msgs) != 2 || msgs[1].Role != ai.RoleSystem {
		t.Fatalf("messages=%+v", msgs)
	}
}

func TestListFilesWithProgress(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "sessions")
	m := NewManager(dir)
	_, _ = m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "x"})
	path := filepath.Join(SessionDir(root, dir), "test.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	var calls [][2]int
	files, err := ListFilesWithProgress(root, dir, func(loaded, total int) {
		calls = append(calls, [2]int{loaded, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%v", files)
	}
	if len(calls) != 1 || calls[0][0] != 1 || calls[0][1] != 1 {
		t.Fatalf("progress=%v", calls)
	}
}
