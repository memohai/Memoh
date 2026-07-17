package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestFlowAndPipelinePathsApplyTheSameHistoryWindow(t *testing.T) {
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	now := time.Now().UTC()
	oldAt := now.Add(-25 * time.Hour)
	recentAt := now.Add(-time.Hour)
	rows := []messagepkg.Message{
		pipelineHistoryMessage(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, "old", oldAt, "user", "outside history window"),
		pipelineHistoryMessage(t, "cccccccc-cccc-cccc-cccc-cccccccccccc", botID, sessionID, "", oldAt.Add(time.Minute), "assistant", "outside history response"),
		pipelineHistoryMessage(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, "recent", recentAt, "user", "inside history window"),
		pipelineHistoryMessage(t, "dddddddd-dddd-dddd-dddd-dddddddddddd", botID, sessionID, "", recentAt.Add(time.Minute), "assistant", "inside history response"),
	}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	oldEvent := parityPipelineEvent(sessionID, "old", oldAt, "outside history window")
	oldEvent.EventCursor = oldAt.UnixMilli()
	pipeline.PushEvent(sessionID, oldEvent)
	recentEvent := parityPipelineEvent(sessionID, "recent", recentAt, "inside history window")
	recentEvent.EventCursor = recentAt.UnixMilli()
	pipeline.PushEvent(sessionID, recentEvent)
	service := &windowedParityMessageService{rows: rows}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       pipeline,
		messageService: service,
	}
	req := conversation.ChatRequest{
		BotID:             botID,
		SessionID:         sessionID,
		ExternalMessageID: "recent",
		Query:             "inside history window",
	}

	flowMessages, _, pipelineBuild := buildUncompactedParityContexts(t, resolver, req, 0)

	assertSemanticMessageParity(t, flowMessages, pipelineBuild.Messages)
	if got := strings.Join(modelMessageTexts(flowMessages), "\n"); strings.Contains(got, "outside history") ||
		!strings.Contains(got, "inside history window") ||
		!strings.Contains(got, "inside history response") {
		t.Fatalf("flow history window = %q, want only the recent turn", got)
	}
	if service.activeSince.IsZero() || service.uncoveredSince.IsZero() {
		t.Fatalf("history readers did not receive a lower bound: active=%v uncovered=%v", service.activeSince, service.uncoveredSince)
	}
}

