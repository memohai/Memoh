package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

func TestDedupePersistedCurrentUserMessageUsesHistoryRecordProvenance(t *testing.T) {
	t.Parallel()

	history := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("---\nmessage-id: qq-msg-1\nchannel: qq\n---\nhello"),
		}, func(record *historyfrag.HistoryRecord) {
			record.ExternalMessageID = "qq-msg-1"
			record.Platform = "qq"
			record.SenderChannelIdentityID = "channel-identity-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("ok"),
		}, nil),
	}

	got := dedupePersistedCurrentUserMessage(history, conversation.ChatRequest{
		UserMessagePersisted:    true,
		RouteID:                 "route-1",
		ExternalMessageID:       "qq-msg-1",
		CurrentChannel:          "qq",
		SourceChannelIdentityID: "channel-identity-1",
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 message after dedupe, got %d", len(got))
	}
	if got[0].ModelMessage.Role != "assistant" {
		t.Fatalf("unexpected remaining role: %s", got[0].ModelMessage.Role)
	}
}

func TestReplaceCompactedHistoryRecordsUsesTypedSummaryRecord(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old 1")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old 2")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
		historyRecord("row-3", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("new")}, nil),
	}

	got := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": "condensed"}, contextfrag.Scope{})
	wantMessages := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("<summary>\ncondensed\n</summary>")},
		{Role: "user", Content: conversation.NewTextContent("new")},
	}
	if gotMessages := historyfrag.ToModelMessages(got); !reflect.DeepEqual(gotMessages, wantMessages) {
		t.Fatalf("replacement messages mismatch:\ngot  %#v\nwant %#v", gotMessages, wantMessages)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	summary := got[0]
	if summary.SourceKind != historyfrag.SourceCompactionLog || summary.Lifecycle != historyfrag.LifecycleActiveSummary {
		t.Fatalf("summary record source/lifecycle mismatch: %#v", summary)
	}
	if summary.Kind != contextfrag.KindConversationSummary {
		t.Fatalf("summary should be conversation_summary, got %s", summary.Kind)
	}
	if summary.Ref.Namespace != "compaction_log" || summary.Ref.ID != "compact-1" || summary.Ref.Durability != contextfrag.RefDurable {
		t.Fatalf("summary ref should be durable compaction log identity: %#v", summary.Ref)
	}
	if summary.Coverage == nil || len(summary.Coverage.CoveredRefs) != 2 {
		t.Fatalf("summary should cover compacted records: %#v", summary.Coverage)
	}
	if summary.Coverage.CoveredRefs[0].ID != "row-1" || summary.Coverage.CoveredRefs[1].ID != "row-2" {
		t.Fatalf("summary coverage should preserve covered record refs: %#v", summary.Coverage.CoveredRefs)
	}
	if frag := historyfrag.ToFrag(summary); frag.Kind != contextfrag.KindConversationSummary || frag.Slot != contextfrag.SlotHistory || frag.Coverage == nil {
		t.Fatalf("summary frag should carry active summary coverage: %#v", frag)
	}
}

func TestReplaceCompactedHistoryRecordsScopesSummaryToConversationNotFirstSender(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old 1")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
			record.Scope = contextfrag.Scope{
				BotID:             "bot-1",
				ChatID:            "chat-1",
				SessionID:         "sess-1",
				ChannelIdentityID: "sender-1",
				DisplayName:       "Alice",
				CurrentMessageID:  "msg-1",
				EventID:           "evt-1",
				ReplyToMessageID:  "msg-0",
			}
		}),
		historyRecord("row-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old 2")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
	}

	conversationScope := compactionSummaryScope("bot-1", "chat-1", "sess-1", "group", "Dev Chat", "target-1")
	got := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": "condensed"}, conversationScope)
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}

	scope := got[0].Scope
	if scope.ChannelIdentityID != "" || scope.DisplayName != "" || scope.CurrentMessageID != "" ||
		scope.EventID != "" || scope.ReplyToMessageID != "" {
		t.Fatalf("summary scope must not carry first sender's identity: %#v", scope)
	}
	if scope.BotID != "bot-1" || scope.ChatID != "chat-1" || scope.SessionID != "sess-1" ||
		scope.ConversationType != "group" || scope.ConversationName != "Dev Chat" || scope.ReplyTarget != "target-1" {
		t.Fatalf("summary scope must carry conversation topology: %#v", scope)
	}
}

