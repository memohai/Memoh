package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func bindTeamQueryFixture(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
CREATE OR REPLACE FUNCTION public.memoh_current_team_id()
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = pg_catalog, pg_temp
AS $$ SELECT pg_catalog.current_setting('memoh.team_id')::uuid $$;
SELECT set_config('memoh.team_id', '00000000-0000-0000-0000-000000000001', true);
`); err != nil {
		t.Fatalf("bind team query fixture: %v", err)
	}
}

func bindTeamMigrationFixture(t *testing.T, ctx context.Context, tx pgx.Tx, tables ...string) {
	t.Helper()
	bindTeamQueryFixture(t, ctx, tx)
	if _, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS public.teams (
  id UUID PRIMARY KEY,
  slug TEXT
);
ALTER TABLE public.teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.teams FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS teams_self_select ON public.teams;
CREATE POLICY teams_self_select ON public.teams
  FOR SELECT USING (id = public.memoh_current_team_id());
INSERT INTO public.teams (id, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'default')
ON CONFLICT (id) DO NOTHING;
`); err != nil {
		t.Fatalf("create team migration fixture: %v", err)
	}
	for _, table := range tables {
		if _, err := tx.Exec(ctx, "ALTER TABLE "+pgx.Identifier{table}.Sanitize()+" ADD COLUMN IF NOT EXISTS team_id UUID NOT NULL DEFAULT public.memoh_current_team_id()"); err != nil {
			t.Fatalf("scope migration fixture table %s: %v", table, err)
		}
	}
}
