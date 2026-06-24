package flow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type requiredHistoryMessageService struct {
	recordingMessageService
	visible []messagepkg.Message
	byID    map[string]messagepkg.Message
}

func (s *requiredHistoryMessageService) ListVisibleFromBySession(context.Context, string, string) ([]messagepkg.Message, error) {
	return append([]messagepkg.Message(nil), s.visible...), nil
}

func (s *requiredHistoryMessageService) GetByIDBySession(_ context.Context, _ string, messageID string) (messagepkg.Message, error) {
	msg, ok := s.byID[messageID]
	if !ok {
		return messagepkg.Message{}, errors.New("message not found")
	}
	return msg, nil
}

func TestEnsureRequiredHistoryMessageMergesVisibleWindowInOrder(t *testing.T) {
	t.Parallel()

	requiredUser := persistedHistoryMessage(t, "user-1", "user", "retry me")
	toolCallAssistant := persistedHistoryMessage(t, "assistant-tool-1", "assistant", "calling tool")
	toolResult := persistedHistoryMessage(t, "tool-1", "tool", "tool result")
	finalAssistant := persistedHistoryMessage(t, "assistant-final-1", "assistant", "old answer")
	service := &requiredHistoryMessageService{
		visible: []messagepkg.Message{requiredUser, toolCallAssistant, toolResult, finalAssistant},
		byID:    map[string]messagepkg.Message{"user-1": requiredUser},
	}
	resolver := &Resolver{messageService: service}
	existing := []historyfrag.HistoryRecord{
		historyRecord("prior", conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("prior context"),
		}, nil),
		historyRecord("assistant-tool-1", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("calling tool")}, nil),
		historyRecord("tool-1", conversation.ModelMessage{Role: "tool", Content: conversation.NewTextContent("tool result")}, nil),
	}

	got, err := resolver.ensureRequiredHistoryMessage(context.Background(), existing, conversation.ChatRequest{
		ChatID:                       "bot-1",
		SessionID:                    "session-1",
		RequiredHistoryMessageID:     "user-1",
		HistoryCutoffBeforeMessageID: "assistant-final-1",
	})
	if err != nil {
		t.Fatalf("ensureRequiredHistoryMessage() error = %v", err)
	}
	if gotIDs := historyRecordIDs(got); !equalStrings(gotIDs, []string{"prior", "user-1", "assistant-tool-1", "tool-1"}) {
		t.Fatalf("message ids = %v, want prior + ordered required window", gotIDs)
	}
	if !got[1].Required {
		t.Fatalf("required message was not marked")
	}
}

func persistedHistoryMessage(t *testing.T, id string, role string, text string) messagepkg.Message {
	t.Helper()
	content, err := json.Marshal(conversation.ModelMessage{
		Role:    role,
		Content: conversation.NewTextContent(text),
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return messagepkg.Message{
		ID:      id,
		Role:    role,
		Content: content,
	}
}

func historyRecordIDs(messages []historyfrag.HistoryRecord) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, msg.DBMessageID)
	}
	return out
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
