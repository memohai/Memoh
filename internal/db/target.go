package db

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/config"
)

const (
	DriverPostgres = "postgres"
	DriverSQLite   = "sqlite"
)

type MigrationTarget struct {
	Driver string
	DSN    string
}

func DriverFromConfig(cfg config.Config) string {
	return strings.TrimSpace(strings.ToLower(cfg.Database.DriverOrDefault()))
}

func MigrationTargetFromConfig(cfg config.Config) (MigrationTarget, error) {
	switch driver := DriverFromConfig(cfg); driver {
	case DriverPostgres:
		return MigrationTarget{Driver: DriverPostgres, DSN: DSN(cfg.Postgres)}, nil
	case DriverSQLite:
		return MigrationTarget{Driver: DriverSQLite, DSN: SQLiteDSN(cfg.SQLite)}, nil
	default:
		return MigrationTarget{}, fmt.Errorf("unsupported database driver %q", driver)
	}
}

func SQLiteDSN(cfg config.SQLiteConfig) string {
	if dsn := strings.TrimSpace(cfg.DSN); dsn != "" {
		return dsn
	}
	path := filepath.Clean(cfg.PathOrDefault())
	query := url.Values{}
	if cfg.WAL {
		query.Set("_journal_mode", "WAL")
	}
	busyTimeout := cfg.BusyTimeoutMS
	if busyTimeout <= 0 {
		busyTimeout = config.DefaultSQLiteBusyMS
	}
	query.Set("_busy_timeout", strconv.Itoa(busyTimeout))
	if encoded := query.Encode(); encoded != "" {
		return "sqlite://" + path + "?" + encoded
	}
	return "sqlite://" + path
}
