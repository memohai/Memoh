package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// inventoryRoot holds the pinned tenant-schema inventory artifacts, keyed by the
// immutable UPSTREAM_BASE_COMMIT the tenant work is based on. See the tenant
// schema contract phase-1 plan.
const inventoryRoot = "testdata/tenant_schema_inventory"

var fullSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// engineMatrix is the minimal shape of engine_matrix.json. It records the pinned
// upstream base commit and asserts the repository is PostgreSQL single-engine.
type engineMatrix struct {
	BaseCommit string   `json:"base_commit"`
	Engines    []string `json:"engines"`
	Postgres   struct {
		Present        bool   `json:"present"`
		Canonical      string `json:"canonical"`
		MigrationsGlob string `json:"migrations_glob"`
		QueryDir       string `json:"query_dir"`
		SqlcOut        string `json:"sqlc_out"`
		HeadVersion    string `json:"head_version"`
	} `json:"postgres"`
	Sqlc struct {
		Engine string `json:"engine"`
	} `json:"sqlc"`
	Blockers []string `json:"blockers"`
}

// pinnedBaseCommit resolves the single pinned UPSTREAM_BASE_COMMIT directory
// under inventoryRoot. Exactly one such directory must exist and its name must be
// a full 40-hex git SHA. If UPSTREAM_BASE_COMMIT is set in the environment it must
// match the pinned directory (guards against drift between env and committed pin).
func pinnedBaseCommit(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(inventoryRoot)
	if err != nil {
		t.Fatalf("read inventory root %s: %v", inventoryRoot, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 1 {
		t.Fatalf("expected exactly one pinned base-commit dir under %s, found %v", inventoryRoot, dirs)
	}
	base := dirs[0]
	if !fullSHARe.MatchString(base) {
		t.Fatalf("UPSTREAM_BASE_COMMIT must be a full 40-hex git SHA, got %q", base)
	}
	if env := os.Getenv("UPSTREAM_BASE_COMMIT"); env != "" && env != base {
		t.Fatalf("UPSTREAM_BASE_COMMIT env %q != pinned inventory dir %q", env, base)
	}
	return base
}

// repoPath resolves a repo-root-relative path from this package directory
// (internal/db -> repo root is two levels up).
func repoPath(rel string) string {
	return filepath.Join("..", "..", rel)
}

func TestUpstreamEngineMatrixPinned(t *testing.T) {
	base := pinnedBaseCommit(t)

	raw, err := os.ReadFile(filepath.Join(inventoryRoot, base, "engine_matrix.json"))
	if err != nil {
		t.Fatalf("read engine_matrix.json: %v", err)
	}
	var m engineMatrix
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse engine_matrix.json: %v", err)
	}

	if m.BaseCommit != base {
		t.Fatalf("engine_matrix base_commit %q != pinned dir %q", m.BaseCommit, base)
	}
	if len(m.Engines) != 1 || m.Engines[0] != "postgresql" {
		t.Fatalf("engines must be exactly [\"postgresql\"], got %v", m.Engines)
	}
	if !m.Postgres.Present {
		t.Fatal("postgres.present must be true")
	}
	if m.Sqlc.Engine != "postgresql" {
		t.Fatalf("sqlc engine must be postgresql, got %q", m.Sqlc.Engine)
	}
	if m.Postgres.HeadVersion == "" {
		t.Fatal("postgres.head_version must record the migration head")
	}
	if len(m.Blockers) != 0 {
		t.Fatalf("engine matrix has blockers (second engine or unresolved fact): %v", m.Blockers)
	}

	// The referenced canonical schema, query dir and sqlc output must actually
	// exist in the repository at the pinned base.
	for _, rel := range []string{m.Postgres.Canonical, m.Postgres.QueryDir, m.Postgres.SqlcOut} {
		if rel == "" {
			t.Fatal("engine matrix postgres paths must be non-empty")
		}
		if _, err := os.Stat(repoPath(rel)); err != nil {
			t.Fatalf("engine matrix references missing path %q: %v", rel, err)
		}
	}

	// There must be no non-postgres engine directory under db/.
	dbEntries, err := os.ReadDir(repoPath("db"))
	if err != nil {
		t.Fatalf("read db/: %v", err)
	}
	for _, e := range dbEntries {
		if !e.IsDir() {
			continue
		}
		if e.Name() != "postgres" {
			t.Fatalf("db/ contains a non-postgres engine dir %q; a second engine requires a new ADR", e.Name())
		}
	}
}
