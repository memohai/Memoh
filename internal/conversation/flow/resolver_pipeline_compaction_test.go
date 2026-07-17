package flow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestLoadPipelineCompactionArtifactsUsesOrderedActiveFrontier(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactA  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		artifactB  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		parentID   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		artifactID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	)
	base := time.UnixMilli(1_000).UTC()
	messages := []messagepkg.Message{
		pipelineHistoryMessage(t, "row-a", botID, sessionID, "external-a", base, "user", "source a"),
		pipelineHistoryMessage(t, "row-b", botID, sessionID, "external-b", base.Add(time.Second), "user", "source b"),
		pipelineHistoryMessage(t, "row-parent", botID, sessionID, "external-parent", base.Add(2*time.Second), "user", "parent source"),
	}
	records := pipelineHistoryRecords(t, messages)
	rowA := pipelineArtifactRow(t, artifactA, botID, sessionID, "summary a", messages[:1], base.Add(time.Minute))
	var rowACoverage []compaction.CoveredSource
	if err := json.Unmarshal(rowA.Coverage, &rowACoverage); err != nil {
		t.Fatalf("decode artifact A coverage: %v", err)
	}
	rowACoverage[0].EventCursor = 42
	encodedCoverage, err := json.Marshal(rowACoverage)
	if err != nil {
		t.Fatalf("encode artifact A coverage: %v", err)
	}
	rowA.Coverage = encodedCoverage
	rowB := pipelineArtifactRow(t, artifactB, botID, sessionID, "summary b", messages[1:2], base.Add(2*time.Minute))
	parent := pipelineArtifactRow(t, parentID, botID, sessionID, "superseded parent", messages[2:], base.Add(3*time.Minute))
	child := pipelineArtifactRow(t, artifactID, botID, sessionID, "restacked parent", messages[2:], base.Add(4*time.Minute))
	parent.SupersededBy = mustPGUUID(t, artifactID)
	parent.SupersededAt = pgtype.Timestamptz{Time: base.Add(4 * time.Minute), Valid: true}
	child.ParentIds = []pgtype.UUID{mustPGUUID(t, parentID)}

	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{rowB, child, parent, rowA},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat-1", sessionID, "group", "room", "target")

	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, records)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if got, want := pipelineArtifactIDs(artifacts), []string{artifactA, artifactB, artifactID}; !equalStrings(got, want) {
		t.Fatalf("artifact order = %#v, want %#v", got, want)
	}
	if got, want := pipelineSummaryIDs(summaries), []string{artifactA, artifactB, artifactID}; !equalStrings(got, want) {
		t.Fatalf("summary identities = %#v, want %#v", got, want)
	}
	if got := artifacts[2].Summary; got != "restacked parent" {
		t.Fatalf("terminal summary = %q, want restacked parent", got)
	}
	if got := artifacts[0].Sources[0].EventCursor; got != 42 {
		t.Fatalf("projected event cursor = %d, want 42", got)
	}
	for i := range artifacts {
		if len(artifacts[i].Sources) != 1 || len(summaries[i].Coverage.CoveredRefs) != 1 {
			t.Fatalf("artifact %d lost independent coverage: artifact=%#v summary=%#v", i, artifacts[i], summaries[i])
		}
	}
}

func TestLoadPipelineCompactionArtifactsRejectsOnlyConflictingArtifact(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifactA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		artifactB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	base := time.UnixMilli(1_000).UTC()
	originalA := pipelineHistoryMessage(t, "row-a", botID, sessionID, "external-a", base, "user", "original a")
	messageB := pipelineHistoryMessage(t, "row-b", botID, sessionID, "external-b", base.Add(time.Second), "user", "source b")
	rowA := pipelineArtifactRow(t, artifactA, botID, sessionID, "summary a", []messagepkg.Message{originalA}, base.Add(time.Minute))
	rowB := pipelineArtifactRow(t, artifactB, botID, sessionID, "summary b", []messagepkg.Message{messageB}, base.Add(2*time.Minute))
	mutatedA := pipelineHistoryMessage(t, "row-a", botID, sessionID, "external-a", base, "user", "mutated after compaction")
	records := pipelineHistoryRecords(t, []messagepkg.Message{mutatedA, messageB})
	queries := &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{rowA, rowB}}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat-1", sessionID, "group", "room", "target")

	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, records)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if got, want := pipelineArtifactIDs(artifacts), []string{artifactB}; !equalStrings(got, want) {
		t.Fatalf("active artifacts = %#v, want %#v", got, want)
	}
	if got, want := pipelineSummaryIDs(summaries), []string{artifactB}; !equalStrings(got, want) {
		t.Fatalf("active summary records = %#v, want %#v", got, want)
	}
}

