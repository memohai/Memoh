package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

type acknowledgementRecordingACPPrompter struct {
	events      []event.StreamEvent
	acks        []bool
	afterEvents func()
}

func (p *acknowledgementRecordingACPPrompter) Prompt(_ context.Context, input acpagent.PromptInput) (acpclient.PromptResult, error) {
	for _, streamEvent := range p.events {
		p.acks = append(p.acks, input.Sink.EmitStreamEvent(streamEvent))
	}
	if p.afterEvents != nil {
		p.afterEvents()
	}
	return withTranscriptOutput(acpclient.PromptResult{Events: p.events}), nil
}

func TestStreamChatDiscussACPReportsRoundPersistenceFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist ACP round")
	resolver := &Resolver{
		messageService: &recordingMessageService{persistErr: wantErr},
		acpPool: &recordingACPPrompter{result: acpclient.PromptResult{
			Text:       "done from codex",
			StopReason: "end_turn",
		}},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return discussACPTestSession(sessionID), nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	chunks, errs := resolver.StreamChat(context.Background(), persistedDiscussACPRequest())
	events := drainStreamChunks(t, chunks)
	if !containsStreamEvent(events, agentpkg.EventEnd) {
		t.Fatalf("events = %#v, want terminal event", events)
	}
	if err := <-errs; !errors.Is(err, wantErr) {
		t.Fatalf("StreamChat() error = %v, want %v", err, wantErr)
	}
}

func TestStreamChatACPDoesNotPublishUnpersistedPendingDecision(t *testing.T) {
	t.Parallel()

	decision := event.StreamEvent{
		Type:       event.ToolApprovalRequest,
		ToolCallID: "write-1",
		ToolName:   "write",
		ApprovalID: "approval-1",
		Status:     "pending",
	}
	messages := &nthPersistFailureMessageService{failOnCall: 1, err: errors.New("persist pending projection")}
	projectionPersistCalls := 0
	pool := &acknowledgementRecordingACPPrompter{
		events: []event.StreamEvent{decision, decision},
		afterEvents: func() {
			projectionPersistCalls = messages.persistCalls
		},
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool:        pool,
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}
	req := persistedDiscussACPRequest()
	req.StreamID = "stream-1"
	eventCh := make(chan WSStreamEvent, 8)

	if err := resolver.streamACPAgentWS(context.Background(), req, eventCh, nil); err != nil {
		t.Fatalf("streamACPAgentWS() error = %v", err)
	}
	if len(pool.acks) != 2 || pool.acks[0] || pool.acks[1] {
		t.Fatalf("decision acknowledgements = %v, want [false false]", pool.acks)
	}
	if projectionPersistCalls != 1 {
		t.Fatalf("projection persist calls = %d, want one sticky failed pending attempt", projectionPersistCalls)
	}
	for _, persisted := range messages.persisted {
		if calls := extractAssistantToolCallParts(persistedModelMessage(t, persisted.Content)); len(calls) != 0 {
			t.Fatalf("failed pending projection leaked into terminal transcript: %#v", calls)
		}
	}
	close(eventCh)
	for raw := range eventCh {
		var streamed agentpkg.StreamEvent
		if err := json.Unmarshal(raw, &streamed); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if isACPDecisionProjectionEvent(streamed) {
			t.Fatalf("unpersisted decision was published: %#v", streamed)
		}
	}
}

