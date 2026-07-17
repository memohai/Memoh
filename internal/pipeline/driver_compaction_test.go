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
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestHandleReplyWithAgentConsumesCompactionArtifacts(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{MessageID: "old", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "old rendered context"}}},
		{MessageID: "new", ReceivedAtMs: 300, Content: []RenderedContentPiece{{Type: "text", Text: "new rendered context"}}},
	}
	resolver := &fakeRunConfigResolver{
		artifacts: []CompactionArtifact{{
			ID:            "artifact-a",
			Summary:       "condensed old context",
			AnchorStartMs: 100,
			Sources: []CompactionSource{{
				Ref: contextfrag.ContextRef{
					Namespace:   "bot_history_message",
					ID:          "row-old",
					Version:     1,
					HashAlgo:    contextfrag.HashAlgoSHA256,
					HashScope:   contextfrag.HashScopeSourcePayload,
					ContentHash: "source-hash",
					Schema:      contextfrag.SchemaContextRef,
					Durability:  contextfrag.RefDurable,
				},
				ExternalMessageID: "old",
				CreatedAtMs:       200,
			}},
		}},
	}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig == nil {
		t.Fatal("expected agent call")
	}
	joined := sdkMessageText(agent.lastConfig.Messages)
	if strings.Contains(joined, "old rendered context") {
		t.Fatalf("covered RC replay reached model: %s", joined)
	}
	if !strings.Contains(joined, "<summary>\ncondensed old context\n</summary>") ||
		!strings.Contains(joined, "new rendered context") {
		t.Fatalf("composed context lost summary or new RC: %s", joined)
	}
	if len(agent.lastConfig.ContextManifest.CoverageTrace) != 1 {
		t.Fatalf("discuss manifest coverage traces = %d, want 1: %#v", len(agent.lastConfig.ContextManifest.CoverageTrace), agent.lastConfig.ContextManifest)
	}
	foundSummary := false
	for _, frag := range agent.lastConfig.ContextFrags {
		if frag.Kind != contextfrag.KindConversationSummary {
			continue
		}
		foundSummary = true
		if frag.Ref.ID != "artifact-a" || frag.Coverage == nil || len(frag.Coverage.CoveredRefs) != 1 || frag.Coverage.CoveredRefs[0].ID != "row-old" {
			t.Fatalf("discuss summary frag lost artifact identity: %#v", frag)
		}
	}
	if !foundSummary {
		t.Fatal("discuss context has no typed summary fragment")
	}
}

func TestHandleReplyWithAgentRetainsExactTriggerCoveredByCompactionArtifact(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:       "exact",
			ReceivedAtMs:    100,
			LastEventCursor: 10,
			Content:         []RenderedContentPiece{{Type: "text", Text: "exact delayed trigger"}},
		},
		{
			MessageID:       "unrelated",
			ReceivedAtMs:    200,
			LastEventCursor: 20,
			Content:         []RenderedContentPiece{{Type: "text", Text: "unrelated covered replay"}},
		},
	}
	resolver := &fakeRunConfigResolver{artifacts: []CompactionArtifact{{
		ID:      "artifact-a",
		Summary: "condensed earlier context",
		Sources: []CompactionSource{
			{ExternalMessageID: "exact", CreatedAtMs: 300, EventCursor: 10},
			{ExternalMessageID: "unrelated", CreatedAtMs: 300, EventCursor: 20},
		},
	}}}
	agent := &fakeDiscussStreamer{}
	delivery := DiscussEventDelivery{EventID: "event-10", EventCursor: 10}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "bot", SessionID: "session", EventDelivery: &delivery},
		lastProcessedCursor: 20,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig == nil {
		t.Fatal("exact trigger covered by an artifact did not call the agent")
	}
	joined := sdkMessageText(agent.lastConfig.Messages)
	if !strings.Contains(joined, "<summary>\ncondensed earlier context\n</summary>") ||
		!strings.Contains(joined, "exact delayed trigger") {
		t.Fatalf("context lost artifact summary or exact trigger: %s", joined)
	}
	if strings.Contains(joined, "unrelated covered replay") {
		t.Fatalf("artifact-covered unrelated replay reached the model: %s", joined)
	}
}