func TestFlowAndPipelineTrimmingKeepMultipleArtifactsAtTightBudget(t *testing.T) {
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifactA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		artifactB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	base := time.Now().UTC().Add(-time.Minute)
	at := base.Add
	oldRaw := strings.Repeat("old raw context", 200)
	rows := []messagepkg.Message{
		pipelineHistoryMessage(t, "11111111-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, "external-a", at(time.Second), "user", "artifact a question"),
		pipelineHistoryMessage(t, "22222222-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, "", at(2*time.Second), "assistant", "artifact a answer"),
		pipelineHistoryMessage(t, "33333333-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, "old-raw", at(3*time.Second), "user", oldRaw),
		pipelineHistoryMessage(t, "11111111-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, "external-b", at(4*time.Second), "user", "artifact b question"),
		pipelineHistoryMessage(t, "22222222-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, "", at(5*time.Second), "assistant", "artifact b answer"),
		pipelineHistoryMessage(t, "33333333-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, "current", at(6*time.Second), "user", "current request"),
	}
	for _, index := range []int{0, 1} {
		rows[index].CompactID = artifactA
	}
	for _, index := range []int{3, 4} {
		rows[index].CompactID = artifactB
	}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	for _, event := range []pipelinepkg.MessageEvent{
		parityPipelineEvent(sessionID, "external-a", at(time.Second), "artifact a question"),
		parityPipelineEvent(sessionID, "old-raw", at(3*time.Second), oldRaw),
		parityPipelineEvent(sessionID, "external-b", at(4*time.Second), "artifact b question"),
		parityPipelineEvent(sessionID, "current", at(6*time.Second), "current request"),
	} {
		event.EventCursor = event.ReceivedAtMs
		pipeline.PushEvent(sessionID, event)
	}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       pipeline,
		messageService: &windowedParityMessageService{rows: rows},
		queries: &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{
			pipelineArtifactRow(t, artifactB, botID, sessionID, "summary b", rows[3:5], at(8*time.Second)),
			pipelineArtifactRow(t, artifactA, botID, sessionID, "summary a", rows[0:2], at(7*time.Second)),
		}},
	}
	req := conversation.ChatRequest{
		BotID:             botID,
		SessionID:         sessionID,
		ExternalMessageID: "current",
		Query:             "current request",
	}
	ctx := context.Background()
	loaded, err := resolver.loadHistoryRecords(ctx, historyScopeFallbackFromChatRequest(req), sessionID, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadHistoryRecords() error = %v", err)
	}
	loaded, err = resolver.replaceCompactedMessages(ctx, sessionID, compactionSummaryScope(
		req.BotID,
		req.ChatID,
		req.SessionID,
		req.ConversationType,
		req.ConversationName,
		req.ReplyTarget,
	), pruneHistoryForGateway(loaded), compactionArtifactBoundary{})
	if err != nil {
		t.Fatalf("replaceCompactedMessages() error = %v", err)
	}
	current := conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(req.Query)}
	budget := estimateMessageTokens(current)
	flowMessages, flowRecords, _ := trimMessagesAndRecordsByTokens(nil, loaded, budget)
	pipelineBuild, err := resolver.buildPipelineContext(ctx, req, budget, req.Query)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}

	assertSemanticMessageParity(t, flowMessages, pipelineBuild.Messages)
	if got, want := normalizedParityTexts(flowMessages), []string{
		historyTruncationNotice().TextContent(),
		"summary a",
		"summary b",
		"current request",
	}; !equalStrings(got, want) {
		t.Fatalf("tight-budget messages = %#v, want %#v", got, want)
	}
	if strings.Contains(strings.Join(modelMessageTexts(flowMessages), "\n"), oldRaw) ||
		strings.Contains(strings.Join(modelMessageTexts(pipelineBuild.Messages), "\n"), oldRaw) {
		t.Fatal("tight-budget context retained old raw history")
	}
	assertParitySummaryFrags(t, "flow", historyContextFragsForMessages(flowMessages, flowRecords), []string{artifactA, artifactB})
	assertParitySummaryFrags(t, "pipeline", historyContextFragsForMessages(pipelineBuild.Messages, pipelineBuild.HistoryRecords), []string{artifactA, artifactB})
}

func TestFlowAndPipelinePathsUseDurableToolExchangePosition(t *testing.T) {
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	base := time.Now().UTC().Add(-time.Minute)
	toolCall := sdkMessagesToModelMessages([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec", Input: map[string]any{}},
		},
	}})[0]
	toolResult := sdkMessagesToModelMessages([]sdk.Message{
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "exec", Result: "ok"}),
	})[0]
	call := pipelineModelHistoryMessage(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, base, toolCall)
	result := pipelineModelHistoryMessage(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, base.Add(20*time.Second), toolResult)
	user := pipelineHistoryMessage(t, "cccccccc-cccc-cccc-cccc-cccccccccccc", botID, sessionID, "correction", base.Add(10*time.Second), "user", "correction after tool result")
	call.TurnPosition, call.TurnMessageSequence = 1, 2
	result.TurnPosition, result.TurnMessageSequence = 1, 3
	user.TurnPosition, user.TurnMessageSequence = 2, 1
	rows := []messagepkg.Message{call, result, user}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	event := parityPipelineEvent(sessionID, "correction", user.CreatedAt, "correction after tool result")
	event.EventCursor = user.CreatedAt.UnixMilli()
	pipeline.PushEvent(sessionID, event)
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       pipeline,
		messageService: &windowedParityMessageService{rows: rows},
	}

	flowMessages, _, pipelineBuild := buildUncompactedParityContexts(t, resolver, conversation.ChatRequest{
		BotID:     botID,
		SessionID: sessionID,
	}, 0)

	assertSemanticMessageParity(t, flowMessages, pipelineBuild.Messages)
	assertValidToolClosures(t, "flow", flowMessages)
	assertValidToolClosures(t, "pipeline", pipelineBuild.Messages)
	if len(pipelineBuild.Messages) != 3 || pipelineBuild.Messages[0].Role != "assistant" || pipelineBuild.Messages[1].Role != "tool" {
		t.Fatalf("durable tool exchange roles = %#v, want assistant/tool/user", pipelineBuild.Messages)
	}
	if role, text := normalizeParityMessage(pipelineBuild.Messages[2]); role != "user" || text != "correction after tool result" {
		t.Fatalf("durable tool exchange tail = (%q, %q), want correction after call/result", role, text)
	}
}

