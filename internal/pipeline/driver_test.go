package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

func TestLatestRCEventAtMs(t *testing.T) {
	t.Parallel()

	if got := latestRCEventCursor(nil); got != 0 {
		t.Fatalf("empty RC = %d, want 0", got)
	}
	got := latestRCEventCursor(RenderedContext{
		{ReceivedAtMs: 100},
		{ReceivedAtMs: 900},
		{ReceivedAtMs: 500, LastEventAtMs: 1_100, IsMyself: true},
	})
	if got != 1_100 {
		t.Fatalf("latest = %d, want 1100", got)
	}
}

// TestHandleReplyWithAgent_ColdStartAnchoredByTR simulates idle-timeout
// restart: the session's in-memory lastProcessedCursor is 0, but RC replay has
// brought back old user messages that were already answered in prior
// LLM rounds (represented by TRs). The driver MUST NOT re-answer them.
func TestHandleReplyWithAgent_ColdStartAnchoredByTR(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 100,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="old">task 1</message>`}},
		},
	}

	fakeAgent := &fakeDiscussStreamer{}
	resolver := &fakeRunConfigResolver{}

	driver := NewDiscussDriver(DiscussDriverDeps{
		Resolver: resolver,
	})

	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedCursor: 0,
	}

	// Simulate the cursor after a previously answered round.
	sess.lastProcessedCursor = 200

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig != nil {
		t.Fatal("agent must not be invoked when all RC segments predate lastProcessedCursor")
	}
}

// TestHandleReplyWithAgent_CursorAdvancesToRCNotWallClock ensures that after
// a turn we set lastProcessedCursor to the max ReceivedAtMs actually consumed in
// the RC snapshot, not time.Now(). This matters for messages that arrive
// mid-turn: they end up in a fresher RC with ReceivedAtMs > cursor, which
// correctly triggers the next round.
func TestHandleReplyWithAgent_CursorAdvancesToRCNotWallClock(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 777,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="x">hello</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	resolver := &fakeRunConfigResolver{}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Resolver: resolver,
	})
	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedCursor: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be invoked")
	}
	if sess.lastProcessedCursor != 777 {
		t.Fatalf("lastProcessedCursor = %d, want 777 (max RC ReceivedAtMs)", sess.lastProcessedCursor)
	}
}

func TestHandleReplyWithAgent_UsesPersistedDiscussCursor(t *testing.T) {
	store := &fakeDiscussCursorStore{cursor: 500, sourceCursor: 400}
	fakeAgent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Resolver:    &fakeRunConfigResolver{},
		CursorStore: store,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:           "b",
			SessionID:       "s",
			RouteID:         "route-1",
			CurrentPlatform: "telegram",
		},
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{
		{ReceivedAtMs: 400, Content: []RenderedContentPiece{{Type: "text", Text: `<message id="old">old</message>`}}},
	}, driver.logger, fakeAgent)

	if fakeAgent.lastConfig != nil {
		t.Fatal("agent must not be invoked for RC covered by persisted cursor")
	}
	if sess.lastProcessedCursor != 500 {
		t.Fatalf("lastProcessedCursor = %d, want persisted cursor 500", sess.lastProcessedCursor)
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{
		{ReceivedAtMs: 700, Content: []RenderedContentPiece{{Type: "text", Text: `<message id="new">new</message>`}}},
	}, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be invoked for RC past persisted cursor")
	}
	if store.upsertCursor.EventCursor != 700 || store.upsertCursor.SourceCursor != 700 {
		t.Fatalf("persisted cursor = %#v, want event/source 700", store.upsertCursor)
	}
	if store.upsertScope != "route:route-1" {
		t.Fatalf("scope = %q, want route:route-1", store.upsertScope)
	}
	if store.upsertRouteID != "route-1" {
		t.Fatalf("route id = %q, want route-1", store.upsertRouteID)
	}
}

func TestHandleReplyWithAgent_DoesNotCompareLegacySourceCursorToEventOrder(t *testing.T) {
	store := &fakeDiscussCursorStore{sourceCursor: 500}
	fakeAgent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Resolver:    &fakeRunConfigResolver{},
		CursorStore: store,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{BotID: "b", SessionID: "s"},
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{
		{
			MessageID:       "old",
			ReceivedAtMs:    400,
			EventCursor:     100,
			LastEventCursor: 100,
			Content:         []RenderedContentPiece{{Type: "text", Text: `<message id="old">old</message>`}},
		},
		{
			MessageID:       "new",
			ReceivedAtMs:    700,
			EventCursor:     200,
			LastEventCursor: 200,
			Content:         []RenderedContentPiece{{Type: "text", Text: `<message id="new">new</message>`}},
		},
	}, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected event 200 past the legacy source boundary to trigger a reply")
	}
	if sess.lastProcessedCursor != 200 {
		t.Fatalf("lastProcessedCursor = %d, want exact event cursor 200", sess.lastProcessedCursor)
	}
	if store.upsertCursor.EventCursor != 200 || store.upsertCursor.SourceCursor != 700 {
		t.Fatalf("persisted cursor = %#v, want event:200 source:700", store.upsertCursor)
	}
}

func TestHandleReplyWithAgent_DoesNotCompareEventCursorToHistoryWindow(t *testing.T) {
	nowMs := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC).UnixMilli()
	fakeAgent := &fakeDiscussStreamer{}
	resolver := &fakeRunConfigResolver{windowStartAtMs: nowMs - (24 * time.Hour).Milliseconds()}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedCursor: 100,
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{{
		MessageID:       "new",
		ReceivedAtMs:    nowMs,
		EventCursor:     101,
		LastEventCursor: 101,
		Content:         []RenderedContentPiece{{Type: "text", Text: `<message id="new">new</message>`}},
	}}, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("recent event with a sequence cursor did not trigger the agent")
	}
	if sess.lastProcessedCursor != 101 {
		t.Fatalf("lastProcessedCursor = %d, want 101", sess.lastProcessedCursor)
	}
}

func TestAgentEventToChannelEventMapsACPDecisionRequests(t *testing.T) {
	approval, ok := agentEventToChannelEvent(agentpkg.StreamEvent{
		Type:       agentpkg.EventToolApprovalRequest,
		ToolName:   "edit",
		ToolCallID: "call-1",
		ApprovalID: "approval-1",
		ShortID:    7,
		Input:      map[string]any{"path": "main.go"},
	})
	if !ok || approval.Type != channel.StreamEventToolCallStart || approval.ToolCall == nil {
		t.Fatalf("approval event = %#v, ok=%v", approval, ok)
	}
	if approval.ToolCall.ApprovalID != "approval-1" || len(approval.ToolCall.Actions) != 2 {
		t.Fatalf("approval tool call = %#v", approval.ToolCall)
	}

	userInput, ok := agentEventToChannelEvent(agentpkg.StreamEvent{
		Type:        agentpkg.EventUserInputRequest,
		ToolName:    "ask_user",
		ToolCallID:  "call-2",
		ApprovalID:  "approval-2",
		UserInputID: "input-1",
		ShortID:     8,
		Status:      "pending",
		Input:       map[string]any{"question": "Proceed?"},
	})
	if !ok || userInput.Type != channel.StreamEventToolCallStart || userInput.ToolCall == nil {
		t.Fatalf("user input event = %#v, ok=%v", userInput, ok)
	}
	payload, ok := userInput.ToolCall.Input.(map[string]any)
	if !ok {
		t.Fatalf("user input payload = %#v", userInput.ToolCall.Input)
	}
	if payload["user_input_id"] != "input-1" || payload["status"] != "pending" || len(userInput.ToolCall.Actions) != 1 {
		t.Fatalf("user input tool call = %#v payload=%#v", userInput.ToolCall, payload)
	}
}

func TestHandleReplyWithAgentRefreshesContextFragAfterLateBinding(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="x">hello</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{System: "base system"},
			ModelID:   "model-1",
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Resolver: resolver,
	})
	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedCursor: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be invoked")
	}
	cfg := fakeAgent.lastConfig
	if cfg.ContextManifest.Counts.Messages != len(cfg.Messages) {
		t.Fatalf("manifest message count = %d, messages = %d", cfg.ContextManifest.Counts.Messages, len(cfg.Messages))
	}
	if !lastMessageFragContains(cfg.ContextFrags, "IMPORTANT: You MUST use the `send` tool") {
		t.Fatalf("context frags do not include late-binding prompt: %#v", cfg.ContextManifest.Items)
	}
}

// --- Test helpers ---

type fakeDiscussStreamer struct {
	lastConfig *agentpkg.RunConfig
	endUsage   []byte
	events     []agentpkg.StreamEvent
}

func (f *fakeDiscussStreamer) Stream(_ context.Context, cfg agentpkg.RunConfig) <-chan agentpkg.StreamEvent {
	f.lastConfig = &cfg
	events := f.events
	if events == nil {
		events = []agentpkg.StreamEvent{{Type: agentpkg.EventAgentEnd, Usage: f.endUsage}}
	}
	ch := make(chan agentpkg.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

type fakeDiscussRuntimeStreamer struct {
	calls             int
	lastReq           conversation.ChatRequest
	streamErr         error
	noDurableResponse bool
	abort             bool
	emitErrorEvent    bool
	postTerminalErr   error
}

func (f *fakeDiscussRuntimeStreamer) StreamChat(_ context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	f.calls++
	f.lastReq = req
	chunks := make(chan conversation.StreamChunk, 2)
	errs := make(chan error, 1)
	if f.streamErr != nil {
		errs <- f.streamErr
	} else {
		if f.emitErrorEvent {
			chunks <- conversation.StreamChunk(`{"type":"error","error":"runtime failed after partial output"}`)
		}
		terminalType := "agent_end"
		if f.abort {
			terminalType = "agent_abort"
		}
		committed := !f.noDurableResponse
		chunks <- conversation.StreamChunk(fmt.Sprintf(`{"type":%q,"metadata":{"discuss_cursor_committed":%t}}`, terminalType, committed))
		if f.postTerminalErr != nil {
			errs <- f.postTerminalErr
		}
	}
	close(chunks)
	close(errs)
	return chunks, errs
}

type fakeRunConfigResolver struct {
	resolveResult         ResolveRunConfigResult
	resolveFn             func(botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (ResolveRunConfigResult, error)
	inlineFn              func(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart
	turnResponses         []TurnResponseEntry
	artifacts             []CompactionArtifact
	windowStartAtMs       int64
	artifactErr           error
	compactionCalls       int
	compactionInputTokens int
	compactionBudget      int
	compactionUserID      string
	compactionDone        chan struct{}
	compactionStarted     chan struct{}
	compactionBlock       <-chan struct{}
	trimCalls             int
	trimBudget            int
	trimFn                func(messages []ContextMessage, contextTokenBudget int, afterCursor int64) ([]ContextMessage, int)
	storeRoundErr         error
	storeRoundCursor      DiscussCursorCommit
	storeRoundNoResponse  bool
}

func (f *fakeRunConfigResolver) TrimDiscussContext(messages []ContextMessage, contextTokenBudget int, afterCursor int64) ([]ContextMessage, int) {
	f.trimCalls++
	f.trimBudget = contextTokenBudget
	if f.trimFn != nil {
		return f.trimFn(messages, contextTokenBudget, afterCursor)
	}
	return messages, 0
}

func (f *fakeRunConfigResolver) ResolveRunConfig(_ context.Context, botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (ResolveRunConfigResult, error) {
	if f.resolveFn != nil {
		return f.resolveFn(botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken)
	}
	return f.resolveResult, nil
}

func (f *fakeRunConfigResolver) InlineImageAttachments(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart {
	if f.inlineFn != nil {
		return f.inlineFn(ctx, botID, refs)
	}
	return nil
}

func (f *fakeRunConfigResolver) LoadContextHistoryProjection(context.Context, string, string) (ContextHistoryProjection, error) {
	latestTurnResponseAtMs := int64(0)
	for _, response := range f.turnResponses {
		if response.RequestedAtMs > latestTurnResponseAtMs {
			latestTurnResponseAtMs = response.RequestedAtMs
		}
	}
	return ContextHistoryProjection{
		TurnResponses:          f.turnResponses,
		CompactionArtifacts:    f.artifacts,
		LatestTurnResponseAtMs: latestTurnResponseAtMs,
		WindowStartAtMs:        f.windowStartAtMs,
	}, f.artifactErr
}

func (f *fakeRunConfigResolver) ScheduleCompaction(_ context.Context, _, _, userID string, inputTokens, contextTokenBudget int) {
	go func() {
		if f.compactionStarted != nil {
			select {
			case f.compactionStarted <- struct{}{}:
			default:
			}
		}
		if f.compactionBlock != nil {
			<-f.compactionBlock
		}
		f.compactionCalls++
		f.compactionInputTokens = inputTokens
		f.compactionBudget = contextTokenBudget
		f.compactionUserID = userID
		if f.compactionDone != nil {
			f.compactionDone <- struct{}{}
		}
	}()
}

func waitForFakeCompaction(t *testing.T, resolver *fakeRunConfigResolver) {
	t.Helper()
	if resolver.compactionDone == nil {
		t.Fatal("fake compaction completion channel is not configured")
	}
	select {
	case <-resolver.compactionDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for discuss compaction")
	}
}

func (f *fakeRunConfigResolver) StoreRound(_ context.Context, _, _, _, _, _ string, _ []sdk.Message, _ string) error {
	return f.storeRoundErr
}

func (f *fakeRunConfigResolver) StoreRoundWithCursor(
	_ context.Context,
	_, _, _, _, _ string,
	_ []sdk.Message,
	_ string,
	cursor DiscussCursorCommit,
) (bool, error) {
	f.storeRoundCursor = cursor
	return f.storeRoundErr == nil && !f.storeRoundNoResponse, f.storeRoundErr
}

type fakeDiscussCursorStore struct {
	cursor        int64
	sourceCursor  int64
	upsertCalls   int
	upsertCursor  DiscussCursorPosition
	upsertScope   string
	upsertRouteID string
}

func (f *fakeDiscussCursorStore) GetDiscussCursor(_ context.Context, _, _ string) (DiscussCursorPosition, error) {
	return DiscussCursorPosition{EventCursor: f.cursor, SourceCursor: f.sourceCursor}, nil
}

func (f *fakeDiscussCursorStore) UpsertDiscussCursor(_ context.Context, _, scopeKey, routeID, _ string, cursor DiscussCursorPosition) error {
	f.upsertCalls++
	f.upsertScope = scopeKey
	f.upsertRouteID = routeID
	f.upsertCursor = cursor
	return nil
}

func lastMessageFragContains(frags []contextfrag.ContextFrag, needle string) bool {
	for i := len(frags) - 1; i >= 0; i-- {
		frag := frags[i]
		if frag.Kind != contextfrag.KindConversationEvent || len(frag.Parts) == 0 || frag.Parts[0].SDKMessage == nil {
			continue
		}
		for _, part := range frag.Parts[0].SDKMessage.Content {
			if text, ok := part.(sdk.TextPart); ok && strings.Contains(text.Text, needle) {
				return true
			}
		}
		return false
	}
	return false
}

func TestContextMessagesToSDKEntriesPreservesSystemRole(t *testing.T) {
	t.Parallel()

	entries := contextMessagesToSDKEntries([]ContextMessage{{Role: "system", Content: "[System Notice] history trimmed"}})

	if len(entries) != 1 || entries[0].Message.Role != sdk.MessageRoleSystem {
		t.Fatalf("system context message reached the model as %q, want system role", entries[0].Message.Role)
	}
}
