package conversation

import (
	"testing"

	"github.com/memohai/memoh/internal/agent/event"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestUIStreamEventAdapterPreservesRowIdentities(t *testing.T) {
	adapted := UIStreamEventFromAgentEvent(event.StreamEvent{
		Type:           event.ToolCallEnd,
		StableID:       "assistant-row",
		TurnID:         "turn-1",
		TurnPosition:   4,
		TurnMessageSeq: 2,
		RowIdentities: []event.RowIdentity{
			{StableID: "assistant-row", Role: "assistant", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 2},
			{StableID: "tool-row", Role: "tool", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 3},
		},
	})
	if len(adapted.RowIdentities) != 2 || adapted.RowIdentities[0].StableID != "assistant-row" || adapted.RowIdentities[1].StableID != "tool-row" {
		t.Fatalf("adapted row identities = %#v", adapted.RowIdentities)
	}
}

func TestUIMessageStreamConverterKeepsToolCallAndResultRows(t *testing.T) {
	converter := NewUIMessageStreamConverter()
	assistant := UIRowIdentity{StableID: "assistant-row", Role: "assistant", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 2}
	tool := UIRowIdentity{StableID: "tool-row", Role: "tool", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 3}
	start := converter.HandleEvent(UIMessageStreamEvent{
		Type: "tool_call_start", ToolName: "read", ToolCallID: "call-1",
		StableID: assistant.StableID, TurnID: assistant.TurnID, TurnPosition: assistant.TurnPosition, TurnMessageSeq: assistant.TurnMessageSeq,
		RowIdentities: []UIRowIdentity{assistant},
	})
	if len(start) != 1 || len(start[0].RowIdentities) != 1 {
		t.Fatalf("tool start = %#v", start)
	}
	end := converter.HandleEvent(UIMessageStreamEvent{
		Type: "tool_call_end", ToolName: "read", ToolCallID: "call-1", Output: map[string]any{"ok": true},
		StableID: assistant.StableID, TurnID: assistant.TurnID, TurnPosition: assistant.TurnPosition, TurnMessageSeq: assistant.TurnMessageSeq,
		RowIdentities: []UIRowIdentity{assistant, tool},
	})
	if len(end) != 1 || end[0].StableID != "assistant-row" || len(end[0].RowIdentities) != 2 {
		t.Fatalf("tool end = %#v", end)
	}
	if end[0].RowIdentities[0].StableID != "assistant-row" || end[0].RowIdentities[1].StableID != "tool-row" {
		t.Fatalf("tool ledger = %#v", end[0].RowIdentities)
	}
}

func TestConvertMessagesToUITurnsKeepsToolCallAndResultRows(t *testing.T) {
	messages := []messagepkg.Message{
		{
			ID: "assistant-row", Role: "assistant", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 2,
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "assistant",
				Content: mustUIRawJSON(t, []map[string]any{{
					"type": "tool-call", "toolCallId": "call-1", "toolName": "read", "input": map[string]any{"path": "README.md"},
				}}),
			}),
		},
		{
			ID: "tool-row", Role: "tool", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 3,
			Content: mustUIMessageJSON(t, ModelMessage{
				Role: "tool",
				Content: mustUIRawJSON(t, []map[string]any{{
					"type": "tool-result", "toolCallId": "call-1", "toolName": "read", "result": map[string]any{"ok": true},
				}}),
			}),
		},
	}
	turns := ConvertMessagesToUITurns(messages)
	if len(turns) != 1 || len(turns[0].Messages) != 1 {
		t.Fatalf("turns = %#v", turns)
	}
	block := turns[0].Messages[0]
	if block.Type != UIMessageTool || block.StableID != "assistant-row" || len(block.RowIdentities) != 2 {
		t.Fatalf("tool block = %#v", block)
	}
	if block.RowIdentities[0].StableID != "assistant-row" || block.RowIdentities[0].TurnMessageSeq != 2 || block.RowIdentities[1].StableID != "tool-row" || block.RowIdentities[1].TurnMessageSeq != 3 {
		t.Fatalf("tool block ledger = %#v", block.RowIdentities)
	}
}
