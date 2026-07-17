// Package hooks — in-process шина хуков агента (имена событий и Bus).
package hooks

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Event передаёт входные данные хука и изменяемые поля результата. Обработчики
// мутируют событие in-place (семантика stell) вместо возврата map-ответа.
type Event struct {
	Name      string
	SessionID string
	Payload   map[string]any

	// Изменяемые поля результата. Какие учитываются — зависит от хука:
	// AppendSystem — context/before_agent_start/session_before_compact;
	// Cancel — input/user_bash/session_before_switch/session_before_fork;
	// Block+Args — tool_call; Text — input; Command — user_bash;
	// Header — before_provider_headers.
	AppendSystem string
	Cancel       bool
	Block        bool
	Args         map[string]any
	Text         string
	Command      string
	Header       http.Header
}

// AppendSystemText добавляет текст в AppendSystem.
func (e *Event) AppendSystemText(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if e.AppendSystem == "" {
		e.AppendSystem = s
		return
	}
	e.AppendSystem += "\n\n" + s
}

// Handler — in-process callback хука. Обработчики выполняются синхронно
// в порядке регистрации и могут мутировать Event.
type Handler func(ctx context.Context, hc *Ctx, ev *Event) error

// ExecResult отражает результат bash-инструмента, доступный обработчикам.
type ExecResult struct {
	Output    string
	ExitCode  int
	Truncated bool
	Cancelled bool
}

// Ctx предоставляет host-возможности in-process обработчикам.
// Поля-функции подключаются при bootstrap; незаданные возможности возвращают
// ошибку при вызове.
type Ctx struct {
	Workspace string

	ExecFn            func(ctx context.Context, command string) (ExecResult, error)
	SendUserMessageFn func(ctx context.Context, message, deliverAs string) error
	AppendEntryFn     func(text string) (string, error)
	UIInputFn         func(ctx context.Context, message, placeholder, value string) (result string, cancelled bool, err error)
}

var errCapabilityUnavailable = fmt.Errorf("hook host capability not available")

// Exec выполняет shell-команду в workspace.
func (c *Ctx) Exec(ctx context.Context, command string) (ExecResult, error) {
	if c == nil || c.ExecFn == nil {
		return ExecResult{}, errCapabilityUnavailable
	}
	return c.ExecFn(ctx, command)
}

// SendUserMessage доставляет сообщение агенту (deliverAs: steer/followUp).
func (c *Ctx) SendUserMessage(ctx context.Context, message, deliverAs string) error {
	if c == nil || c.SendUserMessageFn == nil {
		return errCapabilityUnavailable
	}
	return c.SendUserMessageFn(ctx, message, deliverAs)
}

// AppendEntry записывает custom-запись в сессию.
func (c *Ctx) AppendEntry(text string) (string, error) {
	if c == nil || c.AppendEntryFn == nil {
		return "", errCapabilityUnavailable
	}
	return c.AppendEntryFn(text)
}

// UIInput показывает блокирующий input overlay в TUI.
func (c *Ctx) UIInput(ctx context.Context, message, placeholder, value string) (string, bool, error) {
	if c == nil || c.UIInputFn == nil {
		return "", false, errCapabilityUnavailable
	}
	return c.UIInputFn(ctx, message, placeholder, value)
}

// Bus — in-process шина хуков. Именованные обработчики выполняются первыми,
// затем catch-all (адаптер subprocess-расширений), всё синхронно в порядке
// регистрации. Emit на nil *Bus безопасен.
type Bus struct {
	mu          sync.RWMutex
	handlers    map[string][]Handler
	anyHandlers []Handler
	interests   []func(name string) bool
	host        *Ctx
}

// NewBus создаёт пустую шину хуков.
func NewBus() *Bus {
	return &Bus{handlers: map[string][]Handler{}}
}

// SetHostCtx подключает host-возможности, передаваемые каждому обработчику.
func (b *Bus) SetHostCtx(hc *Ctx) {
	b.mu.Lock()
	b.host = hc
	b.mu.Unlock()
}

// HostCtx возвращает подключённые host-возможности (может быть nil).
func (b *Bus) HostCtx() *Ctx {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.host
}

// On регистрирует in-process обработчик для именованного хука.
func (b *Bus) On(name string, h Handler) {
	if b == nil || h == nil {
		return
	}
	b.mu.Lock()
	b.handlers[name] = append(b.handlers[name], h)
	b.mu.Unlock()
}

// OnAny регистрирует catch-all обработчик для каждого emit хука.
// Обработчик должен фильтровать по ev.Name самостоятельно.
func (b *Bus) OnAny(h Handler) {
	if b == nil || h == nil {
		return
	}
	b.mu.Lock()
	b.anyHandlers = append(b.anyHandlers, h)
	b.mu.Unlock()
}

// RegisterInterest добавляет динамический предикат подписки. Catch-all обработчики,
// чьи предикаты возвращают false для имени, не делают HasSubscriber(name) true —
// это держит throttled-хуки (message_update) дёшевыми, когда никто не слушает.
func (b *Bus) RegisterInterest(fn func(name string) bool) {
	if b == nil || fn == nil {
		return
	}
	b.mu.Lock()
	b.interests = append(b.interests, fn)
	b.mu.Unlock()
}

// HasSubscriber сообщает, выполнится ли хотя бы один обработчик для хука.
func (b *Bus) HasSubscriber(name string) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	named := len(b.handlers[name]) > 0
	interests := b.interests
	b.mu.RUnlock()
	if named {
		return true
	}
	for _, fn := range interests {
		if fn(name) {
			return true
		}
	}
	return false
}

// Emit диспетчеризует событие именованным обработчикам, затем catch-all.
// Обработчики выполняются синхронно и могут мутировать ev. Выполнение
// продолжается после ошибок обработчиков; возвращается первая ошибка.
func (b *Bus) Emit(ctx context.Context, ev *Event) error {
	if b == nil || ev == nil {
		return nil
	}
	b.mu.RLock()
	named := append([]Handler(nil), b.handlers[ev.Name]...)
	catchAll := append([]Handler(nil), b.anyHandlers...)
	host := b.host
	b.mu.RUnlock()

	var firstErr error
	for _, h := range named {
		if err := h(ctx, host, ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, h := range catchAll {
		if err := h(ctx, host, ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// EmitNamed — удобная обёртка Emit: создаёт событие, диспетчеризует его и
// возвращает для чтения изменённых полей результата.
func (b *Bus) EmitNamed(ctx context.Context, name, sessionID string, payload map[string]any) (*Event, error) {
	ev := &Event{Name: name, SessionID: sessionID, Payload: payload}
	err := b.Emit(ctx, ev)
	return ev, err
}
