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

// DSN builds a PostgreSQL connection string for the owner/DDL role. This role
// is used for migrations (which create/alter tables and the memoh_app role) and
// is the table owner, so it is not subject to FORCE ROW LEVEL SECURITY.
func DSN(cfg config.PostgresConfig) string {
	return dsnWithCredentials(cfg, cfg.User, cfg.Password)
}

// AppDSN builds a PostgreSQL connection string for the restricted runtime role
// (memoh_app): a non-owner, non-superuser role so FORCE ROW LEVEL SECURITY
// enforces team isolation. When AppUser is empty it falls back to the owner DSN
// so OSS first boot works before the role-creating migration has run; callers
// should log a warning in that case.
func AppDSN(cfg config.PostgresConfig) string {
	user := strings.TrimSpace(cfg.AppUser)
	if user == "" {
		return DSN(cfg)
	}
	return dsnWithCredentials(cfg, cfg.AppUser, cfg.AppPassword)
}

func dsnWithCredentials(cfg config.PostgresConfig, user, password string) string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
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

// IsUniqueViolation reports whether err is a PostgreSQL UNIQUE constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
