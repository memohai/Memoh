package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestLoadActiveSessionHydratesOnlyActiveTerminalPayload(t *testing.T) {
	t.Parallel()

	owner, parents, terminal := projectionPayloadLineage(t)
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			parents[0].ID: parents[0],
			parents[1].ID: parents[1],
			terminal.ID:   terminal,
		},
		lineage: []sqlc.BotHistoryMessageCompact{parents[0], parents[1], terminal},
	}

	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if queries.metadataCalls != 1 || queries.fullLineageCalls != 0 || len(queries.payloadCalls) != 1 ||
		!sameProjectionUUIDSet(queries.payloadCalls[0], terminal.ID) {
		t.Fatalf("projection calls = metadata %d, full %d, payload %#v", queries.metadataCalls, queries.fullLineageCalls, queries.payloadCalls)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != terminal.ID.String() || frontier.Artifacts[0].Summary != terminal.Summary {
		t.Fatalf("frontier = %#v, want hydrated terminal %s", frontier.Artifacts, terminal.ID)
	}
	for _, id := range []pgtype.UUID{parents[0].ID, parents[1].ID, terminal.ID} {
		resolved, ok := frontier.Resolve(id.String())
		if !ok || resolved.ID != terminal.ID.String() {
			t.Fatalf("Resolve(%s) = %#v, %v, want terminal", id, resolved, ok)
		}
	}
}

func TestLoadActiveSessionFallsBackForUnvalidatedLineage(t *testing.T) {
	t.Parallel()

	owner, terminal := projectionPayloadTerminal(t)
	queries := &sourceValidityProjectionQueries{
		rows:        map[pgtype.UUID]sqlc.BotHistoryMessageCompact{terminal.ID: terminal},
		lineage:     []sqlc.BotHistoryMessageCompact{terminal},
		unvalidated: true,
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if queries.fullLineageCalls != 1 || len(queries.payloadCalls) != 0 {
		t.Fatalf("unvalidated loads = full %d, payload %#v, want full lineage", queries.fullLineageCalls, queries.payloadCalls)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != terminal.ID.String() {
		t.Fatalf("frontier = %#v, want legacy terminal", frontier.Artifacts)
	}
}

func TestLoadActiveSessionFallsBackForMixedValidationProvenance(t *testing.T) {
	t.Parallel()

	owner, parents, terminal := projectionPayloadLineage(t)
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			parents[0].ID: parents[0],
			parents[1].ID: parents[1],
			terminal.ID:   terminal,
		},
		lineage: []sqlc.BotHistoryMessageCompact{parents[0], parents[1], terminal},
		unvalidatedIDs: map[pgtype.UUID]struct{}{
			parents[0].ID: {},
		},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if queries.fullLineageCalls != 1 || len(queries.payloadCalls) != 0 {
		t.Fatalf("mixed-provenance loads = full %d, payload %#v, want full lineage", queries.fullLineageCalls, queries.payloadCalls)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != terminal.ID.String() {
		t.Fatalf("frontier = %#v, want resolved terminal", frontier.Artifacts)
	}
}

func TestLoadActiveSessionKeepsValidatedInvalidationOnMetadataPath(t *testing.T) {
	t.Parallel()

	owner, parents, terminal := projectionPayloadLineage(t)
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			parents[0].ID: parents[0],
			parents[1].ID: parents[1],
			terminal.ID:   terminal,
		},
		lineage:    []sqlc.BotHistoryMessageCompact{parents[0], parents[1], terminal},
		invalidIDs: []pgtype.UUID{parents[0].ID},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if queries.fullLineageCalls != 0 || len(queries.payloadCalls) != 1 || !sameProjectionUUIDSet(queries.payloadCalls[0], parents[1].ID) {
		t.Fatalf("validated invalidation loads = full %d, payload %#v, want valid fallback only", queries.fullLineageCalls, queries.payloadCalls)
	}
	if len(frontier.Artifacts) != 1 || frontier.Artifacts[0].ID != parents[1].ID.String() {
		t.Fatalf("frontier = %#v, want valid parent fallback", frontier.Artifacts)
	}
}

func TestLoadActiveSessionRejectsInvalidPayloadResultSet(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("payload unavailable")
	tests := []struct {
		name      string
		payloadFn func(sqlc.BotHistoryMessageCompact) func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error)
		want      string
		wantErr   error
	}{
		{name: "missing", payloadFn: func(sqlc.BotHistoryMessageCompact) func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
			return func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) { return nil, nil }
		}, want: "got 0 artifacts, want 1"},
		{name: "duplicate", payloadFn: func(row sqlc.BotHistoryMessageCompact) func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
			return func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
				return []sqlc.BotHistoryMessageCompact{row, row}, nil
			}
		}, want: "duplicate artifact"},
		{name: "unexpected", payloadFn: func(row sqlc.BotHistoryMessageCompact) func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
			return func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
				other := row
				other.ID = mustProjectionUUID(t, "00000000-0000-0000-0000-00000000deff")
				return []sqlc.BotHistoryMessageCompact{row, other}, nil
			}
		}, want: "unexpected artifact"},
		{name: "storage error", payloadFn: func(sqlc.BotHistoryMessageCompact) func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
			return func([]pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) { return nil, sentinel }
		}, wantErr: sentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, terminal := projectionPayloadTerminal(t)
			queries := &sourceValidityProjectionQueries{
				rows:      map[pgtype.UUID]sqlc.BotHistoryMessageCompact{terminal.ID: terminal},
				lineage:   []sqlc.BotHistoryMessageCompact{terminal},
				payloadFn: tt.payloadFn(terminal),
			}
			frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("LoadActiveSession() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) || len(frontier.Artifacts) != 0 {
				t.Fatalf("LoadActiveSession() = %#v, %v, want error containing %q", frontier, err, tt.want)
			}
		})
	}
}

