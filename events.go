package agent

import "github.com/stelmakhdigital/ai"

// EventType — события цикла агента.
type EventType string

const (
	EventToken          EventType = "token"
	EventThinkingToken  EventType = "thinking"
	EventMessageStart   EventType = "messageStart"
	EventMessage        EventType = "message"
	EventMessageUpdate  EventType = "messageUpdate"
	EventToolCall       EventType = "toolCall"
	EventToolCallDelta  EventType = "toolCallDelta"
	EventToolProgress   EventType = "toolProgress"
	EventToolResult     EventType = "toolResult"
	EventDone           EventType = "done"
	EventError          EventType = "error"
	EventAutoRetryStart EventType = "autoRetryStart"
	EventAutoRetryEnd   EventType = "autoRetryEnd"
	EventNotice         EventType = "notice"
	EventLabel          EventType = "label"
)

// ToolResult — наблюдение за завершённым вызовом инструмента.
type ToolResult struct {
	CallID         string
	Name           string
	Content        string
	Error          string
	FullOutputPath string
	Truncated      bool
}

// MessageUpdate — патч streaming assistant-сообщения.
type MessageUpdate struct {
	EventType    string
	ContentIndex int
	Delta        string
	Partial      ai.Message
	ToolCall     *ai.ToolCall
}

// AutoRetryInfo описывает автоматический цикл повтора провайдера.
type AutoRetryInfo struct {
	Attempt      int
	MaxAttempts  int
	DelayMs      int
	ErrorMessage string
	WillRetry    bool
	Success      bool
	FinalError   string
}

// Event генерируется циклом агента для подписчиков UI, RPC и SDK.
type Event struct {
	Type          EventType
	Token         string
	Thinking      string
	Message       ai.Message
	MessageUpdate *MessageUpdate
	AutoRetry     *AutoRetryInfo
	ToolCall      *ai.ToolCall
	ToolCallDelta string
	ToolCallIndex int
	ToolCallID    string
	ToolCallName  string
	ToolResult    *ToolResult
	Usage         *ai.Usage
	StopReason    string
	WillRetry     bool
	Notice        string
	Err           error
}