func TestStreamChatACPDeliversExternalChannelDecisionWithoutStreamID(t *testing.T) {
	t.Parallel()

	streamed := []event.StreamEvent{
		{
			Type:       event.ToolApprovalRequest,
			ToolCallID: "write-1",
			ToolName:   "write",
			ApprovalID: "approval-1",
			Status:     toolapproval.StatusPending,
		},
		{
			Type:       event.ToolApprovalRequest,
			ToolCallID: "write-1",
			ToolName:   "write",
			ApprovalID: "approval-1",
			Status:     toolapproval.StatusRejected,
		},
		{
			Type:        event.UserInputRequest,
			ToolCallID:  "ask-1",
			ToolName:    userinput.ToolNameAskUser,
			UserInputID: "input-1",
			Status:      userinput.StatusPending,
		},
		{
			Type:        event.UserInputRequest,
			ToolCallID:  "ask-1",
			ToolName:    userinput.ToolNameAskUser,
			UserInputID: "input-1",
			Status:      userinput.StatusCanceled,
		},
	}
	pool := &recordingACPPrompter{
		result:       withTranscriptOutput(acpclient.PromptResult{Events: streamed}),
		streamEvents: streamed,
	}
	resolver := &Resolver{
		messageService: &recordingMessageService{},
		acpPool:        pool,
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		userInput:      &fakeUserInputService{},
		logger:         slog.New(slog.DiscardHandler),
	}

	chunks, errs := resolver.StreamChat(context.Background(), conversation.ChatRequest{
		BotID:          "bot-1",
		SessionID:      "session-1",
		Query:          "inspect the app",
		CurrentChannel: "telegram",
	})
	events := drainStreamChunks(t, chunks)
	if err := <-errs; err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if pool.input.StreamID != "" || !pool.input.CanRequestUserInput {
		t.Fatalf("ACP prompt stream/capability = %q/%t, want empty/true", pool.input.StreamID, pool.input.CanRequestUserInput)
	}
	if !containsStreamEvent(events, agentpkg.EventToolApprovalRequest) || !containsStreamEvent(events, agentpkg.EventUserInputRequest) {
		t.Fatalf("events = %#v, want external approval and ask_user delivery", events)
	}
}

func TestStreamChatDiscussACPReportsFailedRoundPersistenceFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist failed ACP round")
	resolver := &Resolver{
		messageService: &recordingMessageService{persistErr: wantErr},
		acpPool: &recordingACPPrompter{
			result: acpclient.PromptResult{Text: "partial output"},
			err:    errors.New("ACP runtime failed"),
		},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return discussACPTestSession(sessionID), nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	chunks, errs := resolver.StreamChat(context.Background(), persistedDiscussACPRequest())
	events := drainStreamChunks(t, chunks)
	if !containsStreamEvent(events, agentpkg.EventAgentAbort) {
		t.Fatalf("events = %#v, want terminal abort", events)
	}
	if err := <-errs; !errors.Is(err, wantErr) {
		t.Fatalf("StreamChat() error = %v, want %v", err, wantErr)
	}
}

