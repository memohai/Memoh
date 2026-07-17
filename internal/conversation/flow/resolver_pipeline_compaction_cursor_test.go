package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
)

var _ pipelineArtifactEventCursorReader = (*postgresstore.Queries)(nil)

func TestLoadPipelineCompactionArtifactsHydratesLegacyEventCursorsInOneBatch(t *testing.T) {
	t.Parallel()

	const (
		botID       = "11111111-1111-1111-1111-111111111111"
		sessionID   = "22222222-2222-2222-2222-222222222222"
		artifactAID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		artifactBID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		sourceAID   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		sourceBID   = "dddddddd-dddd-dddd-dddd-dddddddddddd"
		eventAID    = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
		eventBID    = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	)
	base := time.UnixMilli(1_000).UTC()
	sourceA := pipelineHistoryMessage(t, sourceAID, botID, sessionID, "external-a", base, "user", "source a")
	sourceB := pipelineHistoryMessage(t, sourceBID, botID, sessionID, "external-b", base.Add(time.Second), "user", "source b")
	queries := &cursorHydrationQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			pipelineArtifactRow(t, artifactBID, botID, sessionID, "summary b", []messagepkg.Message{sourceB}, base.Add(2*time.Minute)),
			pipelineArtifactRow(t, artifactAID, botID, sessionID, "summary a", []messagepkg.Message{sourceA}, base.Add(time.Minute)),
		},
		cursorRows: []sqlc.ListMessageEventCursorsByIDsRow{
			cursorHydrationRow(t, sourceA, artifactAID, eventAID, 41),
			cursorHydrationRow(t, sourceB, artifactBID, eventBID, 42),
		},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 2 || artifacts[0].Sources[0].EventCursor != 41 || artifacts[1].Sources[0].EventCursor != 42 {
		t.Fatalf("hydrated artifacts = %#v, want ordered cursors 41/42", artifacts)
	}
	if len(queries.cursorCalls) != 1 {
		t.Fatalf("cursor batch calls = %d, want 1", len(queries.cursorCalls))
	}
	call := queries.cursorCalls[0]
	if call.BotID != mustPGUUID(t, botID) || call.SessionID != mustPGUUID(t, sessionID) {
		t.Fatalf("cursor batch owner = %#v/%#v, want %s/%s", call.BotID, call.SessionID, botID, sessionID)
	}
	if len(call.MessageIds) != 2 || call.MessageIds[0] != mustPGUUID(t, sourceAID) || call.MessageIds[1] != mustPGUUID(t, sourceBID) {
		t.Fatalf("cursor batch ids = %#v, want source A/B once", call.MessageIds)
	}
}

func TestLoadPipelineCompactionArtifactsDoesNotHydrateCursorFromAnotherArtifact(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		foreignID  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		sourceID   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		eventID    = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	)
	createdAt := time.UnixMilli(1_000).UTC()
	source := pipelineHistoryMessage(t, sourceID, botID, sessionID, "external", createdAt, "user", "source")
	queries := &cursorHydrationQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{source}, createdAt.Add(time.Minute)),
		},
		cursorRows: []sqlc.ListMessageEventCursorsByIDsRow{
			cursorHydrationRow(t, source, foreignID, eventID, 41),
		},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Sources[0].EventCursor != 0 {
		t.Fatalf("foreign artifact cursor hydrated projection: %#v", artifacts)
	}
}

func TestLoadPipelineCompactionArtifactsRequiresExactCursorSourceRow(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		sourceID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		eventID    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	foreignBotID := mustPGUUID(t, "dddddddd-dddd-dddd-dddd-dddddddddddd")
	foreignSessionID := mustPGUUID(t, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	tests := []struct {
		name   string
		mutate func(*sqlc.ListMessageEventCursorsByIDsRow)
	}{
		{name: "different bot", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) { row.BotID = foreignBotID }},
		{name: "different session", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) { row.SessionID = foreignSessionID }},
		{name: "different external message", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) {
			row.ExternalMessageID = pgtype.Text{String: "other", Valid: true}
		}},
		{name: "different reply target", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) {
			row.SourceReplyToMessageID = pgtype.Text{String: "other-reply", Valid: true}
		}},
		{name: "different created at", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) {
			row.CreatedAt.Time = row.CreatedAt.Time.Add(time.Millisecond)
		}},
		{name: "missing event id", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) { row.EventID = pgtype.UUID{} }},
		{name: "zero cursor", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) { row.EventCursor = 0 }},
		{name: "negative cursor", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) { row.EventCursor = -1 }},
		{name: "cursor above JSON safe range", mutate: func(row *sqlc.ListMessageEventCursorsByIDsRow) { row.EventCursor = 1 << 53 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			createdAt := time.UnixMilli(1_000).UTC()
			source := pipelineHistoryMessage(t, sourceID, botID, sessionID, "external", createdAt, "user", "source")
			source.SourceReplyToMessageID = "reply"
			row := cursorHydrationRow(t, source, artifactID, eventID, 41)
			test.mutate(&row)
			queries := &cursorHydrationQueries{
				logs: []sqlc.BotHistoryMessageCompact{
					pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{source}, createdAt.Add(time.Minute)),
				},
				cursorRows: []sqlc.ListMessageEventCursorsByIDsRow{row},
			}
			resolver := &Resolver{queries: queries}
			scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

			artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
			if err != nil {
				t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
			}
			if len(artifacts) != 1 || artifacts[0].Sources[0].EventCursor != 0 {
				t.Fatalf("mismatched cursor row hydrated projection: %#v", artifacts)
			}
		})
	}
}

