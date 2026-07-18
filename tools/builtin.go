package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/stelmakhdigital/stell-ai"
)

// CoreTools — стандартные встроенные инструменты (всегда регистрируются, кроме --no-tools).
var CoreTools = []string{"read", "write", "edit", "bash"}

// CodingTools — опциональные coding-инструменты (включаются через --tools).
var CodingTools = []string{"grep", "find", "ls"}

// RegisterBuiltins регистрирует все встроенные инструменты (core + coding).
// Чтобы ограничить набор, доступный LLM, вызовите ResolveActiveTools и Runtime.SetActiveTools.
func RegisterBuiltins(r *Runtime) error {
	return RegisterBuiltinTools(r, true)
}

// RegisterBuiltinTools регистрирует core-инструменты и при includeCoding=true — grep/find/ls.
func RegisterBuiltinTools(r *Runtime, includeCoding bool) error {
	core := []Tool{readTool{}, writeTool{}, editTool{}, bashTool{}}
	for _, t := range core {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	if !includeCoding {
		return nil
	}
	for _, t := range []Tool{lsTool{}, grepTool{}, findTool{}} {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

type readTool struct{}

func (readTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:             "read",
		Description:      "Read a file from the workspace.",
		PromptSnippet:    "Read file contents",
		PromptGuidelines: []string{"Use read to examine files instead of cat or sed."},
		Parameters: schema(map[string]any{
			"path":      prop("string", "File path relative to workspace"),
			"startLine": prop("integer", "First line (1-based)"),
			"endLine":   prop("integer", "Last line (1-based)"),
		}, "path"),
	}
}

func (readTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	p, err := requiredStr(args, "path")
	if err != nil {
		return Result{}, err
	}
	abs, rel, err := ResolvePath(env.Workspace, p)
	if err != nil {
		return Result{}, err
	}
	if env.Ignore != nil && env.Ignore.Skip(rel) {
		return Result{}, fmt.Errorf("path %q is ignored", rel)
	}
	data, err := env.FS.ReadFile(ctx, abs)
	if err != nil {
		return Result{}, err
	}
	content := string(data)
	start, end := intArg(args, "startLine", 0), intArg(args, "endLine", 0)
	if start > 0 || end > 0 {
		lines := strings.Split(content, "\n")
		if start < 1 {
			start = 1
		}
		if end < 1 || end > len(lines) {
			end = len(lines)
		}
	if start > len(lines) {
		return Result{}, fmt.Errorf("startLine beyond file length")
	}
	content = strings.Join(lines[start-1:end], "\n")
	}
	return spillToolOutput(content, "stell-read")
}

func spillToolOutput(content, prefix string) (Result, error) {
	if len(content) <= MaxOutputBytes {
		return Result{Content: content}, nil
	}
	f, err := os.CreateTemp("", prefix+"-*.txt")
	if err != nil {
		content = content[:MaxOutputBytes] + fmt.Sprintf("\n… (truncated at %d bytes)", MaxOutputBytes)
		return Result{Content: content, Truncated: true}, nil
	}
	_, _ = f.WriteString(content)
	fullPath := f.Name()
	_ = f.Close()
	preview := content[:MaxOutputBytes] + "\n… (truncated)"
	preview += fmt.Sprintf("\nFull output: %s", fullPath)
	return Result{Content: preview, FullOutputPath: fullPath, Truncated: true}, nil
}

type writeTool struct{}

func (writeTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:             "write",
		Description:      "Create or overwrite a file in the workspace.",
		PromptSnippet:    "Create or overwrite files",
		PromptGuidelines: []string{"Use write only for new files or complete rewrites."},
		Parameters: schema(map[string]any{
			"path":    prop("string", "File path"),
			"content": prop("string", "Full file content"),
		}, "path", "content"),
	}
}

func (writeTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	p, err := requiredStr(args, "path")
	if err != nil {
		return Result{}, err
	}
	content, ok := strArg(args, "content")
	if !ok {
		return Result{}, fmt.Errorf("argument content is required")
	}
	abs, rel, err := ResolvePath(env.Workspace, p)
	if err != nil {
		return Result{}, err
	}
	if shouldSkip(env, rel, false) {
		return Result{}, fmt.Errorf("path %q is ignored", rel)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Result{}, err
	}
	if err := env.FS.WriteFile(ctx, abs, []byte(content)); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("Wrote %s (%d bytes)", rel, len(content))}, nil
}

type editTool struct{}

func (editTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:          "edit",
		Description:   "Replace an exact unique fragment in a file.",
		PromptSnippet: "Make precise file edits with exact text replacement",
		PromptGuidelines: []string{
			"Use edit for precise changes (oldText must match the file exactly and be unique)",
			"Keep oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions.",
		},
		Parameters: schema(map[string]any{
			"path":    prop("string", "File path"),
			"oldText": prop("string", "Exact text to replace (must be unique)"),
			"newText": prop("string", "Replacement text"),
		}, "path", "oldText", "newText"),
	}
}

func (editTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	p, err := requiredStr(args, "path")
	if err != nil {
		return Result{}, err
	}
	oldText, err := requiredStr(args, "oldText")
	if err != nil {
		return Result{}, err
	}
	newText, ok := strArg(args, "newText")
	if !ok {
		return Result{}, fmt.Errorf("argument newText is required")
	}
	abs, rel, err := ResolvePath(env.Workspace, p)
	if err != nil {
		return Result{}, err
	}
	if shouldSkip(env, rel, false) {
		return Result{}, fmt.Errorf("path %q is ignored", rel)
	}
	data, err := env.FS.ReadFile(ctx, abs)
	if err != nil {
		return Result{}, err
	}
	content := string(data)
	switch n := strings.Count(content, oldText); {
	case n == 0:
		return Result{}, fmt.Errorf("oldText not found in %s", rel)
	case n > 1:
		return Result{}, fmt.Errorf("oldText occurs %d times in %s", n, rel)
	}
	updated := strings.Replace(content, oldText, newText, 1)
	if err := env.FS.WriteFile(ctx, abs, []byte(updated)); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("Edited %s", rel)}, nil
}

