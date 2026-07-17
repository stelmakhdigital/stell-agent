package agent_test

import (
	"testing"

	"github.com/stelmakhdigital/ai"
	"stell/agent"
)

func TestConvertToLlmDropsCustom(t *testing.T) {
	in := []ai.Message{
		{Role: ai.RoleUser, Content: "hi"},
		{Role: ai.RoleCustom, Content: "note"},
		{Role: ai.RoleAssistant, Content: "ok"},
		{Role: ai.RoleTool, Content: "tool out", ToolCallID: "c1"},
	}
	out := agent.ConvertToLlm(in)
	if len(out) != 3 {
		t.Fatalf("len=%d want 3", len(out))
	}
}