func TestHandleReplyWithAgentConsumesCoveredReplayWithoutCallingModel(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{{
		MessageID:    "covered",
		ReceivedAtMs: 100,
		Content:      []RenderedContentPiece{{Type: "text", Text: "covered rendered context"}},
	}}
	resolver := &fakeRunConfigResolver{artifacts: []CompactionArtifact{{
		ID:      "artifact-a",
		Summary: "covered context",
		Sources: []CompactionSource{{ExternalMessageID: "covered", CreatedAtMs: 200}},
	}}}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig != nil {
		t.Fatal("covered replay must not trigger a model call")
	}
	if sess.lastProcessedCursor != 100 {
		t.Fatalf("consumed cursor = %d, want 100", sess.lastProcessedCursor)
	}
}

func TestHandleReplyWithAgentAnchorsColdStartFromProjectedTurnResponses(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{turnResponses: []TurnResponseEntry{{RequestedAtMs: 200, Role: "assistant", Content: "prior response"}}}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{{MessageID: "old", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "already answered"}}}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig != nil {
		t.Fatal("cold start replay older than projected turn response must not call the model")
	}
	if sess.lastProcessedCursor != 100 {
		t.Fatalf("cold start cursor = %d, want rendered event cursor 100 before projected response", sess.lastProcessedCursor)
	}
}

func TestHandleReplyWithAgentUsesPersistedCursorBeforeTurnResponseFallback(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{turnResponses: []TurnResponseEntry{{
		RequestedAtMs: 200,
		Role:          "assistant",
		Content:       "prior response",
	}}}
	store := &fakeDiscussCursorStore{cursor: 10}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, CursorStore: store})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{
		{MessageID: "old", ReceivedAtMs: 100, LastEventCursor: 10, Content: []RenderedContentPiece{{Type: "text", Text: "already answered"}}},
		{MessageID: "delayed", ReceivedAtMs: 150, LastEventCursor: 20, Content: []RenderedContentPiece{{Type: "text", Text: "arrived after prior response"}}},
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig == nil {
		t.Fatal("event past the durable cursor was treated as covered by an earlier turn response")
	}
	if got := sdkMessageText(agent.lastConfig.Messages); !strings.Contains(got, "arrived after prior response") {
		t.Fatalf("delayed event missing from model context: %s", got)
	}
	if sess.lastProcessedCursor != 20 {
		t.Fatalf("consumed cursor = %d, want 20", sess.lastProcessedCursor)
	}
}

func TestHandleReplyWithAgentStopsOnArtifactProjectionFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("projection unavailable")
	resolver := &fakeRunConfigResolver{artifactErr: wantErr}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{{MessageID: "new", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "new"}}}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig != nil {
		t.Fatal("projection failure must stop the model call")
	}
	if sess.lastProcessedCursor != 0 {
		t.Fatalf("projection failure advanced cursor to %d", sess.lastProcessedCursor)
	}
}

func TestHandleReplyWithAgentIgnoresCoveredMentionForACPGate(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{MessageID: "covered", ReceivedAtMs: 100, MentionsMe: true, Content: []RenderedContentPiece{{Type: "text", Text: "old mention"}}},
		{MessageID: "new", ReceivedAtMs: 300, Content: []RenderedContentPiece{{Type: "text", Text: "new passive message"}}},
	}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent},
		artifacts: []CompactionArtifact{{
			ID:      "artifact-a",
			Summary: "old mention summarized",
			Sources: []CompactionSource{{ExternalMessageID: "covered", CreatedAtMs: 200}},
		}},
	}
	runtime := &fakeDiscussRuntimeStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: runtime})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:            "bot",
		SessionID:        "session",
		ConversationType: channel.ConversationTypeGroup,
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})

	if runtime.calls != 0 {
		t.Fatalf("covered mention started ACP runtime %d times", runtime.calls)
	}
	if sess.lastProcessedCursor != 300 {
		t.Fatalf("passive batch cursor = %d, want 300", sess.lastProcessedCursor)
	}
}

