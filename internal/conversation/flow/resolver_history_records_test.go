package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
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

	got := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": "condensed"})
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

	gotMissing := replaceCompactedHistoryRecords(records, map[string]string{})
	if gotMessages := historyfrag.ToModelMessages(gotMissing); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("missing summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
	}

	gotEmpty := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": ""})
	if gotMessages := historyfrag.ToModelMessages(gotEmpty); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("empty summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
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
		logs: []dbsqlc.BotHistoryMessageCompact{
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

	got := resolver.replaceCompactedMessages(context.Background(), sessionID, contextfrag.Scope{BotID: "bot-1", SessionID: sessionID}, recent)

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
	queries := &recordingCompactionLogQueries{
		logs: []dbsqlc.BotHistoryMessageCompact{
			{
				ID:        mustPGUUID(t, compactID),
				SessionID: mustPGUUID(t, sessionID),
				Status:    "ok",
				Summary:   "older condensed context",
			},
		},
		covered: map[pgtype.UUID][]dbsqlc.ListMessagesByCompactIDRow{
			mustPGUUID(t, compactID): {
				{
					ID:      mustPGUUID(t, "00000000-0000-0000-0000-000000000401"),
					BotID:   mustPGUUID(t, "00000000-0000-0000-0000-000000000001"),
					Role:    "user",
					Content: []byte(`"covered user"`),
				},
				{
					ID:      mustPGUUID(t, "00000000-0000-0000-0000-000000000402"),
					BotID:   mustPGUUID(t, "00000000-0000-0000-0000-000000000001"),
					Role:    "assistant",
					Content: []byte(`"covered assistant"`),
				},
			},
		},
	}
	resolver := &Resolver{queries: queries}

	got := resolver.replaceCompactedMessages(context.Background(), sessionID, contextfrag.Scope{BotID: "bot-1", SessionID: sessionID}, nil)

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
	frags := historyContextFragsForMessages(historyfrag.ToModelMessages(got), got)
	if len(frags) != 1 || frags[0].Coverage == nil || len(frags[0].Coverage.CoveredRefs) != 2 {
		t.Fatalf("summary frag lost loaded coverage: %#v", frags)
	}
}

func TestReplaceCompactedMessagesResolvesInWindowGroupsFromSessionLogs(t *testing.T) {
	t.Parallel()

	sessionID := "00000000-0000-0000-0000-00000000f005"
	inWindowCompact := "00000000-0000-0000-0000-00000000c005"
	outOfWindowCompact := "00000000-0000-0000-0000-00000000c006"
	queries := &recordingCompactionLogQueries{
		logs: []dbsqlc.BotHistoryMessageCompact{
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
	got := resolver.replaceCompactedMessages(context.Background(), sessionID, contextfrag.Scope{BotID: "bot-1", SessionID: sessionID}, recent)

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
	for _, called := range queries.coveredCalls {
		if called == mustPGUUID(t, inWindowCompact) {
			t.Fatal("covered-ref lookup must be skipped for groups already replaced in-window")
		}
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

func TestBuildPipelineContextUsesCompactionSummaryAndSkipsCoveredReplay(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-1"
	const compactID = "11111111-1111-1111-1111-111111111111"

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-new",
		ReceivedAtMs: 300,
		TimestampSec: 300,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "new user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})

	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
		messageService: &pipelineHistoryMessageService{rows: []messagepkg.Message{
			dbHistoryRow(t, "row-old-user", "user", conversation.NewTextContent("old user"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-old"
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(100).UTC()
			}),
			dbHistoryRow(t, "row-old-assistant", "assistant", mustRawJSON(t, conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("old assistant"),
			}), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(200).UTC()
			}),
			dbHistoryRow(t, "row-new-assistant", "assistant", mustRawJSON(t, conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("new assistant"),
			}), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.CreatedAt = time.UnixMilli(400).UTC()
			}),
		}},
		queries: pipelineCompactionQueries{
			logs: map[string]dbsqlc.BotHistoryMessageCompact{
				compactID: {ID: flowTestUUID(compactID), Status: "ok", Summary: "old user and assistant summarized"},
			},
		},
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0).Messages
	got := make([]string, 0, len(messages))
	for _, msg := range messages {
		got = append(got, msg.TextContent())
	}
	want := []string{
		"[Conversation summary]\nold user and assistant summarized",
		`<message id="external-new" t="1970-01-01T00:05:00+00:00" channel="telegram" type="group">` + "\nnew user\n</message>",
		"new assistant",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline messages mismatch:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestBuildPipelineContextCarriesCompactionSummaryCoverage(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-pipeline-summary-frag"
	const compactID = "22222222-2222-2222-2222-222222222222"

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-new",
		ReceivedAtMs: 300,
		TimestampSec: 300,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "new user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})

	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
		messageService: &pipelineHistoryMessageService{rows: []messagepkg.Message{
			dbHistoryRow(t, "row-old-user", "user", conversation.NewTextContent("old user"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-old"
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(100).UTC()
			}),
			dbHistoryRow(t, "row-old-assistant", "assistant", mustRawJSON(t, conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("old assistant"),
			}), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(200).UTC()
			}),
		}},
		queries: pipelineCompactionQueries{
			logs: map[string]dbsqlc.BotHistoryMessageCompact{
				compactID: {ID: flowTestUUID(compactID), Status: "ok", Summary: "old user and assistant summarized"},
			},
		},
	}

	built := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0)
	frags := historyContextFragsForMessages(built.Messages, built.HistoryRecords)

	if len(frags) != 1 {
		t.Fatalf("summary frags = %d, want 1: %#v", len(frags), frags)
	}
	if frags[0].Kind != contextfrag.KindConversationSummary || frags[0].Coverage == nil || len(frags[0].Coverage.CoveredRefs) != 2 {
		t.Fatalf("summary frag lost typed coverage: %#v", frags[0])
	}
	if len(contextfrag.BuildManifest(frags).CoverageTrace) != 1 {
		t.Fatalf("manifest lost pipeline summary coverage: %#v", contextfrag.BuildManifest(frags))
	}
}

