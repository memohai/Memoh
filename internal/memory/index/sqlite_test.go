package index

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

func TestSQLiteIndexDenseSearch(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	idx, err := NewSQLite(db)
	if err != nil {
		t.Fatalf("NewSQLite() error = %v", err)
	}
	ctx := context.Background()
	if err := idx.Health(ctx); err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if err := idx.UpsertDense(ctx, "point-1", DenseVector{Values: []float32{0, 0, 1}}, testPayload("bot-1", "mem-1", "oolong tea")); err != nil {
		t.Fatalf("UpsertDense point-1 error = %v", err)
	}
	if err := idx.UpsertDense(ctx, "point-2", DenseVector{Values: []float32{1, 0, 0}}, testPayload("bot-1", "mem-2", "berlin")); err != nil {
		t.Fatalf("UpsertDense point-2 error = %v", err)
	}

	results, err := idx.SearchDense(ctx, DenseVector{Values: []float32{0, 0, 1}}, "bot-1", 10)
	if err != nil {
		t.Fatalf("SearchDense() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 dense results, got %d", len(results))
	}
	if results[0].Payload["source_entry_id"] != "mem-1" {
		t.Fatalf("expected mem-1 first, got %#v", results[0])
	}

	count, err := idx.CountDense(ctx, "bot-1")
	if err != nil {
		t.Fatalf("CountDense() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	if err := idx.DeleteByIDs(ctx, []string{"point-1"}); err != nil {
		t.Fatalf("DeleteByIDs() error = %v", err)
	}
	count, err = idx.CountDense(ctx, "bot-1")
	if err != nil {
		t.Fatalf("CountDense after delete error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 after delete, got %d", count)
	}
}

func TestSQLiteIndexSparseSearch(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	idx, err := NewSQLite(db)
	if err != nil {
		t.Fatalf("NewSQLite() error = %v", err)
	}
	ctx := context.Background()

	if err := idx.UpsertSparse(ctx, "point-1", SparseVector{Indices: []uint32{7, 9}, Values: []float32{3, 1}}, testPayload("bot-1", "mem-1", "oolong tea")); err != nil {
		t.Fatalf("UpsertSparse point-1 error = %v", err)
	}
	if err := idx.UpsertSparse(ctx, "point-2", SparseVector{Indices: []uint32{9}, Values: []float32{5}}, testPayload("bot-1", "mem-2", "berlin")); err != nil {
		t.Fatalf("UpsertSparse point-2 error = %v", err)
	}

	results, err := idx.SearchSparse(ctx, SparseVector{Indices: []uint32{7}, Values: []float32{1}}, "bot-1", 10)
	if err != nil {
		t.Fatalf("SearchSparse() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 sparse result, got %d", len(results))
	}
	if results[0].Payload["source_entry_id"] != "mem-1" {
		t.Fatalf("expected mem-1, got %#v", results[0])
	}
}

func testPayload(botID, sourceID, memory string) map[string]string {
	return map[string]string{
		"bot_id":          botID,
		"source_entry_id": sourceID,
		"memory":          memory,
		"hash":            sourceID + "-hash",
		"created_at":      "2026-05-02T00:00:00Z",
		"updated_at":      "2026-05-02T00:00:00Z",
	}
}