func TestLoadPipelineCompactionArtifactsLogsCursorHydrationFailureAndFailsOpen(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		sourceID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	createdAt := time.UnixMilli(1_000).UTC()
	source := pipelineHistoryMessage(t, sourceID, botID, sessionID, "external", createdAt, "user", "source")
	wantErr := errors.New("cursor rows unavailable")
	queries := &cursorHydrationQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{source}, createdAt.Add(time.Minute)),
		},
		cursorErr: wantErr,
	}
	var logs bytes.Buffer
	resolver := &Resolver{
		queries: queries,
		logger:  slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v, want fail-open", err)
	}
	if len(artifacts) != 1 || artifacts[0].Sources[0].EventCursor != 0 {
		t.Fatalf("failed hydration changed artifact projection: %#v", artifacts)
	}
	logLine := logs.String()
	if !strings.Contains(logLine, `"msg":"hydratePipelineArtifactEventCursors: failed to load source events"`) ||
		!strings.Contains(logLine, `"error":"cursor rows unavailable"`) {
		t.Fatalf("cursor hydration failure log = %q", logLine)
	}
}

func TestLoadPipelineCompactionArtifactsFailsOpenWithoutCursorHydrationData(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		sourceID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	createdAt := time.UnixMilli(1_000).UTC()
	source := pipelineHistoryMessage(t, sourceID, botID, sessionID, "external", createdAt, "user", "source")
	logs := []sqlc.BotHistoryMessageCompact{
		pipelineArtifactRow(t, artifactID, botID, sessionID, "summary", []messagepkg.Message{source}, createdAt.Add(time.Minute)),
	}
	tests := []struct {
		name    string
		queries dbstore.Queries
	}{
		{name: "capability missing", queries: &cursorCapabilityMissingQueries{logs: logs}},
		{name: "source row missing", queries: &cursorHydrationQueries{logs: logs}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver := &Resolver{queries: test.queries}
			scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")
			artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
			if err != nil {
				t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
			}
			if len(artifacts) != 1 || artifacts[0].Sources[0].EventCursor != 0 {
				t.Fatalf("missing hydration data changed projection: %#v", artifacts)
			}
		})
	}
}

