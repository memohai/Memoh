package wikistore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/memory/migrate"
	"github.com/memohai/memoh/internal/teams"
)

const (
	memoryNodeSelectColumns = `
id, bot_id, body, hash, layer, fact_type, subject, confidence, metadata,
source_message_ids, profile_ref, topic, captured_at, expires_at, updated_at, created_at`

	memoryEdgeSelectColumns = `
id, bot_id, src_node, dst_node, rel, weight, metadata, created_at`
)

// PostgresStore implements Store over the PostgreSQL memory_wiki tables.
type PostgresStore struct {
	q  *dbsqlc.Queries
	db dbsqlc.DBTX
}

// NewPostgres returns a Store backed by the PostgreSQL sqlc Queries.
func NewPostgres(q *dbsqlc.Queries) *PostgresStore {
	return &PostgresStore{q: q, db: sqlcDBTX(q)}
}

func (s *PostgresStore) UpsertNode(ctx context.Context, node migrate.NodeSpec) (migrate.NodeSpec, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return migrate.NodeSpec{}, err
	}
	r := nodeToRecord(node)
	row, err := scanMemoryNode(dbtx.QueryRow(ctx, `
INSERT INTO memory_nodes (
  team_id, id, bot_id, body, hash, layer, fact_type, subject, confidence,
  metadata, source_message_ids, profile_ref, topic, captured_at, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (team_id, bot_id, id) DO UPDATE SET
  body = EXCLUDED.body,
  hash = EXCLUDED.hash,
  layer = EXCLUDED.layer,
  fact_type = EXCLUDED.fact_type,
  subject = EXCLUDED.subject,
  confidence = EXCLUDED.confidence,
  metadata = EXCLUDED.metadata,
  source_message_ids = EXCLUDED.source_message_ids,
  profile_ref = EXCLUDED.profile_ref,
  topic = EXCLUDED.topic,
  expires_at = EXCLUDED.expires_at,
  updated_at = now()
RETURNING `+memoryNodeSelectColumns+`;
`, teamID, r.ID, pgUUID(r.BotID), r.Body, r.Hash, r.Layer, r.FactType, r.Subject, r.Confidence,
		marshalJSON(r.Metadata), marshalStringList(r.SourceMessageIDs), r.ProfileRef, r.Topic,
		pgTimestamptz(r.CapturedAt), pgTimestamptz(r.ExpiresAt)))
	if err != nil {
		return migrate.NodeSpec{}, fmt.Errorf("wikistore(postgres): upsert node: %w", err)
	}
	return recordToNode(pgMemoryNodeToRecord(row)), nil
}

func (s *PostgresStore) GetNode(ctx context.Context, botID, nodeID string) (migrate.NodeSpec, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return migrate.NodeSpec{}, err
	}
	row, err := scanMemoryNode(dbtx.QueryRow(ctx, `
SELECT `+memoryNodeSelectColumns+`
FROM memory_nodes
WHERE team_id = $1 AND bot_id = $2 AND id = $3;
`, teamID, pgUUID(botID), nodeID))
	if err != nil {
		if isPgNoRows(err) {
			return migrate.NodeSpec{}, ErrNodeNotFound
		}
		return migrate.NodeSpec{}, fmt.Errorf("wikistore(postgres): get node: %w", err)
	}
	return recordToNode(pgMemoryNodeToRecord(row)), nil
}

func (s *PostgresStore) ListNodes(ctx context.Context, botID string) ([]migrate.NodeSpec, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbtx.Query(ctx, `
SELECT `+memoryNodeSelectColumns+`
FROM memory_nodes
WHERE team_id = $1 AND bot_id = $2
ORDER BY captured_at ASC;
`, teamID, pgUUID(botID))
	if err != nil {
		return nil, fmt.Errorf("wikistore(postgres): list nodes: %w", err)
	}
	defer rows.Close()
	records, err := scanMemoryNodes(rows)
	if err != nil {
		return nil, fmt.Errorf("wikistore(postgres): list nodes: %w", err)
	}
	out := make([]migrate.NodeSpec, 0, len(records))
	for _, r := range records {
		out = append(out, recordToNode(pgMemoryNodeToRecord(r)))
	}
	return out, nil
}

func (s *PostgresStore) ListNodesByLayer(ctx context.Context, botID string, layer migrate.MemoryLayer) ([]migrate.NodeSpec, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbtx.Query(ctx, `
SELECT `+memoryNodeSelectColumns+`
FROM memory_nodes
WHERE team_id = $1 AND bot_id = $2 AND layer = $3
ORDER BY captured_at ASC;
`, teamID, pgUUID(botID), string(orDefaultLayer(layer)))
	if err != nil {
		return nil, fmt.Errorf("wikistore(postgres): list nodes by layer: %w", err)
	}
	defer rows.Close()
	records, err := scanMemoryNodes(rows)
	if err != nil {
		return nil, fmt.Errorf("wikistore(postgres): list nodes by layer: %w", err)
	}
	out := make([]migrate.NodeSpec, 0, len(records))
	for _, r := range records {
		out = append(out, recordToNode(pgMemoryNodeToRecord(r)))
	}
	return out, nil
}

