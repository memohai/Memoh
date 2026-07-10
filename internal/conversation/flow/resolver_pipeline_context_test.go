package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestBuildPipelineContextConsumesOrderedArtifactsWithIndependentManifestCoverage(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifactA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		artifactB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	base := time.UnixMilli(1_000).UTC()
	coveredA := pipelineHistoryMessage(t, "row-a", botID, sessionID, "external-a", base.Add(100*time.Millisecond), "user", "covered a")
	coveredB := pipelineHistoryMessage(t, "row-b", botID, sessionID, "external-b", base.Add(1_100*time.Millisecond), "user", "covered b")
	assistantA := pipelineHistoryMessage(t, "assistant-a", botID, sessionID, "", base.Add(300*time.Millisecond), "assistant", "covered assistant a")
	assistantB := pipelineHistoryMessage(t, "assistant-b", botID, sessionID, "", base.Add(1_300*time.Millisecond), "assistant", "covered assistant b")
	newAssistant := pipelineHistoryMessage(t, "assistant-new", botID, sessionID, "", base.Add(2_300*time.Millisecond), "assistant", "new assistant")
	coveredA.CompactID = artifactA
	assistantA.CompactID = artifactA
	coveredB.CompactID = artifactB
	assistantB.CompactID = artifactB
	rows := []messagepkg.Message{coveredA, assistantA, coveredB, assistantB, newAssistant}

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	for _, event := range []pipelinepkg.MessageEvent{
		pipelineMessageEvent(sessionID, "before", 500, "before summaries"),
		pipelineMessageEvent(sessionID, "external-a", 1_000, "covered a"),
		pipelineMessageEvent(sessionID, "between", 1_800, "between summaries"),
		pipelineMessageEvent(sessionID, "external-b", 2_000, "covered b"),
		pipelineMessageEvent(sessionID, "after", 3_000, "after summaries"),
	} {
		p.PushEvent(sessionID, event)
	}
	queries := &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{
		pipelineArtifactRow(t, artifactB, botID, sessionID, "same summary text", []messagepkg.Message{coveredB, assistantB}, base.Add(2*time.Minute)),
		pipelineArtifactRow(t, artifactA, botID, sessionID, "same summary text", []messagepkg.Message{coveredA, assistantA}, base.Add(time.Minute)),
	}}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{rows: rows},
		queries:        queries,
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		BotID:            botID,
		ChatID:           "chat-1",
		SessionID:        sessionID,
		ConversationType: "group",
		ConversationName: "room",
		ReplyTarget:      "target",
	}, 0)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	assertPipelineContextOrder(t, built.Messages, []string{
		"before summaries",
		"<summary>\nsame summary text\n</summary>",
		"between summaries",
		"<summary>\nsame summary text\n</summary>",
		"after summaries",
		"new assistant",
	})
	if got, want := pipelineSummaryIDs(built.HistoryRecords), []string{artifactA, artifactB}; !equalStrings(got, want) {
		t.Fatalf("retained summary identities = %#v, want %#v", got, want)
	}
	frags := historyContextFragsForMessages(built.Messages, built.HistoryRecords)
	if len(frags) != 2 {
		t.Fatalf("summary frags = %d, want 2: %#v", len(frags), frags)
	}
	for i, wantID := range []string{artifactA, artifactB} {
		if frags[i].Ref.ID != wantID || frags[i].Kind != contextfrag.KindConversationSummary {
			t.Fatalf("summary frag %d = %#v, want artifact %s", i, frags[i], wantID)
		}
		if frags[i].Coverage == nil || len(frags[i].Coverage.CoveredRefs) != 2 {
			t.Fatalf("summary frag %d lost independent coverage: %#v", i, frags[i])
		}
	}
	manifest := contextfrag.BuildManifest(frags)
	if len(manifest.CoverageTrace) != 2 {
		t.Fatalf("manifest coverage traces = %d, want 2: %#v", len(manifest.CoverageTrace), manifest)
	}
}

