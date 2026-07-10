package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
)

func TestReplaceCompactedMessagesLoadsSessionSummaryWithoutRecentRows(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f003"
	compactID := "00000000-0000-0000-0000-00000000c003"
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			{
				ID:        mustPGUUID(t, compactID),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "older condensed context",
			},
		},
	}
	resolver := &Resolver{queries: queries}
	recent := []historyfrag.HistoryRecord{
		historyRecord("row-current", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("current")}, nil),
	}

	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, recent)

	if queries.sessionID != mustPGUUID(t, sessionID) {
		t.Fatalf("queried session id = %#v, want %s", queries.sessionID, sessionID)
	}
	if len(got) != 2 {
		t.Fatalf("records = %d, want summary plus recent row: %#v", len(got), got)
	}
	if got[0].CompactID != compactID || got[0].Kind != contextfrag.KindConversationSummary || got[0].Lifecycle != historyfrag.LifecycleActiveSummary {
		t.Fatalf("first record is not loaded active summary: %#v", got[0])
	}
	if got[0].ModelMessage.TextContent() != "<summary>\nolder condensed context\n</summary>" {
		t.Fatalf("summary text mismatch: %q", got[0].ModelMessage.TextContent())
	}
	if got[1].DBMessageID != "row-current" {
		t.Fatalf("recent row lost or reordered: %#v", got)
	}
}

func TestReplaceCompactedMessagesLoadsSessionSummaryCoverageFromCompactedRows(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f004"
	compactID := "00000000-0000-0000-0000-00000000c004"
	coverage, err := json.Marshal([]compaction.CoveredSource{
		{Ordinal: 0, Ref: contextfrag.ContextRef{Namespace: "bot_history_message", ID: "00000000-0000-0000-0000-000000000401", Version: 1, Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable, ContentHash: "hash-401", HashAlgo: contextfrag.HashAlgoSHA256, HashScope: contextfrag.HashScopeSourcePayload}},
		{Ordinal: 1, Ref: contextfrag.ContextRef{Namespace: "bot_history_message", ID: "00000000-0000-0000-0000-000000000402", Version: 1, Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable, ContentHash: "hash-402", HashAlgo: contextfrag.HashAlgoSHA256, HashScope: contextfrag.HashScopeSourcePayload}},
	})
	if err != nil {
		t.Fatalf("marshal coverage: %v", err)
	}
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			{
				ID:        mustPGUUID(t, compactID),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "older condensed context",
				Coverage:  coverage,
			},
		},
	}
	resolver := &Resolver{queries: queries}

	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, nil)

	if len(got) != 1 {
		t.Fatalf("records = %d, want one session summary: %#v", len(got), got)
	}
	if got[0].Coverage == nil || len(got[0].Coverage.CoveredRefs) != 2 {
		t.Fatalf("summary coverage = %#v, want covered message refs", got[0].Coverage)
	}
	if got[0].Coverage.CoveredRefs[0].ID != "00000000-0000-0000-0000-000000000401" ||
		got[0].Coverage.CoveredRefs[1].ID != "00000000-0000-0000-0000-000000000402" {
		t.Fatalf("covered refs mismatch: %#v", got[0].Coverage.CoveredRefs)
	}
	if len(queries.coveredCalls) != 0 {
		t.Fatalf("coverage path must not request full message content, called: %#v", queries.coveredCalls)
	}
	if len(queries.refCalls) != 0 {
		t.Fatalf("persisted artifact coverage must not query message refs, called: %#v", queries.refCalls)
	}
	frags := historyContextFragsForMessages(historyfrag.ToModelMessages(got), got)
	if len(frags) != 1 || frags[0].Coverage == nil || len(frags[0].Coverage.CoveredRefs) != 2 {
		t.Fatalf("summary frag lost loaded coverage: %#v", frags)
	}
}