func TestStreamChatDiscussACPCommitsResponseWithCursor(t *testing.T) {
	t.Parallel()

	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	var ordinaryBeforeTerminal atomic.Bool
	streamed := []event.StreamEvent{
		{
			Type:       event.ToolCallStart,
			ToolCallID: "read-1",
			ToolName:   "read",
			Input:      map[string]any{"path": "README.md"},
		},
		{
			Type:       event.ToolCallEnd,
			ToolCallID: "read-1",
			ToolName:   "read",
			Result:     map[string]any{"content": "project"},
		},
		{Type: event.TextDelta, Delta: "done from codex"},
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool: &recordingACPPrompter{
			result: withTranscriptOutput(acpclient.PromptResult{
				Events:     streamed,
				StopReason: "end_turn",
			}),
			streamEvents: streamed,
			afterEvents: func() {
				ordinaryBeforeTerminal.Store(len(messages.persisted) != 0)
			},
		},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return discussACPTestSession(sessionID), nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}
	req := persistedDiscussACPRequest()
	req.RouteID = "route-1"
	req.CurrentChannel = "telegram"
	req.DiscussCursorScope = "route:route-1"
	req.DiscussConsumedCursor = 300
	req.DiscussConsumedEventCursor = 321
	req.DiscussDeliveryClaims = []conversation.DeliveryClaim{{
		EventID:    "77777777-7777-7777-7777-777777777777",
		ClaimToken: "88888888-8888-8888-8888-888888888888",
	}}

	chunks, errs := resolver.StreamChat(context.Background(), req)
	events := drainStreamChunks(t, chunks)
	if err := <-errs; err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if messages.cursorCalls != 1 || messages.cursor.ConsumedEventCursor != 321 {
		t.Fatalf("atomic ACP cursor calls/cursor = %d/%#v, want 1/321", messages.cursorCalls, messages.cursor)
	}
	if len(messages.persisted) == 0 {
		t.Fatal("ACP response was not persisted with its cursor")
	}
	if ordinaryBeforeTerminal.Load() {
		t.Fatal("exact ACP persisted a decision before its terminal transaction")
	}
	terminal := requireStreamEvent(t, events, agentpkg.EventEnd)
	if committed, ok := terminal.Metadata[agentpkg.MetadataKeyDiscussCursorCommitted].(bool); !ok || !committed {
		t.Fatalf("terminal cursor commit metadata = %#v, want true", terminal.Metadata)
	}
	toolCalls := 0
	finalText := false
	for _, input := range messages.cursorInputs {
		model := persistedModelMessage(t, input.Content)
		for _, call := range extractAssistantToolCallParts(model) {
			if call.ToolCallID == "read-1" {
				toolCalls++
			}
		}
		finalText = finalText || strings.Contains(model.TextContent(), "done from codex")
	}
	if toolCalls != 1 || !finalText {
		t.Fatalf("atomic ACP tool calls/final text = %d/%t, want 1/true", toolCalls, finalText)
	}
	if len(messages.cursor.DeliveryClaims) != 1 || messages.cursor.DeliveryClaims[0].ClaimToken != req.DiscussDeliveryClaims[0].ClaimToken {
		t.Fatalf("atomic ACP delivery claims = %#v, want exact claim", messages.cursor.DeliveryClaims)
	}
}

func TestStreamChatDiscussACPRejectsDecisionEvents(t *testing.T) {
	t.Parallel()

	decision := event.StreamEvent{
		Type:        event.UserInputRequest,
		ToolCallID:  "ask-1",
		ToolName:    userinput.ToolNameAskUser,
		UserInputID: "input-1",
		Status:      userinput.StatusPending,
	}
	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	pool := &recordingACPPrompter{
		result:       withTranscriptOutput(acpclient.PromptResult{Events: []event.StreamEvent{decision}}),
		streamEvents: []event.StreamEvent{decision},
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool:        pool,
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return discussACPTestSession(sessionID), nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}
	resolver.userInput = &fakeUserInputService{}
	req := persistedDiscussACPRequest()
	req.StreamID = "stream-1"
	req.DiscussConsumedEventCursor = 321
	req.DiscussDeliveryClaims = []conversation.DeliveryClaim{{
		EventID:    "77777777-7777-7777-7777-777777777777",
		ClaimToken: "88888888-8888-8888-8888-888888888888",
	}}
	eventCh := make(chan WSStreamEvent, 8)

	err := resolver.streamACPAgentWS(context.Background(), req, eventCh, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot accept decision event") {
		t.Fatalf("streamACPAgentWS() error = %v, want unsupported decision error", err)
	}
	if pool.input.SessionType != session.TypeDiscuss || pool.input.CanRequestUserInput || pool.input.StreamID != "" {
		t.Fatalf("ACP prompt session/capability/stream = %q/%t/%q, want discuss/false/empty", pool.input.SessionType, pool.input.CanRequestUserInput, pool.input.StreamID)
	}
	if len(messages.persisted) != 0 || messages.cursorCalls != 0 {
		t.Fatalf("ordinary/terminal persists = %d/%d, want 0/0", len(messages.persisted), messages.cursorCalls)
	}
	close(eventCh)
	for data := range eventCh {
		var streamed agentpkg.StreamEvent
		if err := json.Unmarshal(data, &streamed); err != nil {
			t.Fatalf("decode ACP event: %v", err)
		}
		if isACPDecisionProjectionEvent(streamed) {
			t.Fatalf("non-interactive decision event was published: %#v", streamed)
		}
	}
}