func TestMaybeCompactDiscussContextDoesNotBlockTheSessionLoop(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	resolver := &fakeRunConfigResolver{
		compactionStarted: started,
		compactionBlock:   release,
		compactionDone:    make(chan struct{}, 1),
	}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	returned := make(chan struct{})
	go func() {
		driver.maybeCompactDiscussContext(context.Background(), DiscussSessionConfig{BotID: "bot", SessionID: "session"}, 100, 200)
		close(returned)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("compaction resolver did not start")
	}
	select {
	case <-returned:
		close(release)
		waitForFakeCompaction(t, resolver)
	case <-time.After(50 * time.Millisecond):
		close(release)
		<-returned
		t.Fatal("discuss compaction blocked the session loop")
	}
}

func TestHandleReplyWithAgentTriggersCompactionFromReportedUsage(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{compactionDone: make(chan struct{}, 1)}
	agent := &fakeDiscussStreamer{endUsage: []byte(`{"inputTokens":321}`)}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:             "bot",
		SessionID:         "session",
		UserID:            "account-user",
		ChannelIdentityID: "channel-identity",
	}}
	rc := RenderedContext{{MessageID: "new", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "new"}}}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)
	waitForFakeCompaction(t, resolver)

	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != 321 {
		t.Fatalf("compaction trigger = calls:%d input:%d, want one call with 321", resolver.compactionCalls, resolver.compactionInputTokens)
	}
	if resolver.compactionUserID != "account-user" {
		t.Fatalf("compaction principal = %q, want account user", resolver.compactionUserID)
	}
}

func TestHandleReplyWithAgentFallsBackToComposedEstimateForCompaction(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{compactionDone: make(chan struct{}, 1)}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{{MessageID: "new", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "fallback estimate"}}}}
	want := ComposeContext(rc, nil).EstimatedTokens

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)
	waitForFakeCompaction(t, resolver)

	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != want {
		t.Fatalf("compaction fallback = calls:%d input:%d, want one call with %d", resolver.compactionCalls, resolver.compactionInputTokens, want)
	}
}

func TestHandleReplyWithAgentTriggersCompactionForACPDiscuss(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{
		resolveResult:  ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent},
		compactionDone: make(chan struct{}, 1),
	}
	runtime := &fakeDiscussRuntimeStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: runtime})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:            "bot",
		SessionID:        "session",
		UserID:           "account-user",
		ConversationType: channel.ConversationTypeGroup,
	}}
	rc := RenderedContext{{
		MessageID:    "new",
		ReceivedAtMs: 100,
		MentionsMe:   true,
		Content:      []RenderedContentPiece{{Type: "text", Text: "compact ACP discuss context"}},
	}}
	want := ComposeContext(rc, nil).EstimatedTokens

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})
	waitForFakeCompaction(t, resolver)

	if runtime.calls != 1 {
		t.Fatalf("ACP runtime calls = %d, want 1", runtime.calls)
	}
	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != want {
		t.Fatalf(
			"ACP compaction trigger = calls:%d input:%d, want one call with %d",
			resolver.compactionCalls,
			resolver.compactionInputTokens,
			want,
		)
	}
	if resolver.compactionUserID != "account-user" {
		t.Fatalf("ACP compaction principal = %q, want account user", resolver.compactionUserID)
	}
}