func TestReplaceCompactedMessagesRejectsMalformedPersistedCoverage(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f008"
	compactID := "00000000-0000-0000-0000-00000000c008"
	coveredID := "00000000-0000-0000-0000-000000000801"
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			{
				ID:        mustPGUUID(t, compactID),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "recoverable condensed context",
				Coverage:  []byte(`{"unexpected":"shape"}`),
			},
		},
		refs: map[pgtype.UUID][]sqlc.ListMessageRefsByCompactIDRow{
			mustPGUUID(t, compactID): {{ID: mustPGUUID(t, coveredID)}},
		},
	}
	resolver := &Resolver{queries: queries}
	recent := []historyfrag.HistoryRecord{
		historyRecord(coveredID, conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("edited raw")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = compactID
		}),
	}

	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, recent)

	if len(got) != 1 || got[0].DBMessageID != coveredID || got[0].ModelMessage.TextContent() != "edited raw" {
		t.Fatalf("malformed non-empty coverage replaced current raw history: %#v", got)
	}
	if len(queries.refCalls) != 0 {
		t.Fatalf("malformed non-empty coverage used legacy refs-only fallback: %#v", queries.refCalls)
	}
}

func TestReplaceCompactedMessagesInWindowGroupCoversRowsOutsideLoadWindow(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f007"
	compactID := "00000000-0000-0000-0000-00000000c007"
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			{
				ID:        mustPGUUID(t, compactID),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "condensed context",
			},
		},
		refs: map[pgtype.UUID][]sqlc.ListMessageRefsByCompactIDRow{
			mustPGUUID(t, compactID): {
				{ID: mustPGUUID(t, "00000000-0000-0000-0000-000000000501")},
				{ID: mustPGUUID(t, "00000000-0000-0000-0000-000000000502")},
			},
		},
	}
	resolver := &Resolver{queries: queries}
	// Only the newer half of the compact group is inside the loaded window; row
	// ...501 already aged out of the 24h load slice but is still part of the group.
	recent := []historyfrag.HistoryRecord{
		historyRecord("00000000-0000-0000-0000-000000000502", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old")}, func(r *historyfrag.HistoryRecord) {
			r.CompactID = compactID
		}),
	}

	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, recent)

	if len(got) != 1 || got[0].Kind != contextfrag.KindConversationSummary {
		t.Fatalf("expected single in-window summary record, got %#v", got)
	}
	if got[0].Coverage == nil || len(got[0].Coverage.CoveredRefs) != 2 {
		t.Fatalf("in-window summary should cover the full compact group, got %#v", got[0].Coverage)
	}
	if got[0].Coverage.CoveredRefs[0].ID != "00000000-0000-0000-0000-000000000501" {
		t.Fatalf("covered refs should include the row outside the load window: %#v", got[0].Coverage.CoveredRefs)
	}
	for _, ref := range got[0].Coverage.CoveredRefs {
		if ref.ContentHash != "" || ref.HashAlgo != "" || ref.HashScope != "" {
			t.Fatalf("refs-only legacy coverage must not claim a source hash: %#v", ref)
		}
	}
	if len(queries.coveredCalls) != 0 {
		t.Fatalf("coverage path must not request full message content, called: %#v", queries.coveredCalls)
	}
	if len(queries.refCalls) != 1 || queries.refCalls[0] != mustPGUUID(t, compactID) {
		t.Fatalf("refs-only query should be called once for the compact group, got: %#v", queries.refCalls)
	}
}

