package wikistore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/memory/migrate"
	"github.com/memohai/memoh/internal/teams"
)

// PostgresStore implements Store over the PostgreSQL memory_wiki tables.
type PostgresStore struct {
	q *dbsqlc.Queries
}

// NewPostgres returns a Store backed by the PostgreSQL sqlc Queries.
func NewPostgres(q *dbsqlc.Queries) *PostgresStore {
	return &PostgresStore{q: q}
}

func (s *PostgresStore) UpsertNode(ctx context.Context, node migrate.NodeSpec) (migrate.NodeSpec, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return migrate.NodeSpec{}, err
	}
	r := nodeToRecord(node)
	row, err := q.UpsertMemoryNode(ctx, dbsqlc.UpsertMemoryNodeParams{
		TeamID:           teamID,
		ID:               r.ID,
		BotID:            pgUUID(r.BotID),
		Body:             r.Body,
		Hash:             r.Hash,
		Layer:            r.Layer,
		FactType:         r.FactType,
		Subject:          r.Subject,
		Confidence:       r.Confidence,
		Metadata:         marshalJSON(r.Metadata),
		SourceMessageIds: marshalStringList(r.SourceMessageIDs),
		ProfileRef:       r.ProfileRef,
		Topic:            r.Topic,
		CapturedAt:       pgTimestamptz(r.CapturedAt),
		ExpiresAt:        pgTimestamptz(r.ExpiresAt),
	})
	if err != nil {
		return migrate.NodeSpec{}, fmt.Errorf("wikistore(postgres): upsert node: %w", err)
	}
	return recordToNode(pgMemoryNodeToRecord(row)), nil
}

func (s *PostgresStore) GetNode(ctx context.Context, botID, nodeID string) (migrate.NodeSpec, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return migrate.NodeSpec{}, err
	}
	row, err := q.GetMemoryNode(ctx, dbsqlc.GetMemoryNodeParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
		ID:     nodeID,
	})
	if err != nil {
		if isPgNoRows(err) {
			return migrate.NodeSpec{}, ErrNodeNotFound
		}
		return migrate.NodeSpec{}, fmt.Errorf("wikistore(postgres): get node: %w", err)
	}
	return recordToNode(pgMemoryNodeToRecord(row)), nil
}

func (s *PostgresStore) ListNodes(ctx context.Context, botID string) ([]migrate.NodeSpec, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return nil, err
	}
	records, err := q.ListMemoryNodesByBot(ctx, dbsqlc.ListMemoryNodesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	})
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
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return nil, err
	}
	records, err := q.ListMemoryNodesByBotLayer(ctx, dbsqlc.ListMemoryNodesByBotLayerParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
		Layer:  string(orDefaultLayer(layer)),
	})
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
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return err
	}
	if err := q.DeleteMemoryEdgesForNode(ctx, dbsqlc.DeleteMemoryEdgesForNodeParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
		NodeID: nodeID,
	}); err != nil {
		return fmt.Errorf("wikistore(postgres): delete node edges: %w", err)
	}
	if err := q.DeleteMemoryNode(ctx, dbsqlc.DeleteMemoryNodeParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
		ID:     nodeID,
	}); err != nil {
		return fmt.Errorf("wikistore(postgres): delete node: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteAllNodes(ctx context.Context, botID string) error {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllMemoryEdgesByBot(ctx, dbsqlc.DeleteAllMemoryEdgesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	}); err != nil {
		return fmt.Errorf("wikistore(postgres): delete all edges: %w", err)
	}
	if err := q.DeleteAllMemoryNodesByBot(ctx, dbsqlc.DeleteAllMemoryNodesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	}); err != nil {
		return fmt.Errorf("wikistore(postgres): delete all nodes: %w", err)
	}
	return nil
}

func (s *PostgresStore) CountNodes(ctx context.Context, botID string) (int, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountMemoryNodesByBot(ctx, dbsqlc.CountMemoryNodesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	})
	if err != nil {
		return 0, fmt.Errorf("wikistore(postgres): count nodes: %w", err)
	}
	return int(n), nil
}

func (s *PostgresStore) UpsertEdges(ctx context.Context, edges []migrate.EdgeSpec) error {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return err
	}
	for _, e := range edges {
		if err := q.InsertMemoryEdge(ctx, dbsqlc.InsertMemoryEdgeParams{
			TeamID:   teamID,
			BotID:    pgUUID(e.BotID),
			SrcNode:  e.SrcNode,
			DstNode:  e.DstNode,
			Rel:      string(e.Rel),
			Weight:   e.Weight,
			Metadata: marshalJSON(e.Metadata),
		}); err != nil {
			return fmt.Errorf("wikistore(postgres): upsert edge: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) ListEdges(ctx context.Context, botID string) ([]migrate.EdgeSpec, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return nil, err
	}
	records, err := q.ListMemoryEdgesByBot(ctx, dbsqlc.ListMemoryEdgesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	})
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
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return err
	}
	if err := q.DeleteMemoryEdgesForNode(ctx, dbsqlc.DeleteMemoryEdgesForNodeParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
		NodeID: nodeID,
	}); err != nil {
		return fmt.Errorf("wikistore(postgres): delete edges for node: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteAllEdges(ctx context.Context, botID string) error {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllMemoryEdgesByBot(ctx, dbsqlc.DeleteAllMemoryEdgesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	}); err != nil {
		return fmt.Errorf("wikistore(postgres): delete all edges: %w", err)
	}
	return nil
}

func (s *PostgresStore) CountEdges(ctx context.Context, botID string) (int, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountMemoryEdgesByBot(ctx, dbsqlc.CountMemoryEdgesByBotParams{
		TeamID: teamID,
		BotID:  pgUUID(botID),
	})
	if err != nil {
		return 0, fmt.Errorf("wikistore(postgres): count edges: %w", err)
	}
	return int(n), nil
}

func (s *PostgresStore) RebuildDerivedEdges(ctx context.Context, botID string) (int, error) {
	q, teamID, err := s.scopedQueries(ctx)
	if err != nil {
		return 0, err
	}
	for _, rel := range migrate.DerivedEdgeRels {
		if err := q.DeleteMemoryEdgesByRelForBot(ctx, dbsqlc.DeleteMemoryEdgesByRelForBotParams{
			TeamID: teamID,
			BotID:  pgUUID(botID),
			Rel:    string(rel),
		}); err != nil {
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

func (s *PostgresStore) scopedQueries(ctx context.Context) (*dbsqlc.Queries, pgtype.UUID, error) {
	if s == nil || s.q == nil {
		return nil, pgtype.UUID{}, errors.New("wikistore(postgres): queries not configured")
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return s.q, teamID, nil
}

func teamUUIDFromContext(ctx context.Context) (pgtype.UUID, error) {
	teamID, err := teams.TeamUUID(ctx)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("wikistore(postgres): invalid team_id: %w", err)
	}
	return teamID, nil
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
