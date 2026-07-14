package flow

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestFlowAndPipelinePathsProduceEquivalentMultiArtifactContext(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifactA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		artifactB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		budget    = 5_000
	)
	base := time.Now().UTC().Add(-time.Minute)
	at := base.Add
	toolCall := sdkMessagesToModelMessages([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec", Input: map[string]any{}},
		},
	}})[0]
	toolResult := sdkMessagesToModelMessages([]sdk.Message{
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "exec", Result: "ok"}),
	})[0]
	rows := []messagepkg.Message{
		pipelineHistoryMessage(t, "user-a", botID, sessionID, "external-a", at(100*time.Millisecond), "user", "old question a"),
		pipelineHistoryMessage(t, "assistant-a", botID, sessionID, "", at(150*time.Millisecond), "assistant", "old answer a"),
		pipelineHistoryMessage(t, "user-mid", botID, sessionID, "external-mid", at(300*time.Millisecond), "user", "middle question"),
		pipelineModelHistoryMessage(t, "assistant-tool", botID, sessionID, at(400*time.Millisecond), toolCall),
		pipelineModelHistoryMessage(t, "tool-result", botID, sessionID, at(450*time.Millisecond), toolResult),
		pipelineHistoryMessage(t, "assistant-final", botID, sessionID, "", at(500*time.Millisecond), "assistant", "tool finished"),
		pipelineHistoryMessage(t, "user-b", botID, sessionID, "external-b", at(600*time.Millisecond), "user", "old question b"),
		pipelineHistoryMessage(t, "assistant-b", botID, sessionID, "", at(650*time.Millisecond), "assistant", "old answer b"),
		pipelineHistoryMessage(t, "user-current", botID, sessionID, "external-current", at(800*time.Millisecond), "user", "current question"),
	}
	for _, index := range []int{0, 1} {
		rows[index].CompactID = artifactA
	}
	for _, index := range []int{6, 7} {
		rows[index].CompactID = artifactB
	}

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	for _, event := range []pipelinepkg.MessageEvent{
		parityPipelineEvent(sessionID, "external-a", at(100*time.Millisecond), "old question a"),
		parityPipelineEvent(sessionID, "external-mid", at(300*time.Millisecond), "middle question"),
		parityPipelineEvent(sessionID, "external-b", at(600*time.Millisecond), "old question b"),
		parityPipelineEvent(sessionID, "external-current", at(800*time.Millisecond), "current question"),
	} {
		p.PushEvent(sessionID, event)
	}
	queries := &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{
		pipelineArtifactRow(t, artifactB, botID, sessionID, "condensed exchange b", rows[6:8], at(2*time.Second)),
		pipelineArtifactRow(t, artifactA, botID, sessionID, "condensed exchange a", rows[0:2], at(time.Second)),
	}}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{rows: rows},
		queries:        queries,
	}
	req := conversation.ChatRequest{
		BotID:             botID,
		ChatID:            "chat-1",
		SessionID:         sessionID,
		ConversationType:  "group",
		ReplyTarget:       "target",
		Query:             "current question",
		ExternalMessageID: "external-current",
	}
	ctx := context.Background()

	loaded, err := resolver.loadHistoryRecords(ctx, historyScopeFallbackFromChatRequest(req), sessionID, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadHistoryRecords() error = %v", err)
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded, err = resolver.replaceCompactedMessages(ctx, sessionID, compactionSummaryScope(
		req.BotID,
		req.ChatID,
		req.SessionID,
		req.ConversationType,
		req.ConversationName,
		req.ReplyTarget,
	), loaded)
	if err != nil {
		t.Fatalf("replaceCompactedMessages() error = %v", err)
	}
	flowMessages, flowRecords, _ := trimMessagesAndRecordsByTokens(nil, loaded, budget)
	pipelineBuild, err := resolver.buildPipelineContext(ctx, req, budget)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}

	if len(flowMessages) != len(pipelineBuild.Messages) {
		t.Fatalf("message counts: flow=%d pipeline=%d\nflow=%#v\npipeline=%#v",
			len(flowMessages), len(pipelineBuild.Messages), modelMessageTexts(flowMessages), modelMessageTexts(pipelineBuild.Messages))
	}
	for i := range flowMessages {
		flowRole, flowText := normalizeParityMessage(flowMessages[i])
		pipelineRole, pipelineText := normalizeParityMessage(pipelineBuild.Messages[i])
		if flowRole != pipelineRole || flowText != pipelineText {
			t.Fatalf("message %d mismatch: flow=(%q, %q) pipeline=(%q, %q)", i, flowRole, flowText, pipelineRole, pipelineText)
		}
	}
	flowFrags := historyContextFragsForMessages(flowMessages, flowRecords)
	pipelineFrags := historyContextFragsForMessages(pipelineBuild.Messages, pipelineBuild.HistoryRecords)
	assertParitySummaryFrags(t, "flow", flowFrags, []string{artifactA, artifactB})
	assertParitySummaryFrags(t, "pipeline", pipelineFrags, []string{artifactA, artifactB})
	assertValidToolClosures(t, "flow", flowMessages)
	assertValidToolClosures(t, "pipeline", pipelineBuild.Messages)
}

