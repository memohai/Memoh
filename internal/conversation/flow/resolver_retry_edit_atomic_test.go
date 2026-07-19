package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/acpclient"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
)

type atomicReplacementMessageService struct {
	recordingMessageService
	replacementRequests  []messagepkg.ReplacementRoundRequest
	replacementErr       error
	replacementCommitted bool
}

type orderedTurnReplacer interface {
	ReplaceTurnOrdered(context.Context, string, string, string, string, []string, string) (messagepkg.HistoryTurn, error)
}

func (r *Resolver) replacePersistedTurn(
	ctx context.Context,
	req conversation.ChatRequest,
	oldTurnID string,
	requestMessageID string,
	reason string,
	persisted []messagepkg.Message,
) error {
	replacementID := firstMessageIDWithRole(persisted, "assistant")
	if replacementID == "" {
		return errors.New("replacement assistant message was not persisted")
	}
	if strings.TrimSpace(requestMessageID) == "" {
		requestMessageID = firstMessageIDWithRole(persisted, "user")
	}
	replacementMessageIDs, err := orderedReplacementMessageIDs(requestMessageID, replacementID, persisted)
	if err != nil {
		return err
	}
	forkAnchorUpdate := r.prepareForkAnchorUpdate(ctx, req.SessionID, req.HistoryCutoffBeforeMessageID)
	replacer, ok := r.messageService.(orderedTurnReplacer)
	if !ok {
		if len(replacementMessageIDs) != 2 || replacementMessageIDs[0] != requestMessageID || replacementMessageIDs[1] != replacementID {
			return errors.New("message service does not support ordered turn replacement")
		}
		if _, err := r.messageService.ReplaceTurn(ctx, req.SessionID, oldTurnID, requestMessageID, replacementID, reason); err != nil {
			return err
		}
	} else if _, err := replacer.ReplaceTurnOrdered(ctx, req.SessionID, oldTurnID, requestMessageID, replacementID, replacementMessageIDs, reason); err != nil {
		return err
	}
	r.applyForkAnchorUpdate(ctx, req.SessionID, forkAnchorUpdate)
	r.publishReplacementMessageCreated(req.BotID, persisted)
	return nil
}

func orderedReplacementMessageIDs(requestMessageID string, assistantMessageID string, persisted []messagepkg.Message) ([]string, error) {
	requestMessageID = strings.TrimSpace(requestMessageID)
	if requestMessageID == "" {
		return nil, errors.New("replacement request message was not persisted")
	}
	messageIDs := make([]string, 0, len(persisted)+1)
	seen := make(map[string]struct{}, len(persisted)+1)
	requestPresent := false
	for _, message := range persisted {
		messageID := strings.TrimSpace(message.ID)
		if messageID == "" {
			return nil, errors.New("replacement contains a message without an id")
		}
		if _, exists := seen[messageID]; exists {
			return nil, fmt.Errorf("replacement contains duplicate message id %q", messageID)
		}
		seen[messageID] = struct{}{}
		messageIDs = append(messageIDs, messageID)
		requestPresent = requestPresent || messageID == requestMessageID
	}
	if _, ok := seen[assistantMessageID]; !ok {
		return nil, errors.New("replacement assistant message is not in the persisted sequence")
	}
	if !requestPresent {
		messageIDs = append([]string{requestMessageID}, messageIDs...)
	}
	if messageIDs[0] != requestMessageID {
		return nil, errors.New("replacement request message must be first in the sequence")
	}
	return messageIDs, nil
}

func firstMessageIDWithRole(messages []messagepkg.Message, role string) string {
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), role) {
			return strings.TrimSpace(message.ID)
		}
	}
	return ""
}

func (s *atomicReplacementMessageService) PersistReplacementRound(_ context.Context, input messagepkg.ReplacementRoundRequest) ([]messagepkg.Message, error) {
	s.replacementRequests = append(s.replacementRequests, input)
	if s.replacementErr != nil {
		return nil, s.replacementErr
	}
	messages := make([]messagepkg.Message, 0, len(input.Messages))
	for i, messageInput := range input.Messages {
		messages = append(messages, messagepkg.Message{
			ID:             fmt.Sprintf("replacement-%d", i+1),
			BotID:          messageInput.BotID,
			SessionID:      messageInput.SessionID,
			Role:           messageInput.Role,
			Content:        messageInput.Content,
			DisplayContent: messageInput.DisplayText,
		})
	}
	s.replacementCommitted = true
	return messages, nil
}

