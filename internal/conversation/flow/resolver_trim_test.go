package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestTrimMessagesByTokens_DropsLeadingOrphanTool(t *testing.T) {
	t.Parallel()

	messages := []messageWithUsage{
		{
			Message: conversation.ModelMessage{
				Role:    "user",
				Content: conversation.NewTextContent("1111"),
			},
		},
		{
			Message: conversation.ModelMessage{
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
			},
		},
		{
			Message: conversation.ModelMessage{
				Role:       "tool",
				ToolCallID: "call-1",
				Content:    conversation.NewTextContent("2"),
			},
		},
		{
			Message: conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("done"),
			},
		},
	}

	trimmed := trimMessagesByTokens(messages, 2)
	if len(trimmed) == 0 {
		t.Fatal("expected non-empty trimmed messages")
	}
	if trimmed[0].Role == "tool" {
		t.Fatal("expected first trimmed message not to be tool")
	}
}

func TestTrimMessagesByTokens_KeepsToolWhenPaired(t *testing.T) {
	t.Parallel()

	messages := []messageWithUsage{
		{
			Message: conversation.ModelMessage{
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
			},
		},
		{
			Message: conversation.ModelMessage{
				Role:       "tool",
				ToolCallID: "call-1",
				Content:    conversation.NewTextContent("2"),
			},
		},
	}

	trimmed := trimMessagesByTokens(messages, 100)
	if len(trimmed) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(trimmed))
	}
	if trimmed[0].Role != "assistant" || trimmed[1].Role != "tool" {
		t.Fatalf("unexpected role order: %q -> %q", trimmed[0].Role, trimmed[1].Role)
	}
}

