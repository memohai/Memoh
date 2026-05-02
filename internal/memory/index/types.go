package index

import "context"

type SparseVector struct {
	Indices []uint32
	Values  []float32
}

type DenseVector struct {
	Values []float32
}

type SearchResult struct {
	ID      string
	Score   float64
	Payload map[string]string
}

type DenseIndex interface {
	Name() string
	EnsureDense(ctx context.Context, dimensions int) error
	UpsertDense(ctx context.Context, id string, vec DenseVector, payload map[string]string) error
	SearchDense(ctx context.Context, vec DenseVector, botID string, limit int) ([]SearchResult, error)
	ScrollDense(ctx context.Context, botID string, limit int) ([]SearchResult, error)
	CountDense(ctx context.Context, botID string) (int, error)
	DeleteByIDs(ctx context.Context, ids []string) error
	DeleteByBotID(ctx context.Context, botID string) error
	Health(ctx context.Context) error
}

type SparseIndex interface {
	Name() string
	EnsureSparse(ctx context.Context) error
	UpsertSparse(ctx context.Context, id string, vec SparseVector, payload map[string]string) error
	SearchSparse(ctx context.Context, vec SparseVector, botID string, limit int) ([]SearchResult, error)
	ScrollSparse(ctx context.Context, botID string, limit int) ([]SearchResult, error)
	CountSparse(ctx context.Context, botID string) (int, error)
	DeleteByIDs(ctx context.Context, ids []string) error
	DeleteByBotID(ctx context.Context, botID string) error
	Health(ctx context.Context) error
}
