package botbackup

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	_ "modernc.org/sqlite"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	modelpkg "github.com/memohai/memoh/internal/models"
	settingspkg "github.com/memohai/memoh/internal/settings"
)

func TestReadZipEntriesRejectsZipSlip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../manifest.json")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := w.Write([]byte(`{}`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := readZipEntries(buf.Bytes()); err == nil {
		t.Fatal("readZipEntries() accepted zip slip path")
	}
}

func TestNormalizeExportOptionsDefaultsAllSections(t *testing.T) {
	opts := NormalizeExportOptions(ExportOptions{})
	if len(opts.Sections) != len(AllExportSections) {
		t.Fatalf("default export should include all sections, got %v", opts.Sections)
	}
	opts = NormalizeExportOptions(ExportOptions{Sections: []Section{SectionHistory}})
	if opts.wants(SectionWorkspace) {
		t.Fatal("explicit non-default scope should not include workspace")
	}
	if !opts.wants(SectionHistory) {
		t.Fatal("explicit history scope should include history")
	}
	if !opts.wants(SectionProfile) {
		t.Fatal("profile is always exported")
	}
}

func TestImportDependenciesLegacyModelEnableCompatibility(t *testing.T) {
	t.Run("missing enable defaults to true", func(t *testing.T) {
		ctx := context.Background()
		conn, queries := newBotBackupModelTestDB(t)
		const providerID = "00000000-0000-0000-0000-000000000501"
		insertBotBackupProvider(t, conn, providerID)
		modelsService := modelpkg.NewService(slog.New(slog.DiscardHandler), queries)
		service := &Service{models: modelsService}
		state := &importState{
			entries: map[string]backupZipEntry{
				"dependencies/models.json": {
					data: []byte(`[{"id":"old-model","model_id":"legacy-gpt","name":"Legacy GPT","provider_id":"00000000-0000-0000-0000-000000000501","type":"chat","config":{}}]`),
				},
			},
		}

		deps, err := service.importDependencies(ctx, state)
		if err != nil {
			t.Fatalf("importDependencies() error = %v", err)
		}
		created, err := modelsService.GetByID(ctx, deps.models["old-model"])
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !created.Enable {
			t.Fatalf("legacy backup model imported disabled, want enabled")
		}
	})

	t.Run("explicit false stays disabled", func(t *testing.T) {
		ctx := context.Background()
		conn, queries := newBotBackupModelTestDB(t)
		const providerID = "00000000-0000-0000-0000-000000000601"
		insertBotBackupProvider(t, conn, providerID)
		modelsService := modelpkg.NewService(slog.New(slog.DiscardHandler), queries)
		service := &Service{models: modelsService}
		state := &importState{
			entries: map[string]backupZipEntry{
				"dependencies/models.json": {
					data: []byte(`[{"id":"old-model","model_id":"disabled-gpt","name":"Disabled GPT","provider_id":"00000000-0000-0000-0000-000000000601","type":"chat","enable":false,"config":{}}]`),
				},
			},
		}

		deps, err := service.importDependencies(ctx, state)
		if err != nil {
			t.Fatalf("importDependencies() error = %v", err)
		}
		created, err := modelsService.GetByID(ctx, deps.models["old-model"])
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if created.Enable {
			t.Fatalf("disabled backup model imported enabled, want disabled")
		}
	})
}

func TestWriteJSONPreservesSensitiveValues(t *testing.T) {
	var buf bytes.Buffer
	manifest := Manifest{}
	writer := &zipBackupWriter{
		zw:       zip.NewWriter(&buf),
		manifest: &manifest,
		checksum: map[string]string{},
	}
	value := []map[string]any{{
		"name": "provider",
		"config": map[string]any{
			"api_key":  "secret-value",
			"base_url": "https://example.com",
		},
	}}
	if err := writer.writeJSON("dependencies/providers.json", "providers", value, ExportOptions{}); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
	if err := writer.zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	entries, err := readZipEntries(buf.Bytes())
	if err != nil {
		t.Fatalf("readZipEntries() error = %v", err)
	}
	raw := string(entries["dependencies/providers.json"].data)
	if !strings.Contains(raw, "secret-value") {
		t.Fatalf("sensitive value was not preserved: %s", raw)
	}
}

