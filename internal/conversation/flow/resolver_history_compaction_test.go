package flow

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
)

func TestReplaceCompactedHistoryRecordsPreservesOnlyRequiredSourceGroupAcrossRestack(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		historyRecord("a-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("a required")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "artifact-a"
			record.Required = true
		}),
		historyRecord("a-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("a peer")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "artifact-a"
		}),
		historyRecord("b-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("b old")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "artifact-b"
		}),
		historyRecord("b-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("b old reply")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "artifact-b"
		}),
	}
	terminal := compaction.Artifact{ID: "artifact-terminal", Summary: "restacked"}

	got := replaceCompactedHistoryRecordsWithResolver(records, contextfrag.Scope{}, func(historyfrag.HistoryRecord) (compaction.Artifact, bool) {
		return terminal, true
	})
	want := []string{"<summary>\nrestacked\n</summary>", "a required", "a peer"}
	if gotTexts := recordTexts(got); !reflect.DeepEqual(gotTexts, want) {
		t.Fatalf("restacked required preservation = %#v, want %#v", gotTexts, want)
	}
}

func TestResolveCatalogArtifactRejectsDurableCoverageHashMismatch(t *testing.T) {
	t.Parallel()

	owner := compaction.ArtifactOwner{BotID: "bot-1", SessionID: "session-1", SessionIDKnown: true}
	record := historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("edited")}, func(record *historyfrag.HistoryRecord) {
		record.BotID = owner.BotID
		record.SessionID = owner.SessionID
		record.SessionIDKnown = true
		record.CompactID = "artifact-1"
		record.Ref.HashAlgo = contextfrag.HashAlgoSHA256
		record.Ref.HashScope = contextfrag.HashScopeSourcePayload
		record.Ref.ContentHash = "new-source-hash"
	})
	covered := record.Ref
	covered.ContentHash = "old-source-hash"
	artifact := compaction.Artifact{
		ID:        "artifact-1",
		BotID:     owner.BotID,
		SessionID: owner.SessionID,
		Summary:   "stale summary",
		Coverage:  []compaction.CoveredSource{{Ref: covered}},
	}
	catalog := compaction.NewArtifactCatalog()
	catalog.Add(owner, compaction.NewArtifactAliasFrontier(artifact.ID, artifact))

	if resolved, ok := resolveCatalogArtifact(catalog, owner, record); ok {
		t.Fatalf("hash-mismatched record resolved to stale artifact: %#v", resolved)
	}
	got := replaceCompactedHistoryRecordsWithResolver([]historyfrag.HistoryRecord{record}, contextfrag.Scope{}, func(record historyfrag.HistoryRecord) (compaction.Artifact, bool) {
		return resolveCatalogArtifact(catalog, owner, record)
	})
	if len(got) != 1 || got[0].ModelMessage.TextContent() != "edited" {
		t.Fatalf("hash-mismatched raw record was not preserved: %#v", got)
	}
}

func TestRecordArtifactOwnerPreservesKnownNullSession(t *testing.T) {
	t.Parallel()

	record := historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("sessionless")}, func(record *historyfrag.HistoryRecord) {
		record.SessionID = ""
		record.SessionIDKnown = true
	})
	owner := recordArtifactOwner(record, contextfrag.Scope{SessionID: "fallback-session"})
	if !owner.SessionIDKnown || owner.SessionID != "" {
		t.Fatalf("known-null session was replaced by fallback: %#v", owner)
	}
}

func TestReplaceRecentCompactedMessagesKeepsArtifactResolutionOwnerBound(t *testing.T) {
	t.Parallel()

	botID := "00000000-0000-0000-0000-00000000b201"
	session1 := "00000000-0000-0000-0000-00000000f201"
	session2 := "00000000-0000-0000-0000-00000000f202"
	artifactID := "00000000-0000-0000-0000-00000000c201"
	corruptRawID := "00000000-0000-0000-0000-000000000201"
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{{
			ID:        mustPGUUID(t, artifactID),
			BotID:     mustPGUUID(t, botID),
			SessionID: mustPGUUID(t, session2),
			Status:    "ok",
			Summary:   "session two summary",
			Coverage:  persistedCoverage(t, corruptRawID),
		}},
	}
	records := []historyfrag.HistoryRecord{
		historyRecord(corruptRawID, conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("session one raw")}, func(record *historyfrag.HistoryRecord) {
			record.BotID = botID
			record.SessionID = session1
			record.SessionIDKnown = true
			record.CompactID = artifactID
		}),
		historyRecord("session-two-trigger", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("session two raw")}, func(record *historyfrag.HistoryRecord) {
			record.BotID = botID
			record.SessionID = session2
			record.SessionIDKnown = true
		}),
	}

	got := mustReplaceCompactedMessages(t, &Resolver{queries: queries}, "", contextfrag.Scope{BotID: botID}, records)
	want := []string{"session one raw", "session two raw"}
	if gotTexts := recordTexts(got); !reflect.DeepEqual(gotTexts, want) {
		t.Fatalf("cross-owner artifact replaced history: %#v, want %#v", gotTexts, want)
	}
}

func TestReplaceRecentCompactedMessagesLoadsEveryKnownNullSessionGroup(t *testing.T) {
	t.Parallel()

	botID := "00000000-0000-0000-0000-00000000b301"
	artifactAID := "00000000-0000-0000-0000-00000000c301"
	artifactBID := "00000000-0000-0000-0000-00000000c302"
	recordAID := "00000000-0000-0000-0000-000000000301"
	recordBID := "00000000-0000-0000-0000-000000000302"
	artifactA := sqlc.BotHistoryMessageCompact{
		ID:       mustPGUUID(t, artifactAID),
		BotID:    mustPGUUID(t, botID),
		Status:   "ok",
		Summary:  "summary a",
		Coverage: persistedCoverage(t, recordAID),
	}
	artifactB := sqlc.BotHistoryMessageCompact{
		ID:       mustPGUUID(t, artifactBID),
		BotID:    mustPGUUID(t, botID),
		Status:   "ok",
		Summary:  "summary b",
		Coverage: persistedCoverage(t, recordBID),
	}
	queries := &recordingCompactionLogQueries{
		byID: map[pgtype.UUID]sqlc.BotHistoryMessageCompact{
			artifactA.ID: artifactA,
			artifactB.ID: artifactB,
		},
	}
	records := []historyfrag.HistoryRecord{
		historyRecord(recordAID, conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("raw a")}, func(record *historyfrag.HistoryRecord) {
			record.BotID = botID
			record.SessionIDKnown = true
			record.CompactID = artifactAID
		}),
		historyRecord(recordBID, conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("raw b")}, func(record *historyfrag.HistoryRecord) {
			record.BotID = botID
			record.SessionIDKnown = true
			record.CompactID = artifactBID
		}),
	}

	got := mustReplaceCompactedMessages(t, &Resolver{queries: queries}, "", contextfrag.Scope{BotID: botID}, records)
	want := []string{"<summary>\nsummary a\n</summary>", "<summary>\nsummary b\n</summary>"}
	if gotTexts := recordTexts(got); !reflect.DeepEqual(gotTexts, want) {
		t.Fatalf("known-null compact groups = %#v, want %#v", gotTexts, want)
	}
	if len(queries.getCalls) != 2 {
		t.Fatalf("point-loaded compact groups = %d, want 2: %#v", len(queries.getCalls), queries.getCalls)
	}
}
