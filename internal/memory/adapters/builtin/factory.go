package builtin

import (
	stdsql "database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbstore "github.com/memohai/memoh/internal/db/store"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	memindex "github.com/memohai/memoh/internal/memory/index"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

// BuiltinMemoryMode represents the operating mode of the built-in memory provider.
type BuiltinMemoryMode string

const (
	ModeOff    BuiltinMemoryMode = "off"
	ModeSparse BuiltinMemoryMode = "sparse"
	ModeDense  BuiltinMemoryMode = "dense"
)

type memorySQLIndex interface {
	memindex.DenseIndex
	memindex.SparseIndex
}

// NewBuiltinRuntimeFromConfig returns the appropriate memoryRuntime based on
// the provider's persisted config (memory_mode field). Returns the file
// runtime for "off" or unknown modes. Returns an error if a sparse or dense
// runtime was explicitly requested but failed to initialise, so that callers
// can surface configuration problems rather than silently degrading.
func NewBuiltinRuntimeFromConfig(_ *slog.Logger, providerConfig map[string]any, fileRuntime any, store *storefs.Service, queries dbstore.Queries, cfg config.Config, pgPool *pgxpool.Pool, sqliteDB *stdsql.DB) (any, error) {
	mode := BuiltinMemoryMode(strings.TrimSpace(adapters.StringFromConfig(providerConfig, "memory_mode")))

	switch mode {
	case ModeSparse:
		index, err := newMemoryIndex(cfg, pgPool, sqliteDB)
		if err != nil {
			return nil, err
		}
		return newSparseRuntime(index, strings.TrimSpace(cfg.Sparse.BaseURL), store)

	case ModeDense:
		index, err := newMemoryIndex(cfg, pgPool, sqliteDB)
		if err != nil {
			return nil, err
		}
		return newDenseRuntime(providerConfig, queries, store, index)

	default:
		return fileRuntime, nil
	}
}

func newMemoryIndex(cfg config.Config, pgPool *pgxpool.Pool, sqliteDB *stdsql.DB) (memorySQLIndex, error) {
	switch db.DriverFromConfig(cfg) {
	case db.DriverPostgres:
		return memindex.NewPostgres(pgPool)
	case db.DriverSQLite:
		return memindex.NewSQLite(sqliteDB)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", db.DriverFromConfig(cfg))
	}
}
