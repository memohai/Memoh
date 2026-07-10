package pipeline

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestRepairSDKToolClosuresDropsOrphanResults(t *testing.T) {
	t.Parallel()

	repaired := repairSDKToolClosures([]sdk.Message{
		sdk.UserMessage("hello"),
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "covered", ToolName: "exec", Result: "ok"}),
		sdk.AssistantMessage("done"),
	})
	if len(repaired) != 2 || repaired[0].Role != sdk.MessageRoleUser || repaired[1].Role != sdk.MessageRoleAssistant {
		t.Fatalf("orphan result survived: %#v", repaired)
	}
}

func TestRepairSDKToolClosuresSynthesizesBeforeFollowingUserMessage(t *testing.T) {
	t.Parallel()

	repaired := repairSDKToolClosures([]sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.TextPart{Text: "checking"},
				sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec", Input: map[string]any{}},
			},
		},
		sdk.UserMessage("follow-up"),
	})
	if len(repaired) != 3 || repaired[1].Role != sdk.MessageRoleTool || repaired[2].Role != sdk.MessageRoleUser {
		t.Fatalf("closure ordering = %#v, want assistant, tool, user", repaired)
	}
	result, ok := repaired[1].Content[0].(sdk.ToolResultPart)
	if !ok || result.ToolCallID != "call-1" || !result.IsError {
		t.Fatalf("synthetic result = %#v", repaired[1].Content)
	}
}

func TestRepairSDKToolClosuresKeepsIntactPair(t *testing.T) {
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
		t.Fatalf("intact closure changed: %#v", repaired)
	}
	for i := range messages {
		if repaired[i].Role != messages[i].Role {
			t.Fatalf("message %d role = %s, want %s", i, repaired[i].Role, messages[i].Role)
		}
	}
}
