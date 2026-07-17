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
	base := time.Now().UTC().Add(-time.Hour)
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
		pipelineMessageEvent(sessionID, "before", base.Add(-500*time.Millisecond).UnixMilli(), "before summaries"),
		pipelineMessageEvent(sessionID, "external-a", base.UnixMilli(), "covered a"),
		pipelineMessageEvent(sessionID, "between", base.Add(800*time.Millisecond).UnixMilli(), "between summaries"),
		pipelineMessageEvent(sessionID, "external-b", base.Add(time.Second).UnixMilli(), "covered b"),
		pipelineMessageEvent(sessionID, "after", base.Add(2*time.Second).UnixMilli(), "after summaries"),
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
	}, 0, "")
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
	}, 0, "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("buildPipelineContext() error = %v, want %v", err, wantErr)
	}
}

func TestBuildPipelineContextLoadsBoundedRecentHistory(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.ReplaySession(sessionID, nil)
	messages := &pipelineContextMessageService{}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: messages,
	}

	if _, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0, ""); err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if messages.activeSinceCalls != 1 {
		t.Fatalf("active history loads = %d, want 1", messages.activeSinceCalls)
	}
	age := time.Since(messages.since)
	if age < 23*time.Hour+59*time.Minute || age > 24*time.Hour+time.Minute {
		t.Fatalf("history window age = %s, want approximately 24h", age)
	}
}

func TestBuildPipelineContextBoundsRenderedEventsToTurnResponseWindow(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	now := time.Now().UTC()
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID: sessionID, MessageID: "old", ReceivedAtMs: now.Add(-48 * time.Hour).UnixMilli(),
		EventCursor: 10, Content: []pipelinepkg.ContentNode{{Type: "text", Text: "old unpaired user message"}},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID: sessionID, MessageID: "recent", ReceivedAtMs: now.Add(-time.Hour).UnixMilli(),
		EventCursor: 20, Content: []pipelinepkg.ContentNode{{Type: "text", Text: "recent user message"}},
	})
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{},
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0, "")
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	joined := strings.Join(modelMessageTexts(built.Messages), "\n")
	if strings.Contains(joined, "old unpaired user message") {
		t.Fatalf("rendered context escaped the bounded history window: %s", joined)
	}
	if !strings.Contains(joined, "recent user message") {
		t.Fatalf("recent rendered context was dropped: %s", joined)
	}
}

func TestLoadContextHistoryProjectionUsesLatestResponseOutsideRecentWindow(t *testing.T) {
	t.Parallel()

	want := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	internalUser := pipelineHistoryMessage(
		t,
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"",
		want.Add(time.Hour),
		"user",
		"internal continuation",
	)
	internalUser.TurnPosition, internalUser.TurnMessageSequence = 1, 2
	messages := &pipelineContextMessageService{
		latestTurnResponseAt:   want,
		uncoveredTurnResponses: []messagepkg.Message{internalUser},
	}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: messages,
	}

	projection, err := resolver.LoadContextHistoryProjection(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if projection.LatestTurnResponseAtMs != want.UnixMilli() {
		t.Fatalf("latest turn response = %d, want %d", projection.LatestTurnResponseAtMs, want.UnixMilli())
	}
	if len(projection.TurnResponses) != 1 || projection.TurnResponses[0].Role != "user" {
		t.Fatalf("internal user continuation was not projected: %#v", projection.TurnResponses)
	}
	if messages.latestTurnResponseCalls != 1 {
		t.Fatalf("latest turn response lookups = %d, want 1", messages.latestTurnResponseCalls)
	}
}

func TestLoadContextHistoryProjectionBoundsUncompactedTurnResponsesToRecentWindow(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	recentAssistant := pipelineHistoryMessage(
		t,
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"11111111-1111-1111-1111-111111111111",
		sessionID,
		"",
		time.Now().UTC().Add(-time.Hour),
		"assistant",
		"recent response",
	)
	messages := &pipelineContextMessageService{uncoveredTurnResponses: []messagepkg.Message{recentAssistant}}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: messages,
	}

	projection, err := resolver.LoadContextHistoryProjection(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		sessionID,
	)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if len(projection.TurnResponses) != 1 || projection.TurnResponses[0].Content != "recent response" {
		t.Fatalf("recent uncompacted turn responses = %#v", projection.TurnResponses)
	}
	if messages.uncoveredTurnResponseCalls != 1 {
		t.Fatalf("uncovered turn response loads = %d, want 1", messages.uncoveredTurnResponseCalls)
	}
	age := time.Since(messages.turnResponseSince)
	if age < 23*time.Hour+59*time.Minute || age > 24*time.Hour+time.Minute {
		t.Fatalf("turn response window age = %s, want approximately 24h", age)
	}
}

