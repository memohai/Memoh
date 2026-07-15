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
	"github.com/memohai/memoh/internal/team"
)

const (
	semanticEmbedTimeout = models.DefaultProviderRequestTimeout
	maxPgvectorInt32     = int64(1<<31 - 1)
)

const pgvectorSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS app;

CREATE OR REPLACE FUNCTION app.current_team_id()
RETURNS UUID
LANGUAGE plpgsql
STABLE
SECURITY INVOKER
SET search_path = pg_catalog, pg_temp
AS $current_team$
DECLARE
    raw TEXT;
BEGIN
    raw := pg_catalog.current_setting('app.team_id', true);
    IF raw IS NULL OR pg_catalog.btrim(raw) = '' THEN
        RAISE EXCEPTION 'app.team_id is not set'
            USING ERRCODE = '42501';
    END IF;
    BEGIN
        RETURN raw::UUID;
    EXCEPTION
        WHEN invalid_text_representation THEN
            RAISE EXCEPTION 'app.team_id is invalid'
                USING ERRCODE = '22P02';
    END;
END;
$current_team$;

CREATE TABLE IF NOT EXISTS memory_node_embeddings (
    team_id   UUID        NOT NULL DEFAULT app.current_team_id(),
    bot_id      UUID        NOT NULL,
    node_id     TEXT        NOT NULL,
    model_id    UUID        NOT NULL,
    dimensions  INTEGER     NOT NULL,
    body_hash   TEXT        NOT NULL DEFAULT '',
    embedding   vector      NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, bot_id, node_id, model_id),
    CONSTRAINT memory_node_embeddings_dimensions_check CHECK (dimensions > 0)
);

ALTER TABLE memory_node_embeddings
    ADD COLUMN IF NOT EXISTS team_id UUID;

UPDATE memory_node_embeddings
SET team_id = '00000000-0000-0000-0000-000000000001'
WHERE team_id IS NULL;

ALTER TABLE memory_node_embeddings
    ALTER COLUMN team_id SET DEFAULT app.current_team_id(),
    ALTER COLUMN team_id SET NOT NULL;

DO $memory_embeddings_pk$
DECLARE
    current_pk_name TEXT;
    current_pk_def  TEXT;
BEGIN
    SELECT c.conname, pg_get_constraintdef(c.oid)
      INTO current_pk_name, current_pk_def
      FROM pg_constraint c
     WHERE c.conrelid = 'memory_node_embeddings'::regclass
       AND c.contype = 'p';

    IF current_pk_name IS NOT NULL
       AND current_pk_def <> 'PRIMARY KEY (team_id, bot_id, node_id, model_id)' THEN
        EXECUTE format('ALTER TABLE memory_node_embeddings DROP CONSTRAINT %I', current_pk_name);
        current_pk_name := NULL;
    END IF;

    IF current_pk_name IS NULL THEN
        ALTER TABLE memory_node_embeddings
            ADD CONSTRAINT memory_node_embeddings_pkey
            PRIMARY KEY (team_id, bot_id, node_id, model_id);
    END IF;
END
$memory_embeddings_pk$;

DROP INDEX IF EXISTS idx_memory_node_embeddings_bot_model;
CREATE INDEX IF NOT EXISTS idx_memory_node_embeddings_team_bot_model
    ON memory_node_embeddings (team_id, bot_id, model_id);

DO $memory_embeddings_team_fk$
BEGIN
    IF to_regclass('public.teams') IS NOT NULL
       AND NOT EXISTS (
           SELECT 1
             FROM pg_constraint
            WHERE conrelid = 'memory_node_embeddings'::regclass
              AND conname = 'memory_node_embeddings_team_fk'
       ) THEN
        ALTER TABLE memory_node_embeddings
            ADD CONSTRAINT memory_node_embeddings_team_fk
            FOREIGN KEY (team_id) REFERENCES teams(id)
            ON DELETE CASCADE NOT VALID;
        ALTER TABLE memory_node_embeddings
            VALIDATE CONSTRAINT memory_node_embeddings_team_fk;
    END IF;
END
$memory_embeddings_team_fk$;

