package flow

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestLoadContextHistoryProjectionRestoresTurnResponsesForConflictingArtifact(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		userID     = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		responseID = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	now := time.Now().UTC()
	originalUser := pipelineHistoryMessage(t, userID, botID, sessionID, "external", now.Add(-time.Hour), "user", "original")
	oldResponse := pipelineHistoryMessage(t, responseID, botID, sessionID, "", now.Add(-48*time.Hour), "assistant", "raw response")
	mutatedUser := pipelineHistoryMessage(t, userID, botID, sessionID, "external", originalUser.CreatedAt, "user", "mutated")
	queries := &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{
		pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{oldResponse, originalUser}, now),
	}}
	messages := &pipelineContextMessageService{
		rows:                   []messagepkg.Message{mutatedUser},
		uncoveredTurnResponses: []messagepkg.Message{oldResponse},
	}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), messageService: messages, queries: queries}

	projection, err := resolver.LoadContextHistoryProjection(context.Background(), botID, sessionID)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if len(projection.CompactionArtifacts) != 0 || len(projection.TurnResponses) != 1 || projection.TurnResponses[0].SourceMessageID != responseID {
		t.Fatalf("conflicting projection = %#v", projection)
	}
	if len(messages.coveredMessageIDs) != 0 {
		t.Fatalf("conflicting artifact excluded raw rows: %#v", messages.coveredMessageIDs)
	}
}

func TestLoadContextHistoryProjectionRestoresTurnResponsesForInvalidArtifact(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		responseID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	response := pipelineHistoryMessage(t, responseID, botID, sessionID, "", time.Now().UTC().Add(-48*time.Hour), "assistant", "raw response")
	row := pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{response}, time.Now().UTC())
	row.Coverage = []byte("{")
	messages := &pipelineContextMessageService{uncoveredTurnResponses: []messagepkg.Message{response}}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: messages,
		queries:        &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{row}},
	}

	projection, err := resolver.LoadContextHistoryProjection(context.Background(), botID, sessionID)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if len(projection.CompactionArtifacts) != 0 || len(projection.TurnResponses) != 1 || projection.TurnResponses[0].SourceMessageID != responseID {
		t.Fatalf("invalid-artifact projection = %#v", projection)
	}
	if len(messages.coveredMessageIDs) != 0 {
		t.Fatalf("invalid artifact excluded raw rows: %#v", messages.coveredMessageIDs)
	}
}

func TestLoadContextHistoryProjectionUsesLatestCursorBeyondUncoveredResponses(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	old := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	latest := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	oldResponse := pipelineHistoryMessage(
		t,
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"11111111-1111-1111-1111-111111111111",
		sessionID,
		"",
		old,
		"assistant",
		"old uncovered response",
	)
	messages := &pipelineContextMessageService{
		uncoveredTurnResponses: []messagepkg.Message{oldResponse},
		latestTurnResponseAt:   latest,
	}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), messageService: messages}

	projection, err := resolver.LoadContextHistoryProjection(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		sessionID,
	)
	if err != nil {
		t.Fatalf("LoadContextHistoryProjection() error = %v", err)
	}
	if projection.LatestTurnResponseAtMs != latest.UnixMilli() {
		t.Fatalf("latest turn response = %d, want %d", projection.LatestTurnResponseAtMs, latest.UnixMilli())
	}
}
