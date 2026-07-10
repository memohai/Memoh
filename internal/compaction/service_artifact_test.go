package compaction

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestDoCompactionPersistsDurableCoverageAndAnchor(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"old question"`, 100),
		mkRow(t, "assistant", `"old answer"`, 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	for i := range rows {
		rows[i].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(int64(i+1) * 1000), Valid: true}
	}
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "SUMMARY"}

	if err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(stub, 200)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	coverage, err := DecodeArtifactCoverage(q.completed.Coverage)
	if err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if len(coverage) != 2 || coverage[0].Ref.ID != formatUUID(rows[0].ID) || coverage[1].Ref.ID != formatUUID(rows[1].ID) {
		t.Fatalf("coverage = %#v, want durable refs for the two compacted rows", coverage)
	}
	if coverage[0].Ref.ContentHash == "" || coverage[1].Ref.ContentHash == "" {
		t.Fatalf("coverage must preserve source hashes: %#v", coverage)
	}
	if q.completed.AnchorStartMs != 1000 || q.completed.AnchorEndMs != 2000 {
		t.Fatalf("anchor = %d..%d, want 1000..2000", q.completed.AnchorStartMs, q.completed.AnchorEndMs)
	}
}