func TestHandleReplyWithAgentDoesNotConsumeContextAfterStreamError(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{compactionDone: make(chan struct{}, 1)}
	agent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{
		Type:  agentpkg.EventError,
		Error: "provider failed",
	}}}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{{MessageID: "new", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "retry me"}}}}
	wantEstimate := ComposeContext(rc, nil).EstimatedTokens

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)
	waitForFakeCompaction(t, resolver)

	if sess.lastProcessedCursor != 0 {
		t.Fatalf("failed stream consumed cursor %d", sess.lastProcessedCursor)
	}
	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != wantEstimate {
		t.Fatalf(
			"failed stream compaction = calls:%d input:%d, want one call with %d",
			resolver.compactionCalls,
			resolver.compactionInputTokens,
			wantEstimate,
		)
	}
}

func TestHandleReplyWithAgentConsumesContextAfterTerminalAbort(t *testing.T) {
	t.Parallel()

	agent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{
		{Type: agentpkg.EventError, Error: "loop aborted"},
		{Type: agentpkg.EventAgentAbort},
	}}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: &fakeRunConfigResolver{}})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{{MessageID: "new", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "handled before abort"}}}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if sess.lastProcessedCursor != 100 {
		t.Fatalf("terminal abort cursor = %d, want 100", sess.lastProcessedCursor)
	}
}

func TestCoverageSensitiveGatesUseLatestExternalMutation(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{ReceivedAtMs: 100, LastEventAtMs: 500, MentionsMe: true, ImageRefs: []ImageAttachmentRef{{ContentHash: "external"}}},
		{ReceivedAtMs: 200, LastEventAtMs: 600, IsSelfSent: true, MentionsMe: true, ImageRefs: []ImageAttachmentRef{{ContentHash: "self"}}},
	}
	if !wasRecentlyMentioned(rc, 400) {
		t.Fatal("external post-compaction mutation should retain its mention gate")
	}
	refs := extractNewImageRefs(rc, 400)
	if len(refs) != 1 || refs[0].ContentHash != "external" {
		t.Fatalf("external image refs = %#v, want only external mutation", refs)
	}
	if wasRecentlyMentioned(rc[1:], 400) {
		t.Fatal("self-sent mutation must not activate mention gate")
	}
}

func sdkMessageText(messages []sdk.Message) string {
	var text strings.Builder
	for _, message := range messages {
		for _, part := range message.Content {
			if value, ok := part.(sdk.TextPart); ok {
				text.WriteString(value.Text)
				text.WriteByte('\n')
			}
		}
	}
	return text.String()
}

func TestHandleReplyWithAgentTrimsDiscussContextToModelBudget(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{MessageID: "old", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: strings.Repeat("old rendered context ", 50)}}},
		{MessageID: "new", ReceivedAtMs: 300, Content: []RenderedContentPiece{{Type: "text", Text: "new rendered context"}}},
	}
	trimmed := []ContextMessage{{Role: "user", Content: "trimmed discuss context"}}
	resolver := &fakeRunConfigResolver{
		resolveResult:  ResolveRunConfigResult{ContextTokenBudget: 64},
		compactionDone: make(chan struct{}, 1),
		trimFn: func(messages []ContextMessage, _ int, _ int64) ([]ContextMessage, int) {
			if len(messages) == 0 {
				t.Error("trim received no messages")
			}
			return trimmed, 42
		},
	}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)
	waitForFakeCompaction(t, resolver)

	if agent.lastConfig == nil {
		t.Fatal("expected agent call")
	}
	if resolver.trimCalls != 1 || resolver.trimBudget != 64 {
		t.Fatalf("trim calls = %d budget = %d, want 1 call with budget 64", resolver.trimCalls, resolver.trimBudget)
	}
	joined := sdkMessageText(agent.lastConfig.Messages)
	if !strings.Contains(joined, "trimmed discuss context") || strings.Contains(joined, "old rendered context") {
		t.Fatalf("model did not receive trimmed context: %s", joined)
	}
	if resolver.compactionCalls != 1 || resolver.compactionBudget != 64 || resolver.compactionInputTokens != 42 {
		t.Fatalf("compaction trigger = (calls=%d, budget=%d, tokens=%d), want (1, 64, 42)",
			resolver.compactionCalls, resolver.compactionBudget, resolver.compactionInputTokens)
	}
}
