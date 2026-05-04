package db

import (
	"fmt"
	"io/fs"

	"github.com/memohai/memoh/internal/config"
)

func MigrationsFSForConfig(cfg config.Config, embedded fs.FS) (fs.FS, error) {
	switch driver := DriverFromConfig(cfg); driver {
	case DriverPostgres:
		return fs.Sub(embedded, "postgres/migrations")
	case DriverSQLite:
		return fs.Sub(embedded, "sqlite/migrations")
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
}
