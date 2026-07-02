package main

import (
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestAssistantContentWithToolCallsConvertsToUIToolBlock(t *testing.T) {
	content := assistantContentWithToolCalls("bench assistant", []pendingToolCallSeed{{
		id:    "tool-call-1",
		name:  "write",
		input: jsonRaw(`{"path":"/tmp/bench.txt"}`),
	}})
	turns := conversation.ConvertMessagesToUITurns([]messagepkg.Message{{
		ID:        "assistant-1",
		Role:      "assistant",
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}})
	if len(turns) != 1 {
		t.Fatalf("turns = %d", len(turns))
	}
	if len(turns[0].Messages) != 2 {
		t.Fatalf("messages = %#v", turns[0].Messages)
	}
	tool := turns[0].Messages[1]
	if tool.Type != conversation.UIMessageTool || tool.ToolCallID != "tool-call-1" || tool.Name != "write" {
		t.Fatalf("tool block = %#v", tool)
	}
}
