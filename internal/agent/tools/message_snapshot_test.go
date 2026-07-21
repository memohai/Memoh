package tools

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestMessageSnapshotIsImmutableAndProviderNeutral(t *testing.T) {
	original := []sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.TextPart{
				Text:             "answer",
				CacheControl:     &sdk.CacheControl{Type: "ephemeral"},
				ProviderMetadata: map[string]any{"signature": "provider-only"},
			},
			sdk.ToolCallPart{
				ToolCallID:       "call-1",
				ToolName:         "read",
				Input:            map[string]any{"path": "a.txt"},
				ProviderMetadata: map[string]any{"opaque": "value"},
			},
		},
	}}
	snapshot := NewMessageSnapshot(original)
	original[0].Content[0] = sdk.TextPart{Text: "mutated"}

	messages, err := snapshot.Messages()
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	text := messages[0].Content[0].(sdk.TextPart)
	if text.Text != "answer" || text.CacheControl != nil || len(text.ProviderMetadata) != 0 {
		t.Fatalf("unexpected provider-neutral text part: %+v", text)
	}
	call := messages[0].Content[1].(sdk.ToolCallPart)
	if call.ToolCallID != "call-1" || len(call.ProviderMetadata) != 0 {
		t.Fatalf("unexpected provider-neutral tool call: %+v", call)
	}
}