ALTER TABLE memory_node_embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_node_embeddings FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS memory_node_embeddings_team_isolation ON memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_select ON memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_insert ON memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_update ON memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_delete ON memory_node_embeddings;

CREATE POLICY memory_node_embeddings_team_select
ON memory_node_embeddings FOR SELECT
USING (team_id = app.current_team_id());

CREATE POLICY memory_node_embeddings_team_insert
ON memory_node_embeddings FOR INSERT
WITH CHECK (team_id = app.current_team_id());

CREATE POLICY memory_node_embeddings_team_update
ON memory_node_embeddings FOR UPDATE
USING (team_id = app.current_team_id())
WITH CHECK (team_id = app.current_team_id());

CREATE POLICY memory_node_embeddings_team_delete
ON memory_node_embeddings FOR DELETE
USING (team_id = app.current_team_id());
`

type pgvectorIndex struct {
	pool        *pgxpool.Pool
	lookup      dbstore.Queries
	embedModel  *sdk.EmbeddingModel
	model       embeddingModelSpec
	modelRef    string
	resolveTeam adapters.TeamIDResolver
	logger      *slog.Logger
}

type embeddingModelSpec struct {
	uuid       pgtype.UUID
	modelID    string
	clientType string
	baseURL    string
	apiKey     string
	dimensions int
}

func newPGVectorIndex(ctx context.Context, logger *slog.Logger, providerConfig map[string]any, queries dbstore.Queries, vectorConfig config.PGVectorConfig, resolver adapters.TeamIDResolver) (*pgvectorIndex, error) {
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
	if resolver == nil {
		resolver = adapters.FixedTeamIDResolver(team.DefaultTeamID)
	}
	spec, err := resolveEmbeddingModel(ctx, queries, modelRef)
	if err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(db.DSN(vectorConfig.PostgresConfig()))
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic index: parse dsn: %w", err)
	}
	// Bootstrap the schema on a plain connection first: RegisterTypes fails on
	// a fresh database where the vector type does not exist yet, so the
	// extension must be created before the typed pool opens its first conn.
	if err := bootstrapPgvectorSchema(ctx, poolCfg.ConnConfig); err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = pgxvec.RegisterTypes
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic index: connect: %w", err)
	}
	index := &pgvectorIndex{
		pool:        pool,
		lookup:      queries,
		embedModel:  models.NewSDKEmbeddingModel(spec.clientType, spec.baseURL, spec.apiKey, spec.modelID, semanticEmbedTimeout, nil),
		model:       spec,
		modelRef:    modelRef,
		resolveTeam: resolver,
		logger:      logger,
	}
	if err := index.ensureStore(ctx); err != nil {
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

func (r *pgvectorIndex) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}

// bootstrapPgvectorSchema creates the vector extension and embeddings table
// over a one-off untyped connection, so the typed pool can register the
// vector type on its first connection even against a fresh database.
func bootstrapPgvectorSchema(ctx context.Context, connCfg *pgx.ConnConfig) error {
	conn, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		return fmt.Errorf("pgvector semantic index: bootstrap connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "SELECT set_config('app.team_id', $1, false)", team.DefaultTeamID); err != nil {
		return fmt.Errorf("pgvector semantic index: bind bootstrap team: %w", err)
	}
	if _, err := conn.Exec(ctx, pgvectorSchemaSQL); err != nil {
		return fmt.Errorf("pgvector semantic index: bootstrap schema: %w", err)
	}
	return nil
}

func (r *pgvectorIndex) ensureStore(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgvector semantic index: begin ensure store: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.team_id', $1, true)", team.DefaultTeamID); err != nil {
		return fmt.Errorf("pgvector semantic index: bind bootstrap team: %w", err)
	}
	if _, err := tx.Exec(ctx, pgvectorSchemaSQL); err != nil {
		return fmt.Errorf("pgvector semantic index: ensure store: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgvector semantic index: commit ensure store: %w", err)
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

func (r *pgvectorIndex) teamUUID(ctx context.Context) (pgtype.UUID, error) {
	if r == nil || r.resolveTeam == nil {
		return pgtype.UUID{}, errors.New("pgvector semantic index: team resolver is not configured")
	}
	teamID, err := r.resolveTeam(ctx)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("pgvector semantic index: resolve team: %w", err)
	}
	teamUUID, err := db.ParseUUID(strings.TrimSpace(teamID))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("pgvector semantic index: invalid team id: %w", err)
	}
	return teamUUID, nil
}

// withTeamTx binds app.team_id transaction-locally on the dedicated
// pgvector pool. Every data operation also carries an explicit team_id
// predicate, so the RLS context is never the only isolation boundary.
func (r *pgvectorIndex) withTeamTx(ctx context.Context, fn func(pgx.Tx, pgtype.UUID) error) error {
	if r == nil || r.pool == nil {
		return nil
	}
	teamUUID, err := r.teamUUID(ctx)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgvector semantic index: begin team transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.team_id', $1, true)", teamUUID.String()); err != nil {
		return fmt.Errorf("pgvector semantic index: bind team: %w", err)
	}
	if err := fn(tx, teamUUID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgvector semantic index: commit team transaction: %w", err)
	}
	return nil
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
	err = r.withTeamTx(ctx, func(tx pgx.Tx, teamUUID pgtype.UUID) error {
		_, execErr := tx.Exec(ctx, `