func (s *PostgresStore) DeleteNode(ctx context.Context, botID, nodeID string) error {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return err
	}
	if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_edges
WHERE team_id = $1 AND bot_id = $2 AND (src_node = $3 OR dst_node = $3);
`, teamID, pgUUID(botID), nodeID); err != nil {
		return fmt.Errorf("wikistore(postgres): delete node edges: %w", err)
	}
	if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_nodes
WHERE team_id = $1 AND bot_id = $2 AND id = $3;
`, teamID, pgUUID(botID), nodeID); err != nil {
		return fmt.Errorf("wikistore(postgres): delete node: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteAllNodes(ctx context.Context, botID string) error {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return err
	}
	if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_edges
WHERE team_id = $1 AND bot_id = $2;
`, teamID, pgUUID(botID)); err != nil {
		return fmt.Errorf("wikistore(postgres): delete all edges: %w", err)
	}
	if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_nodes
WHERE team_id = $1 AND bot_id = $2;
`, teamID, pgUUID(botID)); err != nil {
		return fmt.Errorf("wikistore(postgres): delete all nodes: %w", err)
	}
	return nil
}

func (s *PostgresStore) CountNodes(ctx context.Context, botID string) (int, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := dbtx.QueryRow(ctx, `
SELECT COUNT(*)
FROM memory_nodes
WHERE team_id = $1 AND bot_id = $2;
`, teamID, pgUUID(botID)).Scan(&n); err != nil {
		return 0, fmt.Errorf("wikistore(postgres): count nodes: %w", err)
	}
	return int(n), nil
}