func TestReplaceCompactedMessagesResolvesInWindowGroupsFromSessionLogs(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f005"
	inWindowCompact := "00000000-0000-0000-0000-00000000c005"
	outOfWindowCompact := "00000000-0000-0000-0000-00000000c006"
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			{
				ID:        mustPGUUID(t, inWindowCompact),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "in-window condensed context",
			},
			{
				ID:        mustPGUUID(t, outOfWindowCompact),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "aged-out condensed context",
			},
		},
		refs: map[pgtype.UUID][]sqlc.ListMessageRefsByCompactIDRow{
			mustPGUUID(t, inWindowCompact):    {{ID: mustPGUUID(t, "00000000-0000-0000-0000-000000000600")}},
			mustPGUUID(t, outOfWindowCompact): {{ID: mustPGUUID(t, "00000000-0000-0000-0000-000000000601")}},
		},
	}
	resolver := &Resolver{queries: queries}
	recent := []historyfrag.HistoryRecord{
		historyRecord("row-compacted", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old")}, func(r *historyfrag.HistoryRecord) {
			r.CompactID = inWindowCompact
		}),
		historyRecord("row-current", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("current")}, nil),
	}

	// The fake does not implement GetCompactionLogByID: resolving in-window
	// groups must come from the single session log load, not per-group lookups.
	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, recent)

	if queries.listCalls != 1 {
		t.Fatalf("session logs loaded %d times, want exactly once", queries.listCalls)
	}
	if len(got) != 3 {
		t.Fatalf("records = %d, want prepended summary + in-window summary + recent row: %#v", len(got), got)
	}
	if got[0].CompactID != outOfWindowCompact || got[0].Kind != contextfrag.KindConversationSummary {
		t.Fatalf("first record should be the aged-out session summary: %#v", got[0])
	}
	if got[1].CompactID != inWindowCompact || got[1].Kind != contextfrag.KindConversationSummary {
		t.Fatalf("in-window group was not replaced by its summary: %#v", got[1])
	}
	if got[2].DBMessageID != "row-current" {
		t.Fatalf("recent row lost or reordered: %#v", got)
	}
	if len(queries.coveredCalls) != 0 {
		t.Fatalf("coverage path must not request full message content, called: %#v", queries.coveredCalls)
	}
	wantRefCalls := map[pgtype.UUID]bool{
		mustPGUUID(t, inWindowCompact):    true,
		mustPGUUID(t, outOfWindowCompact): true,
	}
	if len(queries.refCalls) != len(wantRefCalls) {
		t.Fatalf("refs-only query calls = %#v, want exactly one per compact group %#v", queries.refCalls, wantRefCalls)
	}
	for _, called := range queries.refCalls {
		if !wantRefCalls[called] {
			t.Fatalf("unexpected refs-only query for compact id: %#v", called)
		}
	}
}

func TestReplaceCompactedMessagesResolvesSupersededGroupToActiveArtifact(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f009"
	parentID := "00000000-0000-0000-0000-00000000c009"
	activeID := "00000000-0000-0000-0000-00000000c010"
	coverage := persistedCoverage(t, "row-parent")
	supersededAt := pgtype.Timestamptz{Time: time.Unix(10, 0), Valid: true}
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{
			{
				ID:           mustPGUUID(t, parentID),
				SessionID:    mustPGUUID(t, sessionID),
				Status:       "ok",
				Summary:      "stale parent summary",
				Coverage:     coverage,
				SupersededBy: mustPGUUID(t, activeID),
				SupersededAt: supersededAt,
			},
			{
				ID:        mustPGUUID(t, activeID),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "active restacked summary",
				Coverage:  coverage,
				ParentIds: []pgtype.UUID{mustPGUUID(t, parentID)},
			},
		},
	}
	resolver := &Resolver{queries: queries}
	recent := []historyfrag.HistoryRecord{
		historyRecord("row-parent", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("covered by parent")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = parentID
		}),
	}

	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, recent)

	if len(got) != 1 || got[0].CompactID != activeID {
		t.Fatalf("superseded raw group must resolve once to active artifact %s: %#v", activeID, got)
	}
	if got[0].ModelMessage.TextContent() != "<summary>\nactive restacked summary\n</summary>" {
		t.Fatalf("resolved summary = %q", got[0].ModelMessage.TextContent())
	}
}

func TestReplaceCompactedMessagesReconcilesStaleRawRowsByDurableCoverage(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f010"
	compactID := "00000000-0000-0000-0000-00000000c013"
	coveredID := "00000000-0000-0000-0000-000000000910"
	for _, required := range []bool{false, true} {
		t.Run(fmt.Sprintf("required=%v", required), func(t *testing.T) {
			t.Parallel()
			queries := &recordingCompactionLogQueries{
				logs: []sqlc.BotHistoryMessageCompact{{
					ID:        mustPGUUID(t, compactID),
					SessionID: mustPGUUID(t, sessionID),
					Status:    "ok",
					Summary:   "newly completed summary",
					Coverage:  persistedCoverage(t, coveredID),
				}},
			}
			resolver := &Resolver{queries: queries}
			recent := []historyfrag.HistoryRecord{
				historyRecord("before-row", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("before")}, nil),
				historyRecord(coveredID, conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("stale raw")}, func(record *historyfrag.HistoryRecord) {
					record.Required = required
				}),
				historyRecord("after-row", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("after")}, nil),
			}

			got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, recent)

			wantText := []string{"before", "<summary>\nnewly completed summary\n</summary>"}
			if required {
				wantText = append(wantText, "stale raw")
			}
			wantText = append(wantText, "after")
			if gotText := recordTexts(got); !reflect.DeepEqual(gotText, wantText) {
				t.Fatalf("reconciled history = %#v, want %#v", gotText, wantText)
			}
		})
	}
}

