package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestExtractNewImageRefs(t *testing.T) {
	rc := RenderedContext{
		{ReceivedAtMs: 100, ImageRefs: []ImageAttachmentRef{{ContentHash: "old-hash", Mime: "image/png"}}},
		{ReceivedAtMs: 200, IsMyself: true, ImageRefs: []ImageAttachmentRef{{ContentHash: "self-hash"}}},
		{ReceivedAtMs: 300, ImageRefs: []ImageAttachmentRef{{ContentHash: "new-hash", Mime: "image/jpeg"}}},
		{ReceivedAtMs: 400, ImageRefs: nil},
	}

	refs := extractNewImageRefs(rc, 150)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].ContentHash != "new-hash" {
		t.Fatalf("expected new-hash, got %q", refs[0].ContentHash)
	}
	if refs[0].Mime != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", refs[0].Mime)
	}
}

func TestExtractNewImageRefs_IncludesMultiple(t *testing.T) {
	rc := RenderedContext{
		{ReceivedAtMs: 100},
		{ReceivedAtMs: 200, ImageRefs: []ImageAttachmentRef{
			{ContentHash: "a"},
			{ContentHash: "b"},
		}},
		{ReceivedAtMs: 300, ImageRefs: []ImageAttachmentRef{{ContentHash: "c"}}},
	}
	refs := extractNewImageRefs(rc, 50)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}
}

func TestInjectImagePartsIntoLastUserMessage(t *testing.T) {
	msgs := []sdk.Message{
		sdk.UserMessage("hello"),
		sdk.AssistantMessage("hi"),
		sdk.UserMessage("look at this"),
	}
	parts := []sdk.ImagePart{
		{Image: "data:image/png;base64,abc", MediaType: "image/png"},
	}

	injectImagePartsIntoLastUserMessage(msgs, parts)

	lastUser := msgs[2]
	if len(lastUser.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(lastUser.Content))
	}
	imgPart, ok := lastUser.Content[1].(sdk.ImagePart)
	if !ok {
		t.Fatalf("expected ImagePart, got %T", lastUser.Content[1])
	}
	if imgPart.Image != "data:image/png;base64,abc" {
		t.Fatalf("unexpected image: %q", imgPart.Image)
	}
}

func TestInjectImagePartsIntoLastUserMessage_Empty(t *testing.T) {
	msgs := []sdk.Message{sdk.UserMessage("hello")}
	injectImagePartsIntoLastUserMessage(msgs, nil)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected no change, got %d parts", len(msgs[0].Content))
	}
}

func TestInjectImagePartsIntoLastUserMessage_SkipsEmptyImage(t *testing.T) {
	msgs := []sdk.Message{sdk.UserMessage("hello")}
	parts := []sdk.ImagePart{{Image: "", MediaType: "image/png"}}
	injectImagePartsIntoLastUserMessage(msgs, parts)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected no change, got %d parts", len(msgs[0].Content))
	}
}

func TestHandleReplyWithAgent_InlinesImages(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">photo</message>`}},
			ImageRefs:    []ImageAttachmentRef{{ContentHash: "img-hash", Mime: "image/jpeg"}},
		},
	}

	fakeAgent := &fakeDiscussStreamer{}

	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{
				SupportsImageInput: true,
			},
			ModelID: "model-1",
		},
		inlineFn: func(_ context.Context, _ string, refs []ImageAttachmentRef) []sdk.ImagePart {
			if len(refs) != 1 || refs[0].ContentHash != "img-hash" {
				t.Fatalf("unexpected refs: %v", refs)
			}
			return []sdk.ImagePart{{Image: "data:image/jpeg;base64,FAKE", MediaType: "image/jpeg"}}
		},
	}

	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})

	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-1",
			SessionID: "sess-1",
		},
		lastProcessedMs: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be called")
	}

	msgs := fakeAgent.lastConfig.Messages
	var userMsgs []sdk.Message
	for _, m := range msgs {
		if m.Role == sdk.MessageRoleUser {
			userMsgs = append(userMsgs, m)
		}
	}
	if len(userMsgs) < 2 {
		t.Fatalf("expected at least 2 user messages (rc + late binding), got %d", len(userMsgs))
	}
	rcMsg := userMsgs[0]
	hasImage := false
	for _, part := range rcMsg.Content {
		if imgPart, ok := part.(sdk.ImagePart); ok {
			hasImage = true
			if !strings.HasPrefix(imgPart.Image, "data:image/jpeg;base64,") {
				t.Fatalf("unexpected image data: %q", imgPart.Image)
			}
		}
	}
	if !hasImage {
		t.Fatal("expected image part in RC user message")
	}
}

