package session

import (
	"path/filepath"
	"testing"

	"github.com/stelmakhdigital/ai"
)

func TestForkAndTree(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	id1, _ := m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "a"})
	id2, _ := m.AppendMessage(ai.Message{Role: ai.RoleAssistant, Content: "b"})
	if err := m.ForkAt(id1); err != nil {
		t.Fatal(err)
	}
	id3, _ := m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "c"})
	if id3 == "" {
		t.Fatal("expected branch message")
	}
	tree := m.GetTree()
	if len(tree) != 3 {
		t.Fatalf("tree len=%d", len(tree))
	}
	children := m.ListChildren(id1)
	if len(children) != 2 {
		t.Fatalf("children of %s = %v", id1, children)
	}
	_ = id2
	msgs := m.BuildMessages()
	if len(msgs) != 2 || msgs[1].Content != "c" {
		t.Fatalf("branch messages=%+v", msgs)
	}
}

func TestCloneAndListFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "sessions")
	m := NewManager(dir)
	_, _ = m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "x"})
	path := filepath.Join(SessionDir(root, dir), "test.jsonl")
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	clone := m.Clone()
	if clone.Header.ID == m.Header.ID {
		t.Fatal("clone should have new session id")
	}
	files, err := ListFiles(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%v", files)
	}
}

func TestCompactLinear(t *testing.T) {
	m := NewManager(t.TempDir())
	for i := 0; i < 6; i++ {
		role := ai.RoleUser
		if i%2 == 1 {
			role = ai.RoleAssistant
		}
		_, _ = m.AppendMessage(ai.Message{Role: role, Content: "msg"})
	}
	r := m.CompactLinear("summary", 2)
	if r.Removed != 4 {
		t.Fatalf("removed=%d", r.Removed)
	}
	if len(m.BuildMessages()) != 3 { // summary + 2 kept
		t.Fatalf("messages=%d", len(m.BuildMessages()))
	}
}