INSERT INTO memory_node_embeddings (
  team_id, bot_id, node_id, model_id, dimensions, body_hash, embedding
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (team_id, bot_id, node_id, model_id) DO UPDATE SET
  dimensions = EXCLUDED.dimensions,
  body_hash = EXCLUDED.body_hash,
  embedding = EXCLUDED.embedding,
  updated_at = now();
`, teamUUID, botUUID, strings.TrimSpace(nodeID), r.model.uuid, dimensions, strings.TrimSpace(hash), pgvector.NewVector(vec))
		return execErr
	})
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
	seeds := map[string]float64{}
	err = r.withTeamTx(ctx, func(tx pgx.Tx, teamUUID pgtype.UUID) error {
		rows, queryErr := tx.Query(ctx, `
SELECT
  node_id,
  CAST(1.0 - (embedding <=> $1::vector) AS double precision) AS score
FROM memory_node_embeddings
WHERE team_id = $2
  AND bot_id = $3
  AND model_id = $4
ORDER BY embedding <=> $1::vector
LIMIT $5;
`, pgvector.NewVector(vec), teamUUID, botUUID, r.model.uuid, rowLimit)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()

		for rows.Next() {
			var nodeID string
			var score float64
			if scanErr := rows.Scan(&nodeID, &score); scanErr != nil {
				return fmt.Errorf("scan search row: %w", scanErr)
			}
			nodeID = strings.TrimSpace(nodeID)
			if nodeID != "" {
				seeds[nodeID] = score
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			return fmt.Errorf("iterate search rows: %w", rowsErr)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pgvector semantic index: search: %w", err)
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
	err = r.withTeamTx(ctx, func(tx pgx.Tx, teamUUID pgtype.UUID) error {
		_, execErr := tx.Exec(ctx, `
DELETE FROM memory_node_embeddings
WHERE team_id = $1
  AND bot_id = $2
  AND node_id = ANY($3::text[]);
`, teamUUID, botUUID, ids)
		return execErr
	})
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
	err = r.withTeamTx(ctx, func(tx pgx.Tx, teamUUID pgtype.UUID) error {
		_, execErr := tx.Exec(ctx, `
DELETE FROM memory_node_embeddings
WHERE team_id = $1
  AND bot_id = $2;
`, teamUUID, botUUID)
		return execErr
	})
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
	err = r.withTeamTx(ctx, func(tx pgx.Tx, teamUUID pgtype.UUID) error {
		return tx.QueryRow(ctx, `
SELECT COUNT(*)
FROM memory_node_embeddings
WHERE team_id = $1
  AND bot_id = $2
  AND model_id = $3;
`, teamUUID, botUUID, r.model.uuid).Scan(&count)
	})
	if err != nil {
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
	var ok bool
	err := r.withTeamTx(ctx, func(tx pgx.Tx, teamUUID pgtype.UUID) error {
		return tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM memory_node_embeddings
  WHERE team_id = $1
  LIMIT 1
);
`, teamUUID).Scan(&ok)
	})
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