func TestFlowAndPipelineTrimmingProduceEquivalentNotice(t *testing.T) {
	t.Parallel()

	old := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(strings.Repeat("old context", 100)),
	}
	latest := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("latest context"),
	}
	budget := estimateMessageTokens(historyTruncationNotice()) + estimateMessageTokens(latest)
	flowBuild, err := assembleHistoryContext(nil, []historyfrag.HistoryRecord{
		{ModelMessage: old},
		{ModelMessage: latest},
	}, &budget)
	if err != nil {
		t.Fatalf("assemble flow: %v", err)
	}
	pipelineBuild, err := assembleComposedPipelineContext(nil, []composedPipelineMessage{
		{message: old},
		{message: latest},
	}, &budget)
	if err != nil {
		t.Fatalf("assemble pipeline: %v", err)
	}

	if len(flowBuild.Messages) != len(pipelineBuild.Messages) {
		t.Fatalf("message counts: flow=%d pipeline=%d\nflow=%#v\npipeline=%#v",
			len(flowBuild.Messages), len(pipelineBuild.Messages), modelMessageTexts(flowBuild.Messages), modelMessageTexts(pipelineBuild.Messages))
	}
	for i := range flowBuild.Messages {
		flowRole, flowText := normalizeParityMessage(flowBuild.Messages[i])
		pipelineRole, pipelineText := normalizeParityMessage(pipelineBuild.Messages[i])
		if flowRole != pipelineRole || flowText != pipelineText {
			t.Fatalf("message %d mismatch: flow=(%q, %q) pipeline=(%q, %q)", i, flowRole, flowText, pipelineRole, pipelineText)
		}
	}
	if flowBuild.EmittedTokens != pipelineBuild.EstimatedTokens {
		t.Fatalf("estimated tokens: flow=%d pipeline=%d", flowBuild.EmittedTokens, pipelineBuild.EstimatedTokens)
	}
	if !reflect.DeepEqual(allocationWithoutIDs(flowBuild.Allocation), allocationWithoutIDs(pipelineBuild.Allocation)) {
		t.Fatalf("allocations differ:\nflow     %#v\npipeline %#v", flowBuild.Allocation, pipelineBuild.Allocation)
	}
}

func pipelineModelHistoryMessage(t *testing.T, id, botID, sessionID string, createdAt time.Time, message conversation.ModelMessage) messagepkg.Message {
	t.Helper()
	raw := mustRawJSON(t, message)
	return messagepkg.Message{ID: id, BotID: botID, SessionID: sessionID, Role: message.Role, Content: raw, CreatedAt: createdAt}
}

func parityPipelineEvent(sessionID, messageID string, at time.Time, text string) pipelinepkg.MessageEvent {
	return pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    messageID,
		ReceivedAtMs: at.UnixMilli(),
		TimestampSec: at.Unix(),
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: text}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	}
}

func normalizeParityMessage(message conversation.ModelMessage) (string, string) {
	role := strings.ToLower(strings.TrimSpace(message.Role))
	text := message.TextContent()
	if looksLikeSummaryMessage(message) {
		return role, strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(text), "<summary>\n"), "\n</summary>")
	}
	if role == "user" {
		text = stripRenderedMessageWrapper(text)
	}
	return role, text
}

func stripRenderedMessageWrapper(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "<message ") || !strings.HasSuffix(trimmed, "</message>") {
		return text
	}
	open := strings.Index(trimmed, ">")
	if open < 0 {
		return text
	}
	return strings.Trim(trimmed[open+1:len(trimmed)-len("</message>")], "\n")
}

