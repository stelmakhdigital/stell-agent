package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stelmakhdigital/stell-ai"
)

// BranchWithSummary переключает активную ветку на branchFromID и добавляет
// дочернюю запись branch_summary в дереве сессии.
func (m *Manager) BranchWithSummary(branchFromID, summary string, details json.RawMessage, fromHook bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if branchFromID == "" {
		return "", fmt.Errorf("branchFromID required")
	}
	found := false
	for _, e := range m.Entries {
		if e.ID == branchFromID {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("entry %q not found", branchFromID)
	}
	m.leafID = branchFromID
	summary = trimSummary(summary)
	return m.appendEntry(Entry{
		Type:      "branch_summary",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Summary:   summary,
		FromID:    branchFromID,
		Details:   details,
		FromHook:  fromHook,
		Message:   branchSummaryMessage(summary),
	})
}

func branchSummaryMessage(summary string) *ai.Message {
	if strings.TrimSpace(summary) == "" {
		return nil
	}
	return &ai.Message{Role: ai.RoleSystem, Content: "Branch summary:\n" + summary}
}

func trimSummary(s string) string {
	const max = 8000
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
