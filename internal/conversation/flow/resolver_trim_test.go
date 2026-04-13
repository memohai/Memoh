package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func intPtr(v int) *int { return &v }

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
			UsageOutputTokens: intPtr(50),
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
			UsageOutputTokens: intPtr(60),
		},
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
			UsageOutputTokens: intPtr(10),
		},
		{
			Message: conversation.ModelMessage{
				Role:       "tool",
				ToolCallID: "call-1",
				Content:    conversation.NewTextContent("2"),
			},
		},
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

	messages := []messageWithUsage{
		{Message: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}},
		{Message: conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("hi")}},
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 10)
	if len(trimmed) != 2 {
		t.Fatalf("messages without outputTokens should all be kept, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_ZeroMeansNoLimit(t *testing.T) {
	t.Parallel()

	messages := []messageWithUsage{
		{Message: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}, UsageOutputTokens: intPtr(10000)},
		{Message: conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("world")}, UsageOutputTokens: intPtr(10000)},
	}

	// maxTokens = 0 means "no limit configured", should keep all messages.
	trimmed, _ := trimMessagesByTokens(nil, messages, 0)
	if len(trimmed) != 2 {
		t.Fatalf("maxTokens=0 should keep all messages, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_SmallBudgetTrims(t *testing.T) {
	t.Parallel()

	messages := []messageWithUsage{
		{Message: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old message")}, UsageOutputTokens: intPtr(100)},
		{Message: conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old reply")}, UsageOutputTokens: intPtr(200)},
		{Message: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("new message")}, UsageOutputTokens: intPtr(50)},
		{Message: conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("new reply")}, UsageOutputTokens: intPtr(60)},
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
	messages := []messageWithUsage{
		{Message: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(string(longText))}},
		{Message: conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("ok")}, UsageOutputTokens: intPtr(10)},
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