func TestExportScrubsACPManagedSecretsFromProfile(t *testing.T) {
	ctx := context.Background()
	conn, queries := newBotBackupExportTestDB(t)
	const ownerID = "00000000-0000-0000-0000-000000000101"
	const botID = "00000000-0000-0000-0000-000000000102"
	metadata := `{"acp":{"agents":{"hermes":{"enabled":true,"setup_mode":"api_key","managed":{"provider":"openrouter","model":"nousresearch/hermes","api_key":"secret-value"}}}}}`
	if _, err := conn.ExecContext(ctx, `
INSERT INTO users (id, username, email, role)
VALUES (?, ?, ?, ?)`, ownerID, "owner", "owner@example.com", "member"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bots (id, owner_user_id, type, name, display_name, is_active, status, metadata)
VALUES (?, ?, 'personal', ?, ?, 1, 'ready', ?)`, botID, ownerID, "hermes-bot", "Hermes Bot", metadata); err != nil {
		t.Fatalf("insert bot: %v", err)
	}

	service := New(Params{
		Queries:  queries,
		Bots:     bots.NewService(slog.New(slog.DiscardHandler), queries),
		Settings: settingspkg.NewService(slog.New(slog.DiscardHandler), queries, nil, nil),
	})
	var out bytes.Buffer
	if err := service.Export(ctx, botID, ExportOptions{Sections: []Section{SectionProfile}}, &out); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	entries, err := readZipEntries(out.Bytes())
	if err != nil {
		t.Fatalf("readZipEntries() error = %v", err)
	}
	profileRaw := string(entries["bot/profile.json"].data)
	if strings.Contains(profileRaw, "secret-value") || strings.Contains(profileRaw, `"api_key":`) {
		t.Fatalf("bot/profile.json leaked ACP secret: %s", profileRaw)
	}
	var manifest Manifest
	if err := json.Unmarshal(entries[ManifestPath].data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if !containsWarning(manifest.Warnings, "ACP managed secrets were excluded") {
		t.Fatalf("manifest warnings = %v, want ACP secret warning", manifest.Warnings)
	}
}

func TestRebuildRestoredHistoryTurnsLinksVisibleLinearTurns(t *testing.T) {
	ctx := context.Background()
	botID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000701")
	sessionID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000702")
	userID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000711")
	assistantID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000712")
	toolID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000713")
	finalID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000714")
	nextUserID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000715")
	nextAssistantID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000716")
	q := &recordingRestoredTurnQueries{}

	err := rebuildRestoredHistoryTurns(ctx, q, botID, []restoredHistoryMessage{
		{id: userID, sessionID: sessionID, role: "user"},
		{id: assistantID, sessionID: sessionID, role: "assistant"},
		{id: toolID, sessionID: sessionID, role: "tool"},
		{id: finalID, sessionID: sessionID, role: "assistant"},
		{id: nextUserID, sessionID: sessionID, role: "user"},
		{id: nextAssistantID, sessionID: sessionID, role: "assistant"},
	})
	if err != nil {
		t.Fatalf("rebuildRestoredHistoryTurns() error = %v", err)
	}
	if got, want := len(q.turns), 2; got != want {
		t.Fatalf("created turns = %d, want %d", got, want)
	}
	if got := q.turns[0].RequestMessageID.String(); got != userID.String() {
		t.Fatalf("first request message = %s, want %s", got, userID.String())
	}
	if got := q.turns[1].RequestMessageID.String(); got != nextUserID.String() {
		t.Fatalf("second request message = %s, want %s", got, nextUserID.String())
	}
	if got, want := len(q.binds), 2; got != want {
		t.Fatalf("assistant binds = %d, want %d", got, want)
	}
	if got := q.binds[0].AssistantMessageID.String(); got != assistantID.String() {
		t.Fatalf("first assistant bind = %s, want %s", got, assistantID.String())
	}
	wantSeq := []int64{1, 2, 3, 4, 1, 2}
	if got, want := len(q.links), len(wantSeq); got != want {
		t.Fatalf("links = %d, want %d", got, want)
	}
	for i, want := range wantSeq {
		if got := q.links[i].TurnMessageSeq.Int64; got != want {
			t.Fatalf("link %d seq = %d, want %d", i, got, want)
		}
	}
}

func TestRebuildRestoredHistoryTurnsKeepsOrphanAssistantSeq(t *testing.T) {
	ctx := context.Background()
	botID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000741")
	sessionID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000742")
	assistantID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000743")
	toolID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000744")
	finalID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000745")
	q := &recordingRestoredTurnQueries{}

	err := rebuildRestoredHistoryTurns(ctx, q, botID, []restoredHistoryMessage{
		{id: assistantID, sessionID: sessionID, role: "assistant"},
		{id: toolID, sessionID: sessionID, role: "tool"},
		{id: finalID, sessionID: sessionID, role: "assistant"},
	})
	if err != nil {
		t.Fatalf("rebuildRestoredHistoryTurns() error = %v", err)
	}
	if got, want := len(q.turns), 1; got != want {
		t.Fatalf("created turns = %d, want %d", got, want)
	}
	if got := q.turns[0].AssistantMessageID.String(); got != assistantID.String() {
		t.Fatalf("orphan assistant message = %s, want %s", got, assistantID.String())
	}
	wantSeq := []int64{2, 3, 4}
	if got, want := len(q.links), len(wantSeq); got != want {
		t.Fatalf("links = %d, want %d", got, want)
	}
	for i, want := range wantSeq {
		if got := q.links[i].TurnMessageSeq.Int64; got != want {
			t.Fatalf("link %d seq = %d, want %d", i, got, want)
		}
	}
}

func TestRestoreHistoryRebuildsImportedReadModel(t *testing.T) {
	ctx := context.Background()
	conn, queries := newBotBackupExportTestDB(t)
	const ownerID = "00000000-0000-0000-0000-000000000731"
	const botID = "00000000-0000-0000-0000-000000000732"
	sourceSessionID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000733")
	sourceUserID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000734")
	sourceAssistantID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000735")
	if _, err := conn.ExecContext(ctx, `
INSERT INTO users (id, username, email, role)
VALUES (?, ?, ?, ?)`, ownerID, "restore-owner", "restore-owner@example.com", "member"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bots (id, owner_user_id, type, name, display_name, is_active, status, metadata)
VALUES (?, ?, 'personal', ?, ?, 1, 'ready', '{}')`, botID, ownerID, "restore-bot", "Restore Bot"); err != nil {
		t.Fatalf("insert bot: %v", err)
	}

	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json": jsonEntry(t, []dbsqlc.ListSessionsByBotRow{{
				ID:              sourceSessionID,
				BotID:           mustTestPGUUID(t, botID),
				ChannelType:     pgtype.Text{String: "local", Valid: true},
				Type:            "chat",
				SessionMode:     "chat",
				RuntimeType:     "model",
				RuntimeMetadata: []byte(`{}`),
				Title:           "Restored",
				Metadata:        []byte(`{}`),
			}}),
			"history/messages.json": jsonEntry(t, []dbsqlc.ListMessagesRow{
				{
					ID:          sourceUserID,
					BotID:       mustTestPGUUID(t, botID),
					SessionID:   sourceSessionID,
					Role:        "user",
					Content:     []byte(`{"role":"user","content":"hello"}`),
					Metadata:    []byte(`{}`),
					Usage:       []byte(`{}`),
					SessionMode: "chat",
					RuntimeType: "model",
					DisplayText: pgtype.Text{String: "hello", Valid: true},
				},
				{
					ID:          sourceAssistantID,
					BotID:       mustTestPGUUID(t, botID),
					SessionID:   sourceSessionID,
					Role:        "assistant",
					Content:     []byte(`{"role":"assistant","content":"world"}`),
					Metadata:    []byte(`{}`),
					Usage:       []byte(`{}`),
					SessionMode: "chat",
					RuntimeType: "model",
					DisplayText: pgtype.Text{String: "world", Valid: true},
				},
			}),
		},
		counts: map[Section]int{},
	}
	service := &Service{queries: queries}
	if err := service.restoreHistory(ctx, ownerID, botID, state, false, false); err != nil {
		t.Fatalf("restoreHistory() error = %v", err)
	}
	sessions, err := queries.ListSessionsByBot(ctx, mustTestPGUUID(t, botID))
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if got, want := len(sessions), 1; got != want {
		t.Fatalf("restored sessions = %d, want %d", got, want)
	}
	rows, err := queries.ListMessagesBySession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatalf("list restored messages: %v", err)
	}
	if got, want := len(rows), 2; got != want {
		t.Fatalf("visible restored messages = %d, want %d", got, want)
	}
	if rows[0].Role != "user" || rows[1].Role != "assistant" {
		t.Fatalf("restored roles = [%s %s], want [user assistant]", rows[0].Role, rows[1].Role)
	}
	if got := state.counts[SectionHistory]; got != 2 {
		t.Fatalf("history count = %d, want 2", got)
	}
}

