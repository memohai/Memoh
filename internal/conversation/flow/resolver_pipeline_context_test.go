package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/contextassembly"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
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

	if _, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0); err != nil {
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

func TestLoadContextHistoryProjectionUsesLatestResponseOutsideRecentWindow(t *testing.T) {
	t.Parallel()

	want := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	messages := &pipelineContextMessageService{latestTurnResponseAt: want}
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
	if messages.latestTurnResponseCalls != 1 {
		t.Fatalf("latest turn response lookups = %d, want 1", messages.latestTurnResponseCalls)
	}
}

func TestLoadContextHistoryProjectionKeepsOldUncompactedTurnResponses(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	oldAssistant := pipelineHistoryMessage(
		t,
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"11111111-1111-1111-1111-111111111111",
		sessionID,
		"",
		time.Now().UTC().Add(-48*time.Hour),
		"assistant",
		"old response",
	)
	messages := &pipelineContextMessageService{uncoveredTurnResponses: []messagepkg.Message{oldAssistant}}
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
	if len(projection.TurnResponses) != 1 || projection.TurnResponses[0].Content != "old response" {
		t.Fatalf("old uncompacted turn responses = %#v", projection.TurnResponses)
	}
	if messages.uncoveredTurnResponseCalls != 1 {
		t.Fatalf("uncovered turn response loads = %d, want 1", messages.uncoveredTurnResponseCalls)
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

func TestBuildPipelineContextKeepsRepeatedCurrentQueryWithoutExternalID(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "old", 1_000, "yes"))
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), pipeline: p}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID: sessionID,
		Query:     "yes",
	}, 0)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 2 || !strings.Contains(built.Messages[0].TextContent(), "yes") || built.Messages[1].TextContent() != "yes" {
		t.Fatalf("repeated current query was not appended: %#v", modelMessageTexts(built.Messages))
	}
}

func TestBuildPipelineContextReportsCurrentOverflowWithoutNotice(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "current", 1_000, strings.Repeat("current", 100)))
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), pipeline: p}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "current",
		Query:             strings.Repeat("current", 100),
	}, 1)
	var overflow *contextassembly.OverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("buildPipelineContext() error = %v, want *OverflowError", err)
	}
	if len(built.Messages) != 1 || !strings.Contains(built.Messages[0].TextContent(), "current") {
		t.Fatalf("required current source was lost: %#v", modelMessageTexts(built.Messages))
	}
	if built.Allocation.BudgetTrimmed || built.Allocation.SourcesFit {
		t.Fatalf("required overflow was treated as truncation: %#v", built.Allocation)
	}
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
	latest := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("latest raw context"),
	}
	budget := estimateMessageTokens(historyTruncationNotice()) +
		estimateMessageTokens(summaryA.ModelMessage) +
		estimateMessageTokens(summaryB.ModelMessage) +
		estimateMessageTokens(latest)

	built, err := assembleComposedPipelineContext(nil, entries, &budget)
	if err != nil {
		t.Fatalf("assembleComposedPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 4 || !strings.HasPrefix(built.Messages[0].TextContent(), "[System Notice]") {
		t.Fatalf("trimmed messages = %#v, want notice plus both summaries and latest raw", modelMessageTexts(built.Messages))
	}
	if got, want := pipelineSummaryIDs(built.HistoryRecords), []string{"artifact-a", "artifact-b"}; !equalStrings(got, want) {
		t.Fatalf("retained summary identities = %#v, want %#v", got, want)
	}
	frags := historyContextFragsForMessages(built.Messages, built.HistoryRecords)
	if len(frags) != 2 || frags[0].Ref.ID != "artifact-a" || frags[1].Ref.ID != "artifact-b" {
		t.Fatalf("retained summary frags = %#v, want artifact-a and artifact-b", frags)
	}
}

func TestTrimMessagesAndRecordsPreservesEveryActiveSummary(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	summaryA := historyfrag.SummaryRecord("artifact-a", "summary a", nil, scope)
	summaryB := historyfrag.SummaryRecord("artifact-b", "summary b", nil, scope)
	oldRaw := historyfrag.HistoryRecord{ModelMessage: conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(strings.Repeat("old raw context", 200)),
	}}
	latest := historyfrag.HistoryRecord{ModelMessage: conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("latest raw context"),
	}}
	budget := estimateMessageTokens(historyTruncationNotice()) +
		estimateMessageTokens(summaryA.ModelMessage) +
		estimateMessageTokens(summaryB.ModelMessage) +
		estimateMessageTokens(latest.ModelMessage)

	messages, retained, estimate := trimMessagesAndRecordsByTokens(nil, []historyfrag.HistoryRecord{
		summaryA,
		oldRaw,
		summaryB,
		latest,
	}, budget)
	if got, want := pipelineSummaryIDs(retained), []string{"artifact-a", "artifact-b", ""}; !equalStrings(got, want) {
		t.Fatalf("retained records = %#v, want both summaries plus latest raw", got)
	}
	if len(messages) != 4 || !strings.HasPrefix(messages[0].TextContent(), "[System Notice]") {
		t.Fatalf("trimmed messages = %#v, want notice plus both summaries and latest raw", modelMessageTexts(messages))
	}
	wantEstimate := 0
	for _, message := range messages {
		wantEstimate += estimateMessageTokens(message)
	}
	if estimate != wantEstimate {
		t.Fatalf("estimated tokens = %d, want emitted-message total %d", estimate, wantEstimate)
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
	coveredMessageIDs          []string
	turnResponseErr            error
}