func TestACPDecisionProjectionEventRecognizesMissingToolCallID(t *testing.T) {
	t.Parallel()

	for _, eventType := range []agentpkg.StreamEventType{
		agentpkg.EventUserInputRequest,
		agentpkg.EventToolApprovalRequest,
	} {
		if !isACPDecisionProjectionEvent(agentpkg.StreamEvent{Type: eventType}) {
			t.Fatalf("decision event %q with no tool call id was not recognized", eventType)
		}
	}
}

func TestStreamChatACPWaitsForDecisionProjectionBeforePublishing(t *testing.T) {
	t.Parallel()

	decision := event.StreamEvent{
		Type:        event.UserInputRequest,
		ToolCallID:  "ask-1",
		ToolName:    userinput.ToolNameAskUser,
		UserInputID: "input-1",
		Status:      userinput.StatusPending,
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseProjection := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseProjection()
	messages := &blockingACPProjectionMessageService{
		recordingMessageService: &recordingMessageService{},
		started:                 started,
		release:                 release,
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool: &recordingACPPrompter{
			result:       withTranscriptOutput(acpclient.PromptResult{Events: []event.StreamEvent{decision}, Text: "done"}),
			streamEvents: []event.StreamEvent{decision},
		},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}
	resolver.userInput = &fakeUserInputService{}
	req := persistedDiscussACPRequest()
	req.StreamID = "stream-1"
	eventCh := make(chan WSStreamEvent, 8)
	done := make(chan error, 1)
	go func() { done <- resolver.streamACPAgentWS(context.Background(), req, eventCh, nil) }()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ACP decision projection")
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("decision projection committed before release: %#v", messages.persisted)
	}
	for len(eventCh) > 0 {
		var streamed agentpkg.StreamEvent
		if err := json.Unmarshal(<-eventCh, &streamed); err != nil {
			t.Fatalf("decode ACP event: %v", err)
		}
		if isACPDecisionProjectionEvent(streamed) {
			t.Fatalf("decision event published before projection commit: %#v", streamed)
		}
	}

	releaseProjection()
	if err := <-done; err != nil {
		t.Fatalf("streamACPAgentWS() error = %v", err)
	}
	close(eventCh)
	promptSeen := false
	for data := range eventCh {
		var streamed agentpkg.StreamEvent
		if err := json.Unmarshal(data, &streamed); err != nil {
			t.Fatalf("decode ACP event: %v", err)
		}
		promptSeen = promptSeen || streamed.Type == agentpkg.EventUserInputRequest
	}
	if !promptSeen || len(messages.persisted) == 0 {
		t.Fatalf("prompt/persisted = %t/%d, want true/nonzero", promptSeen, len(messages.persisted))
	}
}

func TestPersistACPLeadingUserMessageRejectsUnpersistedExactDelivery(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	req := persistedDiscussACPRequest()
	req.UserMessagePersisted = false
	req.PersistedUserMessageID = ""
	req.DiscussDeliveryClaims = []conversation.DeliveryClaim{{
		EventID:    "77777777-7777-7777-7777-777777777777",
		ClaimToken: "88888888-8888-8888-8888-888888888888",
	}}

	if _, err := resolver.persistACPLeadingUserMessage(context.Background(), req); err == nil {
		t.Fatal("persistACPLeadingUserMessage() error = nil")
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("ordinary leading messages = %d, want 0", len(messages.persisted))
	}
}

