package builtin

import (
	"errors"
	"log/slog"

	pgvectordb "github.com/memohai/memoh/internal/db/pgvector"
	dbstore "github.com/memohai/memoh/internal/db/store"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/memory/wikistore"
)

// NewBuiltinRuntimeFromConfig returns the graph Runtime for the provider's
// persisted config. Graph is the only supported mode: PG memory nodes/edges are
// the source of truth, with Markdown as a derived view. Returns an error if
// the wiki store is not configured.
//
// queries is used to resolve the optional embedding_model_id from the main
// relational store. The pgvector semantic seed index itself uses the dedicated
// [pgvector] database, so Local stores intentionally run graph-only.
func NewBuiltinRuntimeFromConfig(logger *slog.Logger, providerConfig map[string]any, store *storefs.Service, queries dbstore.Queries, vectorStore *pgvectordb.Store, wikiStore wikistore.Store) (Runtime, error) {
	if wikiStore == nil {
		return nil, errors.New("graph runtime: wiki store not configured")
	}
	runtime := NewGraphRuntime(logger, wikiStore, store)
	semantic, err := newPGVectorIndex(logger, providerConfig, queries, vectorStore)
	if err != nil {
		return nil, err
	}
	runtime.SetSemanticIndex(semantic)
	return runtime, nil
}
