package agent

import (
	"context"
	"sync"

	"stell/agent/session"
	"stell/agent/tools"

	"github.com/stelmakhdigital/ai"
	"github.com/stelmakhdigital/ai/provider"
)

// QueueMode — режим очереди steer/follow-up.
type QueueMode string

const (
	QueueOneAtATime QueueMode = "one-at-a-time"
	QueueAll        QueueMode = "all"
)

// Agent — публичный API: очереди + subscribe + Loop.
type Agent struct {
	mu sync.Mutex

	Loop Loop

	SteeringMode QueueMode
	FollowUpMode QueueMode

	steerQ    []string
	followUpQ []string

	subs   []chan Event
	subsMu sync.Mutex
}

// NewAgent создаёт Agent, привязанный к registry, tools и session.
func NewAgent(reg *provider.Registry, rt *tools.Runtime, sess *session.Manager, modelName, modelID string) *Agent {
	return &Agent{
		Loop: Loop{
			Registry:      reg,
			Tools:         rt,
			Sessions:      sess,
			ModelName:     modelName,
			ModelID:       modelID,
			ToolExecution: ToolExecutionParallel,
		},
		SteeringMode: QueueOneAtATime,
		FollowUpMode: QueueOneAtATime,
	}
}

// Subscribe возвращает канал событий агента. Для остановки вызовите unsubscribe.
func (a *Agent) Subscribe(buf int) (<-chan Event, func()) {
	if buf <= 0 {
		buf = 64
	}
	ch := make(chan Event, buf)
	a.subsMu.Lock()
	a.subs = append(a.subs, ch)
	a.subsMu.Unlock()
	unsub := func() {
		a.subsMu.Lock()
		defer a.subsMu.Unlock()
		for i, s := range a.subs {
			if s == ch {
				a.subs = append(a.subs[:i], a.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsub
}

func (a *Agent) broadcast(ev Event) {
	a.subsMu.Lock()
	defer a.subsMu.Unlock()
	for _, s := range a.subs {
		select {
		case s <- ev:
		default:
		}
	}
}

// Steer ставит steering-сообщение в очередь для активного хода.
func (a *Agent) Steer(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.SteeringMode == QueueOneAtATime {
		a.steerQ = []string{message}
		return
	}
	a.steerQ = append(a.steerQ, message)
}

// FollowUp ставит follow-up в очередь после завершения текущего хода.
func (a *Agent) FollowUp(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.FollowUpMode == QueueOneAtATime {
		a.followUpQ = []string{message}
		return
	}
	a.followUpQ = append(a.followUpQ, message)
}

func (a *Agent) takeSteer() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.steerQ) == 0 {
		return "", false
	}
	msg := a.steerQ[0]
	if a.SteeringMode == QueueAll {
		a.steerQ = a.steerQ[1:]
	} else {
		a.steerQ = nil
	}
	return msg, true
}

func (a *Agent) takeFollowUp() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.followUpQ) == 0 {
		return "", false
	}
	msg := a.followUpQ[0]
	a.followUpQ = a.followUpQ[1:]
	return msg, true
}

// Prompt запускает цикл агента для сообщения пользователя и рассылает события
// подписчикам и в опциональный канал events (не закрывается, если nil).
func (a *Agent) Prompt(ctx context.Context, message string, events chan<- Event) {
	a.Loop.SteerFn = func() (string, bool) { return a.takeSteer() }
	out := make(chan Event, 64)
	go a.Loop.Run(ctx, message, out)
	for ev := range out {
		a.broadcast(ev)
		if events != nil {
			events <- ev
		}
	}
	for {
		fu, ok := a.takeFollowUp()
		if !ok {
			break
		}
		a.Prompt(ctx, fu, events)
	}
}

// ConvertToLlm доступен для embedders.
func ConvertMessagesDefault(msgs []ai.Message) []ai.Message {
	return ConvertToLlm(msgs)
}
