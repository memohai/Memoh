package wikistore

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/db/sqlite/sqlc"
	"github.com/memohai/memoh/internal/memory/migrate"

	_ "modernc.org/sqlite"
)

// TestSQLiteStoreRoundTrip exercises the backend-agnostic Store contract
// against a real SQLite database with the wiki schema. It verifies
// Upsert/Get/List/Delete for nodes, edge derivation via RebuildImplicitEdges,
// and that deletes clear incident edges.
func TestSQLiteStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "wiki.sqlite")
	db := openSQLiteWikiDB(t, dbPath)
	defer db.Close()

	createWikiSchema(t, db)
	seedUserBot(t, db)

	store := NewSQLite(sqlc.New(db))

	botID := "00000000-0000-0000-0000-000000000201"
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)

	// Upsert two nodes sharing a profile_ref (should yield a same_profile edge).
	n1, err := store.UpsertNode(ctx, migrate.NodeSpec{
		ID: botID + ":mem_1", BotID: botID, Body: "User likes oolong tea", Hash: "h1",
		Layer: migrate.LayerPreference, Confidence: 0.9,
		ProfileRef: "user:42", Topic: "drinks", CapturedAt: now,
		Metadata: map[string]any{"k": "v"}, SourceMessageIDs: []string{"m1"},
	})
	if err != nil {
		t.Fatalf("UpsertNode 1: %v", err)
	}
	if n1.Confidence != 0.9 {
		t.Fatalf("round-trip confidence = %v, want 0.9", n1.Confidence)
	}
	if n1.Layer != migrate.LayerPreference {
		t.Fatalf("round-trip layer = %q, want preference", n1.Layer)
	}

	n2, err := store.UpsertNode(ctx, migrate.NodeSpec{
		ID: botID + ":mem_2", BotID: botID, Body: "User lives in Berlin", Hash: "h2",
		Layer: migrate.LayerNote, ProfileRef: "user:42", Topic: "location",
		CapturedAt: now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("UpsertNode 2: %v", err)
	}

	// Get + List.
	got, err := store.GetNode(ctx, botID, n1.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Body != "User likes oolong tea" {
		t.Fatalf("GetNode body = %q", got.Body)
	}
	if _, err := store.GetNode(ctx, botID, "missing"); err == nil {
		t.Fatal("GetNode missing should return ErrNodeNotFound")
	}

	listed, err := store.ListNodes(ctx, botID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("ListNodes = %d, want 2", len(listed))
	}

	byLayer, err := store.ListNodesByLayer(ctx, botID, migrate.LayerPreference)
	if err != nil {
		t.Fatalf("ListNodesByLayer: %v", err)
	}
	if len(byLayer) != 1 || byLayer[0].ID != n1.ID {
		t.Fatalf("ListNodesByLayer = %+v, want only mem_1", byLayer)
	}

	if n, _ := store.CountNodes(ctx, botID); n != 2 {
		t.Fatalf("CountNodes = %d, want 2", n)
	}

	// Rebuild implicit edges: n1+n2 share profile_ref user:42 -> same_profile edge.
	written, err := store.RebuildImplicitEdges(ctx, botID)
	if err != nil {
		t.Fatalf("RebuildImplicitEdges: %v", err)
	}
	if written == 0 {
		t.Fatal("RebuildImplicitEdges wrote 0 edges, expected at least the same_profile edge")
	}
	edges, err := store.ListEdges(ctx, botID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	foundProfile := false
	for _, e := range edges {
		if e.Rel == migrate.EdgeSameProfile {
			foundProfile = true
		}
	}
	if !foundProfile {
		t.Fatalf("no same_profile edge among %d edges", len(edges))
	}
	if c, _ := store.CountEdges(ctx, botID); c != written {
		t.Fatalf("CountEdges = %d, want %d", c, written)
	}

	// Idempotent rebuild: same count.
	written2, err := store.RebuildImplicitEdges(ctx, botID)
	if err != nil {
		t.Fatalf("RebuildImplicitEdges (2nd): %v", err)
	}
	if written2 != written {
		t.Fatalf("RebuildImplicitEdges not idempotent: %d then %d", written, written2)
	}

	// Update n2 to point at a different profile; rebuild should drop the old edge.
	if _, err := store.UpsertNode(ctx, migrate.NodeSpec{
		ID: n2.ID, BotID: botID, Body: n2.Body, Hash: "h2b",
		Layer: migrate.LayerNote, ProfileRef: "user:99", Topic: "location",
		CapturedAt: now.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertNode 2 (update): %v", err)
	}
	if _, err := store.RebuildImplicitEdges(ctx, botID); err != nil {
		t.Fatalf("RebuildImplicitEdges (3rd): %v", err)
	}
	edges2, _ := store.ListEdges(ctx, botID)
	for _, e := range edges2 {
		if e.Rel == migrate.EdgeSameProfile {
			t.Fatal("same_profile edge should be gone after profile_ref change")
		}
	}

	// Delete a node removes its incident edges too.
	if err := store.DeleteNode(ctx, botID, n1.ID); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	if list, _ := store.ListNodes(ctx, botID); len(list) != 1 {
		t.Fatalf("after delete, ListNodes = %d, want 1", len(list))
	}

	// DeleteAll clears everything.
	if err := store.DeleteAllNodes(ctx, botID); err != nil {
		t.Fatalf("DeleteAllNodes: %v", err)
	}
	if n, _ := store.CountNodes(ctx, botID); n != 0 {
		t.Fatalf("after DeleteAll, CountNodes = %d, want 0", n)
	}
	if n, _ := store.CountEdges(ctx, botID); n != 0 {
		t.Fatalf("after DeleteAll, CountEdges = %d, want 0", n)
	}
}

// ---- test helpers ----

func openSQLiteWikiDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// createWikiSchema creates the minimal schema (users, bots, memory_nodes,
// memory_edges) mirroring the 0001 + 0024 migrations, enough to exercise the
// wiki store without running the full migration set.
func createWikiSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, role TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS bots (id TEXT PRIMARY KEY, owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, type TEXT NOT NULL, name TEXT NOT NULL, display_name TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS memory_nodes (
			id TEXT PRIMARY KEY,
			bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
			body TEXT NOT NULL,
			hash TEXT NOT NULL,
			layer TEXT NOT NULL DEFAULT 'note',
			fact_type TEXT NOT NULL DEFAULT '',
			subject TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0.5,
			metadata TEXT NOT NULL DEFAULT '{}',
			source_message_ids TEXT NOT NULL DEFAULT '[]',
			profile_ref TEXT NOT NULL DEFAULT '',
			topic TEXT NOT NULL DEFAULT '',
			captured_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TEXT,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT memory_nodes_confidence_check CHECK (confidence >= 0 AND confidence <= 1)
		)`,
		`CREATE TABLE IF NOT EXISTS memory_edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
			src_node TEXT NOT NULL,
			dst_node TEXT NOT NULL,
			rel TEXT NOT NULL,
			weight REAL NOT NULL DEFAULT 1.0,
			metadata TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT memory_edges_unique UNIQUE (bot_id, src_node, dst_node, rel)
		)`,
	}
	for i, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create schema stmt %d: %v\n%s", i, err, s)
		}
	}
}

func seedUserBot(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, s := range []string{
		`INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000000101','wiki@example.com','member')`,
		`INSERT INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000000201','00000000-0000-0000-0000-000000000101','personal','wikibot','Wiki Bot')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v\n%s", err, fmt.Sprintf("stmt: %s", s))
		}
	}
}