func assertParitySummaryFrags(t *testing.T, label string, frags []contextfrag.ContextFrag, wantIDs []string) {
	t.Helper()
	if len(frags) != len(wantIDs) {
		t.Fatalf("%s summary frags = %d, want %d: %#v", label, len(frags), len(wantIDs), frags)
	}
	for i, wantID := range wantIDs {
		if frags[i].Ref.ID != wantID || frags[i].Coverage == nil || len(frags[i].Coverage.CoveredRefs) != 2 {
			t.Fatalf("%s summary frag %d = %#v, want %s with two refs", label, i, frags[i], wantID)
		}
	}
	if traces := contextfrag.BuildManifest(frags).CoverageTrace; len(traces) != len(wantIDs) {
		t.Fatalf("%s coverage traces = %d, want %d", label, len(traces), len(wantIDs))
	}
}

func assertValidToolClosures(t *testing.T, label string, messages []conversation.ModelMessage) {
	t.Helper()
	pending := make(map[string]struct{})
	for _, message := range messages {
		sdkMessage := modelMessageToSDKMessage(message)
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "assistant":
			for _, part := range sdkMessage.Content {
				if call, ok := part.(sdk.ToolCallPart); ok && strings.TrimSpace(call.ToolCallID) != "" {
					pending[strings.TrimSpace(call.ToolCallID)] = struct{}{}
				}
			}
		case "tool":
			for _, part := range sdkMessage.Content {
				result, ok := part.(sdk.ToolResultPart)
				if !ok {
					continue
				}
				id := strings.TrimSpace(result.ToolCallID)
				if _, ok := pending[id]; !ok {
					t.Fatalf("%s orphan tool result %q", label, id)
				}
				delete(pending, id)
			}
		}
	}
	if len(pending) != 0 {
		t.Fatalf("%s unresolved tool calls: %#v", label, pending)
	}
}

