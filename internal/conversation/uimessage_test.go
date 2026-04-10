package conversation

import (
	"encoding/json"
	"testing"
	"time"

	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestConvertMessagesToUITurnsGroupsAssistantToolAndFiltersCurrentConversationDelivery(t *testing.T) {
	baseTime := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	messages := []messagepkg.Message{
		{
			ID:             "user-1",
			BotID:          "bot-1",
			SessionID:      "session-1",
			Role:           "user",
			DisplayContent: "hello",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role:    "user",
				Content: mustUIRawJSON(t, "hello"),
			}),
			CreatedAt: baseTime,
		},
		{
			ID:        "assistant-1",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "assistant",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "assistant",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "reasoning", "text": "thinking"},
					{"type": "tool-call", "toolCallId": "call-1", "toolName": "read", "input": map[string]any{"path": "/tmp/a.txt"}},
					{"type": "tool-call", "toolCallId": "call-2", "toolName": "send", "input": map[string]any{"message": "hi"}},
				}),
			}),
			Assets: []messagepkg.MessageAsset{{
				ContentHash: "hash-1",
				Mime:        "image/png",
				StorageKey:  "media/hash-1",
				Name:        "image.png",
			}},
			CreatedAt: baseTime.Add(1 * time.Minute),
		},
		{
			ID:        "tool-1",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "tool",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "tool",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "tool-result", "toolCallId": "call-1", "toolName": "read", "result": map[string]any{"structuredContent": map[string]any{"stdout": "hello"}}},
				}),
			}),
			CreatedAt: baseTime.Add(2 * time.Minute),
		},
		{
			ID:        "tool-2",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "tool",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "tool",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "tool-result", "toolCallId": "call-2", "toolName": "send", "result": map[string]any{"delivered": "current_conversation"}},
				}),
			}),
			CreatedAt: baseTime.Add(3 * time.Minute),
		},
		{
			ID:        "assistant-2",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "assistant",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role:    "assistant",
				Content: mustUIRawJSON(t, []map[string]any{{"type": "text", "text": "done"}}),
			}),
			CreatedAt: baseTime.Add(4 * time.Minute),
		},
	}

	turns := ConvertMessagesToUITurns(messages)
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}

	userTurn := turns[0]
	if userTurn.Role != "user" || userTurn.Text != "hello" {
		t.Fatalf("unexpected user turn: %#v", userTurn)
	}

	assistantTurn := turns[1]
	if assistantTurn.Role != "assistant" {
		t.Fatalf("expected assistant turn, got %#v", assistantTurn)
	}
	if len(assistantTurn.Messages) != 4 {
		t.Fatalf("expected 4 assistant messages, got %d", len(assistantTurn.Messages))
	}

	if assistantTurn.Messages[0].Type != UIMessageReasoning || assistantTurn.Messages[0].Content != "thinking" {
		t.Fatalf("unexpected reasoning block: %#v", assistantTurn.Messages[0])
	}
	if assistantTurn.Messages[1].Type != UIMessageTool || assistantTurn.Messages[1].Name != "read" {
		t.Fatalf("unexpected tool block: %#v", assistantTurn.Messages[1])
	}
	if assistantTurn.Messages[1].Running == nil || *assistantTurn.Messages[1].Running {
		t.Fatalf("expected tool block to be completed: %#v", assistantTurn.Messages[1])
	}
	if assistantTurn.Messages[2].Type != UIMessageAttachments || len(assistantTurn.Messages[2].Attachments) != 1 {
		t.Fatalf("unexpected attachment block: %#v", assistantTurn.Messages[2])
	}
	if assistantTurn.Messages[2].Attachments[0].Type != "image" || assistantTurn.Messages[2].Attachments[0].BotID != "bot-1" {
		t.Fatalf("unexpected attachment payload: %#v", assistantTurn.Messages[2].Attachments[0])
	}
	if assistantTurn.Messages[3].Type != UIMessageText || assistantTurn.Messages[3].Content != "done" {
		t.Fatalf("unexpected trailing text block: %#v", assistantTurn.Messages[3])
	}

	for _, block := range assistantTurn.Messages {
		if block.Type == UIMessageTool && block.Name == "send" {
			t.Fatalf("expected current conversation delivery tool to be filtered out")
		}
	}
}

func TestConvertMessagesToUITurnsStripsUserYAMLHeaderFallback(t *testing.T) {
	now := time.Now().UTC()
	turns := ConvertMessagesToUITurns([]messagepkg.Message{{
		ID:        "user-1",
		BotID:     "bot-1",
		SessionID: "session-1",
		Role:      "user",
		Content: mustUIMessageJSON(t, ModelMessage{
			Role:    "user",
			Content: mustUIRawJSON(t, "---\nmessage-id: 1\nchannel: telegram\n---\nhello"),
		}),
		CreatedAt: now,
	}})

	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Text != "hello" {
		t.Fatalf("expected YAML header to be stripped, got %q", turns[0].Text)
	}
}

