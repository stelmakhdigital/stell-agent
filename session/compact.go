package session

import (
	"time"

	"github.com/stelmakhdigital/stell-ai"
)

// CompactResult — итог линейного компактирования ветки.
type CompactResult struct {
	Removed        int
	SummaryPreview string
}

// CompactLinear суммирует активную ветку, сохраняя последние keepRecent сообщений.
func (m *Manager) CompactLinear(summary string, keepRecent int) CompactResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	if keepRecent < 1 {
		keepRecent = 1
	}
	branch := m.branchToLeaf()
	if len(branch) <= keepRecent {
		return CompactResult{}
	}
	removed := len(branch) - keepRecent
	kept := branch[removed:]
	removeIDs := map[string]bool{}
	for i := 0; i < removed; i++ {
		removeIDs[branch[i].ID] = true
	}

	preview := summary
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}

	compID := newID()
	var compParent *string
	if kept[0].ParentID != nil {
		compParent = kept[0].ParentID
	}
	compEntry := Entry{
		Type:      "compaction",
		ID:        compID,
		ParentID:  compParent,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &ai.Message{Role: ai.RoleSystem, Content: "Conversation summary:\n" + summary},
	}

	branchIDs := map[string]bool{}
	for _, e := range branch {
		branchIDs[e.ID] = true
	}

	var newEntries []Entry
	newEntries = append(newEntries, compEntry)
	for _, e := range m.Entries {
		if branchIDs[e.ID] && removeIDs[e.ID] {
			continue
		}
		if e.ID == kept[0].ID {
			e2 := e
			p := compID
			e2.ParentID = &p
			newEntries = append(newEntries, e2)
			continue
		}
		if !branchIDs[e.ID] {
			newEntries = append(newEntries, e)
		} else if !removeIDs[e.ID] {
			newEntries = append(newEntries, e)
		}
	}
	m.Entries = newEntries
	m.leafID = kept[len(kept)-1].ID
	return CompactResult{Removed: removed, SummaryPreview: preview}
}
