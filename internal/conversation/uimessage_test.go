package conversation

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestConvertMessagesToUITurnsStripsUserXMLEnvelopeFallback(t *testing.T) {
	now := time.Now().UTC()
	turns := ConvertMessagesToUITurns([]messagepkg.Message{{
		ID:        "user-1",
		BotID:     "bot-1",
		SessionID: "session-1",
		Role:      "user",
		Content: mustUIMessageJSON(t, ModelMessage{
			Role: "user",
			Content: mustUIRawJSON(t, `<message id="msg-image-only" sender="Test User (@test_user)" t="2026-05-08T19:08:58Z" channel="telegram" conversation="Test Group" type="group" target="test-group">
<attachment path="/data/media/test/test-image.webp"/>

</message>`),
		}),
		Assets: []messagepkg.MessageAsset{{
			ContentHash: "test-image-hash",
			Mime:        "image/webp",
			StorageKey:  "media/test/test-image.webp",
			Name:        "image.webp",
		}},
		CreatedAt: now,
	}})

	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Text != "" {
		t.Fatalf("expected XML envelope to be stripped, got %q", turns[0].Text)
	}
	if len(turns[0].Attachments) != 1 || turns[0].Attachments[0].Type != "image" {
		t.Fatalf("expected image attachment to remain, got %#v", turns[0].Attachments)
	}
}

func TestConvertMessagesToUITurnsIncludesReplyAndForwardMetadata(t *testing.T) {
	now := time.Now().UTC()
	turns := ConvertMessagesToUITurns([]messagepkg.Message{{
		ID:                     "user-1",
		BotID:                  "bot-1",
		SessionID:              "session-1",
		Role:                   "user",
		ExternalMessageID:      "external-user-1",
		SourceReplyToMessageID: "reply-1",
		Content: mustUIMessageJSON(t, ModelMessage{
			Role:    "user",
			Content: mustUIRawJSON(t, "hello"),
		}),
		Metadata: map[string]any{
			"reply": map[string]any{
				"message_id": "reply-1",
				"sender":     "Original Sender",
				"preview":    "quoted text",
				"attachments": []map[string]any{{
					"type":         "image",
					"content_hash": "image-hash",
					"mime":         "image/png",
					"name":         "quoted.png",
					"metadata":     map[string]any{"bot_id": "bot-1", "storage_key": "im/image-hash.png"},
				}},
			},
			"forward": map[string]any{
				"message_id":           "forward-1",
				"from_conversation_id": "source-conversation",
				"sender":               "Source Channel",
				"date":                 float64(1710000000),
			},
		},
		CreatedAt: now,
	}})

	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].ExternalMessageID != "external-user-1" {
		t.Fatalf("unexpected external message id: %q", turns[0].ExternalMessageID)
	}
	if turns[0].Reply == nil || turns[0].Reply.MessageID != "reply-1" || turns[0].Reply.Preview != "quoted text" {
		t.Fatalf("unexpected reply metadata: %#v", turns[0].Reply)
	}
	if len(turns[0].Reply.Attachments) != 1 || turns[0].Reply.Attachments[0].ContentHash != "image-hash" {
		t.Fatalf("unexpected reply attachments: %#v", turns[0].Reply.Attachments)
	}
	if turns[0].Forward == nil || turns[0].Forward.MessageID != "forward-1" || turns[0].Forward.Sender != "Source Channel" {
		t.Fatalf("unexpected forward metadata: %#v", turns[0].Forward)
	}
}

func TestConvertMessagesToUITurnsTruncatesReplyPreview(t *testing.T) {
	now := time.Now().UTC()
	longPreview := strings.Repeat("预览", 80)
	turns := ConvertMessagesToUITurns([]messagepkg.Message{{
		ID:        "user-1",
		BotID:     "bot-1",
		SessionID: "session-1",
		Role:      "user",
		Content: mustUIMessageJSON(t, ModelMessage{
			Role:    "user",
			Content: mustUIRawJSON(t, "hello"),
		}),
		Metadata: map[string]any{
			"reply": map[string]any{
				"message_id": "reply-1",
				"preview":    longPreview,
			},
		},
		CreatedAt: now,
	}})
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Reply == nil {
		t.Fatal("expected reply metadata")
	}
	if got := len([]rune(turns[0].Reply.Preview)); got > uiReplyPreviewMaxRunes {
		t.Fatalf("reply preview too long: %d", got)
	}
	if !strings.HasSuffix(turns[0].Reply.Preview, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", turns[0].Reply.Preview)
	}
}

