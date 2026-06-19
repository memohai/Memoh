package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// The paged session listings are hand-rolled for SQLite rather than generated
// via sqlc. sqlc-sqlite always emits explicit `?N` placeholders, which
// collide with the anonymous `?` placeholders that `sqlc.slice` expands into
// at run time — bound arguments get silently misrouted. Hand-rolling lets the
// query use sequential anonymous `?` only, with a single, unambiguous binding
// order.
//
// The cursor timestamp is bound as a plain `YYYY-MM-DD HH:MM:SS` UTC string so
// it compares lexicographically against `updated_at`, which SQLite stores in
// that same text form via CURRENT_TIMESTAMP.

const sessionPagedColumns = `s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.title, s.metadata,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type`

func listSessionsByBotPagedQuery(types int) string {
	placeholders := strings.TrimPrefix(strings.Repeat(",?", types), ",")
	return `
SELECT ` + sessionPagedColumns + `
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = ?
  AND s.deleted_at IS NULL
  AND (
    ? = 0
    OR s.updated_at < ?
    OR (
      s.updated_at = ?
      AND s.id < ?
    )
  )
  AND s.type IN (` + placeholders + `)
ORDER BY s.updated_at DESC, s.id DESC
LIMIT ?`
}

func listSessionsByBotAndUserPagedQuery(types int) string {
	placeholders := strings.TrimPrefix(strings.Repeat(",?", types), ",")
	return `
SELECT ` + sessionPagedColumns + `
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = ?
  AND s.created_by_user_id = ?
  AND s.deleted_at IS NULL
  AND (
    ? = 0
    OR s.updated_at < ?
    OR (
      s.updated_at = ?
      AND s.id < ?
    )
  )
  AND s.type IN (` + placeholders + `)
ORDER BY s.updated_at DESC, s.id DESC
LIMIT ?`
}

func (q *Queries) listSessionsByBotPaged(ctx context.Context, arg pgsqlc.ListSessionsByBotPagedParams) ([]pgsqlc.ListSessionsByBotPagedRow, error) {
	if q == nil || q.store == nil || q.store.db == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	if len(arg.Types) == 0 {
		// The caller never opts out of filtering — the handler always passes
		// an explicit non-empty types set. Fail loudly so we don't silently
		// match nothing or scan the world.
		return nil, errors.New("ListSessionsByBotPaged: types must not be empty")
	}
	botID, useCursor, cursorTS, cursorID := pagedCursorBindings(arg.BotID, arg.UseCursor, arg.CursorUpdatedAt, arg.CursorID)
	args := make([]any, 0, 5+len(arg.Types)+1)
	args = append(args, botID, useCursor, cursorTS, cursorTS, cursorID)
	for _, t := range arg.Types {
		args = append(args, t)
	}
	args = append(args, arg.LimitCount)

	rows, err := q.store.db.QueryContext(ctx, listSessionsByBotPagedQuery(len(arg.Types)), args...)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	defer func() { _ = rows.Close() }()
	return scanSessionPagedRows(rows, func(r sessionPagedScan) pgsqlc.ListSessionsByBotPagedRow {
		return pgsqlc.ListSessionsByBotPagedRow{
			ID: r.ID, BotID: r.BotID, RouteID: r.RouteID, ChannelType: r.ChannelType, Type: r.Type,
			Title: r.Title, Metadata: r.Metadata, ParentSessionID: r.ParentSessionID, CreatedByUserID: r.CreatedByUserID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
			RouteMetadata: r.RouteMetadata, RouteConversationType: r.RouteConversationType,
		}
	})
}

func (q *Queries) listSessionsByBotAndCreatedByUserPaged(ctx context.Context, arg pgsqlc.ListSessionsByBotAndCreatedByUserPagedParams) ([]pgsqlc.ListSessionsByBotAndCreatedByUserPagedRow, error) {
	if q == nil || q.store == nil || q.store.db == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	if len(arg.Types) == 0 {
		return nil, errors.New("ListSessionsByBotAndCreatedByUserPaged: types must not be empty")
	}
	botID, useCursor, cursorTS, cursorID := pagedCursorBindings(arg.BotID, arg.UseCursor, arg.CursorUpdatedAt, arg.CursorID)
	userID := nullableUUIDToString(arg.CreatedByUserID)
	args := make([]any, 0, 6+len(arg.Types)+1)
	args = append(args, botID, userID, useCursor, cursorTS, cursorTS, cursorID)
	for _, t := range arg.Types {
		args = append(args, t)
	}
	args = append(args, arg.LimitCount)

	rows, err := q.store.db.QueryContext(ctx, listSessionsByBotAndUserPagedQuery(len(arg.Types)), args...)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	defer func() { _ = rows.Close() }()
	return scanSessionPagedRows(rows, func(r sessionPagedScan) pgsqlc.ListSessionsByBotAndCreatedByUserPagedRow {
		return pgsqlc.ListSessionsByBotAndCreatedByUserPagedRow{
			ID: r.ID, BotID: r.BotID, RouteID: r.RouteID, ChannelType: r.ChannelType, Type: r.Type,
			Title: r.Title, Metadata: r.Metadata, ParentSessionID: r.ParentSessionID, CreatedByUserID: r.CreatedByUserID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
			RouteMetadata: r.RouteMetadata, RouteConversationType: r.RouteConversationType,
		}
	})
}

