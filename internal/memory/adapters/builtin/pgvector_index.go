package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"
	"github.com/pgvector/pgvector-go"

	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/models"
)

const (
	semanticEmbedTimeout = models.DefaultProviderRequestTimeout
	maxPgvectorInt32     = int64(1<<31 - 1)
)

type pgvectorQueries interface {
	UpsertMemoryNodeEmbedding(ctx context.Context, arg dbsqlc.UpsertMemoryNodeEmbeddingParams) error
	SearchMemoryNodeEmbeddings(ctx context.Context, arg dbsqlc.SearchMemoryNodeEmbeddingsParams) ([]dbsqlc.SearchMemoryNodeEmbeddingsRow, error)
	CountMemoryNodeEmbeddingsByBotModel(ctx context.Context, arg dbsqlc.CountMemoryNodeEmbeddingsByBotModelParams) (int64, error)
	DeleteMemoryNodeEmbeddings(ctx context.Context, arg dbsqlc.DeleteMemoryNodeEmbeddingsParams) error
	DeleteAllMemoryNodeEmbeddingsByBot(ctx context.Context, botID pgtype.UUID) error
	CheckMemoryNodeEmbeddingsStore(ctx context.Context) (bool, error)
}

type pgvectorIndex struct {
	queries    pgvectorQueries
	lookup     dbstore.Queries
	embedModel *sdk.EmbeddingModel
	model      embeddingModelSpec
	modelRef   string
	logger     *slog.Logger
}

type embeddingModelSpec struct {
	uuid       pgtype.UUID
	modelID    string
	clientType string
	baseURL    string
	apiKey     string
	dimensions int
}

func newPGVectorIndex(logger *slog.Logger, providerConfig map[string]any, queries dbstore.Queries) (*pgvectorIndex, error) {
	modelRef := strings.TrimSpace(adapters.StringFromConfig(providerConfig, "embedding_model_id"))
	if modelRef == "" {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	pgQueries, ok := queries.(pgvectorQueries)
	if !ok {
		logger.Debug("graph: pgvector semantic index disabled for non-postgres store", slog.String("embedding_model_id", modelRef))
		return nil, nil
	}
	spec, err := resolveEmbeddingModel(context.Background(), queries, modelRef)
	if err != nil {
		return nil, err
	}
	return &pgvectorIndex{
		queries:    pgQueries,
		lookup:     queries,
		embedModel: models.NewSDKEmbeddingModel(spec.clientType, spec.baseURL, spec.apiKey, spec.modelID, semanticEmbedTimeout, nil),
		model:      spec,
		modelRef:   modelRef,
		logger:     logger,
	}, nil
}

func (r *pgvectorIndex) Name() string {
	if r == nil {
		return ""
	}
	return "pgvector"
}

func (r *pgvectorIndex) ensureEmbeddingEnabled(ctx context.Context) error {
	if r == nil || r.modelRef == "" {
		return nil
	}
	modelRef := r.modelRef
	var row dbsqlc.Model
	if parsed, err := db.ParseUUID(modelRef); err == nil {
		if dbModel, err := r.lookupQueries().GetModelByID(ctx, parsed); err == nil {
			row = dbModel
		}
	}
	if !row.ID.Valid {
		rows, err := r.lookupQueries().ListModelsByModelID(ctx, modelRef)
		if err != nil || len(rows) == 0 {
			return fmt.Errorf("pgvector semantic index: embedding model not found: %s", modelRef)
		}
		row = rows[0]
	}
	if !row.Enable {
		return fmt.Errorf("pgvector semantic index: embedding model %s is disabled", modelRef)
	}
	return nil
}

func (r *pgvectorIndex) lookupQueries() dbstore.Queries {
	return r.lookup
}

func (r *pgvectorIndex) embedText(ctx context.Context, text string) ([]float32, error) {
	if err := r.ensureEmbeddingEnabled(ctx); err != nil {
		return nil, err
	}
	client := sdk.NewClient()
	vec, err := client.Embed(ctx, text, sdk.WithEmbeddingModel(r.embedModel))
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic embed: %w", err)
	}
	out := float64sToFloat32s(vec)
	if r.model.dimensions > 0 && len(out) != r.model.dimensions {
		return nil, fmt.Errorf("pgvector semantic index: embedding dimensions = %d, want %d", len(out), r.model.dimensions)
	}
	return out, nil
}

