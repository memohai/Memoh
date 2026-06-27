package flow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/userinput"
)

func intPtr(v int) *int { return &v }

func trimRecord(msg conversation.ModelMessage, mutate func(*historyfrag.HistoryRecord)) historyfrag.HistoryRecord {
	return historyRecord("trim-row", msg, mutate)
}

func TestTrimMessagesByTokens_DropsLeadingOrphanTool(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("1111"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent(strings.Repeat("large tool setup ", 80)),
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
			},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("2"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("done"),
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(60)
		}),
	}

	trimmed, retained, totalTokens := trimMessagesAndRecordsByTokens(nil, messages, 20)
	if totalTokens <= 20 {
		t.Fatalf("test setup did not exercise trimming: total tokens = %d", totalTokens)
	}
	if len(retained) >= len(messages) {
		t.Fatalf("expected history to be trimmed, retained=%#v", retained)
	}
	for _, msg := range trimmed {
		if msg.Role == "tool" {
			t.Fatalf("trimmed messages should not contain an orphan tool result: %#v", trimmed)
		}
	}
}

func TestTrimMessagesByTokens_KeepsToolWhenPaired(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
			},
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10)
		}),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("2"),
		}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 100)
	if len(trimmed) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(trimmed))
	}
	if trimmed[0].Role != "assistant" || trimmed[1].Role != "tool" {
		t.Fatalf("unexpected role order: %q -> %q", trimmed[0].Role, trimmed[1].Role)
	}
}

func TestTrimMessagesByTokens_KeepsToolPairWhenBudgetDropsNewerOverflow(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
				{
					ID:   "call-2",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":2}`,
					},
				},
			},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("2"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-2",
			Content:    conversation.NewTextContent("4"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("newer overflow ", 80)),
		}, nil),
	}

	trimmed, retained, _ := trimMessagesAndRecordsByTokens(nil, messages, 5)
	if len(retained) != 3 {
		t.Fatalf("expected paired assistant/tool retained, got %#v", retained)
	}
	if retained[0].ModelMessage.Role != "assistant" || retained[1].ModelMessage.Role != "tool" || retained[2].ModelMessage.Role != "tool" {
		t.Fatalf("unexpected retained roles: %#v", retained)
	}
	if len(trimmed) != 4 || trimmed[1].Role != "assistant" || trimmed[2].Role != "tool" || trimmed[3].Role != "tool" {
		t.Fatalf("unexpected rendered roles: %#v", trimmed)
	}
}

func TestDropOrphanToolRecordsMatchesToolCallID(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-keep",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name: "lookup",
					},
				},
			},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-drop",
			Content:    conversation.NewTextContent("stale orphan result"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-keep",
			Content:    conversation.NewTextContent("matching result"),
		}, nil),
	}

	got := dropOrphanToolRecords(records)

	if len(got) != 2 {
		t.Fatalf("records = %d, want assistant and matching tool only: %#v", len(got), got)
	}
	if got[1].ModelMessage.ToolCallID != "call-keep" {
		t.Fatalf("kept tool call id = %q, want matching call", got[1].ModelMessage.ToolCallID)
	}
}

func TestDropOrphanToolRecordsMatchesNonAdjacentToolCallID(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name: "lookup",
				},
			}},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{{
				ID:   "call-2",
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name: "lookup",
				},
			}},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("result 1"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-2",
			Content:    conversation.NewTextContent("result 2"),
		}, nil),
	}

	got := dropOrphanToolRecords(records)

	if len(got) != 4 {
		t.Fatalf("records = %d, want both assistant/tool pairs retained by id: %#v", len(got), got)
	}
	if got[2].ModelMessage.ToolCallID != "call-1" || got[3].ModelMessage.ToolCallID != "call-2" {
		t.Fatalf("non-adjacent tool results were not matched by id: %#v", got)
	}
}

