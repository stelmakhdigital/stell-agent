package harness

import (
	"testing"

	"github.com/stelmakhdigital/ai"
	"stell/agent/session"
)

func TestEstimateContextTokens(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: "hello world"},
		{Role: ai.RoleAssistant, Content: "response text"},
	}
 est := EstimateContextTokens(msgs)
	if est.Tokens <= 0 {
		t.Fatalf("tokens=%d", est.Tokens)
	}
}

func TestPrepareCompaction(t *testing.T) {
	m := session.NewManager(t.TempDir())
	for i := 0; i < 10; i++ {
		role := ai.RoleUser
		if i%2 == 1 {
			role = ai.RoleAssistant
		}
		_, _ = m.AppendMessage(ai.Message{Role: role, Content: "long message payload"})
	}
	settings := DefaultCompactionSettings()
	settings.KeepRecentTokens = 64 // force small keep window for test
	prep, err := PrepareCompaction(m.ActiveBranch(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if prep == nil || prep.FirstKeptEntryID == "" || prep.RemovedEntries == 0 {
		t.Fatalf("prep=%+v", prep)
	}
}

func TestBranchSummaryPrompt(t *testing.T) {
	p := BranchSummaryPrompt("user: hi", "")
	if p == "" || len(SummarizationSystemPrompt) == 0 {
		t.Fatal("expected prompts")
	}
}
