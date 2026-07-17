package agent

import "github.com/stelmakhdigital/ai"

// ConvertToLlm фильтрует сообщения сессии до ролей, понятных LLM, перед вызовом провайдера.
func ConvertToLlm(messages []ai.Message) []ai.Message {
	out := make([]ai.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case ai.RoleUser, ai.RoleAssistant, ai.RoleSystem:
			out = append(out, m)
		default:
			if ai.IsToolRole(m.Role) {
				out = append(out, m)
			}
		}
	}
	return out
}

// TransformContext — опциональный хук перед ConvertToLlm.
type TransformContext func(messages []ai.Message) []ai.Message

// PrepareMessages выполняет transform (если задан), затем ConvertToLlm.
func PrepareMessages(messages []ai.Message, transform TransformContext) []ai.Message {
	if transform != nil {
		messages = transform(messages)
	}
	return ConvertToLlm(messages)
}
