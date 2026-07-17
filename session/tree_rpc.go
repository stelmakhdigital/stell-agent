package session

// NestedTreeNode — вложенное дерево для RPC get_tree.
type NestedTreeNode struct {
	Entry    TreeNode         `json:"entry"`
	Children []NestedTreeNode `json:"children,omitempty"`
}

// BuildNestedTree возвращает вложенное дерево от корней.
func (m *Manager) BuildNestedTree() []NestedTreeNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	flat := m.getTreeUnlocked()
	byID := map[string]TreeNode{}
	for _, n := range flat {
		byID[n.ID] = n
	}
	var build func(id string) NestedTreeNode
	build = func(id string) NestedTreeNode {
		n := byID[id]
		node := NestedTreeNode{Entry: n}
		for _, cid := range n.Children {
			node.Children = append(node.Children, build(cid))
		}
		return node
	}
	var roots []NestedTreeNode
	for _, n := range flat {
		if n.ParentID == nil {
			roots = append(roots, build(n.ID))
		}
	}
	return roots
}

// FilterEntriesSince возвращает записи после entryID (не включая), или все, если since пуст.
func (m *Manager) FilterEntriesSince(since string) []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if since == "" {
		return append([]Entry(nil), m.Entries...)
	}
	found := false
	var out []Entry
	for _, e := range m.Entries {
		if found {
			out = append(out, e)
			continue
		}
		if e.ID == since {
			found = true
		}
	}
	if !found {
		return append([]Entry(nil), m.Entries...)
	}
	return out
}

// SwitchToEntry устанавливает leaf без fork (навигация по ветке).
func (m *Manager) SwitchToEntry(entryID string) error {
	return m.SetLeaf(entryID)
}
