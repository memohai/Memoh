package adapters

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/mcp"
)

type providerBootstrapQueries struct {
	dbstore.Queries
	providers []sqlc.MemoryProvider
}

func (q *providerBootstrapQueries) ListMemoryProviders(context.Context) ([]sqlc.MemoryProvider, error) {
	return q.providers, nil
}

type bootstrapProvider struct {
	providerType string
}

func (p *bootstrapProvider) Type() string { return p.providerType }

func (*bootstrapProvider) OnBeforeChat(context.Context, BeforeChatRequest) (*BeforeChatResult, error) {
	return nil, nil
}

func (*bootstrapProvider) OnAfterChat(context.Context, AfterChatRequest) error { return nil }

func (*bootstrapProvider) ListTools(context.Context, mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	return nil, nil
}

func (*bootstrapProvider) CallTool(context.Context, mcp.ToolSessionContext, string, map[string]any) (map[string]any, error) {
	return nil, nil
}

func (*bootstrapProvider) Add(context.Context, AddRequest) (SearchResponse, error) {
	return SearchResponse{}, nil
}

func (*bootstrapProvider) Search(context.Context, SearchRequest) (SearchResponse, error) {
	return SearchResponse{}, nil
}

func (*bootstrapProvider) GetAll(context.Context, GetAllRequest) (SearchResponse, error) {
	return SearchResponse{}, nil
}

func (*bootstrapProvider) Update(context.Context, UpdateRequest) (MemoryItem, error) {
	return MemoryItem{}, nil
}

func (*bootstrapProvider) Delete(context.Context, string) (DeleteResponse, error) {
	return DeleteResponse{}, nil
}

func (*bootstrapProvider) DeleteBatch(context.Context, []string) (DeleteResponse, error) {
	return DeleteResponse{}, nil
}

func (*bootstrapProvider) DeleteAll(context.Context, DeleteAllRequest) (DeleteResponse, error) {
	return DeleteResponse{}, nil
}

func (*bootstrapProvider) Compact(context.Context, map[string]any, float64, int) (CompactResult, error) {
	return CompactResult{}, nil
}

func (*bootstrapProvider) Usage(context.Context, map[string]any) (UsageResponse, error) {
	return UsageResponse{}, nil
}

func TestInstantiateAllLoadsConfiguredProvidersIntoRegistry(t *testing.T) {
	t.Parallel()

	providerID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	registry := NewRegistry(slog.Default())
	registry.RegisterFactory(string(ProviderMem0), func(_ string, _ map[string]any) (Provider, error) {
		return &bootstrapProvider{providerType: string(ProviderMem0)}, nil
	})
	service := NewService(slog.Default(), &providerBootstrapQueries{
		providers: []sqlc.MemoryProvider{{
			ID:       providerID,
			Name:     "Mem0",
			Provider: string(ProviderMem0),
			Config:   []byte(`{"api_key":"test"}`),
		}},
	}, config.Config{})
	service.SetRegistry(registry)

	loaded, err := service.InstantiateAll(context.Background())
	if err != nil {
		t.Fatalf("InstantiateAll() error = %v", err)
	}
	if loaded != 1 {
		t.Fatalf("loaded providers = %d, want 1", loaded)
	}
	if _, err := registry.Get(providerID.String()); err != nil {
		t.Fatalf("registry missing configured provider after InstantiateAll(): %v", err)
	}
}