func TestReplaceCompactedHistoryRecordsKeepsOriginalGroupWithoutSummary(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old 1")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old 2")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
	}

	gotMissing := replaceCompactedHistoryRecords(records, map[string]string{}, contextfrag.Scope{})
	if gotMessages := historyfrag.ToModelMessages(gotMissing); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("missing summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
	}

	gotEmpty := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": ""}, contextfrag.Scope{})
	if gotMessages := historyfrag.ToModelMessages(gotEmpty); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("empty summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
	}

	// A legacy status='ok' log can hold a whitespace-only summary (main never
	// trimmed). Substituting it would drop the raw rows for nothing, and the
	// reclaim SQL treats such rows as still eligible — the read path must agree.
	gotWhitespace := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": "  \n\t"}, contextfrag.Scope{})
	if gotMessages := historyfrag.ToModelMessages(gotWhitespace); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("whitespace-only summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
	}
}

func TestReplaceCompactedHistoryRecordsKeepsMustKeepIslandOrdering(t *testing.T) {
	t.Parallel()

	// The selector now marks only the contiguous run before a must-keep island
	// (the ask_user exchange) under one compact_id; the island and the run after
	// it stay raw. The read path must drop the summary in place and preserve
	// order — "mid q" stays AFTER the ask_user exchange, never folded before it.
	records := []historyfrag.HistoryRecord{
		historyRecord("row-old", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old q")}, func(r *historyfrag.HistoryRecord) {
			r.CompactID = "compact-1"
		}),
		historyRecord("row-ask-call", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("ask you something")}, nil),
		historyRecord("row-ask-result", conversation.ModelMessage{Role: "tool", Content: conversation.NewTextContent("answered")}, nil),
		historyRecord("row-mid", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("mid q")}, nil),
		historyRecord("row-current", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("current")}, nil),
	}

	got := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": "condensed"}, contextfrag.Scope{})
	want := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("<summary>\ncondensed\n</summary>")},
		{Role: "assistant", Content: conversation.NewTextContent("ask you something")},
		{Role: "tool", Content: conversation.NewTextContent("answered")},
		{Role: "user", Content: conversation.NewTextContent("mid q")},
		{Role: "user", Content: conversation.NewTextContent("current")},
	}
	if gotMessages := historyfrag.ToModelMessages(got); !reflect.DeepEqual(gotMessages, want) {
		t.Fatalf("must-keep island ordering broken:\ngot  %#v\nwant %#v", gotMessages, want)
	}
}

func TestHistoryContextFragsForMessagesCarriesActiveSummaryCoverage(t *testing.T) {
	t.Parallel()

	covered := []contextfrag.ContextRef{
		{Namespace: "bot_history_message", ID: "row-1", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable},
	}
	summary := historyfrag.SummaryRecord("compact-1", "condensed", covered, contextfrag.Scope{BotID: "bot-1"})
	records := []historyfrag.HistoryRecord{
		summary,
		historyRecord("row-2", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("new")}, nil),
	}
	messages := []conversation.ModelMessage{
		summary.ModelMessage,
		{Role: "user", Content: conversation.NewTextContent("new")},
	}

	frags := historyContextFragsForMessages(messages, records)

	if len(frags) != 1 {
		t.Fatalf("summary frags = %d, want 1: %#v", len(frags), frags)
	}
	if frags[0].ID != "message.000" || frags[0].Provenance.Index != 0 {
		t.Fatalf("summary frag should align with final message index: %#v", frags[0])
	}
	if frags[0].Kind != contextfrag.KindConversationSummary || frags[0].Coverage == nil {
		t.Fatalf("summary frag lost kind/coverage: %#v", frags[0])
	}

	cfg := agentpkg.RunConfig{
		Messages:     modelMessagesToSDKMessages(messages),
		ContextFrags: frags,
	}.RefreshContextFrag()
	if len(cfg.ContextManifest.CoverageTrace) != 1 {
		t.Fatalf("run config manifest lost summary coverage: %#v", cfg.ContextManifest)
	}
	summaryItems := 0
	for _, item := range cfg.ContextManifest.Items {
		if item.Kind == contextfrag.KindConversationSummary {
			summaryItems++
		}
	}
	if summaryItems != 1 {
		t.Fatalf("run config manifest summary items = %d, want 1: %#v", summaryItems, cfg.ContextManifest.Items)
	}
}