func TestTrimMessagesByTokensForceKeepsMarkedToolResultPair(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("old filler ", 100)),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name: "ask_user",
					},
				},
			},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent(strings.Repeat("latest user answer ", 80)),
		}, func(record *historyfrag.HistoryRecord) {
			record.Budget = contextfrag.BudgetPolicy{Overflow: contextfrag.OverflowKeep}
		}),
	}

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 1)
	joined := trimMessageTexts(messages)

	if !strings.Contains(joined, "latest user answer") {
		t.Fatalf("force-kept continuation result was trimmed: %#v", messages)
	}
	if len(retained) != 2 || retained[0].ModelMessage.Role != "assistant" || retained[1].ModelMessage.Role != "tool" {
		t.Fatalf("force-kept tool result should retain its assistant/tool pair: %#v", retained)
	}
}

func TestTrimMessagesByTokensForceKeepsNonAdjacentToolResultPair(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("old filler ", 100)),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name: "ask_user",
				},
			}},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{{
				ID:   "call-2",
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name: "lookup",
				},
			}},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("latest user answer"),
		}, func(record *historyfrag.HistoryRecord) {
			record.Budget = contextfrag.BudgetPolicy{Overflow: contextfrag.OverflowKeep}
		}),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-2",
			Content:    conversation.NewTextContent(strings.Repeat("lookup result ", 80)),
		}, nil),
	}

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 1)
	joined := trimMessageTexts(messages)

	if !strings.Contains(joined, "latest user answer") {
		t.Fatalf("force-kept non-adjacent continuation result was trimmed: %#v", messages)
	}
	if len(retained) != 2 || retained[0].ModelMessage.Role != "assistant" || retained[1].ModelMessage.ToolCallID != "call-1" {
		t.Fatalf("force-kept non-adjacent tool result should retain its owning assistant pair: %#v", retained)
	}
}

func TestTrimMessagesByTokensIgnoresDuplicateToolResultWhenKeepingPair(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name: "lookup",
					},
				},
			},
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("ok"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent(strings.Repeat("duplicate stale result ", 80)),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("newer overflow ", 80)),
		}, nil),
	}

	_, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 5)
	if len(retained) != 2 {
		t.Fatalf("expected assistant plus first matching tool only, got %#v", retained)
	}
	if retained[0].ModelMessage.Role != "assistant" || retained[1].ModelMessage.ToolCallID != "call-1" {
		t.Fatalf("unexpected retained pair: %#v", retained)
	}
	if strings.Contains(retained[1].ModelMessage.TextContent(), "duplicate stale") {
		t.Fatalf("duplicate tool result was retained: %#v", retained)
	}
}

func TestForceKeepToolResultForBudgetMarksLatestMatchingResult(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("old result"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-2",
			Content:    conversation.NewTextContent("other result"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("latest result"),
		}, nil),
	}

	got := forceKeepToolResultForBudget(records, "call-1")
	if got[0].Budget.Overflow == contextfrag.OverflowKeep {
		t.Fatal("old matching result should not be marked")
	}
	if got[1].Budget.Overflow == contextfrag.OverflowKeep {
		t.Fatal("non-matching result should not be marked")
	}
	if got[2].Budget.Overflow != contextfrag.OverflowKeep {
		t.Fatalf("latest matching result budget = %#v, want overflow keep", got[2].Budget)
	}
}

func TestEstimateMessageTokensRoundsUpShortNonEmptyMessages(t *testing.T) {
	t.Parallel()

	if got := estimateMessageTokens(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hi")}); got != 1 {
		t.Fatalf("short non-empty text estimated tokens = %d, want 1", got)
	}
	if got := estimateMessageTokens(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}); got != 2 {
		t.Fatalf("text length above divisor estimated tokens = %d, want rounded-up 2", got)
	}
}

func TestContextSourceTokenBudgetReservesPromptMaterial(t *testing.T) {
	t.Parallel()

	budget := contextSourceTokenBudget(1000, contextSourceReserve{
		System: strings.Repeat("system prompt ", 40),
		Messages: []conversation.ModelMessage{{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("memory context ", 40)),
		}},
		Query:            strings.Repeat("current query ", 20),
		InlineImageCount: 1,
	})

	if budget <= 0 || budget >= 1000 {
		t.Fatalf("source budget = %d, want positive budget below model window", budget)
	}
	if got := contextSourceTokenBudget(0, contextSourceReserve{Query: "ignored"}); got != 0 {
		t.Fatalf("zero context window source budget = %d, want unlimited sentinel 0", got)
	}
}

