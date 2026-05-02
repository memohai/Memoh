package db

import (
	"context"
	"database/sql"
	"net/url"
	"strings"

	// Register the pure-Go SQLite database driver.
	_ "modernc.org/sqlite"
	// Register sqlite-vec with the modernc SQLite driver without requiring CGO.
	_ "modernc.org/sqlite/vec"

	"github.com/memohai/memoh/internal/config"
)

func OpenSQLite(ctx context.Context, cfg config.SQLiteConfig) (*sql.DB, error) {
	conn, err := sql.Open(DriverSQLite, SQLiteFileDSN(cfg))
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func SQLiteFileDSN(cfg config.SQLiteConfig) string {
	if dsn := strings.TrimSpace(cfg.DSN); dsn != "" {
		return strings.TrimPrefix(dsn, "sqlite://")
	}
	path := strings.TrimPrefix(strings.TrimSpace(SQLiteDSN(cfg)), "sqlite://")
	parsed, err := url.Parse(path)
	if err != nil || parsed.RawQuery == "" {
		return path
	}
	query := parsed.Query()
	if busyTimeout := query.Get("_busy_timeout"); busyTimeout != "" {
		query.Set("_pragma", "busy_timeout("+busyTimeout+")")
		query.Del("_busy_timeout")
	}
	if query.Get("_journal_mode") == "WAL" {
		query.Add("_pragma", "journal_mode(WAL)")
		query.Del("_journal_mode")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
