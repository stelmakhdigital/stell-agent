// Package tools — runtime встроенных инструментов агента и выбор активного набора.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/stelmakhdigital/ai"
)

// MaxOutputBytes — лимит размера вывода инструмента до spill на диск.
const (
	MaxOutputBytes = 64 * 1024
)

// Env — окружение выполнения инструментов (workspace, FS, trust/bash).
type Env struct {
	Workspace       string
	FS              FileSystem
	Ignore          *IgnoreMatcher
	Trusted         bool
	BashAutoApprove bool
	BashDeny        bool
}

// Result — результат вызова инструмента.
type Result struct {
	Content        string
	FullOutputPath string
	Truncated      bool
	Terminate      bool // terminate — batch останавливается, когда все true
}

// Tool — встроенный или пользовательский инструмент LLM.
type Tool interface {
	Def() ai.ToolDef
	Call(ctx context.Context, env *Env, args map[string]any) (Result, error)
}

// Runtime хранит зарегистрированные инструменты и активный набор для LLM.
type Runtime struct {
	mu          sync.RWMutex
	tools       map[string]Tool
	env         Env
	active      map[string]bool
	activeOrder []string
}

// NewRuntime создаёт Runtime с окружением env (FS и Ignore подставляются по умолчанию).
func NewRuntime(env Env) *Runtime {
	if env.FS == nil {
		env.FS = OSFileSystem{}
	}
	if env.Ignore == nil && env.Workspace != "" {
		env.Ignore = LoadIgnore(env.Workspace)
	}
	return &Runtime{tools: map[string]Tool{}, env: env}
}

// Register регистрирует инструмент; ошибка, если имя уже занято.
func (r *Runtime) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Def().Name
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// RegisterOrReplace регистрирует инструмент или заменяет существующую регистрацию (переопределение расширением).
func (r *Runtime) RegisterOrReplace(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Def().Name] = t
	return nil
}

// Unregister удаляет инструменты по именам.
func (r *Runtime) Unregister(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		delete(r.tools, name)
	}
}

// SetActiveTools ограничивает инструменты, доступные LLM. Пустой список восстанавливает все инструменты.
func (r *Runtime) SetActiveTools(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(names) == 0 {
		r.active = nil
		r.activeOrder = nil
		return
	}
	r.active = map[string]bool{}
	r.activeOrder = append([]string(nil), names...)
	for _, n := range names {
		r.active[n] = true
	}
}

// ListTools возвращает определения зарегистрированных инструментов (имя и описание).
func (r *Runtime) ListTools() []map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]string, 0, len(names))
	for _, n := range names {
		d := r.tools[n].Def()
		out = append(out, map[string]string{"name": d.Name, "description": d.Description})
	}
	return out
}

// AllNames возвращает имена всех зарегистрированных инструментов.
func (r *Runtime) AllNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}

func (r *Runtime) isActive(name string) bool {
	if r.active == nil {
		return true
	}
	return r.active[name]
}

// Defs возвращает определения активных инструментов для ChatRequest.
func (r *Runtime) Defs() []ai.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active != nil && len(r.activeOrder) > 0 {
		defs := make([]ai.ToolDef, 0, len(r.activeOrder))
		for _, name := range r.activeOrder {
			if t, ok := r.tools[name]; ok && r.isActive(name) {
				defs = append(defs, t.Def())
			}
		}
		return defs
	}
	// Детерминированный порядок: сначала builtins, затем остальные по алфавиту —
	// system prompt и список инструментов в API стабильны между запусками.
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := builtinRank(names[i]), builtinRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	defs := make([]ai.ToolDef, 0, len(names))
	for _, n := range names {
		if r.isActive(n) {
			defs = append(defs, r.tools[n].Def())
		}
	}
	return defs
}

var builtinOrder = map[string]int{
	"read": 0, "bash": 1, "edit": 2, "write": 3, "grep": 4, "find": 5, "ls": 6,
}

func builtinRank(name string) int {
	if r, ok := builtinOrder[name]; ok {
		return r
	}
	return len(builtinOrder)
}

// Execute вызывает зарегистрированный активный инструмент.
func (r *Runtime) Execute(ctx context.Context, call ai.ToolCall) (Result, error) {
	r.mu.RLock()
	t, ok := r.tools[call.Name]
	active := r.isActive(call.Name)
	r.mu.RUnlock()
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", call.Name)
	}
	if !active {
		return Result{}, fmt.Errorf("tool %q is not active", call.Name)
	}
	res, err := t.Call(ctx, &r.env, call.Args)
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// ResolvePath разрешает путь относительно workspace и отклоняет выход за его границы.
func ResolvePath(workspace, p string) (abs, rel string, err error) {
	if p == "" {
		p = "."
	}
	ws, err := evalSymlinksIfExists(filepath.Clean(workspace))
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Join(ws, p)
	}
	abs = filepath.Clean(abs)
	resolved, err := resolveWithinWorkspace(ws, abs)
	if err != nil {
		return "", "", fmt.Errorf("path %q is outside workspace", p)
	}
	rel, err = filepath.Rel(ws, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q is outside workspace", p)
	}
	return resolved, rel, nil
}

// ResolveOutputPath разрешает путь назначения в workspace, используя defaultName при пустом значении.
func ResolveOutputPath(workspace, dest, defaultName string) (abs, rel string, err error) {
	if dest == "" {
		dest = defaultName
	}
	return ResolvePath(workspace, dest)
}

func evalSymlinksIfExists(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if os.IsNotExist(err) {
		return path, nil
	}
	return "", err
}

func resolveWithinWorkspace(workspace, abs string) (string, error) {
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("outside workspace")
		}
		parentResolved, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			if os.IsNotExist(perr) {
				resolved = abs
			} else {
				return "", perr
			}
		} else {
			resolved = filepath.Join(parentResolved, filepath.Base(abs))
		}
	} else {
		abs = resolved
		resolved = abs
	}
	rel, err := filepath.Rel(workspace, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("outside workspace")
	}
	return resolved, nil
}

func strArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func requiredStr(args map[string]any, key string) (string, error) {
	s, ok := strArg(args, key)
	if !ok || s == "" {
		return "", fmt.Errorf("argument %q is required", key)
	}
	return s, nil
}

func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return def
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}
