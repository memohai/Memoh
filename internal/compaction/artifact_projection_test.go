package compaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestArtifactFrontierResolvesMultiLevelLineageToOneTerminal(t *testing.T) {
	t.Parallel()

	a := testArtifact("a")
	b := testArtifact("b")
	c := testArtifact("c")
	a.SupersededBy = b.ID
	a.SupersededAt = time.Unix(1, 0)
	b.ParentIDs = []string{a.ID}
	b.SupersededBy = c.ID
	b.SupersededAt = time.Unix(2, 0)
	c.ParentIDs = []string{b.ID}
	c.Coverage = testCoverage("row-c")

	frontier := buildArtifactFrontier([]Artifact{a, b, c})

	if len(frontier.Issues) != 0 {
		t.Fatalf("valid lineage issues = %#v", frontier.Issues)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != c.ID {
		t.Fatalf("frontier = %#v, want only terminal %q", frontier.Artifacts, c.ID)
	}
	for _, id := range []string{a.ID, b.ID, c.ID} {
		resolved, ok := frontier.Resolve(id)
		if !ok || resolved.ID != c.ID {
			t.Fatalf("Resolve(%q) = %#v, %v; want %q", id, resolved, ok, c.ID)
		}
	}
}

func TestArtifactFrontierQuarantinesBrokenLineageWithoutDroppingValidLeaf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		artifacts []Artifact
		startID   string
		issue     LineageIssueKind
	}{
		{
			name: "cycle",
			artifacts: func() []Artifact {
				a, b := testArtifact("cycle-a"), testArtifact("cycle-b")
				a.SupersededBy, a.SupersededAt, a.ParentIDs = b.ID, time.Unix(1, 0), []string{b.ID}
				b.SupersededBy, b.SupersededAt, b.ParentIDs = a.ID, time.Unix(2, 0), []string{a.ID}
				return []Artifact{a, b}
			}(),
			startID: "cycle-a",
			issue:   LineageIssueCycle,
		},
		{
			name: "missing successor",
			artifacts: func() []Artifact {
				a := testArtifact("missing-a")
				a.SupersededBy, a.SupersededAt = "missing-b", time.Unix(1, 0)
				return []Artifact{a}
			}(),
			startID: "missing-a",
			issue:   LineageIssueMissingSuccessor,
		},
		{
			name: "inactive successor",
			artifacts: func() []Artifact {
				a, b := testArtifact("inactive-a"), testArtifact("inactive-b")
				a.SupersededBy, a.SupersededAt = b.ID, time.Unix(1, 0)
				b.Status, b.ParentIDs = "pending", []string{a.ID}
				return []Artifact{a, b}
			}(),
			startID: "inactive-a",
			issue:   LineageIssueInactiveSuccessor,
		},
		{
			name: "inconsistent marker",
			artifacts: func() []Artifact {
				a, b := testArtifact("marker-a"), testArtifact("marker-b")
				a.SupersededBy = b.ID
				b.ParentIDs, b.Coverage = []string{a.ID}, testCoverage("marker-row")
				return []Artifact{a, b}
			}(),
			startID: "marker-a",
			issue:   LineageIssueInconsistentMarker,
		},
		{
			name: "parent mismatch",
			artifacts: func() []Artifact {
				a, b := testArtifact("parent-a"), testArtifact("parent-b")
				a.SupersededBy, a.SupersededAt = b.ID, time.Unix(1, 0)
				return []Artifact{a, b}
			}(),
			startID: "parent-a",
			issue:   LineageIssueParentMismatch,
		},
		{
			name: "scope mismatch",
			artifacts: func() []Artifact {
				a, b := testArtifact("scope-a"), testArtifact("scope-b")
				a.SupersededBy, a.SupersededAt = b.ID, time.Unix(1, 0)
				b.SessionID, b.ParentIDs, b.Coverage = "other-session", []string{a.ID}, testCoverage("scope-row")
				return []Artifact{a, b}
			}(),
			startID: "scope-a",
			issue:   LineageIssueScopeMismatch,
		},
		{
			name: "derived coverage missing",
			artifacts: func() []Artifact {
				a, b := testArtifact("coverage-a"), testArtifact("coverage-b")
				a.SupersededBy, a.SupersededAt = b.ID, time.Unix(1, 0)
				b.ParentIDs = []string{a.ID}
				return []Artifact{a, b}
			}(),
			startID: "coverage-a",
			issue:   LineageIssueMissingDerivedCoverage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			valid := testArtifact("valid-" + tt.name)
			valid.AnchorStartMs = 1
			frontier := buildArtifactFrontier(append(tt.artifacts, valid))

			if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != valid.ID {
				t.Fatalf("frontier = %#v, want only unrelated valid leaf %q", frontier.Artifacts, valid.ID)
			}
			if _, ok := frontier.Resolve(tt.startID); ok {
				t.Fatalf("broken lineage %q unexpectedly resolved", tt.startID)
			}
			if !hasLineageIssue(frontier.Issues, tt.issue) {
				t.Fatalf("issues = %#v, want kind %q", frontier.Issues, tt.issue)
			}
		})
	}
}