func TestContextSourceTokenBudgetDrivesHistoryTrim(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("old filler ", 80)),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("fresh reply"),
		}, nil),
	}
	sourceBudget := contextSourceTokenBudget(460, contextSourceReserve{
		Messages: []conversation.ModelMessage{{
			Role:    "user",
			Content: conversation.NewTextContent(strings.Repeat("memory reserve ", 30)),
		}},
		Query: strings.Repeat("query reserve ", 20),
	})

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, sourceBudget)
	joined := trimMessageTexts(messages)

	if sourceBudget >= totalHistoryTokens(records) {
		t.Fatalf("test setup did not reduce source budget enough: budget=%d history=%d", sourceBudget, totalHistoryTokens(records))
	}
	if len(retained) >= len(records) || strings.Contains(joined, "old filler") {
		t.Fatalf("source-budget trim should drop old filler before final prompt append: budget=%d messages=%#v", sourceBudget, messages)
	}
	if !strings.Contains(joined, "fresh reply") {
		t.Fatalf("source-budget trim should keep fresh reply: %#v", messages)
	}
}

func TestTrimMessagesByTokens_NoUsageWithinBudgetKeepsAll(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("hi")}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 10)
	if len(trimmed) != 2 {
		t.Fatalf("messages without usage but within budget should all be kept, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_ZeroMeansNoLimit(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10000)
		}),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("world")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10000)
		}),
	}

	// maxTokens = 0 means "no limit configured", should keep all messages.
	trimmed, _ := trimMessagesByTokens(nil, messages, 0)
	if len(trimmed) != 2 {
		t.Fatalf("maxTokens=0 should keep all messages, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_SmallBudgetTrims(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old message")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(100)
		}),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old reply")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(200)
		}),
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("new message")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(50)
		}),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("new reply")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(60)
		}),
	}

	// Budget of 1: should trim aggressively, NOT return all messages.
	trimmed, _ := trimMessagesByTokens(nil, messages, 1)
	if len(trimmed) >= len(messages) {
		t.Fatalf("maxTokens=1 should trim history, but got %d messages (same as input)", len(trimmed))
	}
}

func TestTrimMessagesByTokens_EstimatesFallback(t *testing.T) {
	t.Parallel()

	// Long user message without usage data — should be estimated.
	longText := make([]byte, 400)
	for i := range longText {
		longText[i] = 'x'
	}
	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(string(longText))}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("ok")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10)
		}),
	}

	// Budget of 50: user message is ~100 estimated tokens (400/4), should be trimmed.
	trimmed, _ := trimMessagesByTokens(nil, messages, 50)
	// When trimming occurs, a system truncation notice is prepended.
	// So we expect: 1 system notice + 1 assistant message (kept) = 2 total.
	// The key check is that the long user message was removed.
	if len(trimmed) != 2 || trimmed[0].Role != "system" || trimmed[1].Role != "assistant" {
		t.Fatalf("expected [system notice, assistant message], got %d messages: %+v", len(trimmed), trimmed)
	}
}

func TestTrimMessagesByTokensReturnsRawEstimateAfterBudgetTrim(t *testing.T) {
	t.Parallel()

	longText := strings.Repeat("raw history pressure ", 80)
	records := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(longText)}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("fresh reply")}, nil),
	}

	messages, retained, tokens := trimMessagesAndRecordsByTokens(nil, records, 20)
	retainedTokens := totalModelMessageTokens(messages)
	rawTokens := totalHistoryTokens(records)

	if len(retained) >= len(records) {
		t.Fatalf("test setup did not trim history: retained=%#v", retained)
	}
	if tokens != rawTokens {
		t.Fatalf("returned token estimate = %d, want raw history pressure %d; retained prompt estimate=%d", tokens, rawTokens, retainedTokens)
	}
	if retainedTokens >= rawTokens {
		t.Fatalf("test setup did not create retained/raw token gap: retained=%d raw=%d", retainedTokens, rawTokens)
	}
}