func TestStreamChatDiscussACPCommitsFailedResponseWithCursor(t *testing.T) {
	t.Parallel()

	messages := &cursorRecordingMessageService{recordingMessageService: &recordingMessageService{}}
	resolver := &Resolver{
		messageService: messages,
		acpPool: &recordingACPPrompter{
			result: acpclient.PromptResult{Text: "partial output"},
			err:    errors.New("ACP runtime failed"),
		},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return discussACPTestSession(sessionID), nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}
	req := persistedDiscussACPRequest()
	req.RouteID = "route-1"
	req.CurrentChannel = "telegram"
	req.DiscussCursorScope = "route:route-1"
	req.DiscussConsumedCursor = 300
	req.DiscussConsumedEventCursor = 321

	chunks, errs := resolver.StreamChat(context.Background(), req)
	events := drainStreamChunks(t, chunks)
	if err := <-errs; err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if messages.cursorCalls != 1 || len(messages.persisted) == 0 {
		t.Fatalf("failed response/cursor commits = %d/%d, want one atomic commit", len(messages.persisted), messages.cursorCalls)
	}
	terminal := requireStreamEvent(t, events, agentpkg.EventAgentAbort)
	if committed, ok := terminal.Metadata[agentpkg.MetadataKeyDiscussCursorCommitted].(bool); !ok || !committed {
		t.Fatalf("failed terminal cursor commit metadata = %#v, want true", terminal.Metadata)
	}
}

func TestStreamChatDiscussACPDoesNotCommitAfterDeliveryContextCancellation(t *testing.T) {
	t.Parallel()
	streamed := event.StreamEvent{Type: event.TextDelta, Delta: "partial"}

	for _, tc := range []struct {
		name      string
		promptErr error
	}{
		{name: "successful prompt"},
		{name: "failed prompt", promptErr: errors.New("ACP runtime failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			messages := &contextAwareCursorMessageService{
				cursorRecordingMessageService: &cursorRecordingMessageService{
					recordingMessageService: &recordingMessageService{},
				},
			}
			resolver := &Resolver{
				messageService: messages,
				acpPool: &recordingACPPrompter{
					result:       withTranscriptOutput(acpclient.PromptResult{Events: []event.StreamEvent{streamed}, Text: "done from codex"}),
					err:          tc.promptErr,
					streamEvents: []event.StreamEvent{streamed},
					afterEvents:  cancel,
				},
				botPermissions: allowWorkspaceExecFor("user-1"),
				sessionService: &fakeBackgroundSessionService{
					getFn: func(_ context.Context, sessionID string) (session.Session, error) {
						return discussACPTestSession(sessionID), nil
					},
				},
				logger: slog.New(slog.DiscardHandler),
			}
			req := persistedDiscussACPRequest()
			req.RouteID = "route-1"
			req.CurrentChannel = "telegram"
			req.DiscussCursorScope = "route:route-1"
			req.DiscussConsumedCursor = 300
			req.DiscussConsumedEventCursor = 321
			req.DiscussDeliveryClaims = []conversation.DeliveryClaim{{
				EventID:    "77777777-7777-7777-7777-777777777777",
				ClaimToken: "88888888-8888-8888-8888-888888888888",
			}}

			err := resolver.streamACPAgentWS(ctx, req, make(chan WSStreamEvent, 32), nil)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("streamACPAgentWS() error = %v, want context cancellation", err)
			}
			if messages.attempts != 1 {
				t.Fatalf("atomic persistence attempts = %d, want 1", messages.attempts)
			}
			if messages.cursorCalls != 0 || len(messages.persisted) != 0 {
				t.Fatalf("committed response/cursor = %d/%d after cancellation, want 0/0", len(messages.persisted), messages.cursorCalls)
			}
		})
	}
}

type contextAwareCursorMessageService struct {
	*cursorRecordingMessageService
	attempts int
}

type blockingACPProjectionMessageService struct {
	*recordingMessageService
	started chan struct{}
	release <-chan struct{}
	calls   atomic.Int32
}

func (s *blockingACPProjectionMessageService) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	if s.calls.Add(1) == 1 {
		close(s.started)
		<-s.release
	}
	return s.recordingMessageService.Persist(ctx, input)
}