func (s *PostgresStore) UpsertEdges(ctx context.Context, edges []migrate.EdgeSpec) error {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return err
	}
	for _, e := range edges {
		if _, err := dbtx.Exec(ctx, `
INSERT INTO memory_edges (team_id, bot_id, src_node, dst_node, rel, weight, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (team_id, bot_id, src_node, dst_node, rel) DO UPDATE SET
  weight = EXCLUDED.weight,
  metadata = EXCLUDED.metadata;
`, teamID, pgUUID(e.BotID), e.SrcNode, e.DstNode, string(e.Rel), e.Weight, marshalJSON(e.Metadata)); err != nil {
			return fmt.Errorf("wikistore(postgres): upsert edge: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) ListEdges(ctx context.Context, botID string) ([]migrate.EdgeSpec, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbtx.Query(ctx, `
SELECT `+memoryEdgeSelectColumns+`
FROM memory_edges
WHERE team_id = $1 AND bot_id = $2;
`, teamID, pgUUID(botID))
	if err != nil {
		return nil, fmt.Errorf("wikistore(postgres): list edges: %w", err)
	}
	defer rows.Close()
	records, err := scanMemoryEdges(rows)
	if err != nil {
		return nil, fmt.Errorf("wikistore(postgres): list edges: %w", err)
	}
	out := make([]migrate.EdgeSpec, 0, len(records))
	for _, r := range records {
		out = append(out, pgMemoryEdgeToSpec(r))
	}
	return out, nil
}

func (s *PostgresStore) DeleteEdgesForNode(ctx context.Context, botID, nodeID string) error {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return err
	}
	if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_edges
WHERE team_id = $1 AND bot_id = $2 AND (src_node = $3 OR dst_node = $3);
`, teamID, pgUUID(botID), nodeID); err != nil {
		return fmt.Errorf("wikistore(postgres): delete edges for node: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteAllEdges(ctx context.Context, botID string) error {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return err
	}
	if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_edges
WHERE team_id = $1 AND bot_id = $2;
`, teamID, pgUUID(botID)); err != nil {
		return fmt.Errorf("wikistore(postgres): delete all edges: %w", err)
	}
	return nil
}

func (s *PostgresStore) CountEdges(ctx context.Context, botID string) (int, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := dbtx.QueryRow(ctx, `
SELECT COUNT(*)
FROM memory_edges
WHERE team_id = $1 AND bot_id = $2;
`, teamID, pgUUID(botID)).Scan(&n); err != nil {
		return 0, fmt.Errorf("wikistore(postgres): count edges: %w", err)
	}
	return int(n), nil
}

func (s *PostgresStore) RebuildDerivedEdges(ctx context.Context, botID string) (int, error) {
	dbtx, teamID, err := s.scopedDB(ctx)
	if err != nil {
		return 0, err
	}
	for _, rel := range migrate.DerivedEdgeRels {
		if _, err := dbtx.Exec(ctx, `
DELETE FROM memory_edges
WHERE team_id = $1 AND bot_id = $2 AND rel = $3;
`, teamID, pgUUID(botID), string(rel)); err != nil {
			return 0, fmt.Errorf("wikistore(postgres): clear derived edges: %w", err)
		}
	}
	nodes, err := s.ListNodes(ctx, botID)
	if err != nil {
		return 0, err
	}
	edges := migrate.PlanFromNodes(nodes)
	derived := filterImplicitEdges(edges, migrate.DerivedEdgeRels)
	if err := s.UpsertEdges(ctx, derived); err != nil {
		return 0, err
	}
	return len(derived), nil
}

func (s *PostgresStore) scopedDB(ctx context.Context) (dbsqlc.DBTX, pgtype.UUID, error) {
	dbtx, err := s.queryDB()
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return dbtx, teamID, nil
}

func (s *PostgresStore) queryDB() (dbsqlc.DBTX, error) {
	if s == nil || s.q == nil || s.db == nil {
		return nil, errors.New("wikistore(postgres): queries not configured")
	}
	return s.db, nil
}

func teamUUIDFromContext(ctx context.Context) (pgtype.UUID, error) {
	scope, err := teams.ScopeFromContext(ctx)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("wikistore(postgres): team scope: %w", err)
	}
	return pgUUIDRequired("team_id", scope.TeamID)
}

func pgUUIDRequired(name, value string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(strings.TrimSpace(value)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("wikistore(postgres): invalid %s: %w", name, err)
	}
	if !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("wikistore(postgres): invalid %s", name)
	}
	return u, nil
}

func sqlcDBTX(q *dbsqlc.Queries) dbsqlc.DBTX {
	if q == nil {
		return nil
	}
	field := reflect.ValueOf(q).Elem().FieldByName("db")
	if !field.IsValid() || field.IsNil() || !field.CanAddr() {
		return nil
	}
	// #nosec G103 -- sqlc keeps the DBTX field private; this is a scoped adapter shim.
	value := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	dbtx, _ := value.(dbsqlc.DBTX)
	return dbtx
}

func scanMemoryNode(row pgx.Row) (dbsqlc.MemoryNode, error) {
	var r dbsqlc.MemoryNode
	err := row.Scan(
		&r.ID,
		&r.BotID,
		&r.Body,
		&r.Hash,
		&r.Layer,
		&r.FactType,
		&r.Subject,
		&r.Confidence,
		&r.Metadata,
		&r.SourceMessageIds,
		&r.ProfileRef,
		&r.Topic,
		&r.CapturedAt,
		&r.ExpiresAt,
		&r.UpdatedAt,
		&r.CreatedAt,
	)
	return r, err
}

func scanMemoryNodes(rows pgx.Rows) ([]dbsqlc.MemoryNode, error) {
	var out []dbsqlc.MemoryNode
	for rows.Next() {
		r, err := scanMemoryNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanMemoryEdge(row pgx.Row) (dbsqlc.MemoryEdge, error) {
	var r dbsqlc.MemoryEdge
	err := row.Scan(
		&r.ID,
		&r.BotID,
		&r.SrcNode,
		&r.DstNode,
		&r.Rel,
		&r.Weight,
		&r.Metadata,
		&r.CreatedAt,
	)
	return r, err
}

func scanMemoryEdges(rows pgx.Rows) ([]dbsqlc.MemoryEdge, error) {
	var out []dbsqlc.MemoryEdge
	for rows.Next() {
		r, err := scanMemoryEdge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- Postgres row -> record/spec helpers ----

func pgMemoryNodeToRecord(r dbsqlc.MemoryNode) record {
	rec := record{
		ID:               r.ID,
		BotID:            pgUUIDString(r.BotID),
		Body:             r.Body,
		Hash:             r.Hash,
		Layer:            r.Layer,
		FactType:         r.FactType,
		Subject:          r.Subject,
		Confidence:       r.Confidence,
		Metadata:         unmarshalMetadata(r.Metadata),
		SourceMessageIDs: unmarshalStringList(r.SourceMessageIds),
		ProfileRef:       r.ProfileRef,
		Topic:            r.Topic,
		CapturedAt:       pgTimeValue(r.CapturedAt),
		ExpiresAt:        pgTimeValue(r.ExpiresAt),
	}
	return rec
}

func pgMemoryEdgeToSpec(r dbsqlc.MemoryEdge) migrate.EdgeSpec {
	return migrate.EdgeSpec{
		BotID:    pgUUIDString(r.BotID),
		SrcNode:  r.SrcNode,
		DstNode:  r.DstNode,
		Rel:      migrate.EdgeRel(r.Rel),
		Weight:   r.Weight,
		Metadata: unmarshalMetadata(r.Metadata),
	}
}

// pgUUID parses a string into a pgtype.UUID.
func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

func pgUUIDString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

// pgTimestamptz converts a time.Time into a pgtype.Timestamptz; zero time
// produces an invalid (NULL) value.
func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func pgTimeValue(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time.UTC()
}

// isPgNoRows reports whether err is a pgx "no rows" error.
func isPgNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