func TestStoreRoundReplacementUsesAtomicPersisterBeforePublishing(t *testing.T) {
	t.Parallel()

	messages := &atomicReplacementMessageService{}
	events := &recordingEventPublisher{}
	resolver := &Resolver{
		messageService: messages,
		eventPublisher: events,
		logger:         slog.New(slog.DiscardHandler),
	}
	req := conversation.ChatRequest{
		BotID:                     "bot-1",
		SessionID:                 "session-1",
		Query:                     "hello",
		RawQuery:                  "hello",
		SessionType:               "chat",
		RuntimeType:               "model",
		ReusePersistedUserMessage: true,
		PersistedUserMessageID:    "request-existing",
		SkipHistoryTurn:           true,
		SkipMemoryExtraction:      true,
		TurnReplacement: &conversation.TurnReplacementSpec{
			OldTurnID:                "turn-old",
			ExistingRequestMessageID: "request-existing",
			Reason:                   "retry",
		},
	}

	persisted, err := resolver.storeRoundWithOptionsResult(context.Background(), req, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("replacement")},
	}, "model-1", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeRoundWithOptionsResult() error = %v", err)
	}
	if len(persisted) != 1 || len(messages.replacementRequests) != 1 {
		t.Fatalf("persisted/replacement requests = %d/%d, want 1/1", len(persisted), len(messages.replacementRequests))
	}
	if got := len(messages.persisted); got != 0 {
		t.Fatalf("ordinary persisted messages = %d, want 0", got)
	}
	if !messages.replacementCommitted || len(events.events) != 1 {
		t.Fatalf("replacement committed/events = %v/%d, want true/1", messages.replacementCommitted, len(events.events))
	}
	var payload messagepkg.Message
	if err := json.Unmarshal(events.events[0].Data, &payload); err != nil {
		t.Fatalf("unmarshal replacement event: %v", err)
	}
	if payload.ID != "replacement-1" {
		t.Fatalf("replacement event message id = %q, want replacement-1", payload.ID)
	}
}

func TestStoreRoundReplacementFailurePublishesNothing(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("replace failed")
	messages := &atomicReplacementMessageService{replacementErr: wantErr}
	events := &recordingEventPublisher{}
	resolver := &Resolver{
		messageService: messages,
		eventPublisher: events,
		logger:         slog.New(slog.DiscardHandler),
	}
	req := conversation.ChatRequest{
		BotID:                "bot-1",
		SessionID:            "session-1",
		Query:                "edited",
		RawQuery:             "edited",
		SessionType:          "chat",
		RuntimeType:          "model",
		SkipHistoryTurn:      true,
		SkipMemoryExtraction: true,
		TurnReplacement: &conversation.TurnReplacementSpec{
			OldTurnID: "turn-old",
			Reason:    "edit",
		},
	}

	_, err := resolver.storeRoundWithOptionsResult(context.Background(), req, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("edited")},
		{Role: "assistant", Content: conversation.NewTextContent("replacement")},
	}, "model-1", storeRoundOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("storeRoundWithOptionsResult() error = %v, want %v", err, wantErr)
	}
	if messages.replacementCommitted || len(events.events) != 0 || len(messages.persisted) != 0 {
		t.Fatalf("replacement committed/events/ordinary persists = %v/%d/%d, want false/0/0", messages.replacementCommitted, len(events.events), len(messages.persisted))
	}
}

