package builtin

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/config"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

// BuiltinMemoryMode represents the operating mode of the built-in memory provider.
type BuiltinMemoryMode string

const (
	ModeOff    BuiltinMemoryMode = "off"
	ModeSparse BuiltinMemoryMode = "sparse"
	ModeDense  BuiltinMemoryMode = "dense"
)

// NewBuiltinRuntimeFromConfig returns the appropriate memoryRuntime based on
// the provider's persisted config (memory_mode field). Returns the file
// runtime for "off" or unknown modes. Returns an error if a sparse or dense
// runtime was explicitly requested but failed to initialise, so that callers
// can surface configuration problems rather than silently degrading.
func NewBuiltinRuntimeFromConfig(_ *slog.Logger, providerConfig map[string]any, fileRuntime any, store *storefs.Service, queries *dbsqlc.Queries, cfg config.Config) (any, error) {
	mode := BuiltinMemoryMode(strings.TrimSpace(adapters.StringFromConfig(providerConfig, "memory_mode")))

	switch mode {
	case ModeSparse:
		host, port := parseQdrantHostPort(cfg.Qdrant.BaseURL)
		if host == "" {
			host = "localhost"
		}
		if port == 0 {
			port = 6334
		}
		collection := adapters.StringFromConfig(providerConfig, "qdrant_collection")
		if collection == "" {
			collection = "memory_sparse"
		}
		return newSparseRuntime(
			host,
			port,
			cfg.Qdrant.APIKey,
			collection,
			strings.TrimSpace(cfg.Sparse.BaseURL),
			store,
		)

	case ModeDense:
		return newDenseRuntime(providerConfig, queries, cfg, store)

	default:
		return fileRuntime, nil
	}
}

// parseQdrantHostPort extracts host and gRPC port from a Qdrant base URL.
// Qdrant base URLs are typically HTTP (port 6333), but the gRPC port is 6334.
func parseQdrantHostPort(baseURL string) (string, int) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", 0
	}
	baseURL = strings.TrimPrefix(baseURL, "http://")
	baseURL = strings.TrimPrefix(baseURL, "https://")
	parts := strings.SplitN(baseURL, ":", 2)
	host := parts[0]
	if len(parts) == 2 {
		httpPort, err := strconv.Atoi(strings.TrimRight(parts[1], "/"))
		if err == nil {
			switch httpPort {
			case 6333:
				return host, 6334
			case 6334:
				return host, 6334
			default:
				// Common case: operator already configured the intended gRPC port.
				return host, httpPort
			}
		}
	}
	return host, 6334
}
