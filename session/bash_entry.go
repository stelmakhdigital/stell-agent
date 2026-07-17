package session

import (
	"encoding/json"
	"time"

	"github.com/stelmakhdigital/ai"
)

// BashEntryMeta — метаданные bash-записи в сессии (исключение из контекста и т.д.).
type BashEntryMeta struct {
	ExcludeFromContext bool `json:"excludeFromContext,omitempty"`
	ExitCode           int  `json:"exitCode,omitempty"`
	Cancelled          bool `json:"cancelled,omitempty"`
	Truncated          bool `json:"truncated,omitempty"`
}

// EntryBashMeta извлекает BashEntryMeta из CustomData записи.
func EntryBashMeta(e Entry) BashEntryMeta {
	if len(e.CustomData) == 0 {
		return BashEntryMeta{}
	}
	if e.Type != "bash" && (e.Type != "message" || e.Message == nil || e.Message.Role != ai.RoleBashExecution) {
		return BashEntryMeta{}
	}
	var meta BashEntryMeta
	_ = json.Unmarshal(e.CustomData, &meta)
	return meta
}

// AppendBashEntry добавляет запись выполнения bash в активную ветку.
func (m *Manager) AppendBashEntry(command, output string, meta BashEntryMeta) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	content := "$ " + command + "\n" + output
	data, _ := json.Marshal(meta)
	// Провод сессии: type message + role bashExecution; CustomData хранит meta.
	return m.appendEntry(Entry{
		Type:       "message",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CustomData: data,
		Message: &ai.Message{
			Role:    ai.RoleBashExecution,
			Content: content,
		},
	})
}
