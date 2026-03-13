package matrix

import "testing"

func TestIsMatrixBotMentionedByMentionsMetadata(t *testing.T) {
	content := map[string]any{
		"body": "hi bot",
		"m.mentions": map[string]any{
			"user_ids": []any{"@memoh:example.com"},
		},
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected mention metadata to be detected")
	}
}

func TestIsMatrixBotMentionedByFormattedBody(t *testing.T) {
	content := map[string]any{
		"body":           "hello Memoh",
		"formatted_body": `<a href="https://matrix.to/#/@memoh:example.com">Memoh</a> hello`,
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected formatted body mention to be detected")
	}
}

func TestIsMatrixBotMentionedByBodyFallback(t *testing.T) {
	content := map[string]any{
		"body": "@memoh:example.com ping",
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected body fallback mention to be detected")
	}
}