func TestTrimMessagesByTokens_PreservesCompactionSummaryWhenMiddleHistoryExceedsBudget(t *testing.T) {
	t.Parallel()

	summary := historyfrag.SummaryRecord(
		"compact-prior",
		"critical compact summary",
		[]contextfrag.ContextRef{{Namespace: "bot_history_message", ID: "old-covered", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable}},
		contextfrag.Scope{},
	)
	records := []historyfrag.HistoryRecord{
		summary,
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(strings.Repeat("x", 400))}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("fresh reply")}, nil),
	}

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 20)
	joined := trimMessageTexts(messages)

	if !strings.Contains(joined, "critical compact summary") {
		t.Fatalf("compaction summary should be preserved by budget-aware trim: %#v", messages)
	}
	if strings.Contains(joined, strings.Repeat("x", 80)) {
		t.Fatalf("middle low-priority history should be dropped before summary: %#v", messages)
	}
	if !strings.Contains(joined, "fresh reply") {
		t.Fatalf("fresh reply should be preserved: %#v", messages)
	}
	frags := historyContextFragsForMessages(messages, retained)
	if len(frags) != 1 || frags[0].Coverage == nil || frags[0].Coverage.CoveredRefs[0].ID != "old-covered" {
		t.Fatalf("retained summary coverage mismatch: %#v", frags)
	}
}

func TestTrimMessagesByTokens_ForceKeepsOversizedCompactionSummary(t *testing.T) {
	t.Parallel()

	summary := historyfrag.SummaryRecord(
		"compact-large",
		strings.Repeat("critical compact summary ", 40),
		[]contextfrag.ContextRef{{Namespace: "bot_history_message", ID: "old-covered", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable}},
		contextfrag.Scope{},
	)
	records := []historyfrag.HistoryRecord{
		summary,
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("fresh reply")}, nil),
	}

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 1)
	joined := trimMessageTexts(messages)

	if !strings.Contains(joined, "critical compact summary") {
		t.Fatalf("oversized compaction summary should be force-kept: %#v", messages)
	}
	frags := historyContextFragsForMessages(messages, retained)
	if len(frags) != 1 || frags[0].Coverage == nil || frags[0].Coverage.CoveredRefs[0].ID != "old-covered" {
		t.Fatalf("retained oversized summary coverage mismatch: %#v", frags)
	}
}

func TestTrimMessagesByTokens_DropsOverflowDropRecordBeforeImportantHistory(t *testing.T) {
	t.Parallel()

	important := trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("important short context")}, nil)
	oversized := trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent(strings.Repeat("drop-me ", 80))}, func(record *historyfrag.HistoryRecord) {
		record.Budget = contextfrag.BudgetPolicy{MaxTokens: 5, Overflow: contextfrag.OverflowDrop}
	})
	fresh := trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("fresh reply")}, nil)
	records := []historyfrag.HistoryRecord{important, oversized, fresh}

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 20)
	joined := trimMessageTexts(messages)

	if strings.Contains(joined, "drop-me") {
		t.Fatalf("overflow=drop record should be dropped before lower-cost history: %#v", messages)
	}
	if !strings.Contains(joined, "important short context") || !strings.Contains(joined, "fresh reply") {
		t.Fatalf("budget-aware trim should keep important old context and fresh reply: %#v", messages)
	}
	if len(retained) != 2 || retained[0].ModelMessage.TextContent() != "important short context" || retained[1].ModelMessage.TextContent() != "fresh reply" {
		t.Fatalf("retained records mismatch: %#v", retained)
	}
}

