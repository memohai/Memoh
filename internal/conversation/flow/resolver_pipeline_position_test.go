package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestLoadPipelineHistoryProjectionMapsDurableMessagePositions(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	messages := positionedPipelineHistory(t, botID, sessionID, time.Now().UTC())
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: &pipelineContextMessageService{rows: messages},
	}

	projection, err := resolver.LoadContextHistoryProjection(context.Background(), botID, sessionID)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if got := projection.ExternalMessagePositions["injected"]; got != (pipelinepkg.HistoryPosition{TurnPosition: 2, MessageSequence: 1}) {
		t.Fatalf("injected external position = %#v, want 2/1", got)
	}
	if got := projection.HistoryMessagePositions[messages[1].ID]; got != (pipelinepkg.HistoryPosition{TurnPosition: 1, MessageSequence: 2}) {
		t.Fatalf("assistant history position = %#v, want 1/2", got)
	}
	if len(projection.TurnResponses) != 2 ||
		projection.TurnResponses[0].HistoryPosition != (pipelinepkg.HistoryPosition{TurnPosition: 1, MessageSequence: 2}) ||
		projection.TurnResponses[1].HistoryPosition != (pipelinepkg.HistoryPosition{TurnPosition: 2, MessageSequence: 2}) {
		t.Fatalf("turn response positions = %#v", projection.TurnResponses)
	}
}

func TestBuildPipelineContextUsesDurableMidTurnInjectionPosition(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	now := time.Now().UTC()
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(sessionID, pipelineMessageEvent(sessionID, "original", now.Add(-10*time.Minute).UnixMilli(), "original question"))
	pipeline.PushEvent(sessionID, pipelineMessageEvent(sessionID, "injected", now.Add(-9*time.Minute).UnixMilli(), "injected correction"))
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       pipeline,
		messageService: &pipelineContextMessageService{rows: positionedPipelineHistory(t, botID, sessionID, now)},
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		BotID:     botID,
		SessionID: sessionID,
	}, 0, "")
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	assertPipelineContextOrder(t, built.Messages, []string{
		"original question",
		"assistant before injection",
		"injected correction",
		"assistant after injection",
	})
}

func TestBuildPipelineContextRestoresInternalUserImageWithinDurableTurn(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	now := time.Now().UTC()
	internalRound := sdkMessagesToModelMessages([]sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "read-1",
				ToolName:   "read_media",
				Input:      map[string]any{"path": "image.png"},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "read-1", ToolName: "read_media", Result: "ok"}),
		sdk.UserMessage("", sdk.ImagePart{Image: "data:image/png;base64,aW1hZ2U=", MediaType: "image/png"}),
	})
	rows := []messagepkg.Message{
		pipelineHistoryMessage(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, "original", now.Add(-10*time.Minute), "user", "original question"),
		pipelineModelHistoryMessage(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, now.Add(-9*time.Minute), internalRound[0]),
		pipelineModelHistoryMessage(t, "cccccccc-cccc-cccc-cccc-cccccccccccc", botID, sessionID, now.Add(-8*time.Minute), internalRound[1]),
		pipelineModelHistoryMessage(t, "dddddddd-dddd-dddd-dddd-dddddddddddd", botID, sessionID, now.Add(-7*time.Minute), internalRound[2]),
		pipelineHistoryMessage(t, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", botID, sessionID, "", now.Add(-6*time.Minute), "assistant", "image understood"),
		pipelineHistoryMessage(t, "ffffffff-ffff-ffff-ffff-ffffffffffff", botID, sessionID, "next", now.Add(-5*time.Minute), "user", "next question"),
	}
	for i := range rows[:5] {
		rows[i].TurnPosition = 1
		rows[i].TurnMessageSequence = int64(i + 1)
	}
	rows[5].TurnPosition, rows[5].TurnMessageSequence = 2, 1

	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(sessionID, pipelineMessageEvent(sessionID, "original", now.Add(-10*time.Minute).UnixMilli(), "original question"))
	pipeline.PushEvent(sessionID, pipelineMessageEvent(sessionID, "next", now.Add(-5*time.Minute).UnixMilli(), "next question"))
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       pipeline,
		messageService: &pipelineContextMessageService{rows: rows},
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		BotID:     botID,
		SessionID: sessionID,
	}, 0, "")
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	wantRoles := []string{"user", "assistant", "tool", "user", "assistant", "user"}
	if len(built.Messages) != len(wantRoles) {
		t.Fatalf("message count = %d, want %d: roles=%#v texts=%#v", len(built.Messages), len(wantRoles), modelMessageRoles(built.Messages), modelMessageTexts(built.Messages))
	}
	for i, wantRole := range wantRoles {
		if built.Messages[i].Role != wantRole {
			t.Fatalf("message %d role = %q, want %q: %#v", i, built.Messages[i].Role, wantRole, modelMessageRoles(built.Messages))
		}
	}
	if string(built.Messages[3].Content) != string(internalRound[2].Content) {
		t.Fatalf("internal user image content = %s, want %s", built.Messages[3].Content, internalRound[2].Content)
	}
	if built.Messages[4].TextContent() != "image understood" {
		t.Fatalf("final assistant moved: %#v", modelMessageTexts(built.Messages))
	}
	assertValidToolClosures(t, "pipeline", built.Messages)
}