// pagedCursorBindings converts Postgres-typed cursor inputs into the string /
// int values the SQLite driver expects. The timestamp is formatted at second
// precision to match SQLite's TEXT storage from CURRENT_TIMESTAMP — any
// sub-second precision in the caller's timestamp is discarded deliberately.
func pagedCursorBindings(botID pgtype.UUID, useCursor bool, cursorUpdatedAt pgtype.Timestamptz, cursorID pgtype.UUID) (string, int, string, string) {
	useFlag := 0
	if useCursor {
		useFlag = 1
	}
	tsText := ""
	if cursorUpdatedAt.Valid {
		tsText = cursorUpdatedAt.Time.UTC().Format("2006-01-02 15:04:05")
	}
	return uuidToString(botID), useFlag, tsText, uuidToString(cursorID)
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func nullableUUIDToString(id pgtype.UUID) any {
	if !id.Valid {
		return nil
	}
	return id.String()
}

type sessionPagedScan struct {
	ID                    pgtype.UUID
	BotID                 pgtype.UUID
	RouteID               pgtype.UUID
	ChannelType           pgtype.Text
	Type                  string
	Title                 string
	Metadata              []byte
	ParentSessionID       pgtype.UUID
	CreatedByUserID       pgtype.UUID
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
	DeletedAt             pgtype.Timestamptz
	RouteMetadata         []byte
	RouteConversationType pgtype.Text
}

func scanSessionPagedRows[T any](rows *sql.Rows, conv func(sessionPagedScan) T) ([]T, error) {
	var out []T
	for rows.Next() {
		var (
			id, botID                      string
			routeID, channelType           sql.NullString
			typ, title, metadata           string
			parentSessionID, createdByUser sql.NullString
			createdAt, updatedAt           string
			deletedAt                      sql.NullString
			routeMetadata                  sql.NullString
			routeConversationType          sql.NullString
		)
		if err := rows.Scan(
			&id, &botID, &routeID, &channelType, &typ, &title, &metadata,
			&parentSessionID, &createdByUser, &createdAt, &updatedAt, &deletedAt,
			&routeMetadata, &routeConversationType,
		); err != nil {
			return nil, err
		}
		row := sessionPagedScan{
			Type:                  typ,
			Title:                 title,
			Metadata:              []byte(metadata),
			ChannelType:           textFromNullString(channelType),
			RouteConversationType: textFromNullString(routeConversationType),
		}
		if err := row.ID.Scan(id); err != nil {
			return nil, err
		}
		if err := row.BotID.Scan(botID); err != nil {
			return nil, err
		}
		if routeID.Valid {
			if err := row.RouteID.Scan(routeID.String); err != nil {
				return nil, err
			}
		}
		if parentSessionID.Valid {
			if err := row.ParentSessionID.Scan(parentSessionID.String); err != nil {
				return nil, err
			}
		}
		if createdByUser.Valid {
			if err := row.CreatedByUserID.Scan(createdByUser.String); err != nil {
				return nil, err
			}
		}
		createdTS, err := parseSQLiteTimestamp(createdAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite paged sessions: parse created_at %q: %w", createdAt, err)
		}
		row.CreatedAt = createdTS
		updatedTS, err := parseSQLiteTimestamp(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite paged sessions: parse updated_at %q: %w", updatedAt, err)
		}
		row.UpdatedAt = updatedTS
		if deletedAt.Valid {
			deletedTS, err := parseSQLiteTimestamp(deletedAt.String)
			if err != nil {
				return nil, fmt.Errorf("sqlite paged sessions: parse deleted_at %q: %w", deletedAt.String, err)
			}
			row.DeletedAt = deletedTS
		}
		if routeMetadata.Valid {
			row.RouteMetadata = []byte(routeMetadata.String)
		}
		out = append(out, conv(row))
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func textFromNullString(value sql.NullString) pgtype.Text {
	if !value.Valid {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value.String, Valid: true}
}

func parseSQLiteTimestamp(raw string) (pgtype.Timestamptz, error) {
	if raw == "" {
		return pgtype.Timestamptz{}, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, nil
		}
	}
	return pgtype.Timestamptz{}, errors.New("unsupported timestamp format")
}
