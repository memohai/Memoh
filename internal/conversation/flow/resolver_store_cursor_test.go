package flow

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestStoreRoundWithCursorUsesAtomicPersister(t *testing.T) {
	t.Parallel()

	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	committed, err := resolver.StoreRoundWithCursor(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		[]sdk.Message{sdk.AssistantMessage("reply")},
		"",
		pipelinepkg.DiscussCursorCommit{
			ScopeKey: "route:66666666-6666-6666-6666-666666666666",
			RouteID:  "66666666-6666-6666-6666-666666666666",
			Source:   "telegram",
			Position: pipelinepkg.DiscussCursorPosition{SourceCursor: 100, EventCursor: 123},
			DeliveryClaims: []pipelinepkg.DeliveryClaim{{
				EventID:    "77777777-7777-7777-7777-777777777777",
				ClaimToken: "88888888-8888-8888-8888-888888888888",
			}},
		},
	)
	if err != nil {
		t.Fatalf("StoreRoundWithCursor() error = %v", err)
	}
	if !committed {
		t.Fatal("StoreRoundWithCursor() committed = false, want true")
	}
	if messages.cursorCalls != 1 || len(messages.persisted) != 1 {
		t.Fatalf("atomic cursor calls/messages = %d/%d, want 1/1", messages.cursorCalls, len(messages.persisted))
	}
	if messages.cursor.ConsumedEventCursor != 123 || messages.cursor.ScopeKey != "route:66666666-6666-6666-6666-666666666666" {
		t.Fatalf("persisted cursor = %#v", messages.cursor)
	}
	if len(messages.cursor.DeliveryClaims) != 1 ||
		messages.cursor.DeliveryClaims[0].EventID != "77777777-7777-7777-7777-777777777777" ||
		messages.cursor.DeliveryClaims[0].ClaimToken != "88888888-8888-8888-8888-888888888888" {
		t.Fatalf("persisted delivery claims = %#v", messages.cursor.DeliveryClaims)
	}
}

func TestDiscussCursorUpdateFromRequestPreservesDeliveryClaims(t *testing.T) {
	t.Parallel()

	update := discussCursorUpdateFromRequest(conversation.ChatRequest{
		SessionID:                  "33333333-3333-3333-3333-333333333333",
		DiscussConsumedEventCursor: 123,
		DiscussDeliveryClaims: []conversation.DeliveryClaim{{
			EventID:    "77777777-7777-7777-7777-777777777777",
			ClaimToken: "88888888-8888-8888-8888-888888888888",
		}},
	})
	if update == nil || len(update.DeliveryClaims) != 1 ||
		update.DeliveryClaims[0].EventID != "77777777-7777-7777-7777-777777777777" ||
		update.DeliveryClaims[0].ClaimToken != "88888888-8888-8888-8888-888888888888" {
		t.Fatalf("cursor update = %#v, want delivery claim", update)
	}
}

func TestStoreRoundWithCursorKeepsReadMediaDecorationsAndToolClosures(t *testing.T) {
	t.Parallel()

	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	sdkMessages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "read-1",
				ToolName:   "read_media",
				Input:      map[string]any{"path": "first.png"},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "read-1", ToolName: "read_media", Result: "ok"}),
		sdk.UserMessage("", sdk.ImagePart{Image: "data:image/png;base64,Zmlyc3Q=", MediaType: "image/png"}),
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "read-2",
				ToolName:   "read_media",
				Input:      map[string]any{"path": "second.png"},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "read-2", ToolName: "read_media", Result: "ok"}),
		sdk.UserMessage("", sdk.ImagePart{Image: "data:image/png;base64,c2Vjb25k", MediaType: "image/png"}),
		sdk.AssistantMessage("done"),
	}

	committed, err := resolver.StoreRoundWithCursor(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		sdkMessages,
		"",
		pipelinepkg.DiscussCursorCommit{Position: pipelinepkg.DiscussCursorPosition{EventCursor: 123}},
	)
	if err != nil {
		t.Fatalf("StoreRoundWithCursor() error = %v", err)
	}
	if !committed || messages.cursorCalls != 1 {
		t.Fatalf("committed/cursor calls = %t/%d, want true/1", committed, messages.cursorCalls)
	}
	gotRoles := make([]string, 0, len(messages.cursorInputs))
	continuedUsers := 0
	for _, input := range messages.cursorInputs {
		gotRoles = append(gotRoles, input.Role)
		if input.Role == "user" && input.ContinueHistoryTurn {
			continuedUsers++
		}
	}
	if got := fmt.Sprint(gotRoles); got != "[assistant tool user assistant tool user assistant]" {
		t.Fatalf("persisted roles = %s, want complete mixed transcript", got)
	}
	if continuedUsers != 2 {
		t.Fatalf("continued internal user decorations = %d, want 2", continuedUsers)
	}
}