func TestConvertMessagesToUITurnsKeepsForwardOnlyUserMessage(t *testing.T) {
	now := time.Now().UTC()
	turns := ConvertMessagesToUITurns([]messagepkg.Message{{
		ID:        "user-1",
		BotID:     "bot-1",
		Role:      "user",
		Content:   json.RawMessage(`{"role":"user","content":[{"type":"text","text":""}]}`),
		Metadata:  map[string]any{"forward": map[string]any{"message_id": "forward-1", "sender": "Source"}},
		CreatedAt: now,
	}})
	if len(turns) != 1 {
		t.Fatalf("expected one turn, got %d", len(turns))
	}
	if turns[0].Forward == nil || turns[0].Forward.MessageID != "forward-1" {
		t.Fatalf("unexpected forward metadata: %#v", turns[0].Forward)
	}
}

func TestConvertMessagesToUITurnsConvertsBackgroundNotification(t *testing.T) {
	baseTime := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	messages := []messagepkg.Message{
		{
			ID:        "assistant-1",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "assistant",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "assistant",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "tool-call", "toolCallId": "call-1", "toolName": "exec", "input": map[string]any{"command": "npm test"}},
				}),
			}),
			CreatedAt: baseTime,
		},
		{
			ID:        "tool-1",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "tool",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "tool",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "tool-result", "toolCallId": "call-1", "toolName": "exec", "result": map[string]any{"structuredContent": map[string]any{"status": "background_started", "task_id": "bg_1", "output_file": "/tmp/memoh-bg/bg_1.log"}}},
				}),
			}),
			CreatedAt: baseTime.Add(time.Second),
		},
		{
			ID:             "notification-1",
			BotID:          "bot-1",
			SessionID:      "session-1",
			Role:           "user",
			DisplayContent: "A background task completed:\n<task-notification>\n  <task-id>bg_1</task-id>\n  <status>completed</status>\n  <command>npm test</command>\n  <exit-code>0</exit-code>\n  <duration>1.5s</duration>\n  <output-file>/tmp/memoh-bg/bg_1.log</output-file>\n  <output-tail>\nok\n  </output-tail>\n</task-notification>",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role:    "user",
				Content: mustUIRawJSON(t, "A background task completed"),
			}),
			CreatedAt: baseTime.Add(2 * time.Second),
		},
	}

	turns := ConvertMessagesToUITurns(messages)
	if len(turns) != 2 {
		t.Fatalf("expected assistant turn plus system background turn, got %d", len(turns))
	}

	assistantTurn := turns[0]
	if assistantTurn.Role != "assistant" || len(assistantTurn.Messages) != 1 {
		t.Fatalf("unexpected assistant turn: %#v", assistantTurn)
	}
	tool := assistantTurn.Messages[0]
	if tool.Background == nil || tool.Background.TaskID != "bg_1" {
		t.Fatalf("expected tool background task metadata, got %#v", tool)
	}
	if tool.Running == nil || *tool.Running {
		t.Fatalf("expected completed background task to close exec block: %#v", tool)
	}
	if tool.Background.Status != "completed" || tool.Background.OutputTail != "ok" {
		t.Fatalf("unexpected merged background task: %#v", tool.Background)
	}

	systemTurn := turns[1]
	if systemTurn.Role != "system" || systemTurn.Kind != "background_task" || systemTurn.BackgroundTask == nil {
		t.Fatalf("expected system background task turn, got %#v", systemTurn)
	}
	if systemTurn.BackgroundTask.TaskID != "bg_1" || systemTurn.BackgroundTask.Status != "completed" {
		t.Fatalf("unexpected system background payload: %#v", systemTurn.BackgroundTask)
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

func TestUIMessageStreamConverterMergesRepeatedToolCallStart(t *testing.T) {
	t.Parallel()

	converter := NewUIMessageStreamConverter()

	start := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_start",
		ToolName:   "write",
		ToolCallID: "call-1",
	})
	if len(start) != 1 || start[0].Type != UIMessageTool {
		t.Fatalf("unexpected initial tool placeholder: %#v", start)
	}
	if start[0].Input != nil {
		t.Fatalf("expected initial tool placeholder to have nil input, got %#v", start[0].Input)
	}

	fullInput := map[string]any{"path": "/tmp/long.txt"}
	update := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_start",
		ToolName:   "write",
		ToolCallID: "call-1",
		Input:      fullInput,
	})
	if len(update) != 1 {
		t.Fatalf("expected one updated tool snapshot, got %#v", update)
	}
	if update[0].ID != start[0].ID {
		t.Fatalf("expected repeated tool start to reuse message id, got start=%d update=%d", start[0].ID, update[0].ID)
	}
	if !reflect.DeepEqual(update[0].Input, fullInput) {
		t.Fatalf("expected repeated tool start to backfill input, got %#v", update[0].Input)
	}
	if update[0].Running == nil || !*update[0].Running {
		t.Fatalf("expected merged tool message to stay running, got %#v", update[0])
	}

	end := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_end",
		ToolName:   "write",
		ToolCallID: "call-1",
		Output:     map[string]any{"ok": true},
	})
	if len(end) != 1 || end[0].ID != start[0].ID {
		t.Fatalf("expected tool end to reuse merged message id, got %#v", end)
	}
	if !reflect.DeepEqual(end[0].Input, fullInput) {
		t.Fatalf("expected tool end to preserve merged input, got %#v", end[0].Input)
	}
	if end[0].Running == nil || *end[0].Running {
		t.Fatalf("expected tool end to mark message complete, got %#v", end[0])
	}
}