func TestHandleReplyWithAgent_NoInlineWhenNoVision(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">photo</message>`}},
			ImageRefs:    []ImageAttachmentRef{{ContentHash: "img-hash", Mime: "image/jpeg"}},
		},
	}

	fakeAgent := &fakeDiscussStreamer{}

	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{
				SupportsImageInput: false,
			},
			ModelID: "model-1",
		},
		inlineFn: func(_ context.Context, _ string, _ []ImageAttachmentRef) []sdk.ImagePart {
			t.Fatal("should not be called when model doesn't support vision")
			return nil
		},
	}

	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})

	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-1",
			SessionID: "sess-1",
		},
		lastProcessedMs: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be called")
	}
	for _, m := range fakeAgent.lastConfig.Messages {
		for _, part := range m.Content {
			if _, ok := part.(sdk.ImagePart); ok {
				t.Fatal("should not have image parts when vision is not supported")
			}
		}
	}
}

func TestHandleReplyWithAgent_UsesRuntimeStreamerForACPDiscuss(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			MentionsMe:   true,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">please inspect the app</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	runtime := &fakeDiscussRuntimeStreamer{}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RunConfig:   agentpkg.RunConfig{},
			ModelID:     "model-1",
			RuntimeType: sessionpkg.RuntimeACPAgent,
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline:        NewPipeline(RenderParams{}),
		Resolver:        resolver,
		RuntimeStreamer: runtime,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:             "bot-1",
			SessionID:         "sess-1",
			RouteID:           "route-1",
			ChannelIdentityID: "acct-1",
			CurrentPlatform:   "telegram",
			ReplyTarget:       "chat-1",
			ConversationType:  "group",
			SessionToken:      "Bearer owner-token",
			ChatToken:         "chat-token",
			ToolHTTPURL:       "http://example.test/bots/bot-1/tools",
		},
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig != nil {
		t.Fatal("ordinary agent should not be invoked for ACP discuss runtime")
	}
	if runtime.calls != 1 {
		t.Fatalf("runtime calls = %d, want 1", runtime.calls)
	}
	if runtime.lastReq.BotID != "bot-1" || runtime.lastReq.SessionID != "sess-1" || runtime.lastReq.SourceChannelIdentityID != "acct-1" {
		t.Fatalf("runtime request = %#v", runtime.lastReq)
	}
	if runtime.lastReq.RouteID != "route-1" || runtime.lastReq.ChatToken != "chat-token" || runtime.lastReq.Token != "Bearer owner-token" {
		t.Fatalf("runtime context = route %q chat token %q token %q", runtime.lastReq.RouteID, runtime.lastReq.ChatToken, runtime.lastReq.Token)
	}
	if runtime.lastReq.ToolHTTPURL != "http://example.test/bots/bot-1/tools" {
		t.Fatalf("ToolHTTPURL = %q", runtime.lastReq.ToolHTTPURL)
	}
	if !strings.Contains(runtime.lastReq.Query, "please inspect the app") || !strings.Contains(runtime.lastReq.Query, "reset each turn") || !strings.Contains(runtime.lastReq.Query, "MUST use the `send` tool") {
		t.Fatalf("runtime query = %q, want full discuss context", runtime.lastReq.Query)
	}
	if !runtime.lastReq.UserMessagePersisted {
		t.Fatal("runtime request should avoid duplicating the full-context prompt as a user history message")
	}
	if !runtime.lastReq.ForceFreshRuntime {
		t.Fatal("discuss ACP runtime request should force a fresh runtime each turn")
	}
	if sess.lastProcessedMs != 200 {
		t.Fatalf("lastProcessedMs = %d, want 200", sess.lastProcessedMs)
	}
}