type lsTool struct{}

func (lsTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:          "ls",
		Description:   "List files in a directory.",
		PromptSnippet: "List directory contents",
		Parameters: schema(map[string]any{
			"path": prop("string", "Directory path (default workspace root)"),
		}),
	}
}

func (lsTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	p, _ := strArg(args, "path")
	abs, relBase, err := ResolvePath(env.Workspace, p)
	if err != nil {
		return Result{}, err
	}
	if shouldSkip(env, relBase, true) {
		return Result{}, fmt.Errorf("path %q is ignored", relBase)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Result{}, err
	}
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		childRel := filepath.Join(relBase, name)
		if relBase == "." {
			childRel = name
		}
		if shouldSkip(env, childRel, e.IsDir()) {
			continue
		}
		if e.IsDir() {
			b.WriteString(name)
			b.WriteString("/\n")
		} else {
			b.WriteString(name)
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		return Result{Content: "(empty)"}, nil
	}
	return Result{Content: strings.TrimRight(b.String(), "\n")}, nil
}

type grepTool struct{}

func (grepTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:          "grep",
		Description:   "Search file contents with a regular expression.",
		PromptSnippet: "Search file contents for patterns (respects .gitignore)",
		Parameters: schema(map[string]any{
			"pattern":    prop("string", "Regex pattern"),
			"path":       prop("string", "File or directory"),
			"maxResults": prop("integer", "Max matching lines"),
		}, "pattern"),
	}
}

func (grepTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	pattern, err := requiredStr(args, "pattern")
	if err != nil {
		return Result{}, err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Result{}, fmt.Errorf("bad pattern: %w", err)
	}
	p, _ := strArg(args, "path")
	abs, _, err := ResolvePath(env.Workspace, p)
	if err != nil {
		return Result{}, err
	}
	max := intArg(args, "maxResults", 100)
	var b strings.Builder
	count := 0
	scanFile := func(path, rel string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				fmt.Fprintf(&b, "%s:%d:%s\n", rel, i+1, line)
				count++
			}
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Result{}, err
	}
	if info.IsDir() {
		_ = walkWorkspace(env, abs, func(rel string, d fs.DirEntry) error {
			if d.IsDir() || count >= max {
				return nil
			}
			scanFile(filepath.Join(env.Workspace, rel), rel)
			if count >= max {
				return fs.SkipAll
			}
			return nil
		})
	} else {
		rel, _ := filepath.Rel(env.Workspace, abs)
		if !shouldSkip(env, rel, false) {
			scanFile(abs, rel)
		}
	}
	if count == 0 {
		return Result{Content: "no matches"}, nil
	}
	return Result{Content: strings.TrimRight(b.String(), "\n")}, nil
}

type findTool struct{}

func (findTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:          "find",
		Description:   "Find files by glob pattern.",
		PromptSnippet: "Find files by glob pattern (respects .gitignore)",
		Parameters: schema(map[string]any{
			"pattern": prop("string", "Glob pattern"),
			"path":    prop("string", "Directory to search"),
		}, "pattern"),
	}
}

func (findTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	pattern, err := requiredStr(args, "pattern")
	if err != nil {
		return Result{}, err
	}
	p, _ := strArg(args, "path")
	abs, _, err := ResolvePath(env.Workspace, p)
	if err != nil {
		return Result{}, err
	}
	var b strings.Builder
	err = walkWorkspace(env, abs, func(rel string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match(pattern, d.Name()); ok {
			b.WriteString(rel)
			b.WriteString("\n")
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if b.Len() == 0 {
		return Result{Content: "no matches"}, nil
	}
	return Result{Content: strings.TrimRight(b.String(), "\n")}, nil
}

type bashTool struct{}

func (bashTool) Def() ai.ToolDef {
	return ai.ToolDef{
		Name:          "bash",
		Description:   "Run a shell command in the workspace.",
		PromptSnippet: "Execute bash commands (ls, grep, find, etc.)",
		Parameters: schema(map[string]any{
			"command": prop("string", "Shell command"),
		}, "command"),
	}
}

func (bashTool) Call(ctx context.Context, env *Env, args map[string]any) (Result, error) {
	if err := bashAllowed(env); err != nil {
		return Result{}, err
	}
	command, err := requiredStr(args, "command")
	if err != nil {
		return Result{}, err
	}
	if err := rejectFilesystemScan(env.Workspace, command); err != nil {
		return Result{}, err
	}
	res, err := runBashCommand(ctx, env.Workspace, command, ProgressFrom(ctx))
	if err != nil {
		return Result{}, err
	}
	// Ненулевой код выхода — ошибка инструмента, чтобы счётчик последовательных
	// ошибок агента (и workspace auto-steer) учитывал неудачные команды.
	if res.ExitCode != 0 && !res.Cancelled {
		return Result{}, fmt.Errorf("command failed (exit %d): %s", res.ExitCode, res.Output)
	}
	out := Result{Content: res.Output, FullOutputPath: res.FullOutputPath, Truncated: res.Truncated}
	if res.FullOutputPath != "" {
		out.Content += fmt.Sprintf("\nFull output: %s", res.FullOutputPath)
	}
	return out, nil
}
