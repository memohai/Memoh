package wikistore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
	"github.com/memohai/memoh/internal/memory/migrate"
)

// SQLiteStore implements Store over the SQLite memory_wiki tables.
type SQLiteStore struct {
	q *sqlitesqlc.Queries
}

// NewSQLite returns a Store backed by the SQLite sqlc Queries.
func NewSQLite(q *sqlitesqlc.Queries) *SQLiteStore {
	return &SQLiteStore{q: q}
}

func (s *SQLiteStore) UpsertNode(ctx context.Context, node migrate.NodeSpec) (migrate.NodeSpec, error) {
	if s.q == nil {
		return migrate.NodeSpec{}, errors.New("wikistore(sqlite): queries not configured")
	}
	r := nodeToRecord(node)
	row, err := s.q.UpsertMemoryNode(ctx, sqlitesqlc.UpsertMemoryNodeParams{
		ID:               r.ID,
		BotID:            r.BotID,
		Body:             r.Body,
		Hash:             r.Hash,
		Layer:            r.Layer,
		FactType:         r.FactType,
		Subject:          r.Subject,
		Confidence:       float64(r.Confidence),
		Metadata:         marshalStringJSON(r.Metadata),
		SourceMessageIds: marshalStringListJSON(r.SourceMessageIDs),
		ProfileRef:       r.ProfileRef,
		Topic:            r.Topic,
		CapturedAt:       r.CapturedAt.UTC().Format(time.RFC3339),
		ExpiresAt:        sql.NullString{},
	})
	if err != nil {
		return migrate.NodeSpec{}, fmt.Errorf("wikistore(sqlite): upsert node: %w", err)
	}
	if !r.ExpiresAt.IsZero() {
		row.ExpiresAt = sql.NullString{String: r.ExpiresAt.UTC().Format(time.RFC3339), Valid: true}
	}
	return recordToNode(sqliteMemoryNodeToRecord(row)), nil
}

func (s *SQLiteStore) GetNode(ctx context.Context, botID, nodeID string) (migrate.NodeSpec, error) {
	if s.q == nil {
		return migrate.NodeSpec{}, errors.New("wikistore(sqlite): queries not configured")
	}
	row, err := s.q.GetMemoryNode(ctx, sqlitesqlc.GetMemoryNodeParams{BotID: botID, ID: nodeID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return migrate.NodeSpec{}, ErrNodeNotFound
		}
		return migrate.NodeSpec{}, fmt.Errorf("wikistore(sqlite): get node: %w", err)
	}
	return recordToNode(sqliteMemoryNodeToRecord(row)), nil
}

func (s *SQLiteStore) ListNodes(ctx context.Context, botID string) ([]migrate.NodeSpec, error) {
	if s.q == nil {
		return nil, errors.New("wikistore(sqlite): queries not configured")
	}
	rows, err := s.q.ListMemoryNodesByBot(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("wikistore(sqlite): list nodes: %w", err)
	}
	out := make([]migrate.NodeSpec, 0, len(rows))
	for _, r := range rows {
		out = append(out, recordToNode(sqliteMemoryNodeToRecord(r)))
	}
	return out, nil
}

func (s *SQLiteStore) ListNodesByLayer(ctx context.Context, botID string, layer migrate.MemoryLayer) ([]migrate.NodeSpec, error) {
	if s.q == nil {
		return nil, errors.New("wikistore(sqlite): queries not configured")
	}
	rows, err := s.q.ListMemoryNodesByBotLayer(ctx, sqlitesqlc.ListMemoryNodesByBotLayerParams{
		BotID: botID,
		Layer: string(orDefaultLayer(layer)),
	})
	if err != nil {
		return nil, fmt.Errorf("wikistore(sqlite): list nodes by layer: %w", err)
	}
	out := make([]migrate.NodeSpec, 0, len(rows))
	for _, r := range rows {
		out = append(out, recordToNode(sqliteMemoryNodeToRecord(r)))
	}
	return out, nil
}