type windowedParityMessageService struct {
	messagepkg.Service
	rows           []messagepkg.Message
	activeSince    time.Time
	uncoveredSince time.Time
}

func (s *windowedParityMessageService) ListActiveSinceBySession(_ context.Context, _ string, since time.Time) ([]messagepkg.Message, error) {
	s.activeSince = since
	return parityMessagesSince(s.rows, since, false, nil), nil
}

func (s *windowedParityMessageService) ListUncoveredTurnResponsesBySession(_ context.Context, _ string, since time.Time, coveredMessageIDs []string) ([]messagepkg.Message, error) {
	s.uncoveredSince = since
	return parityMessagesSince(s.rows, since, true, coveredMessageIDs), nil
}

func parityMessagesSince(rows []messagepkg.Message, since time.Time, responsesOnly bool, coveredMessageIDs []string) []messagepkg.Message {
	covered := make(map[string]struct{}, len(coveredMessageIDs))
	for _, id := range coveredMessageIDs {
		covered[id] = struct{}{}
	}
	filtered := make([]messagepkg.Message, 0, len(rows))
	for _, row := range rows {
		if row.CreatedAt.Before(since) {
			continue
		}
		if responsesOnly && row.Role != "assistant" && row.Role != "tool" {
			continue
		}
		if _, excluded := covered[row.ID]; excluded {
			continue
		}
		if responsesOnly {
			row.TurnPosition = 0
			row.TurnMessageSequence = 0
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func buildUncompactedParityContexts(
	t *testing.T,
	resolver *Resolver,
	req conversation.ChatRequest,
	budget int,
) ([]conversation.ModelMessage, []historyfrag.HistoryRecord, pipelineContextBuild) {
	t.Helper()
	ctx := context.Background()
	loaded, err := resolver.loadHistoryRecords(ctx, historyScopeFallbackFromChatRequest(req), req.SessionID, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadHistoryRecords() error = %v", err)
	}
	loaded = pruneHistoryForGateway(loaded)
	flowMessages, flowRecords, _ := trimMessagesAndRecordsByTokens(nil, loaded, budget)
	pipelineBuild, err := resolver.buildPipelineContext(ctx, req, budget, req.Query)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	return flowMessages, flowRecords, pipelineBuild
}

func assertSemanticMessageParity(t *testing.T, flowMessages, pipelineMessages []conversation.ModelMessage) {
	t.Helper()
	if len(flowMessages) != len(pipelineMessages) {
		t.Fatalf("message counts: flow=%d pipeline=%d\nflow=%#v\npipeline=%#v",
			len(flowMessages), len(pipelineMessages), modelMessageTexts(flowMessages), modelMessageTexts(pipelineMessages))
	}
	for i := range flowMessages {
		flowRole, flowText := normalizeParityMessage(flowMessages[i])
		pipelineRole, pipelineText := normalizeParityMessage(pipelineMessages[i])
		if flowRole != pipelineRole || flowText != pipelineText {
			t.Fatalf("message %d mismatch: flow=(%q, %q) pipeline=(%q, %q)", i, flowRole, flowText, pipelineRole, pipelineText)
		}
	}
}

func normalizedParityTexts(messages []conversation.ModelMessage) []string {
	texts := make([]string, 0, len(messages))
	for _, message := range messages {
		_, text := normalizeParityMessage(message)
		texts = append(texts, text)
	}
	return texts
}