func TestUIMessageStreamConverterKeepsParallelSameNameToolCallsSeparate(t *testing.T) {
	t.Parallel()

	converter := NewUIMessageStreamConverter()

	startA := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_start",
		ToolName:   "search",
		ToolCallID: "call-A",
		Input:      map[string]any{"query": "A"},
	})
	startB := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_start",
		ToolName:   "search",
		ToolCallID: "call-B",
		Input:      map[string]any{"query": "B"},
	})
	startC := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_start",
		ToolName:   "search",
		ToolCallID: "call-C",
		Input:      map[string]any{"query": "C"},
	})

	if len(startA) != 1 || len(startB) != 1 || len(startC) != 1 {
		t.Fatalf("expected one snapshot per start, got A=%#v B=%#v C=%#v", startA, startB, startC)
	}
	if startA[0].ID == startB[0].ID || startB[0].ID == startC[0].ID || startA[0].ID == startC[0].ID {
		t.Fatalf("expected each parallel tool call to receive a distinct id, got A=%d B=%d C=%d",
			startA[0].ID, startB[0].ID, startC[0].ID)
	}
	if startA[0].ToolCallID != "call-A" || startB[0].ToolCallID != "call-B" || startC[0].ToolCallID != "call-C" {
		t.Fatalf("expected each snapshot to keep its own tool_call_id, got A=%q B=%q C=%q",
			startA[0].ToolCallID, startB[0].ToolCallID, startC[0].ToolCallID)
	}

	endA := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_end",
		ToolName:   "search",
		ToolCallID: "call-A",
		Output:     map[string]any{"hits": "A"},
	})
	endB := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_end",
		ToolName:   "search",
		ToolCallID: "call-B",
		Output:     map[string]any{"hits": "B"},
	})
	endC := converter.HandleEvent(UIMessageStreamEvent{
		Type:       "tool_call_end",
		ToolName:   "search",
		ToolCallID: "call-C",
		Output:     map[string]any{"hits": "C"},
	})

	if endA[0].ID != startA[0].ID || endB[0].ID != startB[0].ID || endC[0].ID != startC[0].ID {
		t.Fatalf("expected tool_call_end to reuse the matching start id, got endA=%d endB=%d endC=%d (startA=%d startB=%d startC=%d)",
			endA[0].ID, endB[0].ID, endC[0].ID, startA[0].ID, startB[0].ID, startC[0].ID)
	}
	if !reflect.DeepEqual(endA[0].Input, map[string]any{"query": "A"}) ||
		!reflect.DeepEqual(endB[0].Input, map[string]any{"query": "B"}) ||
		!reflect.DeepEqual(endC[0].Input, map[string]any{"query": "C"}) {
		t.Fatalf("expected each tool_call_end to preserve its own input, got A=%#v B=%#v C=%#v",
			endA[0].Input, endB[0].Input, endC[0].Input)
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

func TestConvertRawModelMessagesToUIAssistantMessagesKeepsBackgroundExecRunning(t *testing.T) {
	raw := mustUIRawJSON(t, []ModelMessage{
		{
			Role: "assistant",
			Content: mustUIRawJSON(t, []map[string]any{
				{"type": "tool-call", "toolCallId": "call-1", "toolName": "exec", "input": map[string]any{"command": "npm test"}},
			}),
		},
		{
			Role: "tool",
			Content: mustUIRawJSON(t, []map[string]any{
				{"type": "tool-result", "toolCallId": "call-1", "toolName": "exec", "result": map[string]any{"structuredContent": map[string]any{"status": "background_started", "task_id": "bg_1", "output_file": "/tmp/memoh-bg/bg_1.log"}}},
			}),
		},
	})

	messages := ConvertRawModelMessagesToUIAssistantMessages(raw)
	if len(messages) != 1 {
		t.Fatalf("expected one exec tool message, got %d", len(messages))
	}
	if messages[0].Running == nil || !*messages[0].Running {
		t.Fatalf("expected background exec to remain running: %#v", messages[0])
	}
	if messages[0].Background == nil || messages[0].Background.TaskID != "bg_1" {
		t.Fatalf("expected background metadata on exec block: %#v", messages[0])
	}
}

func TestApplyBackgroundTaskSnapshotsClosesPersistedStartedExec(t *testing.T) {
	baseTime := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	messages := []messagepkg.Message{
		{
			ID:        "assistant-1",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "assistant",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "assistant",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "tool-call", "toolCallId": "call-1", "toolName": "exec", "input": map[string]any{"command": "npm test"}},
				}),
			}),
			CreatedAt: baseTime,
		},
		{
			ID:        "tool-1",
			BotID:     "bot-1",
			SessionID: "session-1",
			Role:      "tool",
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "tool",
				Content: mustUIRawJSON(t, []map[string]any{
					{"type": "tool-result", "toolCallId": "call-1", "toolName": "exec", "result": map[string]any{"structuredContent": map[string]any{"status": "background_started", "task_id": "bg_1", "output_file": "/tmp/memoh-bg/bg_1.log"}}},
				}),
			}),
			CreatedAt: baseTime.Add(time.Second),
		},
	}

	turns := ConvertMessagesToUITurns(messages)
	if len(turns) != 1 || len(turns[0].Messages) != 1 {
		t.Fatalf("unexpected initial turns: %#v", turns)
	}
	tool := turns[0].Messages[0]
	if tool.Running == nil || !*tool.Running {
		t.Fatalf("expected persisted background_started tool to be running: %#v", tool)
	}

	ApplyBackgroundTaskSnapshots(turns, []UIBackgroundTask{{
		TaskID:     "bg_1",
		Status:     "completed",
		Command:    "npm test",
		OutputFile: "/tmp/memoh-bg/bg_1.log",
		Duration:   "2s",
		OutputTail: "ok\n",
	}})

	tool = turns[0].Messages[0]
	if tool.Running == nil || *tool.Running {
		t.Fatalf("expected snapshot to close exec tool: %#v", tool)
	}
	if tool.Background == nil || tool.Background.Status != "completed" || tool.Background.OutputTail != "ok\n" {
		t.Fatalf("unexpected snapshot merge: %#v", tool.Background)
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
