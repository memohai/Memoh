package builtin

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/config"
	dbstore "github.com/memohai/memoh/internal/db/store"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/memory/wikistore"
)

// BuiltinMemoryMode represents the operating mode of the built-in memory provider.
type BuiltinMemoryMode string

const (
	ModeOff   BuiltinMemoryMode = "off"
	ModeDense BuiltinMemoryMode = "dense"
)

// NewBuiltinRuntimeFromConfig returns the appropriate Runtime based on
// the provider's persisted config (memory_mode field). Returns the file
// runtime for "off" or unknown modes. Returns an error if a dense
// runtime was explicitly requested but failed to initialise, so that callers
// can surface configuration problems rather than silently degrading.
//
// wikiStore is required for ModeGraph (the PG-backed wiki); it is ignored for
// off/dense and may be nil when graph mode is never used.
func NewBuiltinRuntimeFromConfig(logger *slog.Logger, providerConfig map[string]any, store *storefs.Service, queries dbstore.Queries, cfg config.Config, wikiStore wikistore.Store) (Runtime, error) {
	mode := BuiltinMemoryMode(strings.TrimSpace(adapters.StringFromConfig(providerConfig, "memory_mode")))

	switch mode {
	case ModeGraph:
		if wikiStore == nil {
			return nil, errors.New("graph runtime: wiki store not configured")
		}
		return newGraphRuntime(logger, wikiStore, store), nil

	case ModeDense:
		return newDenseRuntime(providerConfig, queries, cfg, store)

	default:
		return NewFileRuntime(store), nil
	}
}