func TestHistoryContextFragsUseRetainedSummaryRecordsAfterTrim(t *testing.T) {
	t.Parallel()

	firstCovered := []contextfrag.ContextRef{{Namespace: "bot_history_message", ID: "old-covered", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable}}
	secondCovered := []contextfrag.ContextRef{{Namespace: "bot_history_message", ID: "new-covered", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable}}
	first := historyfrag.SummaryRecord("compact-old", "same summary", firstCovered, contextfrag.Scope{})
	second := historyfrag.SummaryRecord("compact-new", "same summary", secondCovered, contextfrag.Scope{})
	records := []historyfrag.HistoryRecord{
		first,
		historyRecord("row-long", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(strings.Repeat("x", 400))}, nil),
		second,
	}

	messages, retained, _ := trimMessagesAndRecordsByTokens(nil, records, 20)
	frags := historyContextFragsForMessages(messages, retained)

	if len(frags) != 1 || frags[0].Coverage == nil || len(frags[0].Coverage.CoveredRefs) != 1 {
		t.Fatalf("summary frag coverage mismatch: %#v", frags)
	}
	if got := frags[0].Coverage.CoveredRefs[0].ID; got != "new-covered" {
		t.Fatalf("summary coverage = %q, want retained summary coverage", got)
	}
}

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
		{Ref: contextfrag.ContextRef{Namespace: "bot_history_message", ID: "00000000-0000-0000-0000-000000000401", Version: 1, Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable, ContentHash: "hash-401", HashAlgo: contextfrag.HashAlgoSHA256, HashScope: contextfrag.HashScopeSourcePayload}},
		{Ref: contextfrag.ContextRef{Namespace: "bot_history_message", ID: "00000000-0000-0000-0000-000000000402", Version: 1, Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable, ContentHash: "hash-402", HashAlgo: contextfrag.HashAlgoSHA256, HashScope: contextfrag.HashScopeSourcePayload}},
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

func TestReplaceCompactedMessagesBackfillsMalformedPersistedCoverage(t *testing.T) {
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

	got := mustReplaceCompactedMessages(t, resolver, sessionID, contextfrag.Scope{SessionID: sessionID}, nil)

	if len(got) != 1 || got[0].CompactID != compactID {
		t.Fatalf("malformed coverage must not drop a valid summary artifact: %#v", got)
	}
	if got[0].Coverage == nil || len(got[0].Coverage.CoveredRefs) != 1 || got[0].Coverage.CoveredRefs[0].ID != coveredID {
		t.Fatalf("malformed persisted coverage was not backfilled: %#v", got[0].Coverage)
	}
	if len(queries.refCalls) != 1 || queries.refCalls[0] != mustPGUUID(t, compactID) {
		t.Fatalf("refs-only fallback calls = %#v, want %s", queries.refCalls, compactID)
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
	coverage := persistedCoverage(t, "00000000-0000-0000-0000-000000000901")
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
	coverage := persistedCoverage(t, "00000000-0000-0000-0000-000000000902")
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

func TestTotalCompactableHistoryTokensExcludesSummaries(t *testing.T) {
	t.Parallel()

	summary := historyfrag.SummaryRecord("compact-big", strings.Repeat("s", 4000), nil, contextfrag.Scope{})
	raw := historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(strings.Repeat("r", 400))}, nil)
	records := []historyfrag.HistoryRecord{summary, raw}

	compactable := totalCompactableHistoryTokens(records)
	if compactable <= 0 {
		t.Fatal("raw rows must count toward the compactable estimate")
	}
	if want := estimateMessageTokens(raw.ModelMessage); compactable != want {
		t.Fatalf("compactable = %d, want raw-only estimate %d", compactable, want)
	}
}