func TestBuildPipelineContextCarriesMergedCompactionSummaryCoverage(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-pipeline-merged-summary-frag"
	const compactID1 = "22222222-2222-2222-2222-222222222222"
	const compactID2 = "33333333-3333-3333-3333-333333333333"

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.ReplaySession(sessionID, nil)

	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
		messageService: &pipelineHistoryMessageService{rows: []messagepkg.Message{
			dbHistoryRow(t, "row-old-user-1", "user", conversation.NewTextContent("old user one"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-old-1"
				msg.CompactID = compactID1
				msg.CreatedAt = time.UnixMilli(100).UTC()
			}),
			dbHistoryRow(t, "row-old-user-2", "user", conversation.NewTextContent("old user two"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-old-2"
				msg.CompactID = compactID2
				msg.CreatedAt = time.UnixMilli(200).UTC()
			}),
		}},
		queries: pipelineCompactionQueries{
			logs: map[string]dbsqlc.BotHistoryMessageCompact{
				compactID1: {ID: flowTestUUID(compactID1), Status: "ok", Summary: "summary one"},
				compactID2: {ID: flowTestUUID(compactID2), Status: "ok", Summary: "summary two"},
			},
		},
	}

	built := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0)
	frags := historyContextFragsForMessages(built.Messages, built.HistoryRecords)

	if len(built.HistoryRecords) != 1 {
		t.Fatalf("merged summary records = %d, want 1: %#v", len(built.HistoryRecords), built.HistoryRecords)
	}
	if len(frags) != 1 {
		t.Fatalf("summary frags = %d, want merged summary frag: %#v", len(frags), frags)
	}
	if got := len(frags[0].Coverage.CoveredRefs); got != 2 {
		t.Fatalf("merged summary coverage refs = %d, want 2: %#v", got, frags[0].Coverage)
	}
	if len(contextfrag.BuildManifest(frags).CoverageTrace) != 1 {
		t.Fatalf("manifest lost merged pipeline summary coverage: %#v", contextfrag.BuildManifest(frags))
	}
}

func TestBuildPipelineContextKeepsHistoryWhenRCEmpty(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-empty-rc"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.ReplaySession(sessionID, nil)
	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
		messageService: &pipelineHistoryMessageService{rows: []messagepkg.Message{
			dbHistoryRow(t, "row-assistant", "assistant", mustRawJSON(t, conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("retained assistant"),
			}), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.CreatedAt = time.UnixMilli(200).UTC()
			}),
		}},
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID: sessionID,
		Query:     "current user query",
	}, 0).Messages
	got := make([]string, 0, len(messages))
	for _, msg := range messages {
		got = append(got, msg.TextContent())
	}
	want := []string{"retained assistant", "current user query"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline messages mismatch:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestBuildPipelineContextAppendsCurrentQueryWhenRCStale(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-stale-rc"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "external-current",
		Query:             "current user query",
	}, 0).Messages

	if len(messages) != 2 {
		t.Fatalf("messages = %d, want RC plus current query: %#v", len(messages), messages)
	}
	if got := messages[len(messages)-1].TextContent(); got != "current user query" {
		t.Fatalf("last message = %q, want current query; all=%#v", got, messages)
	}
}

