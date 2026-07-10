package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

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
	if sess.lastProcessedMs != 100 {
		t.Fatalf("consumed cursor = %d, want 100", sess.lastProcessedMs)
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
	if sess.lastProcessedMs != 200 {
		t.Fatalf("cold start cursor = %d, want projected turn response anchor 200", sess.lastProcessedMs)
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
	if sess.lastProcessedMs != 0 {
		t.Fatalf("projection failure advanced cursor to %d", sess.lastProcessedMs)
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
	if sess.lastProcessedMs != 300 {
		t.Fatalf("passive batch cursor = %d, want 300", sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgentTriggersCompactionFromReportedUsage(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{}
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

	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != 321 {
		t.Fatalf("compaction trigger = calls:%d input:%d, want one call with 321", resolver.compactionCalls, resolver.compactionInputTokens)
	}
	if resolver.compactionUserID != "account-user" {
		t.Fatalf("compaction principal = %q, want account user", resolver.compactionUserID)
	}
}

func TestHandleReplyWithAgentFallsBackToComposedEstimateForCompaction(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{BotID: "bot", SessionID: "session"}}
	rc := RenderedContext{{MessageID: "new", ReceivedAtMs: 100, Content: []RenderedContentPiece{{Type: "text", Text: "fallback estimate"}}}}
	want := ComposeContext(rc, nil).EstimatedTokens

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if resolver.compactionCalls != 1 || resolver.compactionInputTokens != want {
		t.Fatalf("compaction fallback = calls:%d input:%d, want one call with %d", resolver.compactionCalls, resolver.compactionInputTokens, want)
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