func TestNotifyRCRefreshesExistingDiscussSessionConfig(t *testing.T) {
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
	})
	defer driver.StopSession("sess-1")

	driver.NotifyRC(context.Background(), "sess-1", RenderedContext{}, DiscussSessionConfig{
		BotID:        "bot-1",
		SessionID:    "sess-1",
		RouteID:      "route-old",
		ChatToken:    "chat-token-old",
		SessionToken: "session-token-old",
		ToolHTTPURL:  "http://old.example/tools",
	})
	driver.NotifyRC(context.Background(), "sess-1", RenderedContext{}, DiscussSessionConfig{
		BotID:        "bot-1",
		SessionID:    "sess-1",
		RouteID:      "route-new",
		ChatToken:    "chat-token-new",
		SessionToken: "session-token-new",
		ToolHTTPURL:  "http://new.example/tools",
	})

	driver.mu.Lock()
	got := driver.sessions["sess-1"].config
	driver.mu.Unlock()
	if got.RouteID != "route-new" || got.ChatToken != "chat-token-new" || got.SessionToken != "session-token-new" || got.ToolHTTPURL != "http://new.example/tools" {
		t.Fatalf("config = %#v, want latest NotifyRC config", got)
	}
}

func TestHandleReplyWithAgentReadsConfigUnderDriverLock(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			MentionsMe:   true,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">please inspect the app</message>`}},
		},
	}
	calls := make(chan string, 1)
	resolver := &fakeRunConfigResolver{
		resolveFn: func(botID, _, _, _, _, _, _ string) (ResolveRunConfigResult, error) {
			calls <- botID
			return ResolveRunConfigResult{}, nil
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-old",
			SessionID: "sess-1",
		},
	}

	driver.mu.Lock()
	done := make(chan struct{})
	go func() {
		driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})
		close(done)
	}()

	select {
	case got := <-calls:
		driver.mu.Unlock()
		t.Fatalf("resolver called before config lock released with bot %q", got)
	case <-time.After(25 * time.Millisecond):
	}

	sess.config = DiscussSessionConfig{
		BotID:     "bot-new",
		SessionID: "sess-1",
	}
	driver.mu.Unlock()

	select {
	case got := <-calls:
		if got != "bot-new" {
			t.Fatalf("resolver bot id = %q, want refreshed config", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resolver call")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler")
	}
}

func TestHandleReplyWithAgent_ACPDiscussDoesNotAdvanceCursorWithoutRuntimeStreamer(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			MentionsMe:   true,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">please inspect the app</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RuntimeType: sessionpkg.RuntimeACPAgent,
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-1",
			SessionID: "sess-1",
		},
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig != nil {
		t.Fatal("ordinary agent should not be invoked for ACP discuss runtime")
	}
	if sess.lastProcessedMs != 0 {
		t.Fatalf("lastProcessedMs = %d, want 0 when ACP runtime streamer is missing", sess.lastProcessedMs)
	}
	if resolver.compactionCalls != 0 {
		t.Fatalf("missing-runtime compaction calls = %d, want 0", resolver.compactionCalls)
	}
}

func TestHandleReplyWithAgent_ACPDiscussDoesNotAdvanceCursorOnRuntimeError(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			MentionsMe:   true,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">please inspect the app</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	runtime := &fakeDiscussRuntimeStreamer{streamErr: errors.New("runtime failed")}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RuntimeType: sessionpkg.RuntimeACPAgent,
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline:        NewPipeline(RenderParams{}),
		Resolver:        resolver,
		RuntimeStreamer: runtime,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-1",
			SessionID: "sess-1",
			UserID:    "account-user",
		},
	}
	wantEstimate := ComposeContext(rc, nil).EstimatedTokens

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if runtime.calls != 1 {
		t.Fatalf("runtime calls = %d, want 1", runtime.calls)
	}
	if sess.lastProcessedMs != 0 {
		t.Fatalf("lastProcessedMs = %d, want 0 when ACP runtime stream fails", sess.lastProcessedMs)
	}
	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != wantEstimate {
		t.Fatalf(
			"failed ACP compaction = calls:%d input:%d, want one call with %d",
			resolver.compactionCalls,
			resolver.compactionInputTokens,
			wantEstimate,
		)
	}
	if resolver.compactionUserID != "account-user" {
		t.Fatalf("failed ACP compaction principal = %q, want account user", resolver.compactionUserID)
	}
}

