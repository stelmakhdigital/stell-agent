package harness

import (
	"strings"

	"github.com/stelmakhdigital/stell-ai"
	"github.com/stelmakhdigital/stell-agent/session"
)

// CompactionSettings — пороги компактирования контекста.
type CompactionSettings struct {
	Enabled          bool
	ReserveTokens    int
	KeepRecentTokens int
}

// DefaultCompactionSettings возвращает значения по умолчанию.
func DefaultCompactionSettings() CompactionSettings {
	return CompactionSettings{
		Enabled:          true,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	}
}

// ContextUsageEstimate описывает приблизительное давление контекста для сообщений.
type ContextUsageEstimate struct {
	Tokens         int
	UsageTokens    int
	TrailingTokens int
	LastUsageIndex *int
}

// EstimateContextTokens оценивает токены по usage провайдера в assistant-сообщениях,
// иначе — эвристика char/4.
func EstimateContextTokens(msgs []ai.Message) ContextUsageEstimate {
	if idx, usage := lastAssistantUsage(msgs); usage != nil {
		usageTokens := contextTokensFromUsage(*usage)
		trailing := 0
		for i := idx + 1; i < len(msgs); i++ {
			trailing += estimateMessageTokens(msgs[i])
		}
		return ContextUsageEstimate{
			Tokens:         usageTokens + trailing,
			UsageTokens:    usageTokens,
			TrailingTokens: trailing,
			LastUsageIndex: &idx,
		}
	}
	total := CompactionEstimate(msgs)
	return ContextUsageEstimate{
		Tokens:         total,
		TrailingTokens: total,
	}
}

func lastAssistantUsage(msgs []ai.Message) (int, *ai.Usage) {
	// Usage в сообщениях ещё не подключён в session wire stell; зарезервировано на будущее.
	return -1, nil
}

func contextTokensFromUsage(u ai.Usage) int {
	total := u.InputTokens + u.OutputTokens + u.CacheRead + u.CacheWrite
	if total > 0 {
		return total
	}
	return u.InputTokens + u.OutputTokens
}

func estimateMessageTokens(m ai.Message) int {
	n := len(m.Content)/4 + 1
	for _, tc := range m.ToolCalls {
		n += len(tc.Name) + 8
	}
	return n
}

// CompactionPreparation описывает запланированную точку разреза компактирования.
type CompactionPreparation struct {
	FirstKeptEntryID string
	TokensBefore     int
	KeptEntries      int
	RemovedEntries   int
	IsSplitTurn      bool
}

// PrepareCompaction находит точку разреза на записях активной ветки.
func PrepareCompaction(entries []session.Entry, settings CompactionSettings) (*CompactionPreparation, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if entries[len(entries)-1].Type == "compaction" {
		return nil, nil
	}
	if settings.KeepRecentTokens <= 0 {
		settings.KeepRecentTokens = DefaultCompactionSettings().KeepRecentTokens
	}
	msgs := entriesToMessages(entries)
	tokensBefore := EstimateContextTokens(msgs).Tokens
	keep := settings.KeepRecentTokens / 64
	if keep < 2 {
		keep = 2
	}
	if len(entries) <= keep {
		return nil, nil
	}
	cut := len(entries) - keep
	firstKept := entries[cut]
	if firstKept.ID == "" {
		return nil, nil
	}
	return &CompactionPreparation{
		FirstKeptEntryID: firstKept.ID,
		TokensBefore:     tokensBefore,
		KeptEntries:      keep,
		RemovedEntries:   cut,
	}, nil
}

func entriesToMessages(entries []session.Entry) []ai.Message {
	m := &session.Manager{Entries: entries}
	if len(entries) > 0 {
		_ = m.SetLeaf(entries[len(entries)-1].ID)
	}
	return m.BuildMessages()
}

// BranchSummaryPrompt формирует user-prompt для суммаризации ветки.
func BranchSummaryPrompt(conversation string, previousSummary string) string {
	if strings.TrimSpace(previousSummary) == "" {
		return branchSummarizationPrompt + "\n\n<conversation>\n" + conversation + "\n</conversation>"
	}
	return branchUpdateSummarizationPrompt + "\n\n<previous-summary>\n" + previousSummary +
		"\n</previous-summary>\n\n<conversation>\n" + conversation + "\n</conversation>"
}

// Промпты суммаризации (подмножество).
const SummarizationSystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI assistant, then produce a structured summary following the exact format specified.`

const branchSummarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.`

const branchUpdateSummarizationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.`