func TestBuildPipelineContextAppendsCurrentQueryWithoutExternalMessageID(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-stale-rc-empty-external-id"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID: sessionID,
		Query:     "current user query",
	}, 0).Messages

	if len(messages) != 2 {
		t.Fatalf("messages = %d, want RC plus current query: %#v", len(messages), messages)
	}
	if got := messages[len(messages)-1].TextContent(); got != "current user query" {
		t.Fatalf("last message = %q, want current query; all=%#v", got, messages)
	}
}

func TestBuildPipelineContextAppendsCurrentQueryWhenOldRCOnlyContainsSubstring(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-stale-rc-empty-external-id-substring"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: strings.Repeat("old stale filler ", 80) + "current user query"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID: sessionID,
		Query:     "current user query",
	}, 20).Messages
	joined := strings.Join(modelMessageTexts(messages), "\n")

	if !strings.Contains(joined, "current user query") {
		t.Fatalf("budgeted pipeline context without external message id lost current query: %#v", messages)
	}
	if strings.Contains(joined, "old stale filler") {
		t.Fatalf("old RC containing current query substring suppressed current turn: %#v", messages)
	}
}

func TestBuildPipelineContextCompactionEndToEnd(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-compact-e2e"
	const compactID = "55555555-5555-5555-5555-555555555555"
	completedAt := time.UnixMilli(200).UTC()

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old compacted user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-edited",
		ReceivedAtMs: 120,
		TimestampSec: 120,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old edit body"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-deleted",
		ReceivedAtMs: 130,
		TimestampSec: 130,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old delete body"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.EditEvent{
		SessionID:    sessionID,
		MessageID:    "external-edited",
		ReceivedAtMs: 250,
		TimestampSec: 250,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "edited after compact"}},
	})
	p.PushEvent(sessionID, pipelinepkg.DeleteEvent{
		SessionID:    sessionID,
		MessageIDs:   []string{"external-deleted"},
		ReceivedAtMs: 260,
		TimestampSec: 260,
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-filler",
		ReceivedAtMs: 280,
		TimestampSec: 280,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: strings.Repeat("budget filler ", 80)}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-new",
		ReceivedAtMs: 300,
		TimestampSec: 300,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "fresh user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})

	resolver := &Resolver{
		logger:   slog.New(slog.DiscardHandler),
		pipeline: p,
		messageService: &pipelineHistoryMessageService{rows: []messagepkg.Message{
			dbHistoryRow(t, "row-old-user", "user", conversation.NewTextContent("old compacted user"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-old"
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(100).UTC()
			}),
			dbHistoryRow(t, "row-edited", "user", conversation.NewTextContent("old edit body"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-edited"
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(120).UTC()
			}),
			dbHistoryRow(t, "row-deleted", "user", conversation.NewTextContent("old delete body"), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.ExternalMessageID = "external-deleted"
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(130).UTC()
			}),
			dbHistoryRow(t, "row-old-assistant", "assistant", mustRawJSON(t, conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("old compacted assistant"),
			}), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.CompactID = compactID
				msg.CreatedAt = time.UnixMilli(150).UTC()
			}),
			dbHistoryRow(t, "row-new-assistant", "assistant", mustRawJSON(t, conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("fresh assistant"),
			}), func(msg *messagepkg.Message) {
				msg.SessionID = sessionID
				msg.CreatedAt = time.UnixMilli(400).UTC()
			}),
		}},
		queries: pipelineCompactionQueries{
			logs: map[string]dbsqlc.BotHistoryMessageCompact{
				compactID: {
					ID:          flowTestUUID(compactID),
					Status:      "ok",
					Summary:     "compacted segment summary",
					CompletedAt: pgtype.Timestamptz{Time: completedAt, Valid: true},
				},
			},
		},
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0).Messages
	got := make([]string, 0, len(messages))
	for _, msg := range messages {
		got = append(got, msg.TextContent())
	}
	joined := strings.Join(got, "\n")

	if len(got) < 3 {
		t.Fatalf("message count = %d, want summary, replayed user context, and assistant response: %#v", len(got), got)
	}
	if got[0] != "[Conversation summary]\ncompacted segment summary" {
		t.Fatalf("first message = %q, want compaction summary; all=%#v", got[0], got)
	}
	if got[len(got)-1] != "fresh assistant" {
		t.Fatalf("last message = %q, want fresh assistant TR; all=%#v", got[len(got)-1], got)
	}
	if strings.Contains(joined, "old compacted user") || strings.Contains(joined, "old compacted assistant") {
		t.Fatalf("covered compacted messages were replayed: %#v", got)
	}
	if !strings.Contains(joined, "edited after compact") || strings.Contains(joined, "old edit body") {
		t.Fatalf("post-compact edit was not preserved cleanly: %#v", got)
	}
	if !strings.Contains(joined, `<message id="external-deleted"`) || !strings.Contains(joined, "/>") || strings.Contains(joined, "old delete body") {
		t.Fatalf("post-compact delete tombstone was not preserved cleanly: %#v", got)
	}
	if !strings.Contains(joined, "fresh user") || !strings.Contains(joined, "fresh assistant") {
		t.Fatalf("fresh RC/TR messages were not retained: %#v", got)
	}

	budgeted := resolver.buildMessagesFromPipeline(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 300)
	budgetedGot := make([]string, 0, len(budgeted))
	for _, msg := range budgeted {
		budgetedGot = append(budgetedGot, msg.TextContent())
	}
	budgetedJoined := strings.Join(budgetedGot, "\n")
	if strings.Contains(budgetedJoined, "budget filler") {
		t.Fatalf("budgeted pipeline context retained filler: %#v", budgetedGot)
	}
	for _, want := range []string{
		"[Conversation summary]\ncompacted segment summary",
		"edited after compact",
		`<message id="external-deleted"`,
		"fresh user",
		"fresh assistant",
	} {
		if !strings.Contains(budgetedJoined, want) {
			t.Fatalf("budgeted pipeline context lost %q: %#v", want, budgetedGot)
		}
	}

	tinyBudgeted := resolver.buildMessagesFromPipeline(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 1)
	tinyGot := make([]string, 0, len(tinyBudgeted))
	for _, msg := range tinyBudgeted {
		tinyGot = append(tinyGot, msg.TextContent())
	}
	tinyJoined := strings.Join(tinyGot, "\n")
	if !strings.Contains(tinyJoined, "fresh user") {
		t.Fatalf("tiny-budget pipeline context lost current trigger: %#v", tinyGot)
	}
}