func TestCollectHistoryExportsSupersededMessagesAndTurns(t *testing.T) {
	ctx := context.Background()
	conn, queries := newBotBackupExportTestDB(t)
	const ownerID = "00000000-0000-0000-0000-000000000761"
	const botID = "00000000-0000-0000-0000-000000000762"
	insertBackupTestOwnerAndBot(t, conn, ownerID, botID)
	session, err := queries.CreateSession(ctx, dbsqlc.CreateSessionParams{
		BotID:           mustTestPGUUID(t, botID),
		ChannelType:     pgtype.Text{String: "local", Valid: true},
		Type:            "chat",
		SessionMode:     "chat",
		RuntimeType:     "model",
		RuntimeMetadata: []byte(`{}`),
		Title:           "Export full history",
		Metadata:        []byte(`{}`),
		CreatedByUserID: mustTestPGUUID(t, ownerID),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	userMsg := createBackupTestMessage(t, ctx, queries, botID, session.ID, "user", "hello")
	oldAssistant := createBackupTestMessage(t, ctx, queries, botID, session.ID, "assistant", "old")
	newAssistant := createBackupTestMessage(t, ctx, queries, botID, session.ID, "assistant", "new")
	oldTurn, err := queries.CreateHistoryTurn(ctx, dbsqlc.CreateHistoryTurnParams{
		BotID:              mustTestPGUUID(t, botID),
		SessionID:          session.ID,
		AssistantMessageID: oldAssistant.ID,
	})
	if err != nil {
		t.Fatalf("create old turn: %v", err)
	}
	newTurn, err := queries.CreateHistoryTurn(ctx, dbsqlc.CreateHistoryTurnParams{
		BotID:              mustTestPGUUID(t, botID),
		SessionID:          session.ID,
		RequestMessageID:   userMsg.ID,
		AssistantMessageID: newAssistant.ID,
	})
	if err != nil {
		t.Fatalf("create replacement turn: %v", err)
	}
	linkBackupTestMessage(t, ctx, queries, oldTurn.ID, oldAssistant.ID, 2)
	linkBackupTestMessage(t, ctx, queries, newTurn.ID, userMsg.ID, 1)
	linkBackupTestMessage(t, ctx, queries, newTurn.ID, newAssistant.ID, 2)
	if _, err := queries.SupersedeHistoryTurn(ctx, dbsqlc.SupersedeHistoryTurnParams{
		OldTurnID:          oldTurn.ID,
		SessionID:          session.ID,
		SupersededByTurnID: newTurn.ID,
		SupersededAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
		SupersededReason:   pgtype.Text{String: "retry", Valid: true},
	}); err != nil {
		t.Fatalf("supersede old turn: %v", err)
	}

	service := &Service{queries: queries}
	history, warnings := service.collectHistory(ctx, botID, false)
	if len(warnings) > 0 {
		t.Fatalf("collectHistory warnings = %v", warnings)
	}
	messages, ok := history.Messages.([]dbsqlc.ListAllMessagesForBackupRow)
	if !ok {
		t.Fatalf("history messages type = %T", history.Messages)
	}
	if got, want := len(messages), 3; got != want {
		t.Fatalf("exported messages = %d, want %d", got, want)
	}
	turns, ok := history.Turns.([]dbsqlc.BotHistoryTurn)
	if !ok {
		t.Fatalf("history turns type = %T", history.Turns)
	}
	if got, want := len(turns), 2; got != want {
		t.Fatalf("exported turns = %d, want %d", got, want)
	}
	foundHiddenOld := false
	for _, msg := range messages {
		if msg.ID.String() == oldAssistant.ID.String() {
			foundHiddenOld = !msg.TurnVisible && msg.TurnID.String() == oldTurn.ID.String()
		}
	}
	if !foundHiddenOld {
		t.Fatal("superseded assistant was not exported as hidden full-history message")
	}
}

func TestRestoreHistoryImportsFullSupersededTurnHistory(t *testing.T) {
	ctx := context.Background()
	conn, queries := newBotBackupExportTestDB(t)
	const ownerID = "00000000-0000-0000-0000-000000000771"
	const botID = "00000000-0000-0000-0000-000000000772"
	insertBackupTestOwnerAndBot(t, conn, ownerID, botID)
	sourceSessionID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000773")
	sourceUserID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000774")
	sourceOldAssistantID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000775")
	sourceNewAssistantID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000776")
	sourceOldTurnID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000777")
	sourceNewTurnID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000778")
	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json": jsonEntry(t, []dbsqlc.ListSessionsByBotRow{{
				ID:              sourceSessionID,
				BotID:           mustTestPGUUID(t, botID),
				ChannelType:     pgtype.Text{String: "local", Valid: true},
				Type:            "chat",
				SessionMode:     "chat",
				RuntimeType:     "model",
				RuntimeMetadata: []byte(`{}`),
				Title:           "Full Restore",
				Metadata:        []byte(`{}`),
			}}),
			"history/messages.json": jsonEntry(t, []dbsqlc.ListAllMessagesForBackupRow{
				backupTestFullMessage(t, botID, sourceSessionID, sourceUserID, sourceNewTurnID, 1, true, "user", "hello"),
				backupTestFullMessage(t, botID, sourceSessionID, sourceOldAssistantID, sourceOldTurnID, 2, false, "assistant", "old"),
				backupTestFullMessage(t, botID, sourceSessionID, sourceNewAssistantID, sourceNewTurnID, 2, true, "assistant", "new"),
			}),
			"history/turns.json": jsonEntry(t, []dbsqlc.BotHistoryTurn{
				{
					ID:                 sourceOldTurnID,
					BotID:              mustTestPGUUID(t, botID),
					SessionID:          sourceSessionID,
					Position:           1,
					AssistantMessageID: sourceOldAssistantID,
					SupersededByTurnID: sourceNewTurnID,
					SupersededAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
					SupersededReason:   pgtype.Text{String: "retry", Valid: true},
				},
				{
					ID:                 sourceNewTurnID,
					BotID:              mustTestPGUUID(t, botID),
					SessionID:          sourceSessionID,
					Position:           2,
					RequestMessageID:   sourceUserID,
					AssistantMessageID: sourceNewAssistantID,
				},
			}),
		},
		counts: map[Section]int{},
	}
	service := &Service{queries: queries}
	if err := service.restoreHistory(ctx, ownerID, botID, state, false, false); err != nil {
		t.Fatalf("restoreHistory() error = %v", err)
	}
	sessions, err := queries.ListSessionsByBot(ctx, mustTestPGUUID(t, botID))
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if got, want := len(sessions), 1; got != want {
		t.Fatalf("restored sessions = %d, want %d", got, want)
	}
	visible, err := queries.ListMessagesBySession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatalf("list visible messages: %v", err)
	}
	if got, want := len(visible), 2; got != want {
		t.Fatalf("visible messages = %d, want %d", got, want)
	}
	if visible[0].Role != "user" || visible[1].DisplayText.String != "new" {
		t.Fatalf("visible restored messages = (%s, %s), want user/new", visible[0].Role, visible[1].DisplayText.String)
	}
	allMessages, err := queries.ListAllMessagesForBackup(ctx, mustTestPGUUID(t, botID))
	if err != nil {
		t.Fatalf("list all messages: %v", err)
	}
	if got, want := len(allMessages), 3; got != want {
		t.Fatalf("all restored messages = %d, want %d", got, want)
	}
	foundHiddenOld := false
	for _, msg := range allMessages {
		if msg.DisplayText.String == "old" {
			foundHiddenOld = !msg.TurnVisible
		}
	}
	if !foundHiddenOld {
		t.Fatal("restored superseded assistant missing from full history or still visible")
	}
	restoredTurns, err := queries.ListHistoryTurnsByBot(ctx, mustTestPGUUID(t, botID))
	if err != nil {
		t.Fatalf("list restored turns: %v", err)
	}
	if got, want := len(restoredTurns), 2; got != want {
		t.Fatalf("restored turns = %d, want %d", got, want)
	}
	if !restoredTurns[0].SupersededAt.Valid || !restoredTurns[0].SupersededByTurnID.Valid {
		t.Fatal("old turn superseded metadata was not restored")
	}
	restoredSession, err := queries.GetSessionByID(ctx, sessions[0].ID)
	if err != nil {
		t.Fatalf("get restored session: %v", err)
	}
	if got, want := restoredSession.NextTurnPosition, int64(3); got != want {
		t.Fatalf("next_turn_position = %d, want %d", got, want)
	}
	if got, want := state.counts[SectionHistory], 3; got != want {
		t.Fatalf("history count = %d, want %d", got, want)
	}
}

