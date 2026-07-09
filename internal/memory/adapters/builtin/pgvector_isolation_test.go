package builtin

import (
	"os"
	"strings"
	"testing"
)

// TestPgvectorSchemaHasTeamIsolationRLS locks the RLS backstop onto the vector
// table: ENABLE RLS + a team_isolation policy keyed on app.team_id. FORCE is
// deliberately absent (the owner bootstrap connection would otherwise be trapped
// at zero rows), so this guards against someone adding FORCE without also giving
// the pool a non-owner role + per-statement app.team_id.
func TestPgvectorSchemaHasTeamIsolationRLS(t *testing.T) {
	if !strings.Contains(pgvectorSchemaSQL, "ENABLE ROW LEVEL SECURITY") {
		t.Error("pgvector schema must ENABLE row level security on memory_node_embeddings")
	}
	if !strings.Contains(pgvectorSchemaSQL, "POLICY team_isolation") {
		t.Error("pgvector schema must create the team_isolation policy")
	}
	if !strings.Contains(pgvectorSchemaSQL, "current_setting('app.team_id'") {
		t.Error("team_isolation policy must key on app.team_id")
	}
	if strings.Contains(pgvectorSchemaSQL, "FORCE ROW LEVEL SECURITY") {
		t.Error("pgvector schema must NOT FORCE RLS: the owner bootstrap connection would be trapped at zero rows")
	}
}

// TestPgvectorDataQueriesFilterByTeam is the app-layer isolation guard: every
// data statement against memory_node_embeddings must carry a team_id predicate.
// This is the ACTIVE isolation for the vector store (the RLS policy above is
// inert on the current owner connection), so a regression here is a real
// cross-team leak.
func TestPgvectorDataQueriesFilterByTeam(t *testing.T) {
	// #nosec G304 -- reads a fixed checked-in source file in-package.
	src, err := os.ReadFile("pgvector_index.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	s := string(src)
	// Required predicates that keep each data statement team-scoped.
	for _, needle := range []string{
		"ON CONFLICT (team_id, bot_id, node_id, model_id)",           // upsert arbiter
		"WHERE team_id = $2\n  AND bot_id = $3",                      // search
		"DELETE FROM memory_node_embeddings\nWHERE team_id = $1",     // delete
		"WHERE team_id = $1\n  AND bot_id = $2\n  AND model_id = $3", // count
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("pgvector data query lost its team scope; missing predicate:\n%s", needle)
		}
	}
}