func TestTrimPipelineMessagesPreservesMidHistoryCompactionSummary(t *testing.T) {
	t.Parallel()

	summary := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("[Conversation summary]\ncovered history"),
	}
	messages := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("must keep trigger")},
		summary,
		{Role: "user", Content: conversation.NewTextContent(strings.Repeat("old filler ", 200))},
		{Role: "assistant", Content: conversation.NewTextContent("fresh reply")},
	}

	trimmed := trimPipelineMessagesByTokens(nil, messages, 10)

	if len(trimmed) == 0 || trimmed[0].TextContent() != summary.TextContent() {
		t.Fatalf("trimmed mid-history messages lost compaction summary: %#v", trimmed)
	}
	if len(trimmed) != 2 || trimmed[1].TextContent() != "fresh reply" {
		t.Fatalf("trimmed messages should keep summary and newest reply, got %#v", trimmed)
	}
}

func TestLoadCompactionSummaryCarriesMessageCutoff(t *testing.T) {
	t.Parallel()

	const compactID = "22222222-2222-2222-2222-222222222222"
	completedAt := time.UnixMilli(200).UTC()
	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		queries: pipelineCompactionQueries{
			logs: map[string]dbsqlc.BotHistoryMessageCompact{
				compactID: {
					ID:          flowTestUUID(compactID),
					Status:      "ok",
					Summary:     "old summarized",
					CompletedAt: pgtype.Timestamptz{Time: completedAt, Valid: true},
				},
			},
		},
	}
	messages := []messagepkg.Message{
		{ID: "row-old-user", ExternalMessageID: "external-old", CompactID: compactID, CreatedAt: time.UnixMilli(100).UTC()},
	}

	summary := resolver.LoadCompactionSummary(context.Background(), messages)

	if got := summary.CoveredMessageCutoffMs["external-old"]; got != 100 {
		t.Fatalf("covered message cutoff = %d, want source message timestamp; summary=%#v", got, summary)
	}
}