func TestStoreMessagesStartsNewTurnForInjectedUser(t *testing.T) {
	t.Parallel()

	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	_, err := resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID:                  storeRoundBotID,
		SessionID:              "33333333-3333-3333-3333-333333333333",
		UserMessagePersisted:   true,
		PersistedUserMessageID: "55555555-5555-5555-5555-555555555555",
	}, []conversation.ModelMessage{
		{Role: "assistant", Content: conversation.NewTextContent("before injection")},
		{
			Role:    "user",
			Content: conversation.NewTextContent("new human input"),
			Injected: &conversation.InjectMessage{
				Text: "new human input",
				Source: conversation.InjectedMessageSource{
					ExternalMessageID: "external-2",
					EventID:           "66666666-6666-6666-6666-666666666666",
				},
			},
		},
		{Role: "assistant", Content: conversation.NewTextContent("after injection")},
	}, "", storeRoundOptions{DiscussCursor: &messagepkg.DiscussCursorUpdate{
		SessionID:           "33333333-3333-3333-3333-333333333333",
		ConsumedEventCursor: 123,
	}})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}
	if len(messages.cursorInputs) != 3 {
		t.Fatalf("cursor inputs = %d, want 3", len(messages.cursorInputs))
	}
	if messages.cursorInputs[1].Role != "user" || messages.cursorInputs[1].ContinueHistoryTurn {
		t.Fatalf("injected user history flags = role:%q continue:%t, want user/false", messages.cursorInputs[1].Role, messages.cursorInputs[1].ContinueHistoryTurn)
	}
}

func TestStoreRoundWithCursorFailsWithoutAtomicPersister(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	_, err := resolver.StoreRoundWithCursor(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		[]sdk.Message{sdk.AssistantMessage("reply")},
		"",
		pipelinepkg.DiscussCursorCommit{Position: pipelinepkg.DiscussCursorPosition{EventCursor: 123}},
	)
	if err == nil {
		t.Fatal("StoreRoundWithCursor() error = nil without atomic persister")
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("fallback persisted messages = %d, want 0", len(messages.persisted))
	}
}

func TestStoreRoundWithCursorReportsNoDurableResponse(t *testing.T) {
	t.Parallel()

	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	committed, err := resolver.StoreRoundWithCursor(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		[]sdk.Message{sdk.AssistantMessage("")},
		"",
		pipelinepkg.DiscussCursorCommit{Position: pipelinepkg.DiscussCursorPosition{EventCursor: 123}},
	)
	if err != nil {
		t.Fatalf("StoreRoundWithCursor() error = %v", err)
	}
	if committed || messages.cursorCalls != 0 {
		t.Fatalf("empty response committed/calls = %t/%d, want false/0", committed, messages.cursorCalls)
	}
}

type cursorRecordingMessageService struct {
	*recordingMessageService
	cursorCalls  int
	cursor       messagepkg.DiscussCursorUpdate
	cursorInputs []messagepkg.PersistInput
}

func (s *cursorRecordingMessageService) PersistTurnResponseWithCursor(
	_ context.Context,
	inputs []messagepkg.PersistInput,
	cursor messagepkg.DiscussCursorUpdate,
) ([]messagepkg.Message, error) {
	s.cursorCalls++
	s.cursor = cursor
	s.cursorInputs = append([]messagepkg.PersistInput(nil), inputs...)
	s.persisted = append(s.persisted, inputs...)
	return recordedMessages(inputs), nil
}