func TestArtifactProjectionLoadActiveByIDReturnsTypedLineageErrors(t *testing.T) {
	t.Parallel()

	aID := "00000000-0000-0000-0000-00000000ca01"
	bID := "00000000-0000-0000-0000-00000000ca02"
	a := projectionRow(t, aID)
	b := projectionRow(t, bID)
	a.SupersededBy, a.SupersededAt, a.ParentIds = b.ID, pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}, []pgtype.UUID{b.ID}
	b.SupersededBy, b.SupersededAt, b.ParentIds = a.ID, pgtype.Timestamptz{Time: time.Unix(2, 0), Valid: true}, []pgtype.UUID{a.ID}

	tests := []struct {
		name  string
		rows  map[pgtype.UUID]sqlc.BotHistoryMessageCompact
		issue LineageIssueKind
	}{
		{
			name:  "cycle",
			rows:  map[pgtype.UUID]sqlc.BotHistoryMessageCompact{a.ID: a, b.ID: b},
			issue: LineageIssueCycle,
		},
		{
			name: "missing successor",
			rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
				a.ID: func() sqlc.BotHistoryMessageCompact {
					row := projectionRow(t, aID)
					row.SupersededBy = b.ID
					row.SupersededAt = pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}
					return row
				}(),
			},
			issue: LineageIssueMissingSuccessor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			queries := &projectionQueries{rows: tt.rows}
			_, err := NewArtifactProjection(queries).LoadActiveByID(context.Background(), aID)
			var lineageErr *LineageError
			if !errors.As(err, &lineageErr) || lineageErr.Issue.Kind != tt.issue {
				t.Fatalf("LoadActiveByID error = %v, want lineage issue %q", err, tt.issue)
			}
		})
	}
}

func testArtifact(id string) Artifact {
	return Artifact{
		ID:        id,
		BotID:     "bot",
		SessionID: "session",
		Status:    "ok",
		Summary:   "summary " + id,
	}
}

func testCoverage(id string) []CoveredSource {
	return []CoveredSource{{
		Ref: contextfrag.ContextRef{
			Namespace:  "bot_history_message",
			ID:         id,
			Version:    1,
			Schema:     contextfrag.SchemaContextRef,
			Durability: contextfrag.RefDurable,
		},
	}}
}

func hasLineageIssue(issues []LineageIssue, kind LineageIssueKind) bool {
	for _, issue := range issues {
		if issue.Kind == kind {
			return true
		}
	}
	return false
}

type projectionQueries struct {
	dbstore.Queries
	rows map[pgtype.UUID]sqlc.BotHistoryMessageCompact
}

func (q *projectionQueries) GetCompactionLogByID(_ context.Context, id pgtype.UUID) (sqlc.BotHistoryMessageCompact, error) {
	row, ok := q.rows[id]
	if !ok {
		return sqlc.BotHistoryMessageCompact{}, pgx.ErrNoRows
	}
	return row, nil
}

func projectionRow(t *testing.T, id string) sqlc.BotHistoryMessageCompact {
	t.Helper()
	parsed, err := db.ParseUUID(id)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", id, err)
	}
	return sqlc.BotHistoryMessageCompact{ID: parsed, Status: "ok", Summary: "summary " + id}
}
