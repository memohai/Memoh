package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestDiscussACPFullContextPromptPreservesStructuredToolHistory(t *testing.T) {
	t.Parallel()

	prompt := discussACPFullContextPrompt([]ContextMessage{
		{
			Role:       "assistant",
			Content:    "[tool call: exec]",
			RawContent: json.RawMessage(`[{"type":"tool_call","tool_call_id":"call-1","tool_name":"exec","input":{"cmd":"unique-command"}}]`),
		},
		{
			Role:       "tool",
			Content:    "[tool result: exec]",
			RawContent: json.RawMessage(`[{"type":"tool_result","tool_call_id":"call-1","tool_name":"exec","result":"unique-result"}]`),
		},
	}, 0, "")

	for _, want := range []string{"unique-command", "unique-result"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("ACP prompt lost structured tool value %q: %s", want, prompt)
		}
	}
}

func TestDiscussACPFullContextPromptTargetsUserMessageWhoseEditTriggeredRun(t *testing.T) {
	t.Parallel()

	prompt := discussACPFullContextPrompt([]ContextMessage{
		{Role: "user", Content: "edited old question", LatestExternalEventCursor: 300},
		{Role: "assistant", Content: "previous answer"},
		{Role: "user", Content: "newer passive message", LatestExternalEventCursor: 200},
	}, 250, "")

	if !strings.Contains(prompt, "[user; current-trigger]\nedited old question") {
		t.Fatalf("ACP prompt did not mark the edited message as the current trigger: %q", prompt)
	}
	if strings.Contains(prompt, "[user; current-trigger]\nnewer passive message") {
		t.Fatalf("ACP prompt targeted the positionally latest user instead of the triggering edit: %q", prompt)
	}
	if !strings.Contains(prompt, "marked as current-trigger") {
		t.Fatalf("ACP prompt did not direct the runtime to the trigger marker: %q", prompt)
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
		Resolver:        resolver,
		RuntimeStreamer: runtime,
	})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:                  "bot-1",
			SessionID:              "sess-1",
			RouteID:                "route-1",
			UserID:                 "user-1",
			ChannelIdentityID:      "acct-1",
			CurrentPlatform:        "telegram",
			ReplyTarget:            "chat-1",
			ConversationType:       "group",
			ConversationName:       "project room",
			SessionToken:           "Bearer owner-token",
			ChatToken:              "chat-token",
			ToolHTTPURL:            "http://example.test/bots/bot-1/tools",
			PersistedUserMessageID: "user-message-1",
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
	if runtime.lastReq.UserID != "user-1" || runtime.lastReq.ConversationName != "project room" {
		t.Fatalf("runtime principal = user %q conversation %q", runtime.lastReq.UserID, runtime.lastReq.ConversationName)
	}
	if runtime.lastReq.RouteID != "route-1" || runtime.lastReq.ChatToken != "chat-token" || runtime.lastReq.Token != "Bearer owner-token" {
		t.Fatalf("runtime context = route %q chat token %q token %q", runtime.lastReq.RouteID, runtime.lastReq.ChatToken, runtime.lastReq.Token)
	}
	if runtime.lastReq.ToolHTTPURL != "http://example.test/bots/bot-1/tools" {
		t.Fatalf("ToolHTTPURL = %q", runtime.lastReq.ToolHTTPURL)
	}
	if runtime.lastReq.PersistedUserMessageID != "user-message-1" {
		t.Fatalf("persisted user message id = %q, want user-message-1", runtime.lastReq.PersistedUserMessageID)
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
	if runtime.lastReq.DiscussCursorScope != "route:route-1" || runtime.lastReq.DiscussConsumedEventCursor != 200 {
		t.Fatalf("runtime discuss cursor = %q/%d, want route scope/200", runtime.lastReq.DiscussCursorScope, runtime.lastReq.DiscussConsumedEventCursor)
	}
	if sess.lastProcessedCursor != 200 {
		t.Fatalf("lastProcessedCursor = %d, want 200", sess.lastProcessedCursor)
	}
}

func TestNotifyRCRefreshesExistingDiscussSessionConfig(t *testing.T) {
	driver := NewDiscussDriver(DiscussDriverDeps{})
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

func TestHandleReplyWithAgentConfigKeepsConfigPairedWithRenderedContext(t *testing.T) {
	t.Parallel()

	usedIdentity := make(chan string, 1)
	resolver := &fakeRunConfigResolver{
		resolveFn: func(_, _, channelIdentityID, _, _, _, _ string) (ResolveRunConfigResult, error) {
			usedIdentity <- channelIdentityID
			return ResolveRunConfigResult{}, nil
		},
	}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:             "bot",
		SessionID:         "session",
		ChannelIdentityID: "newer-notification",
	}}
	rc := RenderedContext{{
		ReceivedAtMs: 100,
		Content:      []RenderedContentPiece{{Type: "text", Text: "paired event"}},
	}}

	driver.handleReplyWithAgentConfig(
		context.Background(),
		sess,
		rc,
		DiscussSessionConfig{BotID: "bot", SessionID: "session", ChannelIdentityID: "paired-notification"},
		driver.logger,
		&fakeDiscussStreamer{},
	)

	if got := <-usedIdentity; got != "paired-notification" {
		t.Fatalf("resolver identity = %q, want config paired with rendered context", got)
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
	if sess.lastProcessedCursor != 0 {
		t.Fatalf("lastProcessedCursor = %d, want 0 when ACP runtime streamer is missing", sess.lastProcessedCursor)
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
		compactionDone: make(chan struct{}, 1),
	}
	driver := NewDiscussDriver(DiscussDriverDeps{
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
	waitForFakeCompaction(t, resolver)

	if runtime.calls != 1 {
		t.Fatalf("runtime calls = %d, want 1", runtime.calls)
	}
	if sess.lastProcessedCursor != 0 {
		t.Fatalf("lastProcessedCursor = %d, want 0 when ACP runtime stream fails", sess.lastProcessedCursor)
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
	if sess.lastProcessedCursor != 200 {
		t.Fatalf("lastProcessedCursor = %d, want 200 (cursor advanced on silent path)", sess.lastProcessedCursor)
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
	if sess.lastProcessedCursor != 200 {
		t.Fatalf("lastProcessedCursor = %d, want 200 (cursor advanced after direct reply)", sess.lastProcessedCursor)
	}
	// A direct conversation is "addressed", so the late-binding prompt must nudge
	// the agent to respond.
	if !strings.Contains(runtime.lastReq.Query, "addressed directly") {
		t.Fatalf("direct-conversation prompt missing the addressed-directly nudge: %q", runtime.lastReq.Query)
	}
}
