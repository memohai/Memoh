package builtin

import (
	"context"
	"errors"
	"strings"
	"testing"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/team"
)

func TestPGVectorSchemaTeamIsolationContract(t *testing.T) {
	t.Parallel()
	required := []string{
		"CREATE OR REPLACE FUNCTION app.current_team_id()",
		"PRIMARY KEY (team_id, bot_id, node_id, model_id)",
		"FOREIGN KEY (team_id) REFERENCES teams(id)",
		"FORCE ROW LEVEL SECURITY",
		"memory_node_embeddings_team_select",
		"memory_node_embeddings_team_insert",
		"memory_node_embeddings_team_update",
		"memory_node_embeddings_team_delete",
		"ON memory_node_embeddings (team_id, bot_id, model_id)",
	}
	for _, fragment := range required {
		if !strings.Contains(pgvectorSchemaSQL, fragment) {
			t.Errorf("pgvector schema missing %q", fragment)
		}
	}
	if strings.Contains(pgvectorSchemaSQL, "NULLIF(current_setting") {
		t.Fatal("pgvector RLS must use the fail-closed team helper")
	}
}

func TestPGVectorTeamResolverDefaultsToSingleton(t *testing.T) {
	t.Parallel()
	index := &pgvectorIndex{resolveTeam: adapters.FixedTeamIDResolver(team.DefaultTeamID)}
	got, err := index.teamUUID(context.Background())
	if err != nil {
		t.Fatalf("teamUUID() error = %v", err)
	}
	if got.String() != team.DefaultTeamID {
		t.Fatalf("teamUUID() = %s, want %s", got.String(), team.DefaultTeamID)
	}
}

func TestPGVectorTeamResolverFailsClosed(t *testing.T) {
	t.Parallel()
	index := &pgvectorIndex{resolveTeam: func(context.Context) (string, error) {
		return "", errors.New("team missing")
	}}
	if _, err := index.teamUUID(context.Background()); err == nil {
		t.Fatal("teamUUID() without team succeeded")
	}
	index.resolveTeam = adapters.FixedTeamIDResolver("not-a-uuid")
	if _, err := index.teamUUID(context.Background()); err == nil {
		t.Fatal("teamUUID() with invalid team succeeded")
	}
}
