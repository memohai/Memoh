package compaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestArtifactFrontierSourceInvalidationPropagatesOnlyToDescendants(t *testing.T) {
	t.Parallel()

	invalidParent := testArtifact("invalid-parent")
	validSibling := testArtifact("valid-sibling")
	derived := testArtifact("derived")
	invalidParent.Coverage = testCoverage("row-invalid")
	validSibling.Coverage = testCoverage("row-valid")
	derived.Coverage = append(testCoverage("row-invalid"), testCoverage("row-valid")...)
	derived.Level = 1
	invalidParent.SupersededBy, invalidParent.SupersededAt = derived.ID, time.Unix(1, 0)
	validSibling.SupersededBy, validSibling.SupersededAt = derived.ID, time.Unix(1, 0)
	derived.ParentIDs = []string{invalidParent.ID, validSibling.ID}

	frontier := buildArtifactFrontierExcludingInvalidSources(
		[]Artifact{invalidParent, validSibling, derived},
		ArtifactOwner{},
		map[string][]CoveredSource{invalidParent.ID: invalidParent.Coverage},
	)

	if len(frontier.Issues) != 0 {
		t.Fatalf("source invalidation produced structural issues: %#v", frontier.Issues)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != validSibling.ID {
		t.Fatalf("frontier = %#v, want valid sibling fallback %q", frontier.Artifacts, validSibling.ID)
	}
	if resolved, ok := frontier.Resolve(validSibling.ID); !ok || resolved.ID != validSibling.ID {
		t.Fatalf("valid sibling resolution = %#v, %v", resolved, ok)
	}
	for _, id := range []string{invalidParent.ID, derived.ID} {
		if _, ok := frontier.Resolve(id); ok {
			t.Fatalf("source-invalid lineage %q unexpectedly resolved", id)
		}
	}
}

func TestArtifactFrontierSourceInvalidationUsesCoverageWithoutPoisoningReplacement(t *testing.T) {
	t.Parallel()

	invalidParent := testArtifact("invalid-parent-without-edges")
	validSibling := testArtifact("valid-sibling-with-edge")
	derived := testArtifact("derived-with-hidden-dependency")
	replacement := testArtifact("replacement")
	replacementDerived := testArtifact("replacement-derived")
	invalidParent.Coverage = testCoverage("row-invalid")
	validSibling.Coverage = testCoverage("row-valid")
	derived.Coverage = append(testCoverage("row-invalid"), testCoverage("row-valid")...)
	derived.Level = 1
	validSibling.SupersededBy, validSibling.SupersededAt = derived.ID, time.Unix(1, 0)
	derived.ParentIDs = []string{validSibling.ID}
	replacement.Coverage = testCoverage("row-invalid")
	replacement.SupersededBy, replacement.SupersededAt = replacementDerived.ID, time.Unix(2, 0)
	replacementDerived.Coverage = testCoverage("row-invalid")
	replacementDerived.Level = 1
	replacementDerived.ParentIDs = []string{replacement.ID}

	frontier := buildArtifactFrontierExcludingInvalidSources(
		[]Artifact{invalidParent, validSibling, derived, replacement, replacementDerived},
		ArtifactOwner{},
		map[string][]CoveredSource{invalidParent.ID: invalidParent.Coverage},
	)

	if len(frontier.Issues) != 0 {
		t.Fatalf("source invalidation produced structural issues: %#v", frontier.Issues)
	}
	if got := artifactIDs(frontier.Artifacts); !equalArtifactIDs(got, []string{replacementDerived.ID, validSibling.ID}) {
		t.Fatalf("frontier = %#v, want replacement successor and valid sibling fallback", got)
	}
	if resolved, ok := frontier.Resolve(replacement.ID); !ok || resolved.ID != replacementDerived.ID {
		t.Fatalf("reverted replacement resolution = %#v, %v", resolved, ok)
	}
	for _, id := range []string{invalidParent.ID, derived.ID} {
		if _, ok := frontier.Resolve(id); ok {
			t.Fatalf("source-invalid lineage %q unexpectedly resolved", id)
		}
	}
}

func TestArtifactProjectionAppliesSourceInvalidationForSessionAndPointLoads(t *testing.T) {
	t.Parallel()

	const (
		botID          = "00000000-0000-0000-0000-00000000ba01"
		sessionID      = "00000000-0000-0000-0000-00000000fa01"
		invalidID      = "00000000-0000-0000-0000-00000000ca01"
		validSiblingID = "00000000-0000-0000-0000-00000000ca02"
		derivedID      = "00000000-0000-0000-0000-00000000ca03"
	)
	invalidParent := projectionRow(t, invalidID)
	validSibling := projectionRow(t, validSiblingID)
	derived := projectionRow(t, derivedID)
	for _, row := range []*sqlc.BotHistoryMessageCompact{&invalidParent, &validSibling, &derived} {
		row.BotID = mustProjectionUUID(t, botID)
		row.SessionID = mustProjectionUUID(t, sessionID)
	}
	invalidParent.Coverage = testCoverageJSON(t, "row-invalid")
	validSibling.Coverage = testCoverageJSON(t, "row-valid")
	derived.Coverage = testCoverageJSON(t, "row-invalid", "row-valid")
	validSibling.SupersededBy, validSibling.SupersededAt = derived.ID, validTimestamp(1)
	derived.ArtifactLevel = 1
	derived.ParentIds = []pgtype.UUID{validSibling.ID}
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			invalidParent.ID: invalidParent,
			validSibling.ID:  validSibling,
			derived.ID:       derived,
		},
		lineage:    []sqlc.BotHistoryMessageCompact{derived, invalidParent, validSibling},
		invalidIDs: []pgtype.UUID{invalidParent.ID},
	}
	projection := NewArtifactProjection(queries)
	owner := ArtifactOwner{BotID: botID, SessionID: sessionID, SessionIDKnown: true}

	frontier, err := projection.LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != validSiblingID {
		t.Fatalf("session frontier = %#v, want valid sibling fallback", frontier.Artifacts)
	}
	active, err := projection.LoadActiveByID(context.Background(), validSiblingID, owner)
	if err != nil || active.ID != validSiblingID {
		t.Fatalf("LoadActiveByID(valid sibling) = %#v, %v", active, err)
	}
	_, err = projection.LoadActiveByID(context.Background(), invalidID, owner)
	var lineageErr *LineageError
	if !errors.As(err, &lineageErr) || lineageErr.Issue.Kind != LineageIssueInactiveSuccessor {
		t.Fatalf("LoadActiveByID(invalid source) error = %v, want inactive lineage", err)
	}
}

