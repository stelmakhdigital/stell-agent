package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stelmakhdigital/ai"
)

func TestSessionJSONLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if _, err := m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendMessage(ai.Message{Role: ai.RoleAssistant, Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s.jsonl")
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Header.Version != FormatVersion {
		t.Fatalf("version=%d", loaded.Header.Version)
	}
	msgs := loaded.BuildMessages()
	if len(msgs) != 2 || msgs[0].Content != "hi" {
		t.Fatalf("messages=%+v", msgs)
	}
	data, _ := os.ReadFile(path)
	if len(data) == 0 {
		t.Fatal("empty session file")
	}
}

func TestManagerConcurrentAppend(t *testing.T) {
	m := NewManager(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = m.AppendMessage(ai.Message{Role: ai.RoleUser, Content: "msg"})
		}(i)
	}
	wg.Wait()
	if len(m.Entries) != 20 {
		t.Fatalf("entries=%d want 20", len(m.Entries))
	}
}
