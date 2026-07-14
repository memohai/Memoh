package compaction

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestDoCompactionInjectsPriorContext(t *testing.T) {
	rows := machineryCorpus(t)
	stub := &stubModel{summary: "S2"}
	cfg := machineryConfig(stub, 450)
	q := &fakeQueries{
		uncompacted: rows,
		priorLogs: []sqlc.BotHistoryMessageCompact{{
			ID:            pgtype.UUID{Bytes: uuid.New(), Valid: true},
			BotID:         pgtype.UUID{Bytes: uuid.MustParse(cfg.BotID), Valid: true},
			SessionID:     pgtype.UUID{Bytes: uuid.MustParse(cfg.SessionID), Valid: true},
			Summary:       "earlier-segment-summary",
			Status:        "ok",
			AnchorStartMs: 1000,
			AnchorEndMs:   2000,
		}},
	}
	svc := newMachineryService(q)

	if _, err := svc.RunCompactionSync(context.Background(), cfg); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if !strings.Contains(stub.prompt, "prior_context") || !strings.Contains(stub.prompt, "earlier-segment-summary") {
		t.Fatalf("prior summary not injected as prior context:\n%s", stub.prompt)
	}
}

func TestDoCompactionPriorContextUsesOnlyActiveArtifactFrontier(t *testing.T) {
	parentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	activeID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	stub := &stubModel{summary: "S2"}
	cfg := machineryConfig(stub, 450)
	botID := pgtype.UUID{Bytes: uuid.MustParse(cfg.BotID), Valid: true}
	sessionID := pgtype.UUID{Bytes: uuid.MustParse(cfg.SessionID), Valid: true}
	coverage := testCoverageJSON(t, "covered-row")
	var covered []CoveredSource
	if err := json.Unmarshal(coverage, &covered); err != nil {
		t.Fatalf("decode test coverage: %v", err)
	}
	covered[0].CreatedAtMs = 1000
	coverage, _ = json.Marshal(covered)
	q := &fakeQueries{
		uncompacted: machineryCorpus(t),
		priorLogs: []sqlc.BotHistoryMessageCompact{
			{
				ID:            parentID,
				BotID:         botID,
				SessionID:     sessionID,
				Status:        "ok",
				Summary:       "stale-parent-summary",
				Coverage:      coverage,
				AnchorStartMs: 1000,
				AnchorEndMs:   1000,
				SupersededBy:  activeID,
				SupersededAt:  pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true},
			},
			{
				ID:            activeID,
				BotID:         botID,
				SessionID:     sessionID,
				Status:        "ok",
				Summary:       "active-frontier-summary",
				Coverage:      coverage,
				AnchorStartMs: 1000,
				AnchorEndMs:   1000,
				ParentIds:     []pgtype.UUID{parentID},
			},
		},
	}

	if _, err := newMachineryService(q).RunCompactionSync(context.Background(), cfg); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if !strings.Contains(stub.prompt, "active-frontier-summary") || strings.Contains(stub.prompt, "stale-parent-summary") {
		t.Fatalf("prior context did not use the active frontier:\n%s", stub.prompt)
	}
}
