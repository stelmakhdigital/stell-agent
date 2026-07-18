package session

import (
	"encoding/json"
	"time"

	"github.com/stelmakhdigital/stell-ai"
)

// BuildContextEntries пересобирает LLM-сообщения из активной ветки с учётом семантики веток.
func (m *Manager) BuildContextEntries() []ai.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	branch := m.branchToLeaf()
	out := make([]ai.Message, 0, len(branch))
	for _, e := range branch {
		switch e.Type {
		case "message":
			if e.Message == nil {
				break
			}
			if e.Message.Role == ai.RoleBashExecution {
				if !EntryBashMeta(e).ExcludeFromContext {
					// Вывод bash передаётся модели как сообщение пользователя.
					out = append(out, ai.Message{Role: ai.RoleUser, Content: e.Message.Content})
				}
				break
			}
			out = append(out, *e.Message)
		case "compaction", "branch_summary":
			text := branchSummaryContext(e)
			if text != "" {
				out = append(out, ai.Message{Role: ai.RoleSystem, Content: text})
			}
		case "model_change", "thinking_level_change":
			// Только метаданные; не включаются в контекст LLM.
		case "bash":
			if e.Message != nil && !EntryBashMeta(e).ExcludeFromContext {
				out = append(out, *e.Message)
			}
		case "custom_message":
			if e.Message != nil && e.Message.Content != "" {
				// custom_message участвует в контексте LLM.
				out = append(out, *e.Message)
			}
		case "label", "session_info", "custom":
			if e.Message != nil && e.Message.Content != "" {
				out = append(out, ai.Message{Role: ai.RoleSystem, Content: e.Message.Content})
			}
		}
	}
	return out
}

func (m *Manager) AppendSessionInfo(text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:      "session_info",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &ai.Message{Role: ai.RoleSystem, Content: text},
	})
}

func (m *Manager) AppendCustomEntry(text string) (string, error) {
	return m.AppendTypedCustomEntry("", text, nil)
}

func (m *Manager) AppendCustomMessage(text string) (string, error) {
	return m.AppendTypedCustomMessage("", text, nil)
}

func (m *Manager) AppendTypedCustomEntry(customType, text string, data json.RawMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:       "custom",
		CustomType: customType,
		CustomData: data,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Message:    &ai.Message{Role: ai.RoleSystem, Content: text},
	})
}

func (m *Manager) AppendTypedCustomMessage(customType, text string, data json.RawMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:       "custom_message",
		CustomType: customType,
		CustomData: data,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Message:    &ai.Message{Role: ai.RoleSystem, Content: text},
	})
}

func (m *Manager) AppendLabel(text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:      "label",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &ai.Message{Role: ai.RoleSystem, Content: text},
	})
}

// branchSummaryContext возвращает текст контекста для записи branch_summary или message.
func branchSummaryContext(e Entry) string {
	if e.Type == "branch_summary" && e.Summary != "" {
		return "Branch summary:\n" + e.Summary
	}
	if e.Message != nil {
		return e.Message.Content
	}
	return ""
}

func (m *Manager) AppendThinkingLevelChange(level string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:      "thinking_level_change",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &ai.Message{Role: ai.RoleSystem, Content: level},
	})
}
