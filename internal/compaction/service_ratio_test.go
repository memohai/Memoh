package compaction

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRunCompactionRatioIgnoresNonCandidatePromptUsage(t *testing.T) {
	t.Parallel()

	rows := machineryCorpus(t)
	run := func(totalInputTokens int) []pgtype.UUID {
		queries := &fakeQueries{uncompacted: rows}
		service := newMachineryService(queries)
		cfg := machineryConfig(&stubModel{summary: "summary"}, 0)
		cfg.Ratio = 50
		cfg.TotalInputTokens = totalInputTokens
		if err := service.RunCompaction(context.Background(), cfg); err != nil {
			t.Fatalf("RunCompaction() error = %v", err)
		}
		return queries.markedIDs
	}

	withoutPromptOverhead := run(1_000)
	withPromptOverhead := run(10_000)
	if len(withoutPromptOverhead) == 0 || len(withoutPromptOverhead) != len(withPromptOverhead) {
		t.Fatalf("marked counts = %d and %d, want the same non-zero selection", len(withoutPromptOverhead), len(withPromptOverhead))
	}
	for i := range withoutPromptOverhead {
		if withoutPromptOverhead[i] != withPromptOverhead[i] {
			t.Fatalf("marked selection differs at %d", i)
		}
	}
}