func TestTrimMessagesByTokens_DropsOverflowDropRecordEvenWhenGlobalBudgetFits(t *testing.T) {
	t.Parallel()

	important := trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("important short context")}, nil)
	oversized := trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent(strings.Repeat("drop-me ", 80))}, func(record *historyfrag.HistoryRecord) {
		record.Budget = contextfrag.BudgetPolicy{MaxTokens: 5, Overflow: contextfrag.OverflowDrop}
	})
	fresh := trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("fresh reply")}, nil)
	records := []historyfrag.HistoryRecord{important, oversized, fresh}

	messages, retained, tokens := trimMessagesAndRecordsByTokens(nil, records, 1000)
	joined := trimMessageTexts(messages)

	if strings.Contains(joined, "drop-me") {
		t.Fatalf("overflow=drop record should be dropped even when global budget fits: %#v", messages)
	}
	if len(retained) != 2 || retained[0].ModelMessage.TextContent() != "important short context" || retained[1].ModelMessage.TextContent() != "fresh reply" {
		t.Fatalf("retained records mismatch: %#v", retained)
	}
	if want := totalHistoryTokens(records); tokens != want {
		t.Fatalf("returned token estimate = %d, want raw history pressure %d", tokens, want)
	}
}

func trimMessageTexts(messages []conversation.ModelMessage) string {
	texts := make([]string, 0, len(messages))
	for _, msg := range messages {
		texts = append(texts, msg.TextContent())
	}
	return strings.Join(texts, "\n")
}

func TestStripToolMessages_RemovesAssistantToolCallContentParts(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{
		{"type": "reasoning", "text": "thinking"},
		{"type": "tool-call", "toolName": "read", "toolCallId": "call-1", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	filtered := stripToolMessages([]conversation.ModelMessage{
		{
			Role:    "assistant",
			Content: content,
		},
		{
			Role:    "assistant",
			Content: conversation.NewTextContent("保留这条消息"),
		},
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 message after filtering, got %d", len(filtered))
	}
	if filtered[0].TextContent() != "保留这条消息" {
		t.Fatalf("unexpected remaining message: %+v", filtered[0])
	}
}

func TestStripToolMessages_PreservesAskUserInteraction(t *testing.T) {
	t.Parallel()

	callContent, err := json.Marshal([]map[string]any{
		{"type": "text", "text": "请回答这一题："},
		{
			"type":       "tool-call",
			"toolName":   userinput.ToolNameAskUser,
			"toolCallId": "ask-1",
			"input": map[string]any{
				"questions": []any{
					map[string]any{
						"text": "选哪一个？",
						"kind": "single_select",
						"options": []any{
							map[string]any{"label": "A"},
							map[string]any{"label": "B"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal call content: %v", err)
	}
	resultContent, err := json.Marshal([]map[string]any{
		{
			"type":       "tool-result",
			"toolName":   userinput.ToolNameAskUser,
			"toolCallId": "ask-1",
			"result": map[string]any{
				"status": "submitted",
				"answers": []any{
					map[string]any{
						"question": "选哪一个？",
						"selected": []any{map[string]any{"label": "B"}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	readContent, err := json.Marshal([]map[string]any{
		{"type": "tool-call", "toolName": "read", "toolCallId": "read-1", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal read content: %v", err)
	}

	filtered := stripToolMessages([]conversation.ModelMessage{
		{Role: "assistant", Content: callContent},
		{Role: "tool", Content: resultContent},
		{Role: "assistant", Content: readContent},
		{Role: "tool", Content: conversation.NewTextContent("large output")},
	})

	if len(filtered) != 2 {
		t.Fatalf("expected ask_user call and result to remain, got %d messages: %+v", len(filtered), filtered)
	}
	if filtered[0].Role != "assistant" || filtered[1].Role != "tool" {
		t.Fatalf("unexpected roles after filtering: %+v", filtered)
	}

	var callParts []map[string]any
	if err := json.Unmarshal(filtered[0].Content, &callParts); err != nil {
		t.Fatalf("unmarshal preserved call content: %v", err)
	}
	if len(callParts) != 2 || callParts[1]["toolName"] != userinput.ToolNameAskUser {
		t.Fatalf("ask_user tool call was not preserved: %#v", callParts)
	}

	var resultParts []map[string]any
	if err := json.Unmarshal(filtered[1].Content, &resultParts); err != nil {
		t.Fatalf("unmarshal preserved result content: %v", err)
	}
	if len(resultParts) != 1 || resultParts[0]["toolName"] != userinput.ToolNameAskUser {
		t.Fatalf("ask_user tool result was not preserved: %#v", resultParts)
	}
}
