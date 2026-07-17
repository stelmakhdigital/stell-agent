package agent

import (
	"context"

	"github.com/stelmakhdigital/ai"
)

// StreamFn подменяет вызов Registry.Chat (например HTTP-прокси).
// Если nil, Loop использует Registry.Get().Chat.
type StreamFn func(ctx context.Context, req ai.ChatRequest) (<-chan ai.ChatEvent, error)

// AssistantTurn — результат прокачки одного потока модели в сессию и события.
type AssistantTurn struct {
	Message    ai.Message
	ToolCalls  []ai.ToolCall
	StopReason string
	Usage      *ai.Usage
	// SkipAppend — ProcessStream уже сохранил assistant-сообщение.
	SkipAppend bool
}

// ProcessStreamFn потребляет поток провайдера и генерирует продуктовые события.
// Если nil, Loop использует стандартный pump token/toolCall и AppendMessage.
type ProcessStreamFn func(ctx context.Context, stream <-chan ai.ChatEvent, ch chan<- Event) (AssistantTurn, error)

// TurnDecision управляет потоком цикла после финализации хода assistant.
type TurnDecision int

const (
	// TurnExecuteTools выполняет вызовы инструментов (по умолчанию, если они есть).
	TurnExecuteTools TurnDecision = iota
	// TurnDone останавливает цикл (отправляет done с StopReason, если ещё не отправлен).
	TurnDone
	// TurnContinue пропускает инструменты и начинает следующую итерацию (например, пустой toolUse).
	TurnContinue
	// TurnRejectTools записывает результаты инструментов с ошибкой и продолжает (усечение по length).
	TurnRejectTools
	// TurnAbort останавливается после пути ошибки (ProcessStream/хуки уже отправили события).
	TurnAbort
)

// AfterAssistantTurn определяет поведение после записи assistant-сообщения.
// Если nil, Loop останавливается без tool calls, иначе выполняет их.
// skipDoneEmit: при Action TurnDone Loop не отправляет EventDone (вызывающий уже отправил).
type AfterAssistantTurn func(ctx context.Context, iter int, turn AssistantTurn) (action TurnDecision, doneStopReason string, skipDoneEmit bool, err error)

// ShouldStopAfterTurn вызывается после инструментов (+ попытка steer):
// при stop=true цикл завершается без обработки follow-up и без следующего вызова LLM.
type ShouldStopAfterTurn func(ctx context.Context, iter int, turn AssistantTurn, outcomes []ToolCallOutcome) (stop bool, err error)

// FollowUpMessage извлекает in-loop follow-up из очереди.
type FollowUpMessage func() (msg ai.Message, ok bool)

// PrepareNextTurn выполняется в начале каждой итерации (wrap-up steers, notices).
type PrepareNextTurn func(ctx context.Context, iter int) error

// AfterTools выполняется после batch инструментов (saveSession, tool-error steer).
// continueLoop=false завершает цикл без max_iterations (вызывающий отправил done).
type AfterTools func(ctx context.Context, iter int, outcomes []ToolCallOutcome) (continueLoop bool, err error)

// SteerMessage вставляет user-сообщение после инструментов (поддерживает изображения).
// Перекрывает SteerFn, если задан.
type SteerMessage func() (msg ai.Message, ok bool)