func TestLoadPipelineCompactionArtifactsBackfillsLegacyCoverageFromMarkedRows(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifact  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	createdAt := time.UnixMilli(1_000).UTC()
	message := pipelineHistoryMessage(t, "row-a", botID, sessionID, "external-a", createdAt, "user", "legacy source")
	message.CompactID = artifact
	row := pipelineArtifactRow(t, artifact, botID, sessionID, "legacy summary", []messagepkg.Message{message}, createdAt.Add(time.Minute))
	row.Coverage = []byte("[]")
	row.AnchorStartMs = 0
	row.AnchorEndMs = 0
	resolver := &Resolver{queries: &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{row}}}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, pipelineHistoryRecords(t, []messagepkg.Message{message}))
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || len(artifacts[0].Sources) != 1 {
		t.Fatalf("legacy artifact projection = %#v, want one covered source", artifacts)
	}
	if artifacts[0].AnchorStartMs != createdAt.UnixMilli() || artifacts[0].Sources[0].ExternalMessageID != "external-a" {
		t.Fatalf("legacy artifact lost derived anchor/source: %#v", artifacts[0])
	}
	if len(summaries) != 1 || summaries[0].Coverage == nil || len(summaries[0].Coverage.CoveredRefs) != 1 {
		t.Fatalf("legacy summary coverage = %#v", summaries)
	}
}

func TestLoadPipelineCompactionArtifactsBackfillsLegacyCoverageFromIdentityRows(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		sourceID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		recentID   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	createdAt := time.UnixMilli(1_000).UTC()
	recentAt := createdAt.Add(time.Hour)
	recent := pipelineHistoryMessage(t, recentID, botID, sessionID, "external-recent", recentAt, "user", "recent legacy source")
	recent.CompactID = artifactID
	row := pipelineArtifactRow(t, artifactID, botID, sessionID, "legacy summary", nil, createdAt.Add(time.Minute))
	row.Coverage = []byte("[]")
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{row},
		refs: map[pgtype.UUID][]sqlc.ListMessageRefsByCompactIDRow{
			mustPGUUID(t, artifactID): {{
				ID:                     mustPGUUID(t, sourceID),
				BotID:                  mustPGUUID(t, botID),
				SessionID:              mustPGUUID(t, sessionID),
				ExternalMessageID:      pgtype.Text{String: "external-old", Valid: true},
				SourceReplyToMessageID: pgtype.Text{String: "reply-old", Valid: true},
				CreatedAt:              pgtype.Timestamptz{Time: createdAt, Valid: true},
			}, {
				ID:                mustPGUUID(t, recentID),
				BotID:             mustPGUUID(t, botID),
				SessionID:         mustPGUUID(t, sessionID),
				ExternalMessageID: pgtype.Text{String: "external-recent", Valid: true},
				CreatedAt:         pgtype.Timestamptz{Time: recentAt, Valid: true},
			}},
		},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(
		context.Background(),
		scope,
		pipelineHistoryRecords(t, []messagepkg.Message{recent}),
	)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || len(artifacts[0].Sources) != 2 {
		t.Fatalf("legacy artifact projection = %#v, want complete identity coverage", artifacts)
	}
	source := artifacts[0].Sources[0]
	if source.Ref.ID != sourceID || source.ExternalMessageID != "external-old" || source.CreatedAtMs != createdAt.UnixMilli() {
		t.Fatalf("legacy source = %#v", source)
	}
	if artifacts[0].AnchorStartMs != createdAt.UnixMilli() || len(summaries) != 1 {
		t.Fatalf("legacy anchor/summary = artifact:%#v summaries:%#v", artifacts[0], summaries)
	}
}

func TestLoadPipelineCompactionArtifactsSkipsUnreconciledLegacyArtifact(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifact  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	row := pipelineArtifactRow(t, artifact, botID, sessionID, "legacy summary", nil, time.UnixMilli(1_000).UTC())
	row.Coverage = []byte("[]")
	resolver := &Resolver{queries: &recordingCompactionLogQueries{logs: []sqlc.BotHistoryMessageCompact{row}}}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 0 || len(summaries) != 0 {
		t.Fatalf("unreconciled legacy artifact became active: artifacts=%#v summaries=%#v", artifacts, summaries)
	}
}

func TestLoadPipelineCompactionArtifactsPropagatesLegacyCoverageFailure(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	wantErr := errors.New("legacy coverage unavailable")
	row := pipelineArtifactRow(t, artifactID, botID, sessionID, "legacy summary", nil, time.UnixMilli(1_000).UTC())
	row.Coverage = []byte("[]")
	resolver := &Resolver{queries: &recordingCompactionLogQueries{
		logs:   []sqlc.BotHistoryMessageCompact{row},
		refErr: wantErr,
	}}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")

	_, _, err := resolver.loadPipelineCompactionArtifacts(context.Background(), scope, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v, want %v", err, wantErr)
	}
}