func TestLoadContextHistoryProjectionExcludesTurnResponsesCoveredByActiveArtifact(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		responseID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	response := pipelineHistoryMessage(t, responseID, botID, sessionID, "", time.Now().UTC().Add(-48*time.Hour), "assistant", "covered response")
	queries := &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{
		pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{response}, time.Now().UTC()),
	}}
	messages := &pipelineContextMessageService{rows: []messagepkg.Message{response}}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), messageService: messages, queries: queries}

	projection, err := resolver.LoadContextHistoryProjection(context.Background(), botID, sessionID)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if len(projection.CompactionArtifacts) != 1 || len(projection.TurnResponses) != 0 {
		t.Fatalf("active projection = %#v", projection)
	}
	if got, want := messages.coveredMessageIDs, []string{responseID}; !equalStrings(got, want) {
		t.Fatalf("covered message ids = %#v, want %#v", got, want)
	}
}

func TestLoadContextHistoryProjectionPropagatesTurnResponseReadFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("turn responses unavailable")
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: &pipelineContextMessageService{turnResponseErr: wantErr},
	}

	_, err := resolver.LoadContextHistoryProjection(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("LoadContextHistoryProjection() error = %v, want %v", err, wantErr)
	}
}

func TestBuildPipelineContextKeepsHistoryAndCurrentQueryWhenRenderedContextIsStale(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	now := time.Now().UTC()
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "external-old", now.Add(-time.Minute).UnixMilli(), "old rendered message"))
	assistant := pipelineHistoryMessage(t, "assistant", "", sessionID, "", now.Add(-30*time.Second), "assistant", "retained assistant")
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{rows: []messagepkg.Message{assistant}},
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "external-current",
		Query:             "current user query",
	}, 0, "current user query")
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	assertPipelineContextOrder(t, built.Messages, []string{
		"old rendered message",
		"retained assistant",
		"current user query",
	})
}

func TestBuildPipelineContextKeepsRepeatedCurrentQueryWithoutExternalID(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "old", time.Now().UTC().Add(-time.Minute).UnixMilli(), "yes"))
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), pipeline: p}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID: sessionID,
		Query:     "yes",
	}, 0, "yes")
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 2 || !strings.Contains(built.Messages[0].TextContent(), "yes") || built.Messages[1].TextContent() != "yes" {
		t.Fatalf("repeated current query was not appended: %#v", modelMessageTexts(built.Messages))
	}
}

func TestBuildPipelineContextForceKeepsCurrentRenderedMessagePastBudget(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "current", time.Now().UTC().Add(-time.Minute).UnixMilli(), strings.Repeat("current", 100)))
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), pipeline: p}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "current",
		Query:             strings.Repeat("current", 100),
	}, 1, strings.Repeat("current", 100))
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 2 ||
		!strings.HasPrefix(built.Messages[0].TextContent(), "[System Notice]") ||
		!strings.Contains(built.Messages[1].TextContent(), "current") {
		t.Fatalf("current rendered message was trimmed: %#v", modelMessageTexts(built.Messages))
	}
}

type pipelineContextMessageService struct {
	messagepkg.Service
	rows                       []messagepkg.Message
	err                        error
	activeSinceCalls           int
	since                      time.Time
	latestTurnResponseAt       time.Time
	latestTurnResponseCalls    int
	uncoveredTurnResponses     []messagepkg.Message
	uncoveredTurnResponseCalls int
	turnResponseSince          time.Time
	coveredMessageIDs          []string
	turnResponseErr            error
}

func (s *pipelineContextMessageService) LatestTurnResponseAtBySession(context.Context, string) (time.Time, error) {
	s.latestTurnResponseCalls++
	return s.latestTurnResponseAt, nil
}

func (s *pipelineContextMessageService) ListUncoveredTurnResponsesBySession(_ context.Context, _ string, since time.Time, coveredMessageIDs []string) ([]messagepkg.Message, error) {
	s.uncoveredTurnResponseCalls++
	s.turnResponseSince = since
	s.coveredMessageIDs = append([]string(nil), coveredMessageIDs...)
	if s.turnResponseErr != nil || s.uncoveredTurnResponses != nil {
		return s.uncoveredTurnResponses, s.turnResponseErr
	}
	covered := make(map[string]struct{}, len(coveredMessageIDs))
	for _, id := range coveredMessageIDs {
		covered[id] = struct{}{}
	}
	responses := make([]messagepkg.Message, 0, len(s.rows))
	for _, message := range s.rows {
		if message.Role != "assistant" && message.Role != "tool" &&
			(message.Role != "user" || message.TurnMessageSequence <= 1) {
			continue
		}
		if _, excluded := covered[message.ID]; excluded {
			continue
		}
		responses = append(responses, message)
	}
	return responses, nil
}

func (s *pipelineContextMessageService) ListActiveSinceBySession(_ context.Context, _ string, since time.Time) ([]messagepkg.Message, error) {
	s.activeSinceCalls++
	s.since = since
	return s.rows, s.err
}

func pipelineMessageEvent(sessionID, messageID string, receivedAtMs int64, text string) pipelinepkg.MessageEvent {
	return pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    messageID,
		ReceivedAtMs: receivedAtMs,
		EventCursor:  time.Now().UnixMilli(),
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
