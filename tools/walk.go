package tools

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// shouldSkip сообщает, нужно ли пропустить путь по правилам IgnoreMatcher.
func shouldSkip(env *Env, rel string, isDir bool) bool {
	if env == nil || env.Ignore == nil {
		return false
	}
	if env.Ignore.Skip(rel) {
		return true
	}
	if isDir && env.Ignore.Skip(rel+"/") {
		return true
	}
	return false
}

// walkWorkspace обходит файлы workspace с учётом ignore-правил.
func walkWorkspace(env *Env, root string, fn func(rel string, d fs.DirEntry) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(env.Workspace, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if shouldSkip(env, rel, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		return fn(rel, d)
	})
}

// bashAllowed проверяет, разрешён ли запуск bash в текущем Env.
func bashAllowed(env *Env) error {
	if env.BashDeny {
		return fmt.Errorf("bash denied (--no-approve)")
	}
	if env.Trusted || env.BashAutoApprove {
		return nil
	}
	return fmt.Errorf("bash requires workspace trust or --approve")
}