func TestLoadActiveSessionQuarantinesMalformedHydratedPayload(t *testing.T) {
	t.Parallel()

	owner, terminal := projectionPayloadTerminal(t)
	malformed := terminal
	var coverage []CoveredSource
	if err := json.Unmarshal(terminal.Coverage, &coverage); err != nil {
		t.Fatalf("decode test coverage: %v", err)
	}
	coverage[0].Ref.ContentHash = ""
	malformed.Coverage, _ = json.Marshal(coverage)
	queries := &sourceValidityProjectionQueries{
		rows:    map[pgtype.UUID]sqlc.BotHistoryMessageCompact{terminal.ID: malformed},
		lineage: []sqlc.BotHistoryMessageCompact{terminal},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if len(frontier.Artifacts) != 0 || !hasLineageIssue(frontier.Issues, LineageIssueMalformedCoverage) {
		t.Fatalf("frontier = %#v, want malformed payload quarantine", frontier)
	}
}

func TestLoadActiveSessionRejectsPayloadMetadataDrift(t *testing.T) {
	t.Parallel()

	owner, terminal := projectionPayloadTerminal(t)
	drifted := terminal
	drifted.Coverage = testCoverageJSON(t, "row-a")
	queries := &sourceValidityProjectionQueries{
		rows:    map[pgtype.UUID]sqlc.BotHistoryMessageCompact{terminal.ID: drifted},
		lineage: []sqlc.BotHistoryMessageCompact{terminal},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err == nil || !strings.Contains(err.Error(), "changed after projection") || len(frontier.Artifacts) != 0 {
		t.Fatalf("LoadActiveSession() = %#v, %v, want fail-closed metadata drift", frontier, err)
	}
}

func TestLoadActiveSessionQuarantinesHydratedCoverageOverlap(t *testing.T) {
	t.Parallel()

	owner, first := projectionPayloadTerminal(t)
	second := first
	second.ID = mustProjectionUUID(t, "00000000-0000-0000-0000-00000000dd04")
	second.Summary = "second terminal"
	queries := &sourceValidityProjectionQueries{
		rows: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			first.ID:  first,
			second.ID: second,
		},
		lineage: []sqlc.BotHistoryMessageCompact{first, second},
	}
	frontier, err := NewArtifactProjection(queries).LoadActiveSession(context.Background(), owner)
	if err != nil {
		t.Fatalf("LoadActiveSession() error = %v", err)
	}
	if len(frontier.Artifacts) != 0 || !hasLineageIssue(frontier.Issues, LineageIssueCoverageOverlap) {
		t.Fatalf("frontier = %#v, want overlap quarantine", frontier)
	}
	if len(queries.payloadCalls) != 1 || !sameProjectionUUIDSet(queries.payloadCalls[0], first.ID, second.ID) {
		t.Fatalf("payload calls = %#v, want both planned terminals", queries.payloadCalls)
	}
}

func projectionPayloadLineage(t *testing.T) (ArtifactOwner, [2]sqlc.BotHistoryMessageCompact, sqlc.BotHistoryMessageCompact) {
	t.Helper()
	const (
		botID     = "00000000-0000-0000-0000-00000000bd01"
		sessionID = "00000000-0000-0000-0000-00000000fd01"
	)
	parents := [2]sqlc.BotHistoryMessageCompact{
		projectionRow(t, "00000000-0000-0000-0000-00000000dd01"),
		projectionRow(t, "00000000-0000-0000-0000-00000000dd02"),
	}
	terminal := projectionRow(t, "00000000-0000-0000-0000-00000000dd03")
	for index := range parents {
		parents[index].BotID = mustProjectionUUID(t, botID)
		parents[index].SessionID = mustProjectionUUID(t, sessionID)
		parents[index].Coverage = testCoverageJSON(t, "row-"+string(rune('a'+index)))
		parents[index].SupersededBy, parents[index].SupersededAt = terminal.ID, validTimestamp(int64(index+1))
	}
	terminal.BotID = mustProjectionUUID(t, botID)
	terminal.SessionID = mustProjectionUUID(t, sessionID)
	terminal.Coverage = testCoverageJSON(t, "row-a", "row-b")
	terminal.ArtifactLevel = 1
	terminal.ParentIds = []pgtype.UUID{parents[0].ID, parents[1].ID}
	return ArtifactOwner{BotID: botID, SessionID: sessionID, SessionIDKnown: true}, parents, terminal
}

func projectionPayloadTerminal(t *testing.T) (ArtifactOwner, sqlc.BotHistoryMessageCompact) {
	t.Helper()
	owner, _, terminal := projectionPayloadLineage(t)
	terminal.ArtifactLevel = 0
	terminal.ParentIds = nil
	return owner, terminal
}

func sameProjectionUUIDSet(got []pgtype.UUID, want ...pgtype.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	remaining := make(map[pgtype.UUID]int, len(want))
	for _, id := range want {
		remaining[id]++
	}
	for _, id := range got {
		if remaining[id] == 0 {
			return false
		}
		remaining[id]--
	}
	return true
}
