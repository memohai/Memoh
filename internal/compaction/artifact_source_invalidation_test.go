package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
		lineage:     []sqlc.BotHistoryMessageCompact{derived, invalidParent, validSibling},
		invalidIDs:  []pgtype.UUID{invalidParent.ID},
		unvalidated: true,
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
	if queries.fullLineageCalls != 1 || len(queries.payloadCalls) != 0 {
		t.Fatalf("non-empty invalid coverage calls = full %d, payload %#v, want full-lineage fallback", queries.fullLineageCalls, queries.payloadCalls)
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

func TestArtifactProjectionRestoresParentForEmptyCoverageDerivedSeed(t *testing.T) {
	t.Parallel()

	const (
		botID     = "00000000-0000-0000-0000-00000000ba06"
		sessionID = "00000000-0000-0000-0000-00000000fa06"
		parentID  = "00000000-0000-0000-0000-00000000ca06"
		derivedID = "00000000-0000-0000-0000-00000000ca07"
	)
	parent := projectionRow(t, parentID)
	derived := projectionRow(t, derivedID)
	for _, row := range []*sqlc.BotHistoryMessageCompact{&parent, &derived} {
		row.BotID = mustProjectionUUID(t, botID)
		row.SessionID = mustProjectionUUID(t, sessionID)
	}
	parent.SupersededBy, parent.SupersededAt = derived.ID, validTimestamp(1)
	derived.ArtifactLevel = 1
	derived.ParentIds = []pgtype.UUID{parent.ID}
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			parent.ID:  parent,
			derived.ID: derived,
		},
		lineage: []sqlc.BotHistoryMessageCompact{derived, parent},
		invalidSeeds: []sqlc.ListInvalidCompactionArtifactSeedsBySessionRow{{
			ID:       derived.ID,
			Coverage: []byte(`[]`),
		}},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), ArtifactOwner{
		BotID:          botID,
		SessionID:      sessionID,
		SessionIDKnown: true,
	})
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != parentID {
		t.Fatalf("frontier = %#v, want restored parent %q", frontier.Artifacts, parentID)
	}
}

func TestArtifactProjectionRestoresLeavesForNestedEmptyCoverageDerivedSeeds(t *testing.T) {
	t.Parallel()

	const (
		botID     = "00000000-0000-0000-0000-00000000ba07"
		sessionID = "00000000-0000-0000-0000-00000000fa07"
	)
	ids := []string{
		"00000000-0000-0000-0000-00000000cb01",
		"00000000-0000-0000-0000-00000000cb02",
		"00000000-0000-0000-0000-00000000cb03",
		"00000000-0000-0000-0000-00000000cb04",
		"00000000-0000-0000-0000-00000000cb05",
	}
	rows := make([]sqlc.BotHistoryMessageCompact, len(ids))
	for index, id := range ids {
		rows[index] = projectionRow(t, id)
		rows[index].BotID = mustProjectionUUID(t, botID)
		rows[index].SessionID = mustProjectionUUID(t, sessionID)
	}
	first, second, third, middle, top := &rows[0], &rows[1], &rows[2], &rows[3], &rows[4]
	first.SupersededBy, first.SupersededAt = middle.ID, validTimestamp(1)
	second.SupersededBy, second.SupersededAt = middle.ID, validTimestamp(1)
	middle.ArtifactLevel = 1
	middle.ParentIds = []pgtype.UUID{first.ID, second.ID}
	middle.SupersededBy, middle.SupersededAt = top.ID, validTimestamp(2)
	third.SupersededBy, third.SupersededAt = top.ID, validTimestamp(2)
	top.ArtifactLevel = 2
	top.ParentIds = []pgtype.UUID{middle.ID, third.ID}
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			first.ID: *first, second.ID: *second, third.ID: *third,
			middle.ID: *middle, top.ID: *top,
		},
		lineage: rows,
		invalidSeeds: []sqlc.ListInvalidCompactionArtifactSeedsBySessionRow{
			{ID: middle.ID, Coverage: []byte(`[]`)},
			{ID: top.ID, Coverage: []byte(`[]`)},
		},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), ArtifactOwner{
		BotID: botID, SessionID: sessionID, SessionIDKnown: true,
	})
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if got := artifactIDs(frontier.Artifacts); !equalArtifactIDs(got, ids[:3]) {
		t.Fatalf("nested fallback frontier = %#v, want leaves %#v", got, ids[:3])
	}
	if queries.fullLineageCalls != 0 || len(queries.payloadCalls) != 1 || !sameProjectionUUIDSet(queries.payloadCalls[0], first.ID, second.ID, third.ID) {
		t.Fatalf("nested fallback loads = full %d, payload %#v, want only leaf payloads", queries.fullLineageCalls, queries.payloadCalls)
	}
	for _, leaf := range []*sqlc.BotHistoryMessageCompact{first, second, third} {
		resolved, ok := frontier.Resolve(leaf.ID.String())
		if !ok || resolved.ID != leaf.ID.String() {
			t.Fatalf("Resolve(%s) = %#v, %v, want restored leaf", leaf.ID, resolved, ok)
		}
	}
	for _, excluded := range []*sqlc.BotHistoryMessageCompact{middle, top} {
		if _, ok := frontier.Resolve(excluded.ID.String()); ok {
			t.Fatalf("Resolve(%s) unexpectedly retained invalid derived alias", excluded.ID)
		}
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
	rows             map[pgtype.UUID]sqlc.BotHistoryMessageCompact
	lineage          []sqlc.BotHistoryMessageCompact
	invalidIDs       []pgtype.UUID
	invalidSeeds     []sqlc.ListInvalidCompactionArtifactSeedsBySessionRow
	invalidErr       error
	unvalidated      bool
	unvalidatedIDs   map[pgtype.UUID]struct{}
	payloadFn        func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error)
	metadataCalls    int
	fullLineageCalls int
	payloadCalls     [][]pgtype.UUID
}