func TestHandleReplyWithAgent_ACPDiscussSkipsRuntimeForPassiveMessage(t *testing.T) {
	// Passive group chatter that does not address the bot must NOT spin up the
	// external ACP runtime, but the consumed cursor must still advance so the
	// same batch is not re-evaluated and stays covered as context next turn.
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			MentionsMe:   false,
			RepliesToMe:  false,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">just chatting amongst ourselves</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	runtime := &fakeDiscussRuntimeStreamer{}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RuntimeType: sessionpkg.RuntimeACPAgent,
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline:        NewPipeline(RenderParams{}),
		Resolver:        resolver,
		RuntimeStreamer: runtime,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot-1",
			SessionID:        "sess-1",
			ConversationType: channel.ConversationTypeGroup,
		},
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want 0 for a passive (unmentioned) group message", runtime.calls)
	}
	if resolver.compactionCalls != 0 {
		t.Fatalf("passive ACP compaction calls = %d, want 0", resolver.compactionCalls)
	}
	if fakeAgent.lastConfig != nil {
		t.Fatal("ordinary agent should not be invoked for ACP discuss runtime")
	}
	if sess.lastProcessedMs != 200 {
		t.Fatalf("lastProcessedMs = %d, want 200 (cursor advanced on silent path)", sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgent_ACPDiscussRepliesInDirectConversation(t *testing.T) {
	// A direct/1:1 conversation is always addressed, so a DM discuss-ACP session
	// must start the runtime even without an explicit @-mention or reply-to.
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			MentionsMe:   false,
			RepliesToMe:  false,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">hey, can you look at this?</message>`}},
		},
	}
	fakeAgent := &fakeDiscussStreamer{}
	runtime := &fakeDiscussRuntimeStreamer{}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RuntimeType: sessionpkg.RuntimeACPAgent,
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline:        NewPipeline(RenderParams{}),
		Resolver:        resolver,
		RuntimeStreamer: runtime,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot-1",
			SessionID:        "sess-1",
			ConversationType: channel.ConversationTypePrivate,
		},
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if runtime.calls != 1 {
		t.Fatalf("runtime calls = %d, want 1 for a direct (1:1) message even without a mention", runtime.calls)
	}
	if sess.lastProcessedMs != 200 {
		t.Fatalf("lastProcessedMs = %d, want 200 (cursor advanced after direct reply)", sess.lastProcessedMs)
	}
	// A direct conversation is "addressed", so the late-binding prompt must nudge
	// the agent to respond.
	if !strings.Contains(runtime.lastReq.Query, "addressed directly") {
		t.Fatalf("direct-conversation prompt missing the addressed-directly nudge: %q", runtime.lastReq.Query)
	}
}

func TestAnchorFromTRs(t *testing.T) {
	t.Parallel()

	if got := anchorFromTRs(nil); got != 0 {
		t.Fatalf("empty TRs anchor = %d, want 0", got)
	}
	got := anchorFromTRs([]TurnResponseEntry{
		{RequestedAtMs: 100},
		{RequestedAtMs: 500},
		{RequestedAtMs: 300},
	})
	if got != 500 {
		t.Fatalf("anchor = %d, want 500", got)
	}
}

