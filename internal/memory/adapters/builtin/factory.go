package builtin

import (
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/config"
	dbstore "github.com/memohai/memoh/internal/db/store"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
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
func NewBuiltinRuntimeFromConfig(_ *slog.Logger, providerConfig map[string]any, store *storefs.Service, queries dbstore.Queries, cfg config.Config) (Runtime, error) {
	mode := BuiltinMemoryMode(strings.TrimSpace(adapters.StringFromConfig(providerConfig, "memory_mode")))

	switch mode {
	case ModeDense:
		return newDenseRuntime(providerConfig, queries, cfg, store)

	default:
		return NewFileRuntime(store), nil
	}
}