func TestUIMessageStreamConverterAccumulatesToolProgress(t *testing.T) {
	converter := NewUIMessageStreamConverter()

	start := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_start",
		ToolName:   "exec",
		ToolCallID: "call-1",
		Input:      map[string]any{"command": "ls"},
	})
	if len(start) != 1 || start[0].Type != UIMessageTool || start[0].Name != "exec" {
		t.Fatalf("unexpected tool start event: %#v", start)
	}
	if start[0].Running == nil || !*start[0].Running {
		t.Fatalf("expected running tool start, got %#v", start[0])
	}

	progressOne := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_progress",
		ToolName:   "exec",
		ToolCallID: "call-1",
		Progress:   "line 1",
	})
	progressTwo := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_progress",
		ToolName:   "exec",
		ToolCallID: "call-1",
		Progress:   map[string]any{"line": 2},
	})
	if len(progressOne) != 1 || len(progressOne[0].Progress) != 1 {
		t.Fatalf("unexpected first progress snapshot: %#v", progressOne)
	}
	if len(progressTwo) != 1 || len(progressTwo[0].Progress) != 2 {
		t.Fatalf("unexpected second progress snapshot: %#v", progressTwo)
	}
	if progressTwo[0].ID != start[0].ID {
		t.Fatalf("expected progress snapshots to reuse tool message id")
	}

	end := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_end",
		ToolName:   "exec",
		ToolCallID: "call-1",
		Output:     map[string]any{"structuredContent": map[string]any{"stdout": "done"}},
	})
	if len(end) != 1 || end[0].Running == nil || *end[0].Running {
		t.Fatalf("expected completed tool snapshot, got %#v", end)
	}
	if end[0].ID != start[0].ID || len(end[0].Progress) != 2 {
		t.Fatalf("expected final snapshot to keep id and progress, got %#v", end[0])
	}
}

func TestUIMessageStreamConverterStartsNewTextBlockAfterTool(t *testing.T) {
	converter := NewUIMessageStreamConverter()

	first := converter.HandleEvent(UIMessageStreamEvent{Type: "text_delta", Delta: "hello"})
	converter.HandleEvent(UIMessageStreamEvent{Type: "tool_call_start", ToolName: "read", ToolCallID: "call-1"})
	converter.HandleEvent(UIMessageStreamEvent{Type: "tool_call_end", ToolName: "read", ToolCallID: "call-1"})
	second := converter.HandleEvent(UIMessageStreamEvent{Type: "text_delta", Delta: "world"})

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected text snapshots, got first=%#v second=%#v", first, second)
	}
	if first[0].ID == second[0].ID {
		t.Fatalf("expected new text block after tool call, got same id %d", first[0].ID)
	}
}

func TestConvertRawModelMessagesToUIAssistantMessagesBuildsTerminalSnapshots(t *testing.T) {
	raw := mustUIRawJSON(t, []ModelMessage{
		{
			Role: "assistant",
			Content: mustUIRawJSON(t, []map[string]any{
				{"type": "reasoning", "text": "thinking"},
				{"type": "tool-call", "toolCallId": "call-1", "toolName": "read", "input": map[string]any{"path": "/tmp/a.txt"}},
			}),
		},
		{
			Role: "tool",
			Content: mustUIRawJSON(t, []map[string]any{
				{"type": "tool-result", "toolCallId": "call-1", "toolName": "read", "result": map[string]any{"structuredContent": map[string]any{"stdout": "ok"}}},
			}),
		},
		{
			Role: "assistant",
			Content: mustUIRawJSON(t, []map[string]any{
				{"type": "text", "text": "final answer"},
			}),
		},
	})

	messages := ConvertRawModelMessagesToUIAssistantMessages(raw)
	if len(messages) != 3 {
		t.Fatalf("expected 3 ui messages, got %d", len(messages))
	}
	if messages[0].ID != 0 || messages[0].Type != UIMessageReasoning {
		t.Fatalf("unexpected first ui message: %#v", messages[0])
	}
	if messages[1].ID != 1 || messages[1].Type != UIMessageTool {
		t.Fatalf("unexpected second ui message: %#v", messages[1])
	}
	if messages[1].Running == nil || *messages[1].Running {
		t.Fatalf("expected terminal tool message to be completed: %#v", messages[1])
	}
	if messages[2].ID != 2 || messages[2].Type != UIMessageText || messages[2].Content != "final answer" {
		t.Fatalf("unexpected final ui message: %#v", messages[2])
	}
}

func mustUIRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	return data
}

func mustUIMessageJSON(t *testing.T, message ModelMessage) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return data
}