func TestHistoryRecordPathPreservesLegacyResolverMessagePipeline(t *testing.T) {
	t.Parallel()

	assistantToolCallSDK := sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ToolCallPart{
				ToolCallID: "call-1",
				ToolName:   "lookup",
				Input:      map[string]any{"q": "memoh"},
			},
		},
	}
	assistantToolCall := sdkMessagesToModelMessages([]sdk.Message{assistantToolCallSDK})[0]
	toolResultSDK := sdk.ToolMessage(sdk.ToolResultPart{
		ToolCallID: "call-1",
		ToolName:   "lookup",
		Result:     "tool result",
	})
	toolResult := sdkMessagesToModelMessages([]sdk.Message{toolResultSDK})[0]
	rows := []messagepkg.Message{
		dbHistoryRow(t, "row-compact-user", "user", conversation.NewTextContent("old compacted user"), func(msg *messagepkg.Message) {
			msg.CompactID = "compact-ok"
		}),
		dbHistoryRow(t, "row-compact-assistant", "assistant", conversation.NewTextContent("old compacted assistant"), func(msg *messagepkg.Message) {
			msg.CompactID = "compact-ok"
		}),
		dbHistoryRow(t, "row-missing-summary", "user", conversation.NewTextContent("missing summary body"), func(msg *messagepkg.Message) {
			msg.CompactID = "compact-missing"
		}),
		dbHistoryRow(t, "row-current", "user", conversation.NewTextContent("already persisted current"), func(msg *messagepkg.Message) {
			msg.SessionID = "sess-1"
			msg.ExternalMessageID = "msg-current"
			msg.Platform = "telegram"
			msg.SenderChannelIdentityID = "sender-1"
		}),
		{
			ID:      "row-plain",
			BotID:   "bot-1",
			Role:    "user",
			Content: conversation.NewTextContent("plain string content"),
		},
		dbHistoryRow(t, "row-tool-call", "assistant", mustRawJSON(t, assistantToolCall), nil),
		dbHistoryRow(t, "row-tool-result", "tool", mustRawJSON(t, toolResult), nil),
	}

	records := make([]historyfrag.HistoryRecord, 0, len(rows))
	for _, row := range rows {
		record, err := historyfrag.FromDBMessage(row, historyfrag.ScopeFallback{ChatID: "chat-1"})
		if err != nil {
			t.Fatalf("FromDBMessage(%s): %v", row.ID, err)
		}
		records = append(records, record)
	}
	records = dedupePersistedCurrentUserMessage(records, conversation.ChatRequest{
		UserMessagePersisted:    true,
		SessionID:               "sess-1",
		ExternalMessageID:       "msg-current",
		CurrentChannel:          "telegram",
		SourceChannelIdentityID: "sender-1",
	})
	records = replaceCompactedHistoryRecords(records, map[string]string{"compact-ok": "condensed"}, contextfrag.Scope{})
	got, tokens := trimMessagesByTokens(nil, records, 0)

	want := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("<summary>\ncondensed\n</summary>")},
		{Role: "user", Content: conversation.NewTextContent("missing summary body")},
		{Role: "user", Content: conversation.NewTextContent("plain string content")},
		assistantToolCall,
		toolResult,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history pipeline payload mismatch:\ngot  %#v\nwant %#v", got, want)
	}
	if tokens == 0 {
		t.Fatal("history pipeline should report estimated tokens for retained records")
	}

	repaired := repairToolCallClosures(sanitizeMessages(got), syntheticToolClosureError)
	assertSameJSON(t, modelMessagesToSDKMessages(nonNilModelMessages(repaired)), []sdk.Message{
		sdk.UserMessage("<summary>\ncondensed\n</summary>"),
		sdk.UserMessage("missing summary body"),
		sdk.UserMessage("plain string content"),
		assistantToolCallSDK,
		toolResultSDK,
	})
}

func TestHistoryScopeFallbackFromChatRequestUsesRequestTopology(t *testing.T) {
	t.Parallel()

	got := historyScopeFallbackFromChatRequest(conversation.ChatRequest{
		ChatID:           " chat-1 ",
		ConversationType: " group ",
		ConversationName: " Dev Chat ",
		ReplyTarget:      " target-1 ",
	})

	if got.ChatID != "chat-1" ||
		got.ConversationType != "group" ||
		got.ConversationName != "Dev Chat" ||
		got.ReplyTarget != "target-1" {
		t.Fatalf("unexpected fallback: %#v", got)
	}
}

func TestResumeHistoryFallbackDoesNotUseBotIDAsChatID(t *testing.T) {
	t.Parallel()

	userInputFallback := historyScopeFallbackFromUserInputRequest(userinput.Request{
		BotID:            "bot-1",
		ConversationType: "group",
		ReplyTarget:      "target-1",
	})
	if userInputFallback.ChatID != "" {
		t.Fatalf("user input fallback ChatID = %q, want empty", userInputFallback.ChatID)
	}
	if userInputFallback.ConversationType != "group" || userInputFallback.ReplyTarget != "target-1" {
		t.Fatalf("user input fallback lost topology: %#v", userInputFallback)
	}

	approvalFallback := historyScopeFallbackFromToolApprovalRequest(toolapproval.Request{
		BotID:            "bot-1",
		ConversationType: "direct",
		ReplyTarget:      "target-2",
	})
	if approvalFallback.ChatID != "" {
		t.Fatalf("approval fallback ChatID = %q, want empty", approvalFallback.ChatID)
	}
	if approvalFallback.ConversationType != "direct" || approvalFallback.ReplyTarget != "target-2" {
		t.Fatalf("approval fallback lost topology: %#v", approvalFallback)
	}
}