func (s *pipelineContextMessageService) LatestTurnResponseAtBySession(context.Context, string) (time.Time, error) {
	s.latestTurnResponseCalls++
	return s.latestTurnResponseAt, nil
}

func (s *pipelineContextMessageService) ListUncoveredTurnResponsesBySession(_ context.Context, _ string, coveredMessageIDs []string) ([]messagepkg.Message, error) {
	s.uncoveredTurnResponseCalls++
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
		if message.Role != "assistant" && message.Role != "tool" {
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

func TestTrimDiscussContextKeepsSummariesAndNotice(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	summary := pipelinepkg.ContextMessage{Role: "user", Content: "<summary>\ncondensed history\n</summary>", CompactionArtifactID: "artifact-a"}
	old := pipelinepkg.ContextMessage{Role: "user", Content: strings.Repeat("old context ", 100)}
	latest := pipelinepkg.ContextMessage{Role: "user", Content: "latest trigger"}
	budget := estimateMessageTokens(contextMessageForMetering(latest))

	messages, estimated := r.TrimDiscussContext([]pipelinepkg.ContextMessage{summary, old, latest}, budget)

	if len(messages) != 3 {
		t.Fatalf("messages = %d, want notice+summary+latest: %#v", len(messages), messages)
	}
	if messages[0].Role != "system" || !strings.Contains(messages[0].Content, "trimmed") {
		t.Fatalf("missing truncation notice: %#v", messages[0])
	}
	if messages[1].CompactionArtifactID != "artifact-a" {
		t.Fatalf("summary not retained: %#v", messages)
	}
	if messages[2].Content != "latest trigger" {
		t.Fatalf("latest trigger not retained: %#v", messages)
	}
	wantEstimate := 0
	for _, message := range messages {
		wantEstimate += estimateMessageTokens(contextMessageForMetering(message))
	}
	if estimated != wantEstimate {
		t.Fatalf("estimated = %d, want %d", estimated, wantEstimate)
	}
}

func TestTrimDiscussContextWithoutBudgetKeepsEverything(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	old := pipelinepkg.ContextMessage{Role: "user", Content: "old context"}
	latest := pipelinepkg.ContextMessage{Role: "user", Content: "latest trigger"}

	messages, estimated := r.TrimDiscussContext([]pipelinepkg.ContextMessage{old, latest}, 0)

	want := estimateMessageTokens(contextMessageForMetering(old)) + estimateMessageTokens(contextMessageForMetering(latest))
	if len(messages) != 2 || estimated != want {
		t.Fatalf("untrimmed passthrough broken: %d messages, estimate %d want %d", len(messages), estimated, want)
	}
}

func TestModelContextTokenBudget(t *testing.T) {
	t.Parallel()

	window := 128000
	if got := modelContextTokenBudget(models.GetResponse{Model: models.Model{Config: models.ModelConfig{ContextWindow: &window}}}); got != 128000 {
		t.Fatalf("declared window budget = %d, want 128000", got)
	}
	if got := modelContextTokenBudget(models.GetResponse{}); got != 0 {
		t.Fatalf("undeclared window budget = %d, want 0", got)
	}
}

func TestTrimDiscussContextPinsLatestUserTriggerOverNewerTurnResponse(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	trigger := pipelinepkg.ContextMessage{Role: "user", Content: strings.Repeat("triggering question ", 30)}
	reply := pipelinepkg.ContextMessage{Role: "assistant", Content: "previous reply"}
	budget := estimateMessageTokens(contextMessageForMetering(reply)) + 1

	messages, _ := r.TrimDiscussContext([]pipelinepkg.ContextMessage{trigger, reply}, budget)

	foundTrigger := false
	for _, message := range messages {
		if message.Role == "user" && strings.Contains(message.Content, "triggering question") {
			foundTrigger = true
		}
	}
	if !foundTrigger {
		t.Fatalf("the triggering user message was trimmed away while the bot's own reply survived: %#v", messages)
	}
}

func TestTrimDiscussContextNeverEmitsOrphanToolResults(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	call := pipelinepkg.ContextMessage{Role: "assistant", RawContent: json.RawMessage(`[{"type":"tool_call","tool_call_id":"call-1","tool_name":"exec","input":{"cmd":"` + strings.Repeat("x", 400) + `"}}]`)}
	result := pipelinepkg.ContextMessage{Role: "tool", RawContent: json.RawMessage(`[{"type":"tool_result","tool_call_id":"call-1","tool_name":"exec","result":"ok"}]`)}
	user := pipelinepkg.ContextMessage{Role: "user", Content: "please run it"}
	budget := estimateMessageTokens(contextMessageForMetering(result)) + 1

	messages, estimated := r.TrimDiscussContext([]pipelinepkg.ContextMessage{user, call, result}, budget)

	haveCall := false
	for _, message := range messages {
		if message.Role == "assistant" {
			haveCall = true
		}
	}
	wantEstimate := 0
	for _, message := range messages {
		if message.Role == "tool" && !haveCall {
			t.Fatalf("orphan tool result emitted without its call: %#v", messages)
		}
		wantEstimate += estimateMessageTokens(contextMessageForMetering(message))
	}
	if estimated != wantEstimate {
		t.Fatalf("estimate = %d counts messages that were not emitted (want %d)", estimated, wantEstimate)
	}
	foundTrigger := false
	for _, message := range messages {
		if message.Role == "user" && strings.Contains(message.Content, "please run it") {
			foundTrigger = true
		}
	}
	if !foundTrigger {
		t.Fatalf("latest user trigger lost: %#v", messages)
	}
}
