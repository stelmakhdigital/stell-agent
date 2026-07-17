package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// IgnoreMatcher применяет правила .gitignore и .stellignore.
type IgnoreMatcher struct {
	patterns []string
}

// LoadIgnore загружает .gitignore и .stellignore из workspace.
func LoadIgnore(workspace string) *IgnoreMatcher {
	var lines []string
	for _, name := range []string{".gitignore", ".stellignore"} {
		data, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			continue
		}
		lines = append(lines, strings.Split(string(data), "\n")...)
	}
	return &IgnoreMatcher{patterns: compilePatterns(lines)}
}

func compilePatterns(lines []string) []string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// Skip сообщает, нужно ли пропустить относительный путь.
func (m *IgnoreMatcher) Skip(relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	if relPath == ".git" || strings.HasPrefix(relPath, ".git/") {
		return true
	}
	if m == nil {
		return false
	}
	for _, pat := range m.patterns {
		if matchIgnore(pat, relPath) {
			return true
		}
	}
	return false
}

func matchIgnore(pat, path string) bool {
	pat = filepath.ToSlash(pat)
	if strings.HasSuffix(pat, "/") {
		pat = strings.TrimSuffix(pat, "/")
		if path == pat || strings.HasPrefix(path, pat+"/") {
			return true
		}
		return false
	}
	if strings.Contains(pat, "*") {
		return simpleGlob(pat, path)
	}
	return path == pat || strings.HasPrefix(path, pat+"/")
}

func simpleGlob(pat, path string) bool {
	if pat == "*" {
		return true
	}
	if strings.HasPrefix(pat, "*") {
		suffix := strings.TrimPrefix(pat, "*")
		return strings.HasSuffix(path, suffix)
	}
	if strings.HasSuffix(pat, "*") {
		prefix := strings.TrimSuffix(pat, "*")
		return strings.HasPrefix(path, prefix)
	}
	return pat == path
}
