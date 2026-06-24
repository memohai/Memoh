package botbackup

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/whatsapp"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	modelpkg "github.com/memohai/memoh/internal/models"
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

func TestFilterExportableChannelsSkipsWhatsApp(t *testing.T) {
	var warnings []string
	got := filterExportableChannels([]channel.ChannelConfig{
		{ChannelType: channel.ChannelType("telegram")},
		{ChannelType: whatsapp.Type},
		{ChannelType: channel.ChannelType("discord")},
	}, &warnings)
	if len(got) != 2 {
		t.Fatalf("exportable channels = %d, want 2", len(got))
	}
	if got[0].ChannelType != channel.ChannelType("telegram") || got[1].ChannelType != channel.ChannelType("discord") {
		t.Fatalf("exportable channels = %#v", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "whatsapp channel config skipped") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestFilterImportableChannelsSkipsWhatsApp(t *testing.T) {
	var warnings []string
	got := filterImportableChannels([]channel.ChannelConfig{
		{ChannelType: whatsapp.Type},
		{ChannelType: channel.ChannelType("telegram")},
	}, &warnings)
	if len(got) != 1 || got[0].ChannelType != channel.ChannelType("telegram") {
		t.Fatalf("importable channels = %#v", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "scan QR again") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestReplaceClearableChannelsPreservesWhatsApp(t *testing.T) {
	got := replaceClearableChannels([]channel.ChannelConfig{
		{ChannelType: channel.ChannelType("telegram")},
		{ChannelType: whatsapp.Type},
		{ChannelType: channel.ChannelType("discord")},
	})
	if len(got) != 2 {
		t.Fatalf("clearable channels = %d, want 2", len(got))
	}
	for _, cfg := range got {
		if cfg.ChannelType == whatsapp.Type {
			t.Fatalf("whatsapp channel should not be replace-clearable: %#v", got)
		}
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