func TestArtifactProjectionAppliesSourceInvalidationToSessionlessLegacyArtifact(t *testing.T) {
	t.Parallel()

	row := projectionRow(t, "00000000-0000-0000-0000-00000000ca04")
	queries := &sourceValidityProjectionQueries{
		rows:       map[pgtype.UUID]sqlc.BotHistoryMessageCompact{row.ID: row},
		invalidIDs: []pgtype.UUID{row.ID},
	}

	_, err := NewArtifactProjection(queries).LoadActiveByID(context.Background(), row.ID.String(), ArtifactOwner{})
	var lineageErr *LineageError
	if !errors.As(err, &lineageErr) || lineageErr.Issue.Kind != LineageIssueInactiveSuccessor {
		t.Fatalf("LoadActiveByID(sessionless invalid source) error = %v, want inactive lineage", err)
	}
}

func TestArtifactProjectionPropagatesSourceValidityStorageError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("source validity unavailable")
	row := projectionRow(t, "00000000-0000-0000-0000-00000000ca05")
	row.SessionID = mustProjectionUUID(t, "00000000-0000-0000-0000-00000000fa02")
	queries := &sourceValidityProjectionQueries{
		rows:       map[pgtype.UUID]sqlc.BotHistoryMessageCompact{row.ID: row},
		invalidErr: sentinel,
	}
	_, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), ArtifactOwner{
		SessionID:      "00000000-0000-0000-0000-00000000fa02",
		SessionIDKnown: true,
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("LoadActiveSession() error = %v, want %v", err, sentinel)
	}
	_, err = NewArtifactProjection(queries).LoadActiveByID(context.Background(), row.ID.String(), ArtifactOwner{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("LoadActiveByID() error = %v, want %v", err, sentinel)
	}
}

func (*fakeQueries) ListInvalidCompactionArtifactSeedsBySession(context.Context, sqlc.ListInvalidCompactionArtifactSeedsBySessionParams) ([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, error) {
	return nil, nil
}

func (*projectionQueries) ListInvalidCompactionArtifactSeedsBySession(context.Context, sqlc.ListInvalidCompactionArtifactSeedsBySessionParams) ([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, error) {
	return nil, nil
}

type sourceValidityProjectionQueries struct {
	rows       map[pgtype.UUID]sqlc.BotHistoryMessageCompact
	lineage    []sqlc.BotHistoryMessageCompact
	invalidIDs []pgtype.UUID
	invalidErr error
}

func (q *sourceValidityProjectionQueries) GetCompactionLogByID(_ context.Context, id pgtype.UUID) (sqlc.BotHistoryMessageCompact, error) {
	row, ok := q.rows[id]
	if !ok {
		return sqlc.BotHistoryMessageCompact{}, pgx.ErrNoRows
	}
	return row, nil
}

func (q *sourceValidityProjectionQueries) ListCompactionArtifactLineageBySession(context.Context, pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	return q.lineage, nil
}

func (q *sourceValidityProjectionQueries) ListCompactionArtifactParentIDsBySuccessor(_ context.Context, arg sqlc.ListCompactionArtifactParentIDsBySuccessorParams) ([]pgtype.UUID, error) {
	var ids []pgtype.UUID
	for _, row := range q.rows {
		if row.Status == "ok" && row.SupersededBy == arg.SuccessorID && row.BotID == arg.BotID && row.SessionID == arg.SessionID {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func (q *sourceValidityProjectionQueries) ListInvalidCompactionArtifactSeedsBySession(context.Context, sqlc.ListInvalidCompactionArtifactSeedsBySessionParams) ([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, error) {
	seeds := make([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, 0, len(q.invalidIDs))
	for _, id := range q.invalidIDs {
		seeds = append(seeds, sqlc.ListInvalidCompactionArtifactSeedsBySessionRow{ID: id, Coverage: q.rows[id].Coverage})
	}
	return seeds, q.invalidErr
}

func artifactIDs(artifacts []Artifact) []string {
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		ids = append(ids, artifact.ID)
	}
	return ids
}

func equalArtifactIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
