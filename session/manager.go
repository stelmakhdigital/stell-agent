// Package session — JSONL-хранилище сессий, дерево веток и компактирование.
package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stelmakhdigital/ai"
)

// FormatVersion — версия заголовка сессии (v3).
const FormatVersion = 3

// Header — JSONL-заголовок файла сессии.
type Header struct {
	Type            string `json:"type"`
	Version         int    `json:"version"`
	ID              string `json:"id"`
	Timestamp       string `json:"timestamp"`
	CWD             string `json:"cwd"`
	ParentSession   string `json:"parentSession,omitempty"`   // путь к файлу родительской сессии
	ParentSessionID string `json:"parentSessionId,omitempty"` // устаревшее поле id stell
}

// Entry — одна запись JSONL-сессии (сообщение, branch_summary, compaction и т.д.).
type Entry struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	ParentID   *string         `json:"parentId"`
	Timestamp  string          `json:"timestamp"`
	CustomType string          `json:"customType,omitempty"`
	CustomData json.RawMessage `json:"customData,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	FromID     string          `json:"fromId,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	FromHook   bool            `json:"fromHook,omitempty"`
	Message    *ai.Message     `json:"message,omitempty"`
	Raw        json.RawMessage `json:"-"`
}

// Manager — потокобезопасное хранилище JSONL-сессии в памяти и на диске.
type Manager struct {
	mu      sync.RWMutex
	Path    string
	Header  Header
	Entries []Entry
	leafID  string
}

// NewManager создаёт пустую сессию для cwd (без записи на диск).
func NewManager(cwd string) *Manager {
	id := newID()
	return &Manager{
		Header: Header{
			Type:      "session",
			Version:   FormatVersion,
			ID:        id,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			CWD:       cwd,
		},
	}
}

// SessionDir возвращает каталог сессий для cwd внутри root.
func SessionDir(root, cwd string) string {
	enc := encodeCWD(cwd)
	return filepath.Join(root, enc)
}

func encodeCWD(cwd string) string {
	s := strings.ReplaceAll(filepath.Clean(cwd), string(filepath.Separator), "-")
	if !strings.HasPrefix(s, "-") {
		s = "--" + s + "--"
	}
	return s
}

// AppendMessage добавляет сообщение в активную ветку и возвращает id записи.
func (m *Manager) AppendMessage(msg ai.Message) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:      "message",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &msg,
	})
}

// BeginAssistantMessage создаёт одну streaming assistant-запись in-place.
func (m *Manager) BeginAssistantMessage() (string, error) {
	return m.AppendMessage(ai.Message{Role: ai.RoleAssistant, Blocks: nil, Content: ""})
}

// PatchAssistantMessage обновляет существующую assistant-запись in-place.
func (m *Manager) PatchAssistantMessage(id string, msg ai.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Entries {
		e := &m.Entries[i]
		if e.ID != id || e.Message == nil || e.Message.Role != ai.RoleAssistant {
			continue
		}
		msg.Role = ai.RoleAssistant
		ai.NormalizeMessage(&msg)
		cp := msg
		e.Message = &cp
		return nil
	}
	return fmt.Errorf("assistant message %q not found", id)
}

// AppendModelChange записывает смену модели в сессию.
func (m *Manager) AppendModelChange(modelName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:      "model_change",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &ai.Message{Role: ai.RoleSystem, Content: "model: " + modelName},
	})
}

// AppendBranchSummary добавляет system-запись с кратким summary ветки.
func (m *Manager) AppendBranchSummary(summary string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendEntry(Entry{
		Type:      "branch_summary",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Message:   &ai.Message{Role: ai.RoleSystem, Content: summary},
	})
}

func (m *Manager) appendEntry(entry Entry) (string, error) {
	id := newID()
	entry.ID = id
	var parent *string
	if m.leafID != "" {
		p := m.leafID
		parent = &p
	}
	entry.ParentID = parent
	m.Entries = append(m.Entries, entry)
	m.leafID = id
	return id, nil
}

// ActiveBranch возвращает записи на пути от корня до текущего leaf.
func (m *Manager) ActiveBranch() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.branchToLeaf()
}

// BuildMessages собирает LLM-сообщения активной ветки (с нормализацией).
func (m *Manager) BuildMessages() []ai.Message {
	msgs := m.BuildContextEntries()
	for i := range msgs {
		ai.NormalizeMessage(&msgs[i])
	}
	return msgs
}