func TestLoadPipelineCompactionArtifactsRejectsCrossSessionLegacyIdentityRows(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		artifactID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	row := pipelineArtifactRow(t, artifactID, botID, sessionID, "legacy summary", nil, time.UnixMilli(1_000).UTC())
	row.Coverage = []byte("[]")
	queries := &recordingCompactionLogQueries{
		logs: []sqlc.BotHistoryMessageCompact{row},
		refs: map[pgtype.UUID][]sqlc.ListMessageRefsByCompactIDRow{
			mustPGUUID(t, artifactID): {{
				ID:        mustPGUUID(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
				BotID:     mustPGUUID(t, botID),
				SessionID: mustPGUUID(t, "cccccccc-cccc-cccc-cccc-cccccccccccc"),
				CreatedAt: pgtype.Timestamptz{Time: time.UnixMilli(1_000).UTC(), Valid: true},
			}},
		},
	}
	resolver := &Resolver{queries: queries}
	scope := compactionSummaryScope(botID, "chat", sessionID, "group", "room", "target")
	recent := pipelineHistoryMessage(
		t,
		"dddddddd-dddd-dddd-dddd-dddddddddddd",
		botID,
		sessionID,
		"recent-external",
		time.UnixMilli(2_000).UTC(),
		"user",
		"recent same-session row",
	)
	recent.CompactID = artifactID

	artifacts, summaries, err := resolver.loadPipelineCompactionArtifacts(
		context.Background(),
		scope,
		pipelineHistoryRecords(t, []messagepkg.Message{recent}),
	)
	if err != nil {
		t.Fatalf("loadPipelineCompactionArtifacts() error = %v", err)
	}
	if len(artifacts) != 0 || len(summaries) != 0 {
		t.Fatalf("cross-session legacy rows activated artifact: artifacts=%#v summaries=%#v", artifacts, summaries)
	}
}

func pipelineHistoryMessage(t *testing.T, id, botID, sessionID, externalID string, createdAt time.Time, role, text string) messagepkg.Message {
	t.Helper()
	modelMessage := conversation.ModelMessage{Role: role, Content: conversation.NewTextContent(text)}
	raw, err := json.Marshal(modelMessage)
	if err != nil {
		t.Fatalf("marshal history message: %v", err)
	}
	return messagepkg.Message{
		ID:                id,
		BotID:             botID,
		SessionID:         sessionID,
		ExternalMessageID: externalID,
		Role:              role,
		Content:           raw,
		CreatedAt:         createdAt,
	}
}

func pipelineHistoryRecords(t *testing.T, messages []messagepkg.Message) []historyfrag.HistoryRecord {
	t.Helper()
	records := make([]historyfrag.HistoryRecord, 0, len(messages))
	for _, message := range messages {
		record, err := historyfrag.FromDBMessage(message, historyfrag.ScopeFallback{})
		if err != nil {
			t.Fatalf("convert history message: %v", err)
		}
		records = append(records, record)
	}
	return records
}

func pipelineArtifactRow(t *testing.T, id, botID, sessionID, summary string, messages []messagepkg.Message, startedAt time.Time) sqlc.BotHistoryMessageCompact {
	t.Helper()
	coverage := make([]compaction.CoveredSource, 0, len(messages))
	for _, record := range pipelineHistoryRecords(t, messages) {
		coverage = append(coverage, compaction.CoveredSource{
			Ref:                    record.Ref,
			ExternalMessageID:      record.ExternalMessageID,
			SourceReplyToMessageID: record.SourceReplyToMessageID,
			CreatedAtMs:            record.CreatedAt.UnixMilli(),
		})
	}
	raw, err := json.Marshal(coverage)
	if err != nil {
		t.Fatalf("marshal artifact coverage: %v", err)
	}
	row := sqlc.BotHistoryMessageCompact{
		ID:              mustPGUUID(t, id),
		BotID:           mustPGUUID(t, botID),
		SessionID:       mustPGUUID(t, sessionID),
		Status:          "ok",
		Summary:         summary,
		ArtifactVersion: compaction.ArtifactVersion,
		Coverage:        raw,
		StartedAt:       pgtype.Timestamptz{Time: startedAt, Valid: true},
	}
	if len(coverage) > 0 {
		row.AnchorStartMs = coverage[0].CreatedAtMs
		row.AnchorEndMs = coverage[len(coverage)-1].CreatedAtMs
	}
	return row
}

func pipelineArtifactIDs(artifacts []pipelinepkg.CompactionArtifact) []string {
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		ids = append(ids, artifact.ID)
	}
	return ids
}

func pipelineSummaryIDs(records []historyfrag.HistoryRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.Ref.ID)
	}
	return ids
}