func (r *pgvectorIndex) Upsert(ctx context.Context, botID, nodeID, body, hash string) error {
	if r == nil || r.queries == nil || strings.TrimSpace(body) == "" {
		return nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	vec, err := r.embedText(ctx, body)
	if err != nil {
		return err
	}
	dimensions, err := checkedPgvectorInt32("dimensions", len(vec))
	if err != nil {
		return err
	}
	return r.queries.UpsertMemoryNodeEmbedding(ctx, dbsqlc.UpsertMemoryNodeEmbeddingParams{
		BotID:      botUUID,
		NodeID:     strings.TrimSpace(nodeID),
		ModelID:    r.model.uuid,
		Dimensions: dimensions,
		BodyHash:   strings.TrimSpace(hash),
		Embedding:  pgvector.NewVector(vec),
	})
}

func (r *pgvectorIndex) SearchSeeds(ctx context.Context, botID, query string, limit int) (map[string]float64, error) {
	if r == nil || r.queries == nil || strings.TrimSpace(query) == "" || limit <= 0 {
		return nil, nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	vec, err := r.embedText(ctx, query)
	if err != nil {
		return nil, err
	}
	rowLimit, err := checkedPgvectorInt32("row_limit", limit)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.SearchMemoryNodeEmbeddings(ctx, dbsqlc.SearchMemoryNodeEmbeddingsParams{
		QueryEmbedding: pgvector.NewVector(vec),
		BotID:          botUUID,
		ModelID:        r.model.uuid,
		RowLimit:       rowLimit,
	})
	if err != nil {
		return nil, err
	}
	seeds := make(map[string]float64, len(rows))
	for _, row := range rows {
		nodeID := strings.TrimSpace(row.NodeID)
		if nodeID == "" {
			continue
		}
		seeds[nodeID] = row.Score
	}
	return seeds, nil
}

func checkedPgvectorInt32(name string, n int) (int32, error) {
	if n < 0 || int64(n) > maxPgvectorInt32 {
		return 0, fmt.Errorf("pgvector semantic index: %s out of int32 range: %d", name, n)
	}
	return int32(n), nil //nolint:gosec // guarded above.
}

func (r *pgvectorIndex) DeleteNodes(ctx context.Context, botID string, nodeIDs []string) error {
	if r == nil || r.queries == nil || len(nodeIDs) == 0 {
		return nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID != "" {
			ids = append(ids, nodeID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return r.queries.DeleteMemoryNodeEmbeddings(ctx, dbsqlc.DeleteMemoryNodeEmbeddingsParams{
		BotID:   botUUID,
		NodeIds: ids,
	})
}

func (r *pgvectorIndex) DeleteBot(ctx context.Context, botID string) error {
	if r == nil || r.queries == nil {
		return nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return r.queries.DeleteAllMemoryNodeEmbeddingsByBot(ctx, botUUID)
}

func (r *pgvectorIndex) Count(ctx context.Context, botID string) (int, error) {
	if r == nil || r.queries == nil {
		return 0, nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return 0, err
	}
	count, err := r.queries.CountMemoryNodeEmbeddingsByBotModel(ctx, dbsqlc.CountMemoryNodeEmbeddingsByBotModelParams{
		BotID:   botUUID,
		ModelID: r.model.uuid,
	})
	if err != nil {
		return 0, err
	}
	if count > int64(^uint(0)>>1) {
		return 0, fmt.Errorf("pgvector semantic index: count overflow: %d", count)
	}
	return int(count), nil
}

func (r *pgvectorIndex) Health(ctx context.Context) error {
	if r == nil || r.queries == nil {
		return nil
	}
	if err := r.ensureEmbeddingEnabled(ctx); err != nil {
		return err
	}
	_, err := r.queries.CheckMemoryNodeEmbeddingsStore(ctx)
	return err
}

func float64sToFloat32s(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

func resolveEmbeddingModel(ctx context.Context, queries dbstore.Queries, modelRef string) (embeddingModelSpec, error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return embeddingModelSpec{}, errors.New("pgvector semantic index: embedding_model_id is required")
	}
	if queries == nil {
		return embeddingModelSpec{}, errors.New("pgvector semantic index: queries are required")
	}
	var row dbsqlc.Model
	if parsed, err := db.ParseUUID(modelRef); err == nil {
		dbModel, err := queries.GetModelByID(ctx, parsed)
		if err == nil {
			row = dbModel
		}
	}
	if !row.ID.Valid {
		rows, err := queries.ListModelsByModelID(ctx, modelRef)
		if err != nil || len(rows) == 0 {
			return embeddingModelSpec{}, fmt.Errorf("pgvector semantic index: embedding model not found: %s", modelRef)
		}
		row = rows[0]
	}
	if row.Type != "embedding" {
		return embeddingModelSpec{}, fmt.Errorf("pgvector semantic index: model %s is not an embedding model", modelRef)
	}
	if !row.Enable {
		return embeddingModelSpec{}, fmt.Errorf("pgvector semantic index: embedding model %s is disabled", modelRef)
	}
	if !row.ProviderID.Valid {
		return embeddingModelSpec{}, fmt.Errorf("pgvector semantic index: model %s has no provider", modelRef)
	}
	provider, err := queries.GetProviderByID(ctx, row.ProviderID)
	if err != nil {
		return embeddingModelSpec{}, fmt.Errorf("pgvector semantic index: get embedding provider: %w", err)
	}
	var modelCfg struct {
		Dimensions *int `json:"dimensions"`
	}
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &modelCfg)
	}
	if modelCfg.Dimensions == nil || *modelCfg.Dimensions <= 0 {
		return embeddingModelSpec{}, fmt.Errorf("pgvector semantic index: embedding model %s missing dimensions", modelRef)
	}
	var providerCfg map[string]any
	if len(provider.Config) > 0 {
		_ = json.Unmarshal(provider.Config, &providerCfg)
	}
	baseURL, _ := providerCfg["base_url"].(string)
	apiKey, _ := providerCfg["api_key"].(string)
	return embeddingModelSpec{
		uuid:       row.ID,
		modelID:    strings.TrimSpace(row.ModelID),
		clientType: strings.TrimSpace(provider.ClientType),
		baseURL:    strings.TrimSpace(baseURL),
		apiKey:     strings.TrimSpace(apiKey),
		dimensions: *modelCfg.Dimensions,
	}, nil
}