func dbHistoryRow(t *testing.T, id string, role string, content json.RawMessage, mutate func(*messagepkg.Message)) messagepkg.Message {
	t.Helper()
	msg := messagepkg.Message{
		ID:      id,
		BotID:   "bot-1",
		Role:    role,
		Content: content,
	}
	if mutate != nil {
		mutate(&msg)
	}
	return msg
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	return raw
}

func assertSameJSON(t *testing.T, got any, want any) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(gotRaw) != string(wantRaw) {
		t.Fatalf("json mismatch:\ngot  %s\nwant %s", gotRaw, wantRaw)
	}
}

func historyRecord(id string, msg conversation.ModelMessage, mutate func(*historyfrag.HistoryRecord)) historyfrag.HistoryRecord {
	record := historyfrag.HistoryRecord{
		Ref: contextfrag.ContextRef{
			Namespace:  "bot_history_message",
			ID:         id,
			Version:    1,
			Schema:     contextfrag.SchemaContextRef,
			Durability: contextfrag.RefDurable,
		},
		Kind:         contextfrag.KindConversationEvent,
		SourceKind:   historyfrag.SourceDBMessage,
		Lifecycle:    historyfrag.LifecyclePersisted,
		ModelMessage: msg,
		DBMessageID:  id,
	}
	if mutate != nil {
		mutate(&record)
	}
	return record
}

type recordingCompactionLogQueries struct {
	dbstore.Queries
	logs         []sqlc.BotHistoryMessageCompact
	covered      map[pgtype.UUID][]sqlc.ListMessagesByCompactIDRow
	refs         map[pgtype.UUID][]sqlc.ListMessageRefsByCompactIDRow
	byID         map[pgtype.UUID]sqlc.BotHistoryMessageCompact
	sessionID    pgtype.UUID
	listCalls    int
	coveredCalls []pgtype.UUID
	refCalls     []pgtype.UUID
	getCalls     []pgtype.UUID
	listErr      error
}

func (q *recordingCompactionLogQueries) GetCompactionLogByID(_ context.Context, compactID pgtype.UUID) (sqlc.BotHistoryMessageCompact, error) {
	q.getCalls = append(q.getCalls, compactID)
	return q.byID[compactID], nil
}

func (q *recordingCompactionLogQueries) ListCompactionArtifactParentIDsBySuccessor(_ context.Context, arg sqlc.ListCompactionArtifactParentIDsBySuccessorParams) ([]pgtype.UUID, error) {
	var ids []pgtype.UUID
	for _, row := range q.byID {
		if row.Status == "ok" && row.SupersededBy == arg.SuccessorID && row.BotID == arg.BotID && row.SessionID == arg.SessionID {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func (q *recordingCompactionLogQueries) ListCompactionArtifactLineageBySession(_ context.Context, sessionID pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	q.sessionID = sessionID
	q.listCalls++
	return q.logs, q.listErr
}

func persistedCoverage(t *testing.T, id string) []byte {
	t.Helper()
	raw, err := json.Marshal([]compaction.CoveredSource{{
		Ref: contextfrag.ContextRef{
			Namespace:  "bot_history_message",
			ID:         id,
			Version:    1,
			Schema:     contextfrag.SchemaContextRef,
			Durability: contextfrag.RefDurable,
		},
	}})
	if err != nil {
		t.Fatalf("marshal persisted coverage: %v", err)
	}
	return raw
}

func recordTexts(records []historyfrag.HistoryRecord) []string {
	texts := make([]string, 0, len(records))
	for _, record := range records {
		texts = append(texts, record.ModelMessage.TextContent())
	}
	return texts
}

func mustReplaceCompactedMessages(t *testing.T, resolver *Resolver, sessionID string, scope contextfrag.Scope, records []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	t.Helper()
	replaced, err := resolver.replaceCompactedMessages(context.Background(), sessionID, scope, records)
	if err != nil {
		t.Fatalf("replaceCompactedMessages: %v", err)
	}
	return replaced
}

func (q *recordingCompactionLogQueries) ListMessagesByCompactID(_ context.Context, compactID pgtype.UUID) ([]sqlc.ListMessagesByCompactIDRow, error) {
	q.coveredCalls = append(q.coveredCalls, compactID)
	return q.covered[compactID], nil
}

func (q *recordingCompactionLogQueries) ListMessageRefsByCompactID(_ context.Context, compactID pgtype.UUID) ([]sqlc.ListMessageRefsByCompactIDRow, error) {
	q.refCalls = append(q.refCalls, compactID)
	return q.refs[compactID], nil
}

func mustPGUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}