func TestFlowAndPipelinePathsAlignAgedOutArtifactsAcrossRounds(t *testing.T) {
	t.Parallel()

	const (
		botID          = "11111111-1111-1111-1111-111111111111"
		sessionID      = "22222222-2222-2222-2222-222222222222"
		artifactOld    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		artifactMid    = "dddddddd-dddd-dddd-dddd-dddddddddddd"
		artifactInside = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		budget         = 5_000
	)
	base := time.Now().UTC().Add(-time.Minute)
	at := base.Add
	toolCall := sdkMessagesToModelMessages([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec", Input: map[string]any{}},
		},
	}})[0]
	toolResult := sdkMessagesToModelMessages([]sdk.Message{
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "exec", Result: "ok"}),
	})[0]

	agedOutOld := []messagepkg.Message{
		pipelineHistoryMessage(t, "user-0", botID, sessionID, "external-0", at(10*time.Millisecond), "user", "ancient question"),
		pipelineHistoryMessage(t, "assistant-0", botID, sessionID, "", at(20*time.Millisecond), "assistant", "ancient answer"),
	}
	agedOutMid := []messagepkg.Message{
		pipelineHistoryMessage(t, "user-mid2", botID, sessionID, "external-mid2", at(550*time.Millisecond), "user", "second round question"),
		pipelineHistoryMessage(t, "assistant-mid2", botID, sessionID, "", at(560*time.Millisecond), "assistant", "second round answer"),
	}
	rows := []messagepkg.Message{
		pipelineHistoryMessage(t, "user-a", botID, sessionID, "external-a", at(100*time.Millisecond), "user", "old question a"),
		pipelineHistoryMessage(t, "assistant-a", botID, sessionID, "", at(150*time.Millisecond), "assistant", "old answer a"),
		pipelineHistoryMessage(t, "user-mid", botID, sessionID, "external-mid", at(300*time.Millisecond), "user", "middle question"),
		pipelineModelHistoryMessage(t, "assistant-tool", botID, sessionID, at(400*time.Millisecond), toolCall),
		pipelineModelHistoryMessage(t, "tool-result", botID, sessionID, at(450*time.Millisecond), toolResult),
		pipelineHistoryMessage(t, "assistant-final", botID, sessionID, "", at(500*time.Millisecond), "assistant", "tool finished"),
		pipelineHistoryMessage(t, "user-b", botID, sessionID, "external-b", at(600*time.Millisecond), "user", "old question b"),
		pipelineHistoryMessage(t, "assistant-b", botID, sessionID, "", at(650*time.Millisecond), "assistant", "old answer b"),
		pipelineHistoryMessage(t, "user-current", botID, sessionID, "external-current", at(800*time.Millisecond), "user", "current question"),
	}
	for _, index := range []int{0, 1} {
		rows[index].CompactID = artifactInside
	}

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	for _, event := range []pipelinepkg.MessageEvent{
		parityPipelineEvent(sessionID, "external-a", at(100*time.Millisecond), "old question a"),
		parityPipelineEvent(sessionID, "external-mid", at(300*time.Millisecond), "middle question"),
		parityPipelineEvent(sessionID, "external-b", at(600*time.Millisecond), "old question b"),
		parityPipelineEvent(sessionID, "external-current", at(800*time.Millisecond), "current question"),
	} {
		p.PushEvent(sessionID, event)
	}
	queries := &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{
		pipelineArtifactRow(t, artifactMid, botID, sessionID, "second round span", agedOutMid, at(3*time.Second)),
		pipelineArtifactRow(t, artifactOld, botID, sessionID, "ancient span", agedOutOld, at(time.Second)),
		pipelineArtifactRow(t, artifactInside, botID, sessionID, "condensed exchange a", rows[0:2], at(2*time.Second)),
	}}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &pipelineContextMessageService{rows: rows},
		queries:        queries,
	}
	req := conversation.ChatRequest{
		BotID:             botID,
		ChatID:            "chat-1",
		SessionID:         sessionID,
		ConversationType:  "group",
		ReplyTarget:       "target",
		Query:             "current question",
		ExternalMessageID: "external-current",
	}
	ctx := context.Background()

	loaded, err := resolver.loadHistoryRecords(ctx, historyScopeFallbackFromChatRequest(req), sessionID, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadHistoryRecords() error = %v", err)
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded, err = resolver.replaceCompactedMessages(ctx, sessionID, compactionSummaryScope(
		req.BotID,
		req.ChatID,
		req.SessionID,
		req.ConversationType,
		req.ConversationName,
		req.ReplyTarget,
	), loaded)
	if err != nil {
		t.Fatalf("replaceCompactedMessages() error = %v", err)
	}
	flowMessages, flowRecords, _ := trimMessagesAndRecordsByTokens(nil, loaded, budget)
	pipelineBuild, err := resolver.buildPipelineContext(ctx, req, budget)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}

	wantTexts := []string{
		"ancient span",
		"condensed exchange a",
		"middle question",
		"", // tool call carries no text
		"", // tool result carries no text
		"tool finished",
		"second round span",
		"old question b",
		"old answer b",
		"current question",
	}
	for label, messages := range map[string][]conversation.ModelMessage{"flow": flowMessages, "pipeline": pipelineBuild.Messages} {
		if len(messages) != len(wantTexts) {
			t.Fatalf("%s message count = %d, want %d: %#v", label, len(messages), len(wantTexts), modelMessageTexts(messages))
		}
		for i, want := range wantTexts {
			if _, text := normalizeParityMessage(messages[i]); text != want {
				t.Fatalf("%s message %d = %q, want %q\nall: %#v", label, i, text, want, modelMessageTexts(messages))
			}
		}
	}
	for i := range flowMessages {
		flowRole, flowText := normalizeParityMessage(flowMessages[i])
		pipelineRole, pipelineText := normalizeParityMessage(pipelineBuild.Messages[i])
		if flowRole != pipelineRole || flowText != pipelineText {
			t.Fatalf("message %d mismatch: flow=(%q, %q) pipeline=(%q, %q)", i, flowRole, flowText, pipelineRole, pipelineText)
		}
	}
	flowFrags := historyContextFragsForMessages(flowMessages, flowRecords)
	pipelineFrags := historyContextFragsForMessages(pipelineBuild.Messages, pipelineBuild.HistoryRecords)
	assertParitySummaryFrags(t, "flow", flowFrags, []string{artifactOld, artifactInside, artifactMid})
	assertParitySummaryFrags(t, "pipeline", pipelineFrags, []string{artifactOld, artifactInside, artifactMid})
	assertValidToolClosures(t, "flow", flowMessages)
	assertValidToolClosures(t, "pipeline", pipelineBuild.Messages)
}