func TestRebindRestoredForkMetadataMapsImportedIDs(t *testing.T) {
	oldSessionID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000781")
	newSessionID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000782")
	oldMessageID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000783")
	newMessageID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000784")
	oldForkMessageID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000785")
	newForkMessageID := mustTestPGUUID(t, "00000000-0000-0000-0000-000000000786")
	raw := []byte(`{"forked_from":{"session_id":"` + oldSessionID.String() + `","message_id":"` + oldMessageID.String() + `","fork_message_id":"` + oldForkMessageID.String() + `"}}`)

	got, changed := rebindRestoredForkMetadata(raw, map[string]pgtype.UUID{
		oldSessionID.String(): newSessionID,
	}, map[string]pgtype.UUID{
		oldMessageID.String():     newMessageID,
		oldForkMessageID.String(): newForkMessageID,
	})
	if !changed {
		t.Fatal("rebindRestoredForkMetadata changed = false, want true")
	}
	var decoded struct {
		ForkedFrom struct {
			SessionID     string `json:"session_id"`
			MessageID     string `json:"message_id"`
			ForkMessageID string `json:"fork_message_id"`
		} `json:"forked_from"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal rebound metadata: %v", err)
	}
	if decoded.ForkedFrom.SessionID != newSessionID.String() ||
		decoded.ForkedFrom.MessageID != newMessageID.String() ||
		decoded.ForkedFrom.ForkMessageID != newForkMessageID.String() {
		t.Fatalf("rebound fork metadata = %+v", decoded.ForkedFrom)
	}
}

func TestScrubImportedProfileACPSecrets(t *testing.T) {
	for _, tc := range []struct {
		name         string
		warnings     []string
		wantWarnings []string
	}{
		{
			name:         "adds warning",
			wantWarnings: []string{acpManagedSecretsWarning},
		},
		{
			name:         "dedupes warning",
			warnings:     []string{acpManagedSecretsWarning},
			wantWarnings: []string{acpManagedSecretsWarning},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &importState{warnings: append([]string(nil), tc.warnings...)}
			profile := bots.Bot{
				Metadata: map[string]any{
					"acp": map[string]any{
						"agents": map[string]any{
							"hermes": map[string]any{
								"enabled":    true,
								"setup_mode": "api_key",
								"managed": map[string]any{
									"provider": "openrouter",
									"model":    "nousresearch/hermes",
									"api_key":  "secret-value",
								},
							},
						},
					},
				},
			}

			scrubbed := scrubImportedProfileACPSecrets(profile, state)
			raw, err := json.Marshal(scrubbed.Metadata)
			if err != nil {
				t.Fatalf("marshal metadata: %v", err)
			}
			if strings.Contains(string(raw), "secret-value") || strings.Contains(string(raw), `"api_key":`) {
				t.Fatalf("imported profile kept ACP secret: %s", raw)
			}
			if len(state.warnings) != len(tc.wantWarnings) {
				t.Fatalf("warnings = %v, want %v", state.warnings, tc.wantWarnings)
			}
			for i := range tc.wantWarnings {
				if state.warnings[i] != tc.wantWarnings[i] {
					t.Fatalf("warnings = %v, want %v", state.warnings, tc.wantWarnings)
				}
			}
		})
	}
}

func TestImportOverwriteScrubsACPSecretsAndClosesRuntimes(t *testing.T) {
	ctx := context.Background()
	conn, queries := newBotBackupExportTestDB(t)
	const ownerID = "00000000-0000-0000-0000-000000000201"
	const targetBotID = "00000000-0000-0000-0000-000000000202"
	const sourceBotID = "00000000-0000-0000-0000-000000000203"
	targetMetadata := `{"acp":{"agents":{"hermes":{"enabled":true,"setup_mode":"api_key","managed":{"provider":"openrouter","model":"target-model","api_key":"target-secret","base_url":"https://target.example"}}}}}`
	sourceProfile := bots.Bot{
		ID:          sourceBotID,
		DisplayName: "Imported Hermes",
		Timezone:    "UTC",
		IsActive:    true,
		Metadata: map[string]any{
			"acp": map[string]any{
				"agents": map[string]any{
					"hermes": map[string]any{
						"enabled":    true,
						"setup_mode": "api_key",
						"managed": map[string]any{
							"provider": "openrouter",
							"model":    "source-model",
							"api_key":  "source-secret",
						},
					},
				},
			},
		},
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO users (id, username, email, role)
VALUES (?, ?, ?, ?)`, ownerID, "owner", "owner@example.com", "member"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bots (id, owner_user_id, type, name, display_name, is_active, status, metadata)
VALUES (?, ?, 'personal', ?, ?, 1, 'ready', ?)`, targetBotID, ownerID, "target-hermes", "Target Hermes", targetMetadata); err != nil {
		t.Fatalf("insert target bot: %v", err)
	}

	closer := &recordingACPRuntimeCloser{}
	service := New(Params{
		Queries:     queries,
		Bots:        bots.NewService(slog.New(slog.DiscardHandler), queries),
		ACPRuntimes: closer,
	})
	result, err := service.Import(ctx, ownerID, makeProfileImportBundle(t, sourceProfile), ImportOptions{
		Mode:        ImportModeOverwrite,
		TargetBotID: targetBotID,
		Sections: map[Section]ImportStrategy{
			SectionProfile: StrategyMerge,
		},
	}, "")
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.BotID != targetBotID || result.Created {
		t.Fatalf("Import() result = %+v, want overwrite target", result)
	}
	if !containsWarning(result.Warnings, "ACP managed secrets were excluded") {
		t.Fatalf("warnings = %v, want ACP secret warning", result.Warnings)
	}
	var rawMetadata string
	if err := conn.QueryRowContext(ctx, `SELECT metadata FROM bots WHERE id = ?`, targetBotID).Scan(&rawMetadata); err != nil {
		t.Fatalf("select target metadata: %v", err)
	}
	if strings.Contains(rawMetadata, "target-secret") || strings.Contains(rawMetadata, "source-secret") {
		t.Fatalf("overwrite import retained ACP secret metadata: %s", rawMetadata)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
		t.Fatalf("unmarshal target metadata: %v", err)
	}
	setup := acpprofile.ParseAgentSetup(metadata, acpprofile.AgentHermesID)
	if _, ok := setup.Managed["api_key"]; ok {
		t.Fatalf("overwrite import retained managed api_key: %s", rawMetadata)
	}
	if got := setup.Managed["model"]; got != "source-model" {
		t.Fatalf("imported Hermes model = %q, want source-model", got)
	}
	if !closer.hasCall(targetBotID, acpprofile.AgentHermesID) {
		t.Fatalf("runtime closer calls = %#v, missing Hermes close for target bot", closer.calls)
	}
}

type recordingACPRuntimeCloser struct {
	calls []string
}

func (r *recordingACPRuntimeCloser) CloseBotAgentRuntimes(botID, agentID string) error {
	r.calls = append(r.calls, botID+"/"+agentID)
	return nil
}

func (r *recordingACPRuntimeCloser) hasCall(botID, agentID string) bool {
	want := botID + "/" + agentID
	for _, call := range r.calls {
		if call == want {
			return true
		}
	}
	return false
}

func makeProfileImportBundle(t *testing.T, profile bots.Bot) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	profileRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	w, err := zw.Create("bot/profile.json")
	if err != nil {
		t.Fatalf("create profile entry: %v", err)
	}
	if _, err := w.Write(profileRaw); err != nil {
		t.Fatalf("write profile entry: %v", err)
	}
	manifestRaw, err := json.Marshal(Manifest{
		SchemaVersion: BackupSchemaVersion,
		SourceBotID:   profile.ID,
		SourceBotName: profile.DisplayName,
		Entries: []ManifestEntry{{
			Path: "bot/profile.json",
			Type: "bot_profile",
		}},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	w, err = zw.Create(ManifestPath)
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := w.Write(manifestRaw); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func jsonEntry(t *testing.T, value any) backupZipEntry {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal backup entry: %v", err)
	}
	return backupZipEntry{data: raw}
}

type recordingRestoredTurnQueries struct {
	turns []dbsqlc.CreateHistoryTurnParams
	binds []dbsqlc.BindHistoryTurnAssistantByRequestParams
	links []dbsqlc.LinkMessageToHistoryTurnParams
}

func (q *recordingRestoredTurnQueries) CreateHistoryTurn(_ context.Context, arg dbsqlc.CreateHistoryTurnParams) (dbsqlc.BotHistoryTurn, error) {
	q.turns = append(q.turns, arg)
	return dbsqlc.BotHistoryTurn{ID: mustPGUUID(fmt.Sprintf("00000000-0000-0000-0000-%012d", len(q.turns)))}, nil
}

func (q *recordingRestoredTurnQueries) BindHistoryTurnAssistantByRequest(_ context.Context, arg dbsqlc.BindHistoryTurnAssistantByRequestParams) (dbsqlc.BotHistoryTurn, error) {
	q.binds = append(q.binds, arg)
	return dbsqlc.BotHistoryTurn{}, nil
}

func (q *recordingRestoredTurnQueries) LinkMessageToHistoryTurn(_ context.Context, arg dbsqlc.LinkMessageToHistoryTurnParams) (pgtype.UUID, error) {
	q.links = append(q.links, arg)
	return arg.MessageID, nil
}

func mustTestPGUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	out, err := parsePGUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return out
}

func mustPGUUID(value string) pgtype.UUID {
	out, err := parsePGUUID(value)
	if err != nil {
		panic(err)
	}
	return out
}

func parsePGUUID(value string) (pgtype.UUID, error) {
	var out pgtype.UUID
	if err := out.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return out, nil
}

func newBotBackupModelTestDB(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	execBotBackupModelSchema(t, conn)
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func newBotBackupExportTestDB(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	migrationPaths, err := filepath.Glob(filepath.Join("..", "..", "db", "sqlite", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob sqlite migrations: %v", err)
	}
	sort.Strings(migrationPaths)
	for _, schemaPath := range migrationPaths {
		schema, err := os.ReadFile(schemaPath) //nolint:gosec // test fixture path
		if err != nil {
			t.Fatalf("read sqlite schema %s: %v", schemaPath, err)
		}
		if _, err := conn.ExecContext(context.Background(), string(schema)); err != nil {
			t.Fatalf("exec sqlite schema %s: %v", schemaPath, err)
		}
	}
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func execBotBackupModelSchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), `
CREATE TABLE providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  client_type TEXT NOT NULL DEFAULT 'openai-completions',
  icon TEXT,
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT providers_name_unique UNIQUE (name)
);

CREATE TABLE models (
  id TEXT PRIMARY KEY,
  model_id TEXT NOT NULL,
  name TEXT,
  provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'chat',
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT models_provider_id_model_id_unique UNIQUE (provider_id, model_id)
);
`)
	if err != nil {
		t.Fatalf("exec botbackup model schema: %v", err)
	}
}

func insertBotBackupProvider(t *testing.T, conn *sql.DB, id string) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), `
INSERT INTO providers (id, name, client_type, icon, enable, config, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		"provider-"+id,
		string(modelpkg.ClientTypeOpenAICompletions),
		"",
		1,
		`{}`,
		`{}`,
	)
	if err != nil {
		t.Fatalf("insert botbackup provider: %v", err)
	}
}

