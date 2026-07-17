// Package harness — helpers компактирования и system prompt для агентов.
// Discovery продукта остаётся в coding-agent.
package harness

import (
	"fmt"
	"strings"

	"github.com/stelmakhdigital/ai"
	"stell/agent/session"
)

// CompactionEstimate возвращает приблизительное давление токенов для сессии.
func CompactionEstimate(msgs []ai.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)/4 + 1
		for _, tc := range m.ToolCalls {
			n += len(tc.Name) + 8
		}
	}
	return n
}

// NeedsCompaction сообщает, превышает ли контекст сессии пороги резерва.
func NeedsCompaction(msgs []ai.Message, contextWindow, reserveTokens int) bool {
	if contextWindow <= 0 {
		return false
	}
	if reserveTokens <= 0 {
		reserveTokens = 16384
	}
	return CompactionEstimate(msgs) > contextWindow-reserveTokens
}

// BranchSummaryLine формирует короткую метку для кончика ветки сессии.
func BranchSummaryLine(s *session.Manager) string {
	if s == nil || s.Header.ID == "" {
		return ""
	}
	msgs := s.BuildMessages()
	if len(msgs) == 0 {
		return fmt.Sprintf("session %s (empty)", s.Header.ID)
	}
	last := msgs[len(msgs)-1]
	snip := strings.TrimSpace(last.Content)
	if len(snip) > 80 {
		snip = snip[:80] + "…"
	}
	return fmt.Sprintf("%s · %s: %s", s.Header.ID, last.Role, snip)
}

// MergeSystemPrompt объединяет base и append-блоки с пустыми строками между ними.
func MergeSystemPrompt(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(p)
	}
	return b.String()
}
