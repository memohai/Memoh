package db

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
)

// DSN builds a PostgreSQL connection string from config.
func DSN(cfg config.PostgresConfig) string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   cfg.Database,
	}
	query := dsn.Query()
	query.Set("sslmode", cfg.SSLMode)
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

// ParseUUID converts a string UUID to pgtype.UUID.
func ParseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}

// ParseUUIDOrEmpty converts a string UUID to pgtype.UUID, returning an invalid UUID if the string is empty or unparsable.
func ParseUUIDOrEmpty(id string) pgtype.UUID {
	id = strings.TrimSpace(id)
	if id == "" {
		return pgtype.UUID{}
	}
	pgID, err := ParseUUID(id)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgID
}

// TimeFromPg converts a pgtype.Timestamptz to time.Time.
func TimeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}

// TextToString returns the string value of pgtype.Text, or "" when invalid.
func TextToString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// IsUniqueViolation reports whether err is a UNIQUE constraint violation for
// the configured database drivers:
//   - PostgreSQL: SQLSTATE 23505
//   - SQLite (modernc): SQLITE_CONSTRAINT_UNIQUE extended code 2067
//
// The string fallback only matches the exact SQLite error prefix so that
// foreign-key, check, and not-null constraint errors are never mis-classified.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	// SQLite (modernc.org/sqlite): Error.Code() returns the extended result code.
	// SQLITE_CONSTRAINT_UNIQUE = 2067 (0x813); the generic SQLITE_CONSTRAINT = 19
	// also covers FK/check/not-null, so we match the specific extended code only.
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		const sqliteConstraintUnique = 2067
		return coded.Code() == sqliteConstraintUnique
	}
	// String fallback for wrapped errors where the typed interface is unavailable.
	// SQLite always prefixes unique violations with this exact phrase.
	if err != nil {
		return strings.Contains(err.Error(), "UNIQUE constraint failed")
	}
	return false
}