func TestLoadCompactionSummaryMergesMultipleCompactsInOrderAndCoverage(t *testing.T) {
	t.Parallel()

	const compactA = "33333333-3333-3333-3333-333333333333"
	const compactB = "44444444-4444-4444-4444-444444444444"
	completedA := time.UnixMilli(300).UTC()
	completedB := time.UnixMilli(400).UTC()
	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		queries: pipelineCompactionQueries{
			logs: map[string]dbsqlc.BotHistoryMessageCompact{
				compactA: {
					ID:          flowTestUUID(compactA),
					Status:      "ok",
					Summary:     "summary A",
					CompletedAt: pgtype.Timestamptz{Time: completedA, Valid: true},
				},
				compactB: {
					ID:          flowTestUUID(compactB),
					Status:      "ok",
					Summary:     "summary B",
					CompletedAt: pgtype.Timestamptz{Time: completedB, Valid: true},
				},
			},
		},
	}
	messages := []messagepkg.Message{
		{ID: "row-a1", ExternalMessageID: "external-a", CompactID: compactA, CreatedAt: time.UnixMilli(100).UTC()},
		{ID: "row-b1", ExternalMessageID: "external-b", CompactID: compactB, CreatedAt: time.UnixMilli(200).UTC()},
		{ID: "row-a2", CompactID: compactA, CreatedAt: time.UnixMilli(250).UTC()},
	}

	summary := resolver.LoadCompactionSummary(context.Background(), messages)

	if summary.Text != "summary A\n\nsummary B" {
		t.Fatalf("summary text = %q", summary.Text)
	}
	if !reflect.DeepEqual(summary.CoveredHistoryMessageIDs, []string{"row-a1", "row-a2", "row-b1"}) {
		t.Fatalf("covered history ids mismatch: %#v", summary.CoveredHistoryMessageIDs)
	}
	if !reflect.DeepEqual(summary.CoveredMessageIDs, []string{"external-a", "external-b"}) {
		t.Fatalf("covered external ids mismatch: %#v", summary.CoveredMessageIDs)
	}
	if summary.CoveredMessageCutoffMs["external-a"] != 100 ||
		summary.CoveredMessageCutoffMs["external-b"] != 200 {
		t.Fatalf("covered cutoffs mismatch: %#v", summary.CoveredMessageCutoffMs)
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
	records = replaceCompactedHistoryRecords(records, map[string]string{"compact-ok": "condensed"})
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

type pipelineHistoryMessageService struct {
	recordingMessageService
	rows []messagepkg.Message
}

func (s *pipelineHistoryMessageService) ListActiveSinceBySession(_ context.Context, _ string, since time.Time) ([]messagepkg.Message, error) {
	filtered := make([]messagepkg.Message, 0, len(s.rows))
	for _, row := range s.rows {
		if row.CreatedAt.IsZero() || !row.CreatedAt.Before(since) {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

type pipelineCompactionQueries struct {
	dbstore.Queries
	logs map[string]dbsqlc.BotHistoryMessageCompact
}

func (q pipelineCompactionQueries) GetCompactionLogByID(_ context.Context, id pgtype.UUID) (dbsqlc.BotHistoryMessageCompact, error) {
	for compactID, log := range q.logs {
		if flowTestUUID(compactID) == id {
			return log, nil
		}
	}
	return dbsqlc.BotHistoryMessageCompact{}, errors.New("compact log not found")
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

func modelMessageTexts(messages []conversation.ModelMessage) []string {
	texts := make([]string, 0, len(messages))
	for _, msg := range messages {
		texts = append(texts, msg.TextContent())
	}
	return texts
}

type recordingCompactionLogQueries struct {
	dbstore.Queries
	logs         []dbsqlc.BotHistoryMessageCompact
	covered      map[pgtype.UUID][]dbsqlc.ListMessagesByCompactIDRow
	sessionID    pgtype.UUID
	listCalls    int
	coveredCalls []pgtype.UUID
}

func (q *recordingCompactionLogQueries) ListCompactionLogsBySession(_ context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotHistoryMessageCompact, error) {
	q.sessionID = sessionID
	q.listCalls++
	return q.logs, nil
}

func (q *recordingCompactionLogQueries) ListMessagesByCompactID(_ context.Context, compactID pgtype.UUID) ([]dbsqlc.ListMessagesByCompactIDRow, error) {
	q.coveredCalls = append(q.coveredCalls, compactID)
	return q.covered[compactID], nil
}

func mustPGUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}
