package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/userinput"
)

type claimProjectionMessageService struct {
	*recordingMessageService
	directPersistCalls int
}

func (s *claimProjectionMessageService) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.directPersistCalls++
	return s.recordingMessageService.Persist(ctx, input)
}

func TestStoreMessagesClaimFencesSingleResponse(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	req := claimBackedRequest()

	persisted, err := resolver.storeMessages(context.Background(), req, []conversation.ModelMessage{{
		Role:    "assistant",
		Content: conversation.NewTextContent("done"),
	}}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}
	if len(persisted) != 1 || len(messages.roundOptions) != 1 {
		t.Fatalf("persisted/round calls = %d/%d, want 1/1", len(persisted), len(messages.roundOptions))
	}
	claims := messages.roundOptions[0].DeliveryClaims
	if len(claims) != 1 || claims[0].EventID != req.EventID || claims[0].ClaimToken != req.EventDeliveryClaim.ClaimToken {
		t.Fatalf("delivery claims = %#v", claims)
	}
	if got := messages.persisted[0].TurnRequestMessageID; got != req.PersistedUserMessageID {
		t.Fatalf("response request id = %q, want %q", got, req.PersistedUserMessageID)
	}
}

func TestStoreMessagesRejectsMismatchedDeliveryClaimEvents(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		request  conversation.ChatRequest
		messages []conversation.ModelMessage
	}{
		{
			name: "request",
			request: func() conversation.ChatRequest {
				req := claimBackedRequest()
				req.EventID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
				return req
			}(),
			messages: []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("done")}},
		},
		{
			name: "injected message",
			request: conversation.ChatRequest{
				BotID:     "bot-1",
				SessionID: "session-1",
			},
			messages: []conversation.ModelMessage{
				{
					Role:    "user",
					Content: conversation.NewTextContent("injected"),
					Injected: &conversation.InjectMessage{
						Text: "injected",
						Source: conversation.InjectedMessageSource{
							EventID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
							DeliveryClaim: &conversation.DeliveryClaim{
								EventID:    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
								ClaimToken: "cccccccc-cccc-cccc-cccc-cccccccccccc",
							},
						},
					},
				},
				{Role: "assistant", Content: conversation.NewTextContent("done")},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := &recordingMessageService{}
			resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
			_, err := resolver.storeMessages(context.Background(), tc.request, tc.messages, "", storeRoundOptions{})
			if err == nil || !strings.Contains(err.Error(), "does not match its event") {
				t.Fatalf("storeMessages() error = %v, want event mismatch", err)
			}
			if len(messages.persisted) != 0 || len(messages.roundOptions) != 0 {
				t.Fatalf("persisted/round calls = %d/%d, want 0/0", len(messages.persisted), len(messages.roundOptions))
			}
		})
	}
}

func TestACPDecisionProjectionUsesClaimFencedPersistence(t *testing.T) {
	t.Parallel()

	messages := &claimProjectionMessageService{recordingMessageService: &recordingMessageService{}}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	req := claimBackedRequest()
	projected := resolver.persistACPDecisionProjection(context.Background(), req, event.StreamEvent{
		Type:        event.UserInputRequest,
		ToolCallID:  "ask-1",
		ToolName:    userinput.ToolNameAskUser,
		UserInputID: "input-1",
		Status:      userinput.StatusPending,
		Input: map[string]any{
			"questions": []any{map[string]any{"id": "q1", "text": "Pick one", "type": "single_choice"}},
		},
	})
	if !projected {
		t.Fatal("persistACPDecisionProjection() = false")
	}
	if messages.directPersistCalls != 0 || len(messages.roundOptions) != 1 {
		t.Fatalf("direct/round persistence calls = %d/%d, want 0/1", messages.directPersistCalls, len(messages.roundOptions))
	}
	claims := messages.roundOptions[0].DeliveryClaims
	if len(claims) != 1 || claims[0].EventID != req.EventID || claims[0].ClaimToken != req.EventDeliveryClaim.ClaimToken {
		t.Fatalf("projection delivery claims = %#v", claims)
	}
	if len(messages.persisted) != 1 || messages.persisted[0].Role != "assistant" ||
		messages.persisted[0].TurnRequestMessageID != req.PersistedUserMessageID {
		t.Fatalf("persisted projection = %#v", messages.persisted)
	}
}

func claimBackedRequest() conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:                  "bot-1",
		SessionID:              "session-1",
		EventID:                "11111111-1111-1111-1111-111111111111",
		UserMessagePersisted:   true,
		PersistedUserMessageID: "22222222-2222-2222-2222-222222222222",
		EventDeliveryClaim: &conversation.DeliveryClaim{
			EventID:    "11111111-1111-1111-1111-111111111111",
			ClaimToken: "33333333-3333-3333-3333-333333333333",
		},
	}
}
