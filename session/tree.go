package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// TreeNode описывает одну запись в дереве сессии.
type TreeNode struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId,omitempty"`
	Type      string  `json:"type"`
	Label     string  `json:"label,omitempty"`
	Timestamp string  `json:"timestamp,omitempty"`
	Children  []string `json:"children,omitempty"`
	IsLeaf    bool    `json:"isLeaf"`
}

// FileInfo описывает файл сессии на диске.
type FileInfo struct {
	Path      string    `json:"path"`
	SessionID string    `json:"sessionId"`
	ModTime   time.Time `json:"modTime"`
	CWD       string    `json:"cwd,omitempty"`
}

// LeafID возвращает id кончика активной ветки.
func (m *Manager) LeafID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.leafID
}

// SetLeaf переключает кончик активной ветки на entryID.
func (m *Manager) SetLeaf(entryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entryID == "" {
		m.leafID = ""
		return nil
	}
	for _, e := range m.Entries {
		if e.ID == entryID {
			m.leafID = entryID
			return nil
		}
	}
	return fmt.Errorf("entry %q not found", entryID)
}

// ForkAt переключает активную ветку на entryID, чтобы следующее сообщение создало дочернюю ветку.
func (m *Manager) ForkAt(entryID string) error {
	return m.SetLeaf(entryID)
}

// ListChildren возвращает ID прямых дочерних записей для parentID (пустой parent = корни).
func (m *Manager) ListChildren(parentID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listChildrenUnlocked(parentID)
}

func (m *Manager) listChildrenUnlocked(parentID string) []string {
	var out []string
	for _, e := range m.Entries {
		switch {
		case parentID == "" && e.ParentID == nil:
			out = append(out, e.ID)
		case e.ParentID != nil && *e.ParentID == parentID:
			out = append(out, e.ID)
		}
	}
	return out
}

// WalkTree обходит все записи в порядке parent-first.
func (m *Manager) WalkTree(fn func(Entry, int) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	byID := map[string]Entry{}
	children := map[string][]string{}
	for _, e := range m.Entries {
		byID[e.ID] = e
		pid := ""
		if e.ParentID != nil {
			pid = *e.ParentID
		}
		children[pid] = append(children[pid], e.ID)
	}
	var walk func(pid string, depth int)
	walk = func(pid string, depth int) {
		for _, id := range children[pid] {
			e := byID[id]
			if !fn(e, depth) {
				return
			}
			walk(id, depth+1)
		}
	}
	walk("", 0)
}

// GetTree возвращает плоский список узлов дерева с id дочерних.
func (m *Manager) GetTree() []TreeNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getTreeUnlocked()
}

func (m *Manager) getTreeUnlocked() []TreeNode {
	var out []TreeNode
	for _, e := range m.Entries {
		label := e.Type
		if e.Message != nil {
			role := string(e.Message.Role)
			snip := e.Message.Content
			if len(snip) > 60 {
				snip = snip[:60] + "…"
			}
			label = role + ": " + snip
		}
		out = append(out, TreeNode{
			ID: e.ID, ParentID: e.ParentID, Type: e.Type, Label: label,
			Timestamp: e.Timestamp,
			Children:  m.listChildrenUnlocked(e.ID),
			IsLeaf:    e.ID == m.leafID,
		})
	}
	return out
}

// Clone возвращает глубокую копию с новым id заголовка сессии.
func (m *Manager) Clone() *Manager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h := m.Header
	h.ID = newID()
	h.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	entries := make([]Entry, len(m.Entries))
	copy(entries, m.Entries)
	return &Manager{
		Header:  h,
		Entries: entries,
		leafID:  m.leafID,
	}
}

// ListProgress отчитывается о прогрессе сканирования файлов сессий (loaded/total).
type ListProgress func(loaded, total int)

// ListFiles возвращает JSONL-файлы сессий для cwd, отсортированные по времени изменения (новые первыми).
func ListFiles(root, cwd string) ([]FileInfo, error) {
	return ListFilesWithProgress(root, cwd, nil)
}

// ListFilesWithProgress сканирует файлы сессий и опционально сообщает о прогрессе.
func ListFilesWithProgress(root, cwd string, onProgress ListProgress) ([]FileInfo, error) {
	dir := SessionDir(root, cwd)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	total := len(paths)
	var out []FileInfo
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		fi := FileInfo{Path: path, ModTime: info.ModTime()}
		if m, err := Open(path); err == nil {
			fi.SessionID = m.Header.ID
			fi.CWD = m.Header.CWD
		}
		out = append(out, fi)
		if onProgress != nil {
			onProgress(i+1, total)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Export копирует файл сессии в destPath.
func (m *Manager) Export(destPath string) error {
	m.mu.RLock()
	path := m.Path
	m.mu.RUnlock()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destPath, data, 0o644)
	}
	return m.Save(destPath)
}