func (m *Manager) branchToLeaf() []Entry {
	if m.leafID == "" {
		return nil
	}
	byID := map[string]Entry{}
	for _, e := range m.Entries {
		byID[e.ID] = e
	}
	var chain []Entry
	cur := m.leafID
	for cur != "" {
		e, ok := byID[cur]
		if !ok {
			break
		}
		chain = append(chain, e)
		if e.ParentID == nil {
			break
		}
		cur = *e.ParentID
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// StripImagesFromActiveBranch удаляет изображения из сообщений активной ветки,
// заменяя их текстовыми заглушками, чтобы text-only модели могли продолжить сессию.
func (m *Manager) StripImagesFromActiveBranch() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	branchIDs := map[string]struct{}{}
	for _, e := range m.branchToLeaf() {
		branchIDs[e.ID] = struct{}{}
	}
	stripped := 0
	for i := range m.Entries {
		if _, ok := branchIDs[m.Entries[i].ID]; !ok {
			continue
		}
		msg := m.Entries[i].Message
		if msg == nil || len(msg.Images) == 0 {
			continue
		}
		placeholder := imagePlaceholderText(msg.Images)
		if strings.TrimSpace(msg.Content) != "" {
			msg.Content = strings.TrimSpace(msg.Content) + "\n\n" + placeholder
		} else {
			msg.Content = placeholder
		}
		msg.Images = nil
		stripped++
	}
	return stripped
}

func imagePlaceholderText(images []ai.ImageContent) string {
	if len(images) == 0 {
		return ""
	}
	var b strings.Builder
	for i, img := range images {
		mime := img.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[image %d: %s, %d bytes base64]", i+1, mime, len(img.Data))
	}
	return b.String()
}

// Save записывает заголовок и записи сессии в JSONL-файл path.
func (m *Manager) Save(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	hb, _ := json.Marshal(m.Header)
	if _, err := w.Write(append(hb, '\n')); err != nil {
		return err
	}
	for _, e := range m.Entries {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return w.Flush()
}

// FilePath возвращает путь к файлу сессии на диске (может быть пустым).
func (m *Manager) FilePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Path
}

// Open загружает сессию из JSONL-файла path.
func Open(path string) (*Manager, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	m := &Manager{Path: path}
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		if raw.Type == "session" {
			if err := json.Unmarshal(line, &m.Header); err != nil {
				return nil, err
			}
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		m.Entries = append(m.Entries, e)
		m.leafID = e.ID
	}
	migrateSession(m)
	return m, sc.Err()
}

func migrateSession(m *Manager) {
	// Двойной reader: принимает v3, legacy v4 и старые форматы; нормализует roles/entries.
	for i := range m.Entries {
		normalizeEntry(&m.Entries[i])
	}
	if m.Header.Version != FormatVersion {
		m.Header.Version = FormatVersion
	}
}

func normalizeEntry(e *Entry) {
	if e == nil {
		return
	}
	// Legacy stell type:"bash" → сохраняется для BuildContextEntries; roles сообщений нормализуются.
	if e.Message != nil {
		ai.NormalizeMessage(e.Message)
	}
}

// RemoveLastAssistantOnBranch удаляет последнее assistant-сообщение на активной ветке.
func (m *Manager) RemoveLastAssistantOnBranch() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	branch := m.branchToLeaf()
	for i := len(branch) - 1; i >= 0; i-- {
		e := branch[i]
		if e.Type != "message" || e.Message == nil || e.Message.Role != ai.RoleAssistant {
			continue
		}
		id := e.ID
		filtered := m.Entries[:0]
		for _, ent := range m.Entries {
			if ent.ID != id {
				filtered = append(filtered, ent)
			}
		}
		m.Entries = filtered
		if e.ParentID != nil {
			m.leafID = *e.ParentID
		} else {
			m.leafID = ""
		}
		return true
	}
	return false
}

// NewSessionPath формирует путь нового JSONL-файла сессии для cwd.
func NewSessionPath(root, cwd string) string {
	dir := SessionDir(root, cwd)
	name := time.Now().UTC().Format("20060102-150405") + "_" + newID() + ".jsonl"
	return filepath.Join(dir, name)
}

func newID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