func TestReplaceCompactedMessagesPropagatesFrontierStorageError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("frontier database unavailable")
	resolver := &Resolver{queries: &recordingCompactionLogQueries{listErr: sentinel}}
	_, err := resolver.replaceCompactedMessages(
		context.Background(),
		"00000000-0000-0000-0000-00000000f011",
		contextfrag.Scope{},
		[]historyfrag.HistoryRecord{historyRecord("row-current", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("current")}, nil)},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("replaceCompactedMessages error = %v, want %v", err, sentinel)
	}
}

func TestReplaceRecentCompactedMessagesFollowsSupersession(t *testing.T) {
	t.Parallel()

	parentID := "00000000-0000-0000-0000-00000000c011"
	activeID := "00000000-0000-0000-0000-00000000c012"
	coverage := persistedCoverage(t, "row-parent")
	parent := sqlc.BotHistoryMessageCompact{
		ID:           mustPGUUID(t, parentID),
		Status:       "ok",
		Summary:      "stale parent summary",
		Coverage:     coverage,
		SupersededBy: mustPGUUID(t, activeID),
		SupersededAt: pgtype.Timestamptz{Time: time.Unix(10, 0), Valid: true},
	}
	active := sqlc.BotHistoryMessageCompact{
		ID:        mustPGUUID(t, activeID),
		Status:    "ok",
		Summary:   "active restacked summary",
		Coverage:  coverage,
		ParentIds: []pgtype.UUID{mustPGUUID(t, parentID)},
	}
	queries := &recordingCompactionLogQueries{
		byID: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			parent.ID: parent,
			active.ID: active,
		},
	}
	resolver := &Resolver{queries: queries}
	recent := []historyfrag.HistoryRecord{
		historyRecord("row-parent", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("covered by parent")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = parentID
		}),
	}

	got := mustReplaceCompactedMessages(t, resolver, "", contextfrag.Scope{}, recent)

	if len(got) != 1 || got[0].CompactID != activeID {
		t.Fatalf("sessionless superseded group must resolve to active artifact %s: %#v", activeID, got)
	}
	if len(queries.getCalls) != 2 || queries.getCalls[0] != parent.ID || queries.getCalls[1] != active.ID {
		t.Fatalf("lineage lookups = %#v, want parent then active", queries.getCalls)
	}
}

func TestReplaceRecentCompactedMessagesUsesRecordSessionAsOwner(t *testing.T) {
	t.Parallel()

	botID := "00000000-0000-0000-0000-00000000b101"
	recordSessionID := "00000000-0000-0000-0000-00000000f101"
	foreignSessionID := "00000000-0000-0000-0000-00000000f102"
	foreignCompactID := "00000000-0000-0000-0000-00000000c101"
	foreign := sqlc.BotHistoryMessageCompact{
		ID:        mustPGUUID(t, foreignCompactID),
		BotID:     mustPGUUID(t, botID),
		SessionID: mustPGUUID(t, foreignSessionID),
		Status:    "ok",
		Summary:   "foreign session summary",
	}
	queries := &recordingCompactionLogQueries{
		byID: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{foreign.ID: foreign},
	}
	resolver := &Resolver{queries: queries}
	recent := []historyfrag.HistoryRecord{
		historyRecord("row-owned-by-session", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("keep local raw")}, func(record *historyfrag.HistoryRecord) {
			record.BotID = botID
			record.SessionID = recordSessionID
			record.CompactID = foreignCompactID
		}),
	}

	got := mustReplaceCompactedMessages(t, resolver, "", contextfrag.Scope{BotID: botID}, recent)

	if gotText := recordTexts(got); !reflect.DeepEqual(gotText, []string{"keep local raw"}) {
		t.Fatalf("cross-session artifact replaced local history: %#v", gotText)
	}
	if len(queries.getCalls) != 0 {
		t.Fatalf("session-owned group fell back to unscoped point lookup: %#v", queries.getCalls)
	}
}