func TestBuildPipelineContextPropagatesArtifactProjectionFailure(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	wantErr := errors.New("projection unavailable")
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.ReplaySession(sessionID, nil)
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{},
		queries:        &recordingCompactionLogQueries{listErr: wantErr},
	}

	_, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: sessionID,
	}, 0)
	if !errors.Is(err, wantErr) {
		t.Fatalf("buildPipelineContext() error = %v, want %v", err, wantErr)
	}
}

func TestBuildPipelineContextKeepsHistoryAndCurrentQueryWhenRenderedContextIsStale(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "external-old", 1_000, "old rendered message"))
	assistant := pipelineHistoryMessage(t, "assistant", "", sessionID, "", time.UnixMilli(1_500).UTC(), "assistant", "retained assistant")
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{rows: []messagepkg.Message{assistant}},
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "external-current",
		Query:             "current user query",
	}, 0)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	assertPipelineContextOrder(t, built.Messages, []string{
		"old rendered message",
		"retained assistant",
		"current user query",
	})
}

func TestTrimComposedPipelineMessagesKeepsRetainedArtifactIdentity(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	summaryA := historyfrag.SummaryRecord("artifact-a", "same summary text", []contextfrag.ContextRef{{Namespace: "source", ID: "a"}}, scope)
	summaryB := historyfrag.SummaryRecord("artifact-b", "same summary text", []contextfrag.ContextRef{{Namespace: "source", ID: "b"}}, scope)
	composed := &pipelinepkg.ComposeContextResult{Messages: []pipelinepkg.ContextMessage{
		{Role: "user", Content: summaryA.ModelMessage.TextContent(), CompactionArtifactID: "artifact-a"},
		{Role: "user", Content: strings.Repeat("old raw context", 200)},
		{Role: "user", Content: summaryB.ModelMessage.TextContent(), CompactionArtifactID: "artifact-b"},
		{Role: "user", Content: "latest raw context"},
	}}
	entries := composedPipelineMessages(composed, []historyfrag.HistoryRecord{summaryA, summaryB})
	budget := estimateMessageTokens(summaryB.ModelMessage) + estimateMessageTokens(conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("latest raw context"),
	})

	built := trimComposedPipelineMessages(nil, entries, budget)
	if got, want := pipelineSummaryIDs(built.HistoryRecords), []string{"artifact-b"}; !equalStrings(got, want) {
		t.Fatalf("retained summary identities = %#v, want %#v", got, want)
	}
	frags := historyContextFragsForMessages(built.Messages, built.HistoryRecords)
	if len(frags) != 1 || frags[0].Ref.ID != "artifact-b" {
		t.Fatalf("retained summary frag = %#v, want artifact-b", frags)
	}
}

type pipelineContextMessageService struct {
	messagepkg.Service
	rows []messagepkg.Message
	err  error
}

func (s *pipelineContextMessageService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return s.rows, s.err
}

func pipelineMessageEvent(sessionID, messageID string, receivedAtMs int64, text string) pipelinepkg.MessageEvent {
	return pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    messageID,
		ReceivedAtMs: receivedAtMs,
		TimestampSec: receivedAtMs / 1_000,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: text}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	}
}

func assertPipelineContextOrder(t *testing.T, messages []conversation.ModelMessage, want []string) {
	t.Helper()
	if len(messages) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(messages), len(want), modelMessageTexts(messages))
	}
	for i := range want {
		got := messages[i].TextContent()
		if i == 0 || i == 2 || i == 4 {
			if !containsText(got, want[i]) {
				t.Fatalf("message %d = %q, want text %q", i, got, want[i])
			}
			continue
		}
		if got != want[i] {
			t.Fatalf("message %d = %q, want %q", i, got, want[i])
		}
	}
}

func containsText(value, needle string) bool {
	return len(needle) == 0 || len(value) >= len(needle) && strings.Contains(value, needle)
}

func modelMessageTexts(messages []conversation.ModelMessage) []string {
	texts := make([]string, 0, len(messages))
	for _, message := range messages {
		texts = append(texts, message.TextContent())
	}
	return texts
}