func (s *SQLiteStore) DeleteNode(ctx context.Context, botID, nodeID string) error {
	if s.q == nil {
		return errors.New("wikistore(sqlite): queries not configured")
	}
	if err := s.q.DeleteMemoryEdgesForNode(ctx, sqlitesqlc.DeleteMemoryEdgesForNodeParams{BotID: botID, NodeID: nodeID}); err != nil {
		return fmt.Errorf("wikistore(sqlite): delete node edges: %w", err)
	}
	if err := s.q.DeleteMemoryNode(ctx, sqlitesqlc.DeleteMemoryNodeParams{BotID: botID, ID: nodeID}); err != nil {
		return fmt.Errorf("wikistore(sqlite): delete node: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteAllNodes(ctx context.Context, botID string) error {
	if s.q == nil {
		return errors.New("wikistore(sqlite): queries not configured")
	}
	if err := s.q.DeleteAllMemoryEdgesByBot(ctx, botID); err != nil {
		return fmt.Errorf("wikistore(sqlite): delete all edges: %w", err)
	}
	if err := s.q.DeleteAllMemoryNodesByBot(ctx, botID); err != nil {
		return fmt.Errorf("wikistore(sqlite): delete all nodes: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CountNodes(ctx context.Context, botID string) (int, error) {
	if s.q == nil {
		return 0, errors.New("wikistore(sqlite): queries not configured")
	}
	n, err := s.q.CountMemoryNodesByBot(ctx, botID)
	if err != nil {
		return 0, fmt.Errorf("wikistore(sqlite): count nodes: %w", err)
	}
	return int(n), nil
}

func (s *SQLiteStore) UpsertEdges(ctx context.Context, edges []migrate.EdgeSpec) error {
	if s.q == nil {
		return errors.New("wikistore(sqlite): queries not configured")
	}
	for _, e := range edges {
		if err := s.q.InsertMemoryEdge(ctx, sqlitesqlc.InsertMemoryEdgeParams{
			BotID:    e.BotID,
			SrcNode:  e.SrcNode,
			DstNode:  e.DstNode,
			Rel:      string(e.Rel),
			Weight:   float64(e.Weight),
			Metadata: marshalStringJSON(e.Metadata),
		}); err != nil {
			return fmt.Errorf("wikistore(sqlite): upsert edge: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) ListEdges(ctx context.Context, botID string) ([]migrate.EdgeSpec, error) {
	if s.q == nil {
		return nil, errors.New("wikistore(sqlite): queries not configured")
	}
	rows, err := s.q.ListMemoryEdgesByBot(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("wikistore(sqlite): list edges: %w", err)
	}
	out := make([]migrate.EdgeSpec, 0, len(rows))
	for _, r := range rows {
		out = append(out, sqliteMemoryEdgeToSpec(r))
	}
	return out, nil
}

func (s *SQLiteStore) DeleteEdgesForNode(ctx context.Context, botID, nodeID string) error {
	if s.q == nil {
		return errors.New("wikistore(sqlite): queries not configured")
	}
	if err := s.q.DeleteMemoryEdgesForNode(ctx, sqlitesqlc.DeleteMemoryEdgesForNodeParams{BotID: botID, NodeID: nodeID}); err != nil {
		return fmt.Errorf("wikistore(sqlite): delete edges for node: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteAllEdges(ctx context.Context, botID string) error {
	if s.q == nil {
		return errors.New("wikistore(sqlite): queries not configured")
	}
	if err := s.q.DeleteAllMemoryEdgesByBot(ctx, botID); err != nil {
		return fmt.Errorf("wikistore(sqlite): delete all edges: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CountEdges(ctx context.Context, botID string) (int, error) {
	if s.q == nil {
		return 0, errors.New("wikistore(sqlite): queries not configured")
	}
	n, err := s.q.CountMemoryEdgesByBot(ctx, botID)
	if err != nil {
		return 0, fmt.Errorf("wikistore(sqlite): count edges: %w", err)
	}
	return int(n), nil
}

func (s *SQLiteStore) RebuildImplicitEdges(ctx context.Context, botID string) (int, error) {
	if s.q == nil {
		return 0, errors.New("wikistore(sqlite): queries not configured")
	}
	nodes, err := s.ListNodes(ctx, botID)
	if err != nil {
		return 0, err
	}
	// Remove prior implicit edges then re-derive.
	implicitRels := []migrate.EdgeRel{migrate.EdgeSameProfile, migrate.EdgeSameTopic, migrate.EdgeSameDay}
	for _, rel := range implicitRels {
		if err := s.q.DeleteMemoryEdgesByRelForBot(ctx, sqlitesqlc.DeleteMemoryEdgesByRelForBotParams{
			BotID: botID,
			Rel:   string(rel),
		}); err != nil {
			return 0, fmt.Errorf("wikistore(sqlite): clear implicit edges: %w", err)
		}
	}
	specs := nodes
	edges := migrate.PlanFromNodes(specs)
	implicit := filterImplicitEdges(edges, implicitRels)
	if err := s.UpsertEdges(ctx, implicit); err != nil {
		return 0, err
	}
	return len(implicit), nil
}

// ---- SQLite row -> record/spec helpers ----

func sqliteMemoryNodeToRecord(r sqlitesqlc.MemoryNode) record {
	rec := record{
		ID:               r.ID,
		BotID:            r.BotID,
		Body:             r.Body,
		Hash:             r.Hash,
		Layer:            r.Layer,
		FactType:         r.FactType,
		Subject:          r.Subject,
		Confidence:       float32(r.Confidence),
		Metadata:         unmarshalMetadata([]byte(r.Metadata)),
		SourceMessageIDs: unmarshalStringList([]byte(r.SourceMessageIds)),
		ProfileRef:       r.ProfileRef,
		Topic:            r.Topic,
		CapturedAt:       parseTime(r.CapturedAt),
	}
	if r.ExpiresAt.Valid {
		rec.ExpiresAt = parseTime(r.ExpiresAt.String)
	}
	return rec
}

func sqliteMemoryEdgeToSpec(r sqlitesqlc.MemoryEdge) migrate.EdgeSpec {
	return migrate.EdgeSpec{
		BotID:    r.BotID,
		SrcNode:  r.SrcNode,
		DstNode:  r.DstNode,
		Rel:      migrate.EdgeRel(r.Rel),
		Weight:   float32(r.Weight),
		Metadata: unmarshalMetadata([]byte(r.Metadata)),
	}
}

// filterImplicitEdges keeps only edges whose rel is in the allowed set. Used
// by RebuildImplicitEdges to avoid clobbering explicit refs/supersedes edges.
func filterImplicitEdges(edges []migrate.EdgeSpec, allowed []migrate.EdgeRel) []migrate.EdgeSpec {
	if len(edges) == 0 {
		return nil
	}
	set := make(map[migrate.EdgeRel]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	out := make([]migrate.EdgeSpec, 0, len(edges))
	for _, e := range edges {
		if _, ok := set[e.Rel]; ok {
			out = append(out, e)
		}
	}
	return out
}