func TestLoadPipelineCompactionArtifactsDoesNotQuerySupersededParentCursor(t *testing.T) {
	t.Parallel()

	const (
		botID         = "11111111-1111-1111-1111-111111111111"
		sessionID     = "22222222-2222-2222-2222-222222222222"
		activeID      = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		parentID      = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		childID       = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		activeSource  = "dddddddd-dddd-dddd-dddd-dddddddddddd"
		lineageSource = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
		eventID       = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	)
	base := time.UnixMilli(1_000).UTC()
	activeMessage := pipelineHistoryMessage(t, activeSource, botID, sessionID, "active", base, "user", "active")
	lineageMessage := pipelineHistoryMessage(t, lineageSource, botID, sessionID, "lineage", base.Add(time.Second), "user", "lineage")
	active := pipelineArtifactRow(t, activeID, botID, sessionID, "active summary", []messagepkg.Message{activeMessage}, base.Add(time.Minute))
	parent := pipelineArtifactRow(t, parentID, botID, sessionID, "parent summary", []messagepkg.Message{lineageMessage}, base.Add(2*time.Minute))
	child := artifactRowWithEventCursor(t,
		pipelineArtifactRow(t, childID, botID, sessionID, "child summary", []messagepkg.Message{lineageMessage}, base.Add(3*time.Minute)),
		42,
	)
	parent.SupersededBy = mustPGUUID(t, childID)
	parent.SupersededAt = pgtype.Timestamptz{Time: base.Add(3 * time.Minute), Valid: true}
	child.ParentIds = []pgtype.UUID{mustPGUUID(t, parentID)}
	queries := &cursorHydrationQueries{
		logs: []sqlc.BotHistoryMessageCompact{parent, child, active},
		cursorRows: []sqlc.ListMessageEventCursorsByIDsRow{
			cursorHydrationRow(t, activeMessage, activeID, eventID, 41),
		},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 2 || len(queries.cursorCalls) != 1 {
		t.Fatalf("active projection/cursor calls = %#v/%#v", artifacts, queries.cursorCalls)
	}
	ids := queries.cursorCalls[0].MessageIds
	if len(ids) != 1 || ids[0] != mustPGUUID(t, activeSource) {
		t.Fatalf("cursor batch ids = %#v, want only active zero-cursor source", ids)
	}
}

func TestLoadPipelineCompactionArtifactsHydratesSourceStillClaimedByLineageParent(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		parentID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		childID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		sourceID  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		eventID   = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	)
	base := time.UnixMilli(1_000).UTC()
	source := pipelineHistoryMessage(t, sourceID, botID, sessionID, "lineage", base, "user", "lineage source")
	parent := pipelineArtifactRow(t, parentID, botID, sessionID, "parent summary", []messagepkg.Message{source}, base.Add(time.Minute))
	child := pipelineArtifactRow(t, childID, botID, sessionID, "child summary", []messagepkg.Message{source}, base.Add(2*time.Minute))
	parent.SupersededBy = mustPGUUID(t, childID)
	parent.SupersededAt = pgtype.Timestamptz{Time: base.Add(2 * time.Minute), Valid: true}
	child.ParentIds = []pgtype.UUID{mustPGUUID(t, parentID)}
	queries := &cursorHydrationQueries{
		logs: []sqlc.BotHistoryMessageCompact{parent, child},
		cursorRows: []sqlc.ListMessageEventCursorsByIDsRow{
			cursorHydrationRow(t, source, parentID, eventID, 41),
		},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != childID || artifacts[0].Sources[0].EventCursor != 41 {
		t.Fatalf("lineage parent source hydration = %#v, want active child cursor 41", artifacts)
	}
}

type cursorHydrationQueries struct {
	dbstore.Queries
	logs        []sqlc.BotHistoryMessageCompact
	cursorRows  []sqlc.ListMessageEventCursorsByIDsRow
	cursorCalls []sqlc.ListMessageEventCursorsByIDsParams
	cursorErr   error
}

func (q *cursorHydrationQueries) ListCompactionArtifactLineageBySession(_ context.Context, _ pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	return q.logs, nil
}

func (q *cursorHydrationQueries) ListMessageEventCursorsByIDs(_ context.Context, arg sqlc.ListMessageEventCursorsByIDsParams) ([]sqlc.ListMessageEventCursorsByIDsRow, error) {
	q.cursorCalls = append(q.cursorCalls, arg)
	return q.cursorRows, q.cursorErr
}

type cursorCapabilityMissingQueries struct {
	dbstore.Queries
	logs []sqlc.BotHistoryMessageCompact
}

func (q *cursorCapabilityMissingQueries) ListCompactionArtifactLineageBySession(_ context.Context, _ pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	return q.logs, nil
}

func artifactRowWithEventCursor(t *testing.T, row sqlc.BotHistoryMessageCompact, cursor int64) sqlc.BotHistoryMessageCompact {
	t.Helper()
	var coverage []compaction.CoveredSource
	if err := json.Unmarshal(row.Coverage, &coverage); err != nil {
		t.Fatalf("decode artifact coverage: %v", err)
	}
	for i := range coverage {
		coverage[i].EventCursor = cursor
	}
	raw, err := json.Marshal(coverage)
	if err != nil {
		t.Fatalf("encode artifact coverage: %v", err)
	}
	row.Coverage = raw
	return row
}

func cursorHydrationRow(t *testing.T, source messagepkg.Message, compactID, eventID string, cursor int64) sqlc.ListMessageEventCursorsByIDsRow {
	t.Helper()
	return sqlc.ListMessageEventCursorsByIDsRow{
		ID:                     mustPGUUID(t, source.ID),
		BotID:                  mustPGUUID(t, source.BotID),
		SessionID:              mustPGUUID(t, source.SessionID),
		CompactID:              mustPGUUID(t, compactID),
		ExternalMessageID:      pgtype.Text{String: source.ExternalMessageID, Valid: source.ExternalMessageID != ""},
		SourceReplyToMessageID: pgtype.Text{String: source.SourceReplyToMessageID, Valid: source.SourceReplyToMessageID != ""},
		EventID:                mustPGUUID(t, eventID),
		CreatedAt:              pgtype.Timestamptz{Time: source.CreatedAt, Valid: true},
		EventCursor:            cursor,
	}
}
