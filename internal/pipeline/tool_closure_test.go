package pipeline

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestRepairSDKToolClosuresDropsOrphanResults(t *testing.T) {
	t.Parallel()

	messages := []sdk.Message{
		sdk.UserMessage("hello"),
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-covered", ToolName: "exec", Result: "ok"}),
		sdk.AssistantMessage("done"),
	}

	repaired := repairSDKToolClosures(messages)

	if len(repaired) != 2 {
		t.Fatalf("messages = %d, want orphan tool result dropped: %#v", len(repaired), repaired)
	}
	for _, msg := range repaired {
		if msg.Role == sdk.MessageRoleTool {
			t.Fatalf("orphan tool result survived: %#v", msg)
		}
	}
}

func TestRepairSDKToolClosuresSynthesizesMissingResults(t *testing.T) {
	t.Parallel()

	messages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.TextPart{Text: "let me check"},
				sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec", Input: map[string]any{}},
			},
		},
		sdk.UserMessage("follow-up"),
	}

	repaired := repairSDKToolClosures(messages)

	if len(repaired) != 3 {
		t.Fatalf("messages = %d, want synthetic result appended: %#v", len(repaired), repaired)
	}
	last := repaired[2]
	if last.Role != sdk.MessageRoleTool {
		t.Fatalf("expected trailing synthetic tool result, got %#v", last)
	}
	result, ok := last.Content[0].(sdk.ToolResultPart)
	if !ok || result.ToolCallID != "call-1" || !result.IsError {
		t.Fatalf("synthetic result mismatch: %#v", last.Content[0])
	}
}

func TestRepairSDKToolClosuresKeepsIntactClosures(t *testing.T) {
	t.Parallel()

	messages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec", Input: map[string]any{}},
			},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "exec", Result: "ok"}),
		sdk.AssistantMessage("done"),
	}

	repaired := repairSDKToolClosures(messages)

	if len(repaired) != len(messages) {
		t.Fatalf("intact closure must pass through unchanged, got %#v", repaired)
	}
	for i, msg := range repaired {
		if msg.Role != messages[i].Role {
			t.Fatalf("message %d role changed: %#v", i, msg)
		}
	}
}
