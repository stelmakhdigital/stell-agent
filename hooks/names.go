package hooks

// Имена in-process хуков (строковые константы шины Bus).
const (
	SessionStart         = "session_start"
	SessionBeforeSwitch  = "session_before_switch"
	SessionBeforeFork    = "session_before_fork"
	BeforeAgentStart     = "before_agent_start"
	Input                = "input"
	ToolCall             = "tool_call"
	ToolResult           = "tool_result"
	SessionBeforeCompact = "session_before_compact"
	SessionBeforeTree    = "session_before_tree"
	TurnStart            = "turn_start"
	TurnEnd              = "turn_end"
	MessageStart         = "message_start"
	MessageEnd           = "message_end"
	UserBash             = "user_bash"
	ModelSelect          = "model_select"
	ThinkingLevelSelect  = "thinking_level_select"
	AgentStart           = "agent_start"
	AgentEnd             = "agent_end"
	SessionCompact       = "session_compact"
	Context              = "context"
	ProjectTrust         = "project_trust"
	MessageUpdate        = "message_update"

	ResourcesDiscover     = "resources_discover"
	SessionInfoChanged    = "session_info_changed"
	SessionTree           = "session_tree"
	SessionShutdown       = "session_shutdown"
	AgentSettled          = "agent_settled"
	ToolExecutionStart    = "tool_execution_start"
	ToolExecutionUpdate   = "tool_execution_update"
	ToolExecutionEnd      = "tool_execution_end"
	BeforeProviderHeaders = "before_provider_headers"
	BeforeProviderRequest = "before_provider_request"
	AfterProviderResponse = "after_provider_response"
)

// IsProviderHook сообщает, перехватывает ли хук HTTP-слой провайдера.
// Эти хуки могут пересылаться subprocess-расширениям как JSON-RPC-уведомления
// с сериализуемым подмножеством payload (URL, map заголовков, status); полные
// тела запросов опускаются ради размера и безопасности.
func IsProviderHook(name string) bool {
	switch name {
	case BeforeProviderHeaders, BeforeProviderRequest, AfterProviderResponse:
		return true
	}
	return false
}