func TestACPReplacementDefersAllPersistenceToAtomicTerminalRound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		existingRequestMessageID string
		wantLeadingPersisted     bool
		wantRoles                []string
	}{
		{
			name:                     "retry reuses request",
			existingRequestMessageID: "request-existing",
			wantLeadingPersisted:     true,
			wantRoles:                []string{"assistant"},
		},
		{
			name:      "edit defers new request",
			wantRoles: []string{"user", "assistant"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			messages := &atomicReplacementMessageService{}
			events := &recordingEventPublisher{}
			resolver := &Resolver{
				messageService: messages,
				eventPublisher: events,
				logger:         slog.New(slog.DiscardHandler),
			}
			req := conversation.ChatRequest{
				BotID:                     "bot-1",
				SessionID:                 "session-1",
				Query:                     "replacement query",
				RawQuery:                  "replacement query",
				SessionType:               "chat",
				RuntimeType:               "acp_agent",
				ReusePersistedUserMessage: tt.existingRequestMessageID != "",
				PersistedUserMessageID:    tt.existingRequestMessageID,
				SkipHistoryTurn:           true,
				SkipMemoryExtraction:      true,
				TurnReplacement: &conversation.TurnReplacementSpec{
					OldTurnID:                "turn-old",
					ExistingRequestMessageID: tt.existingRequestMessageID,
					Reason:                   "replace",
				},
			}

			prepared, err := resolver.persistACPLeadingUserMessage(context.Background(), req)
			if err != nil {
				t.Fatalf("persistACPLeadingUserMessage() error = %v", err)
			}
			if len(messages.persisted) != 0 {
				t.Fatalf("leading ordinary persists = %d, want 0", len(messages.persisted))
			}
			if prepared.UserMessagePersisted != tt.wantLeadingPersisted {
				t.Fatalf("UserMessagePersisted = %v, want %v", prepared.UserMessagePersisted, tt.wantLeadingPersisted)
			}
			if tt.existingRequestMessageID != "" && prepared.PersistedUserMessageID != tt.existingRequestMessageID {
				t.Fatalf("persisted request id = %q, want %q", prepared.PersistedUserMessageID, tt.existingRequestMessageID)
			}
			if projected := resolver.persistACPDecisionProjection(context.Background(), prepared, agentpkg.StreamEvent{
				Type:       agentpkg.EventUserInputRequest,
				ToolCallID: "input-1",
				Status:     "pending",
			}); projected {
				t.Fatal("replacement decision projection was persisted before terminal round")
			}
			if len(messages.persisted) != 0 {
				t.Fatalf("projection ordinary persists = %d, want 0", len(messages.persisted))
			}

			committed, err := resolver.persistACPRoundResult(context.Background(), prepared, "codex", "/workspace", acpclient.PromptResult{
				Output: []sdk.Message{sdk.AssistantMessage("replacement")},
			}, nil)
			if err != nil {
				t.Fatalf("persistACPRoundResult() error = %v", err)
			}
			if !committed || len(messages.replacementRequests) != 1 {
				t.Fatalf("committed/replacement requests = %v/%d, want true/1", committed, len(messages.replacementRequests))
			}
			gotRoles := make([]string, 0, len(messages.replacementRequests[0].Messages))
			for _, input := range messages.replacementRequests[0].Messages {
				gotRoles = append(gotRoles, input.Role)
			}
			if fmt.Sprint(gotRoles) != fmt.Sprint(tt.wantRoles) {
				t.Fatalf("atomic ACP roles = %v, want %v", gotRoles, tt.wantRoles)
			}
			if len(events.events) != 1 {
				t.Fatalf("replacement events = %d, want 1", len(events.events))
			}
		})
	}
}

