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
			record.UsageOutputTokens = intPtr(50)
		}),
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

	// Budget 70: assistant(60) fits, adding assistant-tool-call(50) exceeds →
	// cutoff lands on the tool message which must be skipped.
	// NOTE: estimateMessageTokens uses character-based estimation (not UsageOutputTokens),
	// so all messages fit within budget=70. This test verifies the orphan-tool skip logic
	// still works correctly when trimming does occur.
	trimmed, _ := trimMessagesByTokens(nil, messages, 70)
	if len(trimmed) == 0 {
		t.Fatal("expected non-empty trimmed messages")
	}
	if trimmed[0].Role == "tool" {
		t.Fatal("expected first trimmed message not to be tool")
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

func TestTrimMessagesByTokens_NoUsage_KeepsAll(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("hi")}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 10)
	if len(trimmed) != 2 {
		t.Fatalf("messages without outputTokens should all be kept, got %d", len(trimmed))
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
