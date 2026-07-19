package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

func TestStreamChatWSBindsACPDecisionProjectionToRunningTurn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		toolCallID string
		event      event.StreamEvent
	}{
		{
			name:       "user input",
			toolCallID: "ask-1",
			event: event.StreamEvent{
				Type:        event.UserInputRequest,
				ToolCallID:  "ask-1",
				ToolName:    userinput.ToolNameAskUser,
				UserInputID: "input-1",
				ShortID:     1,
				Status:      userinput.StatusPending,
				Input: map[string]any{
					"questions": []any{
						map[string]any{"id": "q1", "text": "Pick one", "type": "single_choice"},
					},
				},
			},
		},
		{
			name:       "approval",
			toolCallID: "write-1",
			event: event.StreamEvent{
				Type:       event.ToolApprovalRequest,
				ToolCallID: "write-1",
				ToolName:   "write",
				ApprovalID: "approval-1",
				ShortID:    2,
				Status:     toolapproval.StatusPending,
				Input:      map[string]any{"path": "/data/review.txt"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			messages := &acpTurnRecordingMessageService{recordingMessageService: &recordingMessageService{}}
			pool := &recordingACPPrompter{
				result: acpclient.PromptResult{
					Text:       "done",
					StopReason: "end_turn",
				},
				streamEvents: []event.StreamEvent{tt.event},
				onPrompt: func() {
					if len(messages.persisted) != 1 || messages.persisted[0].Role != "user" {
						t.Fatalf("persisted before inserting turn B = %#v, want running turn A user", messages.persisted)
					}
					persisted, err := messages.Persist(context.Background(), messagepkg.PersistInput{
						BotID:     "bot-1",
						SessionID: "session-1",
						Role:      "user",
						Content:   json.RawMessage(`{"role":"user","content":[{"type":"text","text":"turn B"}]}`),
					})
					if err != nil {
						t.Fatalf("persist turn B: %v", err)
					}
					if persisted.ID != "turn-b" {
						t.Fatalf("inserted turn ID = %q, want turn-b", persisted.ID)
					}
				},
			}
			resolver := &Resolver{
				messageService: messages,
				acpPool:        pool,
				botPermissions: allowWorkspaceExecFor("user-1"),
				sessionService: acpRuntimeSessionServiceForTest("user-1"),
				logger:         slog.New(slog.DiscardHandler),
			}
			resolver.userInput = &fakeUserInputService{}

			if err := resolver.StreamChatWS(
				context.Background(),
				conversation.ChatRequest{
					BotID:          "bot-1",
					SessionID:      "session-1",
					StreamID:       "stream-1",
					Query:          "turn A",
					CurrentChannel: "web",
				},
				make(chan WSStreamEvent, 8),
				make(chan struct{}),
			); err != nil {
				t.Fatalf("StreamChatWS() error = %v", err)
			}

			var projection *messagepkg.PersistInput
			for i := range messages.persisted {
				input := &messages.persisted[i]
				if input.Role != "assistant" {
					continue
				}
				calls := extractAssistantToolCallParts(persistedModelMessage(t, input.Content))
				if len(calls) == 1 && calls[0].ToolCallID == tt.toolCallID {
					projection = input
					break
				}
			}
			if projection == nil {
				t.Fatalf("persisted messages = %#v, want %s decision projection", messages.persisted, tt.toolCallID)
			}
			if got := projection.TurnRequestMessageID; got != "turn-a" {
				t.Fatalf("projection turn request message ID = %q, want running turn A ID turn-a", got)
			}
		})
	}
}

func TestStreamChatWSDoesNotStartACPWhenLeadingUserPersistenceFails(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("history unavailable")
	messages := &acpTurnRecordingMessageService{recordingMessageService: &recordingMessageService{persistErr: wantErr}}
	prompted := false
	resolver := &Resolver{
		messageService: messages,
		acpPool: &recordingACPPrompter{onPrompt: func() {
			prompted = true
		}},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.StreamChatWS(
		context.Background(),
		conversation.ChatRequest{
			BotID:          "bot-1",
			SessionID:      "session-1",
			Query:          "turn A",
			CurrentChannel: "web",
		},
		make(chan WSStreamEvent, 8),
		make(chan struct{}),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("StreamChatWS() error = %v, want %v", err, wantErr)
	}
	if prompted {
		t.Fatal("ACP prompt started without a durable request turn")
	}
}

func TestStreamChatWSDoesNotStartACPWithUnboundPersistedUser(t *testing.T) {
	t.Parallel()

	prompted := false
	resolver := &Resolver{
		messageService: &acpTurnRecordingMessageService{recordingMessageService: &recordingMessageService{}},
		acpPool: &recordingACPPrompter{onPrompt: func() {
			prompted = true
		}},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.StreamChatWS(
		context.Background(),
		conversation.ChatRequest{
			BotID:                "bot-1",
			SessionID:            "session-1",
			Query:                "turn A",
			CurrentChannel:       "web",
			UserMessagePersisted: true,
		},
		make(chan WSStreamEvent, 8),
		make(chan struct{}),
	)
	if err == nil {
		t.Fatal("StreamChatWS() error = nil, want missing durable request error")
	}
	if prompted {
		t.Fatal("ACP prompt started with an unbound persisted user")
	}
}

type acpTurnRecordingMessageService struct {
	*recordingMessageService
	userMessages int
}

func (s *acpTurnRecordingMessageService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	if s.persistErr != nil {
		return messagepkg.Message{}, s.persistErr
	}
	s.persisted = append(s.persisted, input)
	messageID := "assistant-message"
	if input.Role == "user" {
		s.userMessages++
		if s.userMessages == 1 {
			messageID = "turn-a"
		} else {
			messageID = "turn-b"
		}
	}
	return messagepkg.Message{
		ID:             messageID,
		SessionID:      input.SessionID,
		Role:           input.Role,
		Content:        input.Content,
		DisplayContent: input.DisplayText,
	}, nil
}
