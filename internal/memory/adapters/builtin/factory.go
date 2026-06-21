package builtin

import (
	"errors"
	"log/slog"

	"github.com/memohai/memoh/internal/config"
	dbstore "github.com/memohai/memoh/internal/db/store"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/memory/wikistore"
)

// NewBuiltinRuntimeFromConfig returns the graph Runtime for the provider's
// persisted config. Graph is the only supported mode: PG memory nodes/edges are
// the source of truth, with Markdown as a derived view. Returns an error if
// the wiki store is not configured.
//
// queries and cfg are retained in the signature for back-compat with the FX
// wiring but unused by the graph runtime.
func NewBuiltinRuntimeFromConfig(logger *slog.Logger, _ map[string]any, store *storefs.Service, _ dbstore.Queries, _ config.Config, wikiStore wikistore.Store) (Runtime, error) {
	if wikiStore == nil {
		return nil, errors.New("graph runtime: wiki store not configured")
	}
	return NewGraphRuntime(logger, wikiStore, store), nil
}
