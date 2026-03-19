package flow

import (
	"encoding/json"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestExtractAssistantOutputsSkipsAssistantToolCallMessages(t *testing.T) {
	outputs := ExtractAssistantOutputs([]conversation.ModelMessage{
		{
			Role:    "assistant",
			Content: conversation.NewTextContent("I will inspect the file first."),
			ToolCalls: []conversation.ToolCall{{
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name:      "read_file",
					Arguments: `{"path":"/tmp/a.txt"}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: conversation.NewTextContent("Done. Here is the final answer."),
		},
	})

	if len(outputs) != 1 {
		t.Fatalf("expected one assistant output, got %d", len(outputs))
	}
	if outputs[0].Content != "Done. Here is the final answer." {
		t.Fatalf("unexpected assistant output: %q", outputs[0].Content)
	}
}

func TestExtractAssistantOutputsExcludesReasoningParts(t *testing.T) {
	content, err := json.Marshal([]conversation.ContentPart{
		{Type: "reasoning", Text: "I should inspect the file first."},
		{Type: "text", Text: "Here is the file summary."},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	outputs := ExtractAssistantOutputs([]conversation.ModelMessage{{
		Role:    "assistant",
		Content: content,
	}})

	if len(outputs) != 1 {
		t.Fatalf("expected one assistant output, got %d", len(outputs))
	}
	if outputs[0].Content != "Here is the file summary." {
		t.Fatalf("unexpected visible assistant output: %q", outputs[0].Content)
	}
	if len(outputs[0].Parts) != 1 || outputs[0].Parts[0].Type != "text" {
		t.Fatalf("unexpected visible parts: %#v", outputs[0].Parts)
	}
}

func TestExtractAssistantOutputsSkipsReasoningOnlyStructuredMessage(t *testing.T) {
	content, err := json.Marshal([]map[string]any{
		{"type": "reasoning", "text": "I should inspect the file first."},
		{"type": "tool-call", "toolName": "read", "toolCallId": "call_1", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	outputs := ExtractAssistantOutputs([]conversation.ModelMessage{{
		Role:    "assistant",
		Content: content,
	}})

	if len(outputs) != 0 {
		t.Fatalf("expected no visible assistant outputs, got %#v", outputs)
	}
}

func TestExtractAssistantOutputsSkipsStructuredToolCallMessageWithVisibleText(t *testing.T) {
	content, err := json.Marshal([]map[string]any{
		{"type": "text", "text": "I will inspect the file first."},
		{"type": "tool-call", "toolName": "read", "toolCallId": "call_1", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	outputs := ExtractAssistantOutputs([]conversation.ModelMessage{{
		Role:    "assistant",
		Content: content,
	}})

	if len(outputs) != 0 {
		t.Fatalf("expected no visible assistant outputs, got %#v", outputs)
	}
}
