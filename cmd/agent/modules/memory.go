package modules

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/embeddings"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/models"
	"go.uber.org/fx"
)


var MemoryModule = fx.Module(
    "memory",
    fx.Provide(
        provideMemoryLLM,
        provideEmbeddingSetup,
        provideEmbeddingsResolver,
        provideTextEmbedderForMemory,
        provideQdrantStore,
        memory.NewBM25Indexer,
        provideMemoryService,
    ),
    fx.Invoke(startMemoryWarmup),
)

// ---------------------------------------------------------------------------
// memory providers
// ---------------------------------------------------------------------------

func provideMemoryLLM(modelsService *models.Service, queries *dbsqlc.Queries, log *slog.Logger) memory.LLM {
	return &lazyLLMClient{
		modelsService: modelsService,
		queries:       queries,
		timeout:       30 * time.Second,
		logger:        log,
	}
}

func provideEmbeddingsResolver(log *slog.Logger, modelsService *models.Service, queries *dbsqlc.Queries) *embeddings.Resolver {
	return embeddings.NewResolver(log, modelsService, queries, 10*time.Second)
}

type embeddingSetup struct {
	Vectors            map[string]int
	TextModel          models.GetResponse
	MultimodalModel    models.GetResponse
	HasEmbeddingModels bool
}

func provideEmbeddingSetup(log *slog.Logger, modelsService *models.Service) (embeddingSetup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vectors, textModel, multimodalModel, hasEmbeddingModels, err := embeddings.CollectEmbeddingVectors(ctx, modelsService)
	if err != nil {
		return embeddingSetup{}, fmt.Errorf("embedding models: %w", err)
	}
	if hasEmbeddingModels && multimodalModel.ModelID == "" {
		log.Warn("No multimodal embedding model configured. Multimodal embedding features will be limited.")
	}
	return embeddingSetup{
		Vectors:            vectors,
		TextModel:          textModel,
		MultimodalModel:    multimodalModel,
		HasEmbeddingModels: hasEmbeddingModels,
	}, nil
}

func provideTextEmbedderForMemory(resolver *embeddings.Resolver, setup embeddingSetup, log *slog.Logger) embeddings.Embedder {
	return buildTextEmbedder(resolver, setup.TextModel, setup.HasEmbeddingModels, log)
}

func provideQdrantStore(log *slog.Logger, cfg config.Config, setup embeddingSetup) (*memory.QdrantStore, error) {
	qcfg := cfg.Qdrant
	timeout := time.Duration(qcfg.TimeoutSeconds) * time.Second
	if setup.HasEmbeddingModels && len(setup.Vectors) > 0 {
		store, err := memory.NewQdrantStoreWithVectors(log, qcfg.BaseURL, qcfg.APIKey, qcfg.Collection, setup.Vectors, "sparse_hash", timeout)
		if err != nil {
			return nil, fmt.Errorf("qdrant named vectors init: %w", err)
		}
		return store, nil
	}
	store, err := memory.NewQdrantStore(log, qcfg.BaseURL, qcfg.APIKey, qcfg.Collection, setup.TextModel.Dimensions, "sparse_hash", timeout)
	if err != nil {
		return nil, fmt.Errorf("qdrant init: %w", err)
	}
	return store, nil
}

func provideMemoryService(log *slog.Logger, llm memory.LLM, embedder embeddings.Embedder, store *memory.QdrantStore, resolver *embeddings.Resolver, bm25 *memory.BM25Indexer, setup embeddingSetup) *memory.Service {
	return memory.NewService(log, llm, embedder, store, resolver, bm25, setup.TextModel.ModelID, setup.MultimodalModel.ModelID)
}

func buildTextEmbedder(resolver *embeddings.Resolver, textModel models.GetResponse, hasModels bool, log *slog.Logger) embeddings.Embedder {
	if !hasModels {
		return nil
	}
	if textModel.ModelID == "" || textModel.Dimensions <= 0 {
		log.Warn("No text embedding model configured. Text embedding features will be limited.")
		return nil
	}
	return &embeddings.ResolverTextEmbedder{
		Resolver: resolver,
		ModelID:  textModel.ModelID,
		Dims:     textModel.Dimensions,
	}
}

func startMemoryWarmup(lc fx.Lifecycle, memoryService *memory.Service, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := memoryService.WarmupBM25(context.Background(), 200); err != nil {
					logger.Warn("bm25 warmup failed", slog.Any("error", err))
				}
			}()
			return nil
		},
	})
}


// ---------------------------------------------------------------------------
// lazy LLM client
// ---------------------------------------------------------------------------

type lazyLLMClient struct {
	modelsService *models.Service
	queries       *dbsqlc.Queries
	timeout       time.Duration
	logger        *slog.Logger
}

func (c *lazyLLMClient) Extract(ctx context.Context, req memory.ExtractRequest) (memory.ExtractResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memory.ExtractResponse{}, err
	}
	return client.Extract(ctx, req)
}

func (c *lazyLLMClient) Decide(ctx context.Context, req memory.DecideRequest) (memory.DecideResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memory.DecideResponse{}, err
	}
	return client.Decide(ctx, req)
}

func (c *lazyLLMClient) Compact(ctx context.Context, req memory.CompactRequest) (memory.CompactResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memory.CompactResponse{}, err
	}
	return client.Compact(ctx, req)
}

func (c *lazyLLMClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return "", err
	}
	return client.DetectLanguage(ctx, text)
}

func (c *lazyLLMClient) resolve(ctx context.Context) (memory.LLM, error) {
	if c.modelsService == nil || c.queries == nil {
		return nil, fmt.Errorf("models service not configured")
	}
	memoryModel, memoryProvider, err := models.SelectMemoryModel(ctx, c.modelsService, c.queries)
	if err != nil {
		return nil, err
	}
	clientType := strings.ToLower(strings.TrimSpace(memoryProvider.ClientType))
	switch clientType {
	case "openai", "openai-compat", "azure", "mistral", "xai", "ollama", "dashscope":
		// These providers support OpenAI-compatible /chat/completions endpoint
	default:
		return nil, fmt.Errorf("memory provider client type not supported: %s", memoryProvider.ClientType)
	}
	return memory.NewLLMClient(c.logger, memoryProvider.BaseUrl, memoryProvider.ApiKey, memoryModel.ModelID, c.timeout)
}


