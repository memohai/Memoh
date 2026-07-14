package flow

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestInvalidAgedOutArtifactIsExcludedFromFlowAndPipeline(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifact  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	base := time.UnixMilli(1_000).UTC()
	agedOut := pipelineHistoryMessage(t, "row-aged-out", botID, sessionID, "external-aged", base, "user", "stale source")
	row := pipelineArtifactRow(t, artifact, botID, sessionID, "stale aged-out summary", []messagepkg.Message{agedOut}, base.Add(time.Minute))
	queries := &recordingCompactionLogQueries{
		logs:       []sqlc.BotHistoryMessageCompact{row},
		invalidIDs: []pgtype.UUID{row.ID},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")
	recent := historyRecord("row-current", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("current")}, func(record *historyfrag.HistoryRecord) {
		record.BotID = botID
		record.SessionID = sessionID
		record.SessionIDKnown = true
	})

	flowRecords := mustReplaceCompactedMessages(t, resolver, sessionID, scope, []historyfrag.HistoryRecord{recent})
	if got := recordTexts(flowRecords); len(got) != 1 || got[0] != "current" {
		t.Fatalf("flow consumed invalid aged-out summary: %#v", got)
	}
	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, []historyfrag.HistoryRecord{recent})
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 0 || len(summaries) != 0 {
		t.Fatalf("pipeline consumed invalid aged-out summary: artifacts=%#v summaries=%#v", artifacts, summaries)
	}
}

func (q *recordingCompactionLogQueries) ListInvalidCompactionArtifactSeedsBySession(context.Context, sqlc.ListInvalidCompactionArtifactSeedsBySessionParams) ([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, error) {
	seeds := make([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, 0, len(q.invalidIDs))
	for _, id := range q.invalidIDs {
		seed := sqlc.ListInvalidCompactionArtifactSeedsBySessionRow{ID: id}
		for _, row := range q.logs {
			if row.ID == id {
				seed.Coverage = row.Coverage
			}
		}
		seeds = append(seeds, seed)
	}
	return seeds, nil
}
