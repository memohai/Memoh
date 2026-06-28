package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/memohai/twilight-ai/sdk"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"github.com/memohai/memoh/internal/config"
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

const pgvectorSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memory_node_embeddings (
    bot_id      UUID        NOT NULL,
    node_id     TEXT        NOT NULL,
    model_id    UUID        NOT NULL,
    dimensions  INTEGER     NOT NULL,
    body_hash   TEXT        NOT NULL DEFAULT '',
    embedding   vector      NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, node_id, model_id),
    CONSTRAINT memory_node_embeddings_dimensions_check CHECK (dimensions > 0)
);

CREATE INDEX IF NOT EXISTS idx_memory_node_embeddings_bot_model ON memory_node_embeddings (bot_id, model_id);
`

type pgvectorIndex struct {
	pool       *pgxpool.Pool
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

func newPGVectorIndex(logger *slog.Logger, providerConfig map[string]any, queries dbstore.Queries, vectorConfig config.PGVectorConfig) (*pgvectorIndex, error) {
	modelRef := strings.TrimSpace(adapters.StringFromConfig(providerConfig, "embedding_model_id"))
	if modelRef == "" {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	if !vectorConfig.Enabled {
		logger.Debug("graph: pgvector semantic index disabled by config", slog.String("embedding_model_id", modelRef))
		return nil, nil
	}
	if queries == nil {
		logger.Debug("graph: pgvector semantic index disabled without relational query store", slog.String("embedding_model_id", modelRef))
		return nil, nil
	}
	spec, err := resolveEmbeddingModel(context.Background(), queries, modelRef)
	if err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(db.DSN(vectorConfig.PostgresConfig()))
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic index: parse dsn: %w", err)
	}
	poolCfg.AfterConnect = pgxvec.RegisterTypes
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic index: connect: %w", err)
	}
	index := &pgvectorIndex{
		pool:       pool,
		lookup:     queries,
		embedModel: models.NewSDKEmbeddingModel(spec.clientType, spec.baseURL, spec.apiKey, spec.modelID, semanticEmbedTimeout, nil),
		model:      spec,
		modelRef:   modelRef,
		logger:     logger,
	}
	if err := index.ensureStore(context.Background()); err != nil {
		pool.Close()
		return nil, err
	}
	return index, nil
}

func (r *pgvectorIndex) Name() string {
	if r == nil {
		return ""
	}
	return "pgvector"
}

func (r *pgvectorIndex) ensureStore(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return nil
	}
	if _, err := r.pool.Exec(ctx, pgvectorSchemaSQL); err != nil {
		return fmt.Errorf("pgvector semantic index: ensure store: %w", err)
	}
	return nil
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
	if r == nil || r.pool == nil || strings.TrimSpace(body) == "" {
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
	_, err = r.pool.Exec(ctx, `
INSERT INTO memory_node_embeddings (
  bot_id, node_id, model_id, dimensions, body_hash, embedding
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (bot_id, node_id, model_id) DO UPDATE SET
  dimensions = EXCLUDED.dimensions,
  body_hash = EXCLUDED.body_hash,
  embedding = EXCLUDED.embedding,
  updated_at = now();
`, botUUID, strings.TrimSpace(nodeID), r.model.uuid, dimensions, strings.TrimSpace(hash), pgvector.NewVector(vec))
	if err != nil {
		return fmt.Errorf("pgvector semantic index: upsert: %w", err)
	}
	return nil
}

func (r *pgvectorIndex) SearchSeeds(ctx context.Context, botID, query string, limit int) (map[string]float64, error) {
	if r == nil || r.pool == nil || strings.TrimSpace(query) == "" || limit <= 0 {
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
	rows, err := r.pool.Query(ctx, `
SELECT
  node_id,
  CAST(1.0 - (embedding <=> $1::vector) AS double precision) AS score
FROM memory_node_embeddings
WHERE bot_id = $2
  AND model_id = $3
ORDER BY embedding <=> $1::vector
LIMIT $4;
`, pgvector.NewVector(vec), botUUID, r.model.uuid, rowLimit)
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic index: search: %w", err)
	}
	defer rows.Close()

	seeds := map[string]float64{}
	for rows.Next() {
		var nodeID string
		var score float64
		if err := rows.Scan(&nodeID, &score); err != nil {
			return nil, fmt.Errorf("pgvector semantic index: scan search row: %w", err)
		}
		nodeID = strings.TrimSpace(nodeID)
		if nodeID != "" {
			seeds[nodeID] = score
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector semantic index: iterate search rows: %w", err)
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
	if r == nil || r.pool == nil || len(nodeIDs) == 0 {
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
	_, err = r.pool.Exec(ctx, `
DELETE FROM memory_node_embeddings
WHERE bot_id = $1
  AND node_id = ANY($2::text[]);
`, botUUID, ids)
	if err != nil {
		return fmt.Errorf("pgvector semantic index: delete nodes: %w", err)
	}
	return nil
}

func (r *pgvectorIndex) DeleteBot(ctx context.Context, botID string) error {
	if r == nil || r.pool == nil {
		return nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
DELETE FROM memory_node_embeddings
WHERE bot_id = $1;
`, botUUID)
	if err != nil {
		return fmt.Errorf("pgvector semantic index: delete bot: %w", err)
	}
	return nil
}

func (r *pgvectorIndex) Count(ctx context.Context, botID string) (int, error) {
	if r == nil || r.pool == nil {
		return 0, nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := r.pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM memory_node_embeddings
WHERE bot_id = $1
  AND model_id = $2;
`, botUUID, r.model.uuid).Scan(&count); err != nil {
		return 0, fmt.Errorf("pgvector semantic index: count: %w", err)
	}
	if count > int64(^uint(0)>>1) {
		return 0, fmt.Errorf("pgvector semantic index: count overflow: %d", count)
	}
	return int(count), nil
}

func (r *pgvectorIndex) Health(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return nil
	}
	if err := r.ensureEmbeddingEnabled(ctx); err != nil {
		return err
	}
	if err := r.ensureStore(ctx); err != nil {
		return err
	}
	var ok bool
	err := r.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM memory_node_embeddings
  LIMIT 1
);
`).Scan(&ok)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("pgvector semantic index: health: %w", err)
	}
	return nil
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