func (s *contextAwareCursorMessageService) PersistTurnResponseWithCursor(
	ctx context.Context,
	inputs []messagepkg.PersistInput,
	cursor messagepkg.DiscussCursorUpdate,
) ([]messagepkg.Message, error) {
	s.attempts++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.cursorRecordingMessageService.PersistTurnResponseWithCursor(ctx, inputs, cursor)
}

func persistedDiscussACPRequest() conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:                  "bot-1",
		SessionID:              "session-1",
		Query:                  "inspect the group thread",
		UserMessagePersisted:   true,
		PersistedUserMessageID: "user-message-1",
	}
}

func discussACPTestSession(sessionID string) session.Session {
	return session.Session{
		ID:          sessionID,
		BotID:       "bot-1",
		Type:        session.TypeDiscuss,
		SessionMode: session.TypeDiscuss,
		RuntimeType: session.RuntimeACPAgent,
		RuntimeMetadata: map[string]any{
			"acp_agent_id":             "codex",
			"project_path":             "/data/app",
			"runtime_owner_account_id": "user-1",
		},
	}
}

func TestStreamChatACPFiltersFailedPendingAcrossTranscriptFlush(t *testing.T) {
	t.Parallel()

	streamed := []event.StreamEvent{
		{Type: event.ToolCallStart, ToolCallID: "write-1", ToolName: "write"},
		{
			Type:       event.ToolApprovalRequest,
			ToolCallID: "write-1",
			ToolName:   "write",
			ApprovalID: "approval-1",
			Status:     toolapproval.StatusPending,
		},
		{Type: event.ToolCallStart, ToolCallID: "read-1", ToolName: "read"},
		{Type: event.ToolCallEnd, ToolCallID: "read-1", ToolName: "read", Result: "done"},
		{
			Type:       event.ToolApprovalRequest,
			ToolCallID: "write-1",
			ToolName:   "write",
			ApprovalID: "approval-1",
			Status:     toolapproval.StatusRejected,
		},
		{Type: event.TextDelta, Delta: "finished"},
	}
	messages := &nthPersistFailureMessageService{failOnCall: 1, err: errors.New("persist pending projection")}
	projectionPersistCalls := 0
	pool := &acknowledgementRecordingACPPrompter{
		events: streamed,
		afterEvents: func() {
			projectionPersistCalls = messages.persistCalls
		},
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool:        pool,
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}
	req := persistedDiscussACPRequest()
	req.StreamID = "stream-1"
	eventCh := make(chan WSStreamEvent, 32)

	if err := resolver.streamACPAgentWS(context.Background(), req, eventCh, nil); err != nil {
		t.Fatalf("streamACPAgentWS() error = %v", err)
	}
	if projectionPersistCalls != 2 {
		t.Fatalf("projection persist calls = %d, want failed pending then rejected", projectionPersistCalls)
	}
	writeCalls := 0
	for _, persisted := range messages.persisted {
		for _, call := range extractAssistantToolCallParts(persistedModelMessage(t, persisted.Content)) {
			if call.ToolCallID != "write-1" {
				continue
			}
			writeCalls++
			if status := toolCallMetadataStatus(call, "approval"); status != toolapproval.StatusRejected {
				t.Fatalf("persisted write status = %q, want rejected", status)
			}
		}
	}
	if writeCalls != 1 {
		t.Fatalf("persisted write calls = %d, want only terminal projection", writeCalls)
	}
	close(eventCh)
	pendingPublished := false
	rejectedPublished := false
	for raw := range eventCh {
		var streamEvent agentpkg.StreamEvent
		if err := json.Unmarshal(raw, &streamEvent); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if streamEvent.Type != agentpkg.EventToolApprovalRequest {
			continue
		}
		pendingPublished = pendingPublished || streamEvent.Status == toolapproval.StatusPending
		rejectedPublished = rejectedPublished || streamEvent.Status == toolapproval.StatusRejected
	}
	if pendingPublished || !rejectedPublished {
		t.Fatalf("pending/rejected published = %t/%t, want false/true", pendingPublished, rejectedPublished)
	}
}