func (q *sourceValidityProjectionQueries) GetCompactionLogByID(_ context.Context, id pgtype.UUID) (sqlc.BotHistoryMessageCompact, error) {
	row, ok := q.rows[id]
	if !ok {
		return sqlc.BotHistoryMessageCompact{}, pgx.ErrNoRows
	}
	return row, nil
}

func (q *sourceValidityProjectionQueries) ListCompactionArtifactLineageBySession(context.Context, pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	q.fullLineageCalls++
	return q.lineage, nil
}

func (q *sourceValidityProjectionQueries) ListCompactionArtifactLineageMetadataBySession(context.Context, pgtype.UUID) ([]sqlc.ListCompactionArtifactLineageMetadataBySessionRow, error) {
	q.metadataCalls++
	rows := projectionMetadataRows(q.lineage)
	for index := range rows {
		_, unvalidated := q.unvalidatedIDs[rows[index].ID]
		if q.unvalidated || unvalidated {
			rows[index].LineageValidated = false
		}
	}
	return rows, nil
}

func (q *sourceValidityProjectionQueries) ListCompactionArtifactPayloadsByIDs(_ context.Context, ids []pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	q.payloadCalls = append(q.payloadCalls, append([]pgtype.UUID(nil), ids...))
	if q.payloadFn != nil {
		return q.payloadFn(ids)
	}
	rows := make([]sqlc.BotHistoryMessageCompact, 0, len(ids))
	for _, id := range ids {
		if row, ok := q.rows[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
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
	if q.invalidSeeds != nil {
		return q.invalidSeeds, q.invalidErr
	}
	seeds := make([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, 0, len(q.invalidIDs))
	for _, id := range q.invalidIDs {
		seeds = append(seeds, sqlc.ListInvalidCompactionArtifactSeedsBySessionRow{ID: id, Coverage: q.rows[id].Coverage})
	}
	return seeds, q.invalidErr
}

func projectionMetadataRows(rows []sqlc.BotHistoryMessageCompact) []sqlc.ListCompactionArtifactLineageMetadataBySessionRow {
	metadata := make([]sqlc.ListCompactionArtifactLineageMetadataBySessionRow, 0, len(rows))
	for _, row := range rows {
		coverageCount := int32(0)
		var coverage []json.RawMessage
		if len(row.Coverage) > 0 && json.Unmarshal(row.Coverage, &coverage) != nil {
			coverageCount = -1
		} else if len(row.Coverage) > 0 {
			coverageCount = int32(len(coverage)) //nolint:gosec // test coverage is bounded
		}
		metadata = append(metadata, sqlc.ListCompactionArtifactLineageMetadataBySessionRow{
			ID: row.ID, BotID: row.BotID, SessionID: row.SessionID, Status: row.Status,
			HasSummary: strings.TrimSpace(row.Summary) != "", LineageValidated: true, CoverageCount: coverageCount,
			AnchorStartMs: row.AnchorStartMs, AnchorEndMs: row.AnchorEndMs,
			ArtifactLevel: row.ArtifactLevel, ParentIds: row.ParentIds,
			SupersededBy: row.SupersededBy, SupersededAt: row.SupersededAt, StartedAt: row.StartedAt,
		})
	}
	return metadata
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