func TestBuildPipelineContextKeepsRecentlyEditedOldMessageAtDurablePosition(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	now := time.Now().UTC()
	oldAt := now.Add(-25 * time.Hour)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	stale := pipelineMessageEvent(sessionID, "stale", now.Add(-26*time.Hour).UnixMilli(), "stale question")
	stale.EventCursor = 10
	pipeline.PushEvent(sessionID, stale)
	old := pipelineMessageEvent(sessionID, "old", oldAt.UnixMilli(), "old question")
	old.EventCursor = 20
	pipeline.PushEvent(sessionID, old)
	pipeline.PushEvent(sessionID, pipelinepkg.EditEvent{
		SessionID:    sessionID,
		MessageID:    "old",
		ReceivedAtMs: now.Add(-time.Minute).UnixMilli(),
		EventCursor:  30,
		TimestampSec: now.Add(-time.Minute).Unix(),
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "edited old question"}},
	})
	newEvent := pipelineMessageEvent(sessionID, "new", now.Add(-time.Hour).UnixMilli(), "new question")
	newEvent.EventCursor = 40
	pipeline.PushEvent(sessionID, newEvent)

	recent := []messagepkg.Message{
		pipelineHistoryMessage(t, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", botID, sessionID, "new", now.Add(-time.Hour), "user", "new question"),
		pipelineHistoryMessage(t, "ffffffff-ffff-ffff-ffff-ffffffffffff", botID, sessionID, "", now.Add(-30*time.Minute), "assistant", "new answer"),
	}
	recent[0].TurnPosition, recent[0].TurnMessageSequence = 2, 1
	recent[1].TurnPosition, recent[1].TurnMessageSequence = 2, 2
	service := &positionedPipelineMessageService{
		pipelineContextMessageService: &pipelineContextMessageService{rows: recent},
		positions: []messagepkg.ExternalMessagePosition{{
			ExternalMessageID:   "old",
			TurnPosition:        1,
			TurnMessageSequence: 1,
		}},
	}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       pipeline,
		messageService: service,
	}

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		BotID:     botID,
		SessionID: sessionID,
	}, 0, "")
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 2 {
		t.Fatalf("message count = %d, want 2: %#v", len(built.Messages), modelMessageTexts(built.Messages))
	}
	userText := built.Messages[0].TextContent()
	oldIndex := strings.Index(userText, "edited old question")
	newIndex := strings.Index(userText, "new question")
	if oldIndex < 0 || newIndex < 0 || oldIndex > newIndex {
		t.Fatalf("old edited message lost its durable position: %q", userText)
	}
	if got := built.Messages[1].TextContent(); got != "new answer" {
		t.Fatalf("assistant message = %q, want new answer", got)
	}
	if len(service.requestedIDs) != 2 || service.requestedIDs[0] != "old" || service.requestedIDs[1] != "new" {
		t.Fatalf("position lookup ids = %#v, want old/new", service.requestedIDs)
	}
}

type positionedPipelineMessageService struct {
	*pipelineContextMessageService
	positions    []messagepkg.ExternalMessagePosition
	requestedIDs []string
}

func (s *positionedPipelineMessageService) ListExternalMessagePositionsBySession(
	_ context.Context,
	_ string,
	externalMessageIDs []string,
) ([]messagepkg.ExternalMessagePosition, error) {
	s.requestedIDs = append([]string(nil), externalMessageIDs...)
	return s.positions, nil
}

func positionedPipelineHistory(t *testing.T, botID, sessionID string, now time.Time) []messagepkg.Message {
	t.Helper()

	messages := []messagepkg.Message{
		pipelineHistoryMessage(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", botID, sessionID, "original", now.Add(-10*time.Minute), "user", "original question"),
		pipelineHistoryMessage(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", botID, sessionID, "", now.Add(-2*time.Minute), "assistant", "assistant before injection"),
		pipelineHistoryMessage(t, "cccccccc-cccc-cccc-cccc-cccccccccccc", botID, sessionID, "injected", now.Add(-9*time.Minute), "user", "injected correction"),
		pipelineHistoryMessage(t, "dddddddd-dddd-dddd-dddd-dddddddddddd", botID, sessionID, "", now.Add(-time.Minute), "assistant", "assistant after injection"),
	}
	messages[0].TurnPosition, messages[0].TurnMessageSequence = 1, 1
	messages[1].TurnPosition, messages[1].TurnMessageSequence = 1, 2
	messages[2].TurnPosition, messages[2].TurnMessageSequence = 2, 1
	messages[3].TurnPosition, messages[3].TurnMessageSequence = 2, 2
	return messages
}

func modelMessageRoles(messages []conversation.ModelMessage) []string {
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	return roles
}
