package store

import (
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

func TestConvertValuePGInt8ToSQLiteNullInt64(t *testing.T) {
	arg := pgsqlc.LinkMessageToHistoryTurnParams{
		TurnID:         pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		TurnMessageSeq: pgtype.Int8{Int64: 2, Valid: true},
		MessageID:      pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
	}
	var sqliteArg sqlitesqlc.LinkMessageToHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		t.Fatalf("convert link arg: %v", err)
	}
	if got, want := sqliteArg.TurnMessageSeq, (sql.NullInt64{Int64: 2, Valid: true}); got != want {
		t.Fatalf("turn message seq = %#v, want %#v", got, want)
	}
}

func TestConvertValueInvalidPGInt8ToSQLiteNullInt64(t *testing.T) {
	arg := pgsqlc.LinkMessageToHistoryTurnParams{
		TurnMessageSeq: pgtype.Int8{Int64: 2, Valid: false},
	}
	var sqliteArg sqlitesqlc.LinkMessageToHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		t.Fatalf("convert link arg: %v", err)
	}
	if sqliteArg.TurnMessageSeq.Valid {
		t.Fatalf("turn message seq = %#v, want invalid", sqliteArg.TurnMessageSeq)
	}
}
