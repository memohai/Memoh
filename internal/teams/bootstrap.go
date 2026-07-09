package teams

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

type bootstrapDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func EnsureDefault(ctx context.Context, db bootstrapDB) error {
	if db == nil {
		return errors.New("team bootstrap requires database connection")
	}
	if _, err := db.Exec(ctx, `
INSERT INTO teams (id, slug, name, is_default)
VALUES ($1::uuid, $2, $3, true)
ON CONFLICT (id) DO UPDATE
SET slug = EXCLUDED.slug,
    is_default = true,
    updated_at = now()
`, DefaultTeamID, DefaultTeamSlug, "Default"); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `
INSERT INTO team_members (team_id, user_id, role)
SELECT $1::uuid,
       id,
       CASE WHEN role = 'admin' THEN 'admin' ELSE 'member' END
FROM users
ON CONFLICT (team_id, user_id) DO NOTHING
`, DefaultTeamID)
	return err
}