func insertBackupTestOwnerAndBot(t *testing.T, conn *sql.DB, ownerID, botID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `
INSERT INTO users (id, username, email, role)
VALUES (?, ?, ?, ?)`, ownerID, "owner-"+ownerID, ownerID+"@example.com", "member"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bots (id, owner_user_id, type, name, display_name, is_active, status, metadata)
VALUES (?, ?, 'personal', ?, ?, 1, 'ready', '{}')`, botID, ownerID, "bot-"+botID, "Backup Bot"); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
}

func createBackupTestMessage(t *testing.T, ctx context.Context, queries *sqlitestore.Queries, botID string, sessionID pgtype.UUID, role, text string) dbsqlc.CreateMessageRow {
	t.Helper()
	created, err := queries.CreateMessage(ctx, dbsqlc.CreateMessageParams{
		BotID:       mustTestPGUUID(t, botID),
		SessionID:   sessionID,
		Role:        role,
		Content:     []byte(fmt.Sprintf(`{"role":%q,"content":%q}`, role, text)),
		Metadata:    []byte(`{}`),
		Usage:       []byte(`{}`),
		SessionMode: "chat",
		RuntimeType: "model",
		DisplayText: pgtype.Text{String: text, Valid: true},
	})
	if err != nil {
		t.Fatalf("create %s message: %v", role, err)
	}
	return created
}

func linkBackupTestMessage(t *testing.T, ctx context.Context, queries *sqlitestore.Queries, turnID, messageID pgtype.UUID, seq int64) {
	t.Helper()
	if _, err := queries.LinkMessageToHistoryTurn(ctx, dbsqlc.LinkMessageToHistoryTurnParams{
		TurnID:         turnID,
		MessageID:      messageID,
		TurnMessageSeq: pgtype.Int8{Int64: seq, Valid: true},
	}); err != nil {
		t.Fatalf("link message to turn: %v", err)
	}
}

func backupTestFullMessage(t *testing.T, botID string, sessionID, messageID, turnID pgtype.UUID, seq int64, visible bool, role, text string) dbsqlc.ListAllMessagesForBackupRow {
	t.Helper()
	return dbsqlc.ListAllMessagesForBackupRow{
		ID:             messageID,
		BotID:          mustTestPGUUID(t, botID),
		SessionID:      sessionID,
		Role:           role,
		Content:        []byte(fmt.Sprintf(`{"role":%q,"content":%q}`, role, text)),
		Metadata:       []byte(`{}`),
		Usage:          []byte(`{}`),
		SessionMode:    "chat",
		RuntimeType:    "model",
		DisplayText:    pgtype.Text{String: text, Valid: true},
		TurnID:         turnID,
		TurnPosition:   pgtype.Int8{Int64: 1, Valid: true},
		TurnMessageSeq: pgtype.Int8{Int64: seq, Valid: true},
		TurnVisible:    visible,
	}
}

func containsWarning(warnings []string, want string) bool {
	for _, item := range warnings {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func TestWorkspaceStoredVerbatimAsTarGz(t *testing.T) {
	// Build a workspace tar.gz as the container would return it.
	var workspace bytes.Buffer
	gw := gzip.NewWriter(&workspace)
	tw := tar.NewWriter(gw)
	body := []byte("hello workspace")
	if err := tw.WriteHeader(&tar.Header{Name: "notes/readme.txt", Typeflag: tar.TypeReg, Mode: 0o640, Size: int64(len(body))}); err != nil {
		t.Fatalf("WriteHeader(file) error = %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("Write(file) error = %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	original := workspace.Bytes()

	// Store it verbatim as the single workspace entry, as writeWorkspace does.
	var backup bytes.Buffer
	manifest := Manifest{}
	writer := &zipBackupWriter{
		zw:       zip.NewWriter(&backup),
		manifest: &manifest,
		checksum: map[string]string{},
	}
	if err := writer.writeStream(workspaceArchivePath, bytes.NewReader(original), 0o640, time.Time{}, zip.Store); err != nil {
		t.Fatalf("writeStream() error = %v", err)
	}
	if err := writer.zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries, err := readZipEntries(backup.Bytes())
	if err != nil {
		t.Fatalf("readZipEntries() error = %v", err)
	}
	// The workspace must be a single nested tar.gz, not exploded files.
	if !hasEntry(entries, workspaceArchivePath) {
		t.Fatalf("workspace archive entry missing; entries=%v", entries)
	}
	if !hasWorkspaceEntries(entries) {
		t.Fatal("hasWorkspaceEntries should be true")
	}

	// The already-gzipped blob must be stored (not deflated again) to avoid
	// pointless double compression.
	if method := workspaceEntryMethod(t, backup.Bytes()); method != zip.Store {
		t.Fatalf("workspace entry method = %d, want zip.Store (%d)", method, zip.Store)
	}

	// The blob round-trips byte-for-byte (no re-packing).
	got, err := workspaceArchive(entries)
	if err != nil {
		t.Fatalf("workspaceArchive() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("workspace archive was not preserved verbatim")
	}

	// File listing reads names straight from the tar.gz headers.
	names := workspaceFileList(entries, sectionItemLimit)
	if len(names) != 1 || names[0] != "notes/readme.txt" {
		t.Fatalf("workspaceFileList = %v, want [notes/readme.txt]", names)
	}
	if n := countWorkspaceFiles(entries); n != 1 {
		t.Fatalf("countWorkspaceFiles = %d, want 1", n)
	}

	plain, err := readTarGzFile(got, "notes/readme.txt")
	if err != nil {
		t.Fatalf("read workspace file: %v", err)
	}
	if string(plain) != string(body) {
		t.Fatalf("workspace file = %q, want %q", plain, body)
	}
}

// workspaceEntryMethod returns the zip compression method used for the
// workspace archive entry within a backup zip.
func workspaceEntryMethod(t *testing.T, raw []byte) uint16 {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	for _, file := range zr.File {
		if file.Name == workspaceArchivePath {
			return file.Method
		}
	}
	t.Fatalf("workspace entry %q not found in zip", workspaceArchivePath)
	return 0
}

func readTarGzFile(raw []byte, name string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		if err != nil {
			return nil, err
		}
		if header.Name != name {
			continue
		}
		return io.ReadAll(tr)
	}
}

func TestIsWorkspaceRestoreRetryable(t *testing.T) {
	retryable := []string{
		"get container: not found",
		"No such container: workspace-123",
		"container not reachable: connection refused",
	}
	for _, msg := range retryable {
		if !isWorkspaceRestoreRetryable(errString(msg)) {
			t.Fatalf("expected retryable error: %s", msg)
		}
	}
	if isWorkspaceRestoreRetryable(io.ErrUnexpectedEOF) {
		t.Fatal("unexpected retryable generic error")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
