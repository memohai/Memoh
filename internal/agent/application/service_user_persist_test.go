package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestPersistUserTurnSkillActivationWithoutPromptDoesNotStoreModelMarker(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Service{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	req := ChatRequest{
		BotID:           "bot-1",
		ThreadID:        "session-1",
		ModelQuery:      "The user activated the following skill for this turn without an additional prompt: alpha.",
		RawQuery:        "",
		UserMessageKind: UserMessageKindSkillActivation,
		SkillActivation: &SkillActivation{
			Skills: []SkillActivationSkill{{Name: "alpha", DisplayName: "Alpha", State: "effective"}},
		},
	}

	if _, err := resolver.persistUserTurn(context.Background(), req); err != nil {
		t.Fatalf("persistUserTurn() error = %v", err)
	}
	if len(messages.persisted) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messages.persisted))
	}
	got := persistedTextContent(t, messages.persisted[0].Content)
	if got != "" {
		t.Fatalf("persisted content = %q, want empty prompt only", got)
	}
	if messages.persisted[0].DisplayText != "" {
		t.Fatalf("display text = %q, want empty", messages.persisted[0].DisplayText)
	}
	if messages.persisted[0].Metadata["user_message_kind"] != UserMessageKindSkillActivation {
		t.Fatalf("metadata kind = %#v, want skill_activation", messages.persisted[0].Metadata["user_message_kind"])
	}
}

func TestPersistUserTurnSkillActivationWithPromptStoresPromptOnly(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Service{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	req := ChatRequest{
		BotID:           "bot-1",
		ThreadID:        "session-1",
		Query:           "Plan the widget implementation",
		RawQuery:        "Plan the widget implementation",
		UserVisibleText: "Plan the widget implementation",
		UserMessageKind: UserMessageKindSkillActivation,
		SkillActivation: &SkillActivation{
			Prompt: "Plan the widget implementation",
			Skills: []SkillActivationSkill{{Name: "alpha", DisplayName: "Alpha", State: "effective"}},
		},
	}

	if _, err := resolver.persistUserTurn(context.Background(), req); err != nil {
		t.Fatalf("persistUserTurn() error = %v", err)
	}
	if len(messages.persisted) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messages.persisted))
	}
	got := persistedTextContent(t, messages.persisted[0].Content)
	if got != "Plan the widget implementation" {
		t.Fatalf("persisted content = %q, want prompt only", got)
	}
	if messages.persisted[0].DisplayText != "Plan the widget implementation" {
		t.Fatalf("display text = %q, want prompt", messages.persisted[0].DisplayText)
	}
}

func persistedTextContent(t *testing.T, content json.RawMessage) string {
	t.Helper()
	var msg ModelMessage
	if err := json.Unmarshal(content, &msg); err != nil {
		t.Fatalf("unmarshal persisted content: %v", err)
	}
	return msg.TextContent()
}
