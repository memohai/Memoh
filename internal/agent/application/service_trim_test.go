package application

import (
	"encoding/json"
	"testing"

	historyfrag "github.com/memohai/memoh/internal/agent/context/history"
	userinput "github.com/memohai/memoh/internal/agent/decision/input"
)

func intPtr(v int) *int { return &v }

func trimRecord(msg ModelMessage, mutate func(*historyfrag.HistoryRecord)) historyfrag.HistoryRecord {
	return historyRecord("trim-row", msg, mutate)
}

func TestTrimMessagesByTokens_DropsLeadingOrphanTool(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(ModelMessage{
			Role:    "user",
			Content: newTextContent("1111"),
		}, nil),
		trimRecord(ModelMessage{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
			},
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(50)
		}),
		trimRecord(ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    newTextContent("2222"),
		}, nil),
		trimRecord(ModelMessage{
			Role:    "assistant",
			Content: newTextContent("done"),
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(60)
		}),
	}

	// Budget 2: newest assistant and tool result fit, adding the older assistant
	// tool call exceeds the budget. The cutoff initially lands on the tool result,
	// which must be skipped to avoid an orphan tool message.
	trimmed, _ := trimMessagesByTokens(nil, messages, 2)
	if len(trimmed) != 2 {
		t.Fatalf("expected truncation notice and latest assistant, got %d messages: %+v", len(trimmed), trimmed)
	}
	if trimmed[0].Role != "system" || trimmed[1].Role != "assistant" {
		t.Fatalf("expected [system, assistant], got %+v", trimmed)
	}
	for _, msg := range trimmed {
		if msg.Role == "tool" {
			t.Fatalf("expected orphan tool to be skipped, got %+v", trimmed)
		}
	}
}

func TestTrimMessagesByTokens_KeepsToolWhenPaired(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(ModelMessage{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
			},
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10)
		}),
		trimRecord(ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    newTextContent("2"),
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

func TestTrimMessagesByTokens_NoUsage_KeepsAll(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(ModelMessage{Role: "user", Content: newTextContent("hello")}, nil),
		trimRecord(ModelMessage{Role: "assistant", Content: newTextContent("hi")}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 10)
	if len(trimmed) != 2 {
		t.Fatalf("messages without outputTokens should all be kept, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_ZeroMeansNoLimit(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(ModelMessage{Role: "user", Content: newTextContent("hello")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10000)
		}),
		trimRecord(ModelMessage{Role: "assistant", Content: newTextContent("world")}, func(record *historyfrag.HistoryRecord) {
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
		trimRecord(ModelMessage{Role: "user", Content: newTextContent("old message")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(100)
		}),
		trimRecord(ModelMessage{Role: "assistant", Content: newTextContent("old reply")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(200)
		}),
		trimRecord(ModelMessage{Role: "user", Content: newTextContent("new message")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(50)
		}),
		trimRecord(ModelMessage{Role: "assistant", Content: newTextContent("new reply")}, func(record *historyfrag.HistoryRecord) {
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
		trimRecord(ModelMessage{Role: "user", Content: newTextContent(string(longText))}, nil),
		trimRecord(ModelMessage{Role: "assistant", Content: newTextContent("ok")}, func(record *historyfrag.HistoryRecord) {
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

func TestTrimMessagesByTokens_PreservesRequiredMessage(t *testing.T) {
	t.Parallel()

	longText := make([]byte, 400)
	for i := range longText {
		longText[i] = 'x'
	}
	messages := []historyfrag.HistoryRecord{
		trimRecord(ModelMessage{
			Role:    "user",
			Content: newTextContent("retry this exact prompt"),
		}, func(record *historyfrag.HistoryRecord) {
			record.Required = true
		}),
		trimRecord(ModelMessage{
			Role:    "assistant",
			Content: newTextContent(string(longText)),
		}, nil),
		trimRecord(ModelMessage{
			Role:    "assistant",
			Content: newTextContent("recent reply"),
		}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 5)
	if len(trimmed) < 2 {
		t.Fatalf("expected system notice and required prompt, got %d", len(trimmed))
	}
	if trimmed[0].Role != "system" {
		t.Fatalf("first message role = %q, want system", trimmed[0].Role)
	}
	if trimmed[1].Role != "user" || trimmed[1].TextContent() != "retry this exact prompt" {
		t.Fatalf("required message was not preserved in order: %+v", trimmed)
	}
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

	filtered := stripToolMessages([]ModelMessage{
		{
			Role:    "assistant",
			Content: content,
		},
		{
			Role:    "assistant",
			Content: newTextContent("保留这条消息"),
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

	filtered := stripToolMessages([]ModelMessage{
		{Role: "assistant", Content: callContent},
		{Role: "tool", Content: resultContent},
		{Role: "assistant", Content: readContent},
		{Role: "tool", Content: newTextContent("large output")},
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