func TestACPProjectionFailureKeepsLastDurableStatusCovered(t *testing.T) {
	t.Parallel()

	streamed := []agentpkg.StreamEvent{
		{Type: agentpkg.EventToolCallStart, ToolCallID: "write-1", ToolName: "write", Input: map[string]any{"path": "/workspace/review.txt"}},
		{Type: agentpkg.EventToolApprovalRequest, ToolCallID: "write-1", ToolName: "write", ApprovalID: "approval-1", Status: toolapproval.StatusPending},
		{Type: agentpkg.EventToolApprovalRequest, ToolCallID: "write-1", ToolName: "write", ApprovalID: "approval-1", Status: toolapproval.StatusApproved},
		{Type: agentpkg.EventToolCallEnd, ToolCallID: "write-1", ToolName: "write", Result: map[string]any{"ok": true}},
		{Type: agentpkg.EventTextDelta, Delta: "done"},
	}
	messages := &nthPersistFailureMessageService{failOnCall: 2, err: errors.New("persist approved projection")}
	resolver := &Resolver{
		messageService: messages,
		acpPool: &recordingACPPrompter{
			result:       withTranscriptOutput(acpclient.PromptResult{Events: streamed}),
			streamEvents: streamed,
		},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return session.Session{
					ID:    sessionID,
					BotID: "bot-1",
					Type:  session.TypeACPAgent,
					Metadata: map[string]any{
						"acp_agent_id":             "codex",
						"project_path":             "/data/app",
						"runtime_owner_account_id": "user-1",
					},
				}, nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	req := persistedDiscussACPRequest()
	req.StreamID = "stream-1"
	eventCh := make(chan WSStreamEvent, 16)
	if err := resolver.streamACPAgentWS(context.Background(), req, eventCh, nil); err != nil {
		t.Fatalf("streamACPAgentWS() error = %v", err)
	}
	if messages.persistCalls != 2 {
		t.Fatalf("projection persist calls = %d, want pending + failed approved", messages.persistCalls)
	}
	approvedCalls := 0
	for _, message := range messages.persisted[1:] {
		for _, call := range extractAssistantToolCallParts(persistedModelMessage(t, message.Content)) {
			if call.ToolCallID == "write-1" && toolCallMetadataStatus(call, "approval") == toolapproval.StatusApproved {
				approvedCalls++
			}
		}
	}
	if approvedCalls != 1 {
		t.Fatalf("terminal transcript approved calls = %d in %#v, want one recovery copy", approvedCalls, messages.persisted)
	}
	close(eventCh)
	approvedStreamed := false
	for raw := range eventCh {
		var streamed agentpkg.StreamEvent
		if err := json.Unmarshal(raw, &streamed); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		approvedStreamed = approvedStreamed ||
			(streamed.Type == agentpkg.EventToolApprovalRequest && streamed.Status == toolapproval.StatusApproved)
	}
	if !approvedStreamed {
		t.Fatal("failed terminal projection was not published for live UI closure")
	}
}

func TestACPReplacementDisablesDecisionCapability(t *testing.T) {
	t.Parallel()

	decision := agentpkg.StreamEvent{
		Type:       agentpkg.EventToolApprovalRequest,
		ToolCallID: "write-1",
		ToolName:   "write",
		Status:     toolapproval.StatusPending,
	}
	messages := &atomicReplacementMessageService{}
	pool := &recordingACPPrompter{
		result:       withTranscriptOutput(acpclient.PromptResult{Events: []agentpkg.StreamEvent{decision}}),
		streamEvents: []agentpkg.StreamEvent{decision},
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool:        pool,
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}
	resolver.userInput = &fakeUserInputService{}
	req := conversation.ChatRequest{
		BotID:                  "bot-1",
		SessionID:              "session-1",
		StreamID:               "stream-1",
		Query:                  "replace turn",
		UserMessagePersisted:   true,
		PersistedUserMessageID: "turn-a",
		TurnReplacement: &conversation.TurnReplacementSpec{
			OldTurnID:                "turn-old",
			ExistingRequestMessageID: "turn-a",
			Reason:                   "replace",
		},
	}

	err := resolver.streamACPAgentWS(context.Background(), req, make(chan WSStreamEvent, 8), nil)
	if err == nil {
		t.Fatal("streamACPAgentWS() error = nil, want unsupported replacement decision")
	}
	if pool.input.CanRequestUserInput || pool.input.StreamID != "" {
		t.Fatalf("replacement ACP prompt capability/stream = %t/%q, want false/empty", pool.input.CanRequestUserInput, pool.input.StreamID)
	}
	if len(messages.persisted) != 0 || len(messages.replacementRequests) != 0 {
		t.Fatalf("ordinary/replacement persists = %d/%d, want 0/0", len(messages.persisted), len(messages.replacementRequests))
	}
}