func TestLatestRCEventAtMs(t *testing.T) {
	t.Parallel()

	if got := latestRCEventAtMs(nil); got != 0 {
		t.Fatalf("empty RC = %d, want 0", got)
	}
	got := latestRCEventAtMs(RenderedContext{
		{ReceivedAtMs: 100},
		{ReceivedAtMs: 900},
		{ReceivedAtMs: 500, LastEventAtMs: 1_100, IsMyself: true},
	})
	if got != 1_100 {
		t.Fatalf("latest = %d, want 1100", got)
	}
}

// TestHandleReplyWithAgent_ColdStartAnchoredByTR simulates idle-timeout
// restart: the session's in-memory lastProcessedMs is 0, but RC replay has
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
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})

	sess := &discussSession{
		config:          DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedMs: 0,
	}

	// Simulate the cursor after a previously answered round.
	sess.lastProcessedMs = 200

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig != nil {
		t.Fatal("agent must not be invoked when all RC segments predate lastProcessedMs")
	}
}

// TestHandleReplyWithAgent_CursorAdvancesToRCNotWallClock ensures that after
// a turn we set lastProcessedMs to the max ReceivedAtMs actually consumed in
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
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})
	sess := &discussSession{
		config:          DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedMs: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be invoked")
	}
	if sess.lastProcessedMs != 777 {
		t.Fatalf("lastProcessedMs = %d, want 777 (max RC ReceivedAtMs)", sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgent_UsesPersistedDiscussCursor(t *testing.T) {
	store := &fakeDiscussCursorStore{cursor: 500}
	fakeAgent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline:    NewPipeline(RenderParams{}),
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
	if sess.lastProcessedMs != 500 {
		t.Fatalf("lastProcessedMs = %d, want persisted cursor 500", sess.lastProcessedMs)
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{
		{ReceivedAtMs: 700, Content: []RenderedContentPiece{{Type: "text", Text: `<message id="new">new</message>`}}},
	}, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be invoked for RC past persisted cursor")
	}
	if store.upsertCursor != 700 {
		t.Fatalf("persisted cursor = %d, want 700", store.upsertCursor)
	}
	if store.upsertScope != "route:route-1" {
		t.Fatalf("scope = %q, want route:route-1", store.upsertScope)
	}
	if store.upsertRouteID != "route-1" {
		t.Fatalf("route id = %q, want route-1", store.upsertRouteID)
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
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})
	sess := &discussSession{
		config:          DiscussSessionConfig{BotID: "b", SessionID: "s"},
		lastProcessedMs: 0,
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
	calls     int
	lastReq   conversation.ChatRequest
	streamErr error
}

func (f *fakeDiscussRuntimeStreamer) StreamChat(_ context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	f.calls++
	f.lastReq = req
	chunks := make(chan conversation.StreamChunk, 1)
	errs := make(chan error, 1)
	if f.streamErr != nil {
		errs <- f.streamErr
	} else {
		chunks <- conversation.StreamChunk(`{"type":"agent_end"}`)
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
	artifactErr           error
	compactionCalls       int
	compactionInputTokens int
	compactionBudget      int
	compactionUserID      string
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
	return ContextHistoryProjection{
		TurnResponses:          f.turnResponses,
		CompactionArtifacts:    f.artifacts,
		LatestTurnResponseAtMs: anchorFromTRs(f.turnResponses),
	}, f.artifactErr
}

func (f *fakeRunConfigResolver) MaybeCompactSession(_ context.Context, _, _, userID string, inputTokens, contextTokenBudget int) {
	f.compactionCalls++
	f.compactionInputTokens = inputTokens
	f.compactionBudget = contextTokenBudget
	f.compactionUserID = userID
}

func (*fakeRunConfigResolver) StoreRound(_ context.Context, _, _, _, _ string, _ []sdk.Message, _ string) error {
	return nil
}

type fakeDiscussCursorStore struct {
	cursor        int64
	upsertCursor  int64
	upsertScope   string
	upsertRouteID string
}

func (f *fakeDiscussCursorStore) GetDiscussConsumedCursor(_ context.Context, _, _ string) (int64, error) {
	return f.cursor, nil
}

func (f *fakeDiscussCursorStore) UpsertDiscussConsumedCursor(_ context.Context, _, scopeKey, routeID, _ string, cursor int64) error {
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
