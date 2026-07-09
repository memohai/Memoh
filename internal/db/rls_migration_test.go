package db

import (
	"strings"
	"testing"

	embeddeddb "github.com/memohai/memoh/db"
)

// TestRLSMigrationForcesAndCreatesRole locks the 0107 enforcement migration and
// the canonical 0001 schema to the two facts Phase 3 depends on: every tenant
// table is FORCE ROW LEVEL SECURITY, and the non-owner memoh_app runtime role is
// created. Without these, the team_isolation policy is bypassed at runtime.
func TestRLSMigrationForcesAndCreatesRole(t *testing.T) {
	for _, path := range []string{
		"postgres/migrations/0107_rls_enforcement.up.sql",
		"postgres/migrations/0001_init.up.sql",
	} {
		data, err := embeddeddb.MigrationsFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		s := string(data)
		if !strings.Contains(s, "FORCE ROW LEVEL SECURITY") {
			t.Errorf("%s missing FORCE ROW LEVEL SECURITY", path)
		}
		if !strings.Contains(s, "memoh_app") {
			t.Errorf("%s missing memoh_app role", path)
		}
		if !strings.Contains(s, "CREATE ROLE memoh_app") {
			t.Errorf("%s missing CREATE ROLE memoh_app", path)
		}
	}

	// The FORCE list in 0107 must match the ENABLE list in 0105 verbatim (same
	// tables). Extract both quoted table-name sets and compare them.
	force := forceTables(t)
	enable := enableTables(t)
	if len(force) == 0 {
		t.Fatal("0107 FORCE list is empty")
	}
	if len(force) != len(enable) {
		t.Fatalf("FORCE list has %d tables, ENABLE list has %d", len(force), len(enable))
	}
	for i := range force {
		if force[i] != enable[i] {
			t.Fatalf("FORCE/ENABLE table mismatch at %d: force=%q enable=%q", i, force[i], enable[i])
		}
	}
}

// forceTables returns the ordered table list from the 0107 FORCE array.
func forceTables(t *testing.T) []string {
	t.Helper()
	data, err := embeddeddb.MigrationsFS.ReadFile("postgres/migrations/0107_rls_enforcement.up.sql")
	if err != nil {
		t.Fatalf("read 0107: %v", err)
	}
	return tablesInFirstArray(string(data))
}

// enableTables returns the ordered table list from the 0105 ENABLE array (the
// DO block that runs ENABLE ROW LEVEL SECURITY on each tenant table).
func enableTables(t *testing.T) []string {
	t.Helper()
	data, err := embeddeddb.MigrationsFS.ReadFile("postgres/migrations/0105_team_multitenancy.up.sql")
	if err != nil {
		t.Fatalf("read 0105: %v", err)
	}
	s := string(data)
	idx := strings.Index(s, "ENABLE ROW LEVEL SECURITY")
	if idx < 0 {
		t.Fatal("0105 missing ENABLE ROW LEVEL SECURITY block")
	}
	// The ENABLE table array is the FOREACH ARRAY that precedes the loop body
	// containing "ENABLE ROW LEVEL SECURITY". Search backwards for its start.
	start := strings.LastIndex(s[:idx], "FOREACH table_name IN ARRAY ARRAY[")
	if start < 0 {
		t.Fatal("0105 missing FOREACH table_name array")
	}
	return tablesInFirstArray(s[start:])
}

// tablesInFirstArray extracts the single-quoted identifiers inside the first
// ARRAY[ ... ] literal found in s, preserving order.
func tablesInFirstArray(s string) []string {
	open := strings.Index(s, "ARRAY[")
	if open < 0 {
		return nil
	}
	rest := s[open+len("ARRAY["):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return nil
	}
	body := rest[:end]
	var out []string
	for _, part := range strings.Split(body, "'") {
		p := strings.TrimSpace(part)
		if p == "" || strings.Contains(p, ",") || strings.HasPrefix(p, "-") {
			continue
		}
		// Only accept bare lowercase identifiers (table names).
		if isTableIdent(p) {
			out = append(out, p)
		}
	}
	return out
}

func isTableIdent(s string) bool {
	for _, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return len(s) > 0
}
