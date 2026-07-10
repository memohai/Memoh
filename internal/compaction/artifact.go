package compaction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/historyfrag"
)

const ArtifactVersion = 1

type CoveredSource struct {
	Ref                    contextfrag.ContextRef `json:"ref"`
	ExternalMessageID      string                 `json:"external_message_id,omitempty"`
	SourceReplyToMessageID string                 `json:"source_reply_to_message_id,omitempty"`
	CreatedAtMs            int64                  `json:"created_at_ms,omitempty"`
}

type Artifact struct {
	ID            string
	BotID         string
	SessionID     string
	Summary       string
	Version       int
	Coverage      []CoveredSource
	AnchorStartMs int64
	AnchorEndMs   int64
	Level         int
	ParentIDs     []string
}

func (a Artifact) HistoryRecord(scope contextfrag.Scope) historyfrag.HistoryRecord {
	if scope.BotID == "" {
		scope.BotID = a.BotID
	}
	if scope.SessionID == "" {
		scope.SessionID = a.SessionID
	}
	coveredRefs := make([]contextfrag.ContextRef, 0, len(a.Coverage))
	for _, source := range a.Coverage {
		coveredRefs = append(coveredRefs, source.Ref)
	}
	record := historyfrag.SummaryRecord(a.ID, a.Summary, coveredRefs, scope)
	if a.AnchorStartMs > 0 {
		record.CreatedAt = time.UnixMilli(a.AnchorStartMs).UTC()
	}
	return record
}

type ArtifactProjection struct {
	queries dbstore.Queries
}

func NewArtifactProjection(queries dbstore.Queries) ArtifactProjection {
	return ArtifactProjection{queries: queries}
}

func (p ArtifactProjection) LoadActiveSession(ctx context.Context, sessionID string) ([]Artifact, error) {
	if p.queries == nil {
		return nil, nil
	}
	sessionUUID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := p.queries.ListActiveCompactionArtifactsBySession(ctx, sessionUUID)
	if err != nil {
		return nil, err
	}
	artifacts := make([]Artifact, 0, len(rows))
	for _, row := range rows {
		artifact, err := ArtifactFromDBRow(row)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (p ArtifactProjection) LoadByID(ctx context.Context, id pgtype.UUID) (Artifact, error) {
	if p.queries == nil {
		return Artifact{}, fmt.Errorf("compaction artifact projection: queries are required")
	}
	row, err := p.queries.GetCompactionLogByID(ctx, id)
	if err != nil {
		return Artifact{}, err
	}
	return ArtifactFromDBRow(row)
}

func ArtifactFromDBRow(row sqlc.BotHistoryMessageCompact) (Artifact, error) {
	id := formatUUID(row.ID)
	if id == "" {
		return Artifact{}, fmt.Errorf("compaction artifact: id is required")
	}
	if row.Status != "ok" || strings.TrimSpace(row.Summary) == "" {
		return Artifact{}, fmt.Errorf("compaction artifact %s is not active", id)
	}
	coverage, err := DecodeArtifactCoverage(row.Coverage)
	if err != nil {
		return Artifact{}, fmt.Errorf("compaction artifact %s: %w", id, err)
	}
	version := int(row.ArtifactVersion)
	if version == 0 {
		version = ArtifactVersion
	}
	parentIDs := make([]string, 0, len(row.ParentIds))
	for _, parentID := range row.ParentIds {
		if value := formatUUID(parentID); value != "" {
			parentIDs = append(parentIDs, value)
		}
	}
	return Artifact{
		ID:            id,
		BotID:         formatUUID(row.BotID),
		SessionID:     formatUUID(row.SessionID),
		Summary:       row.Summary,
		Version:       version,
		Coverage:      coverage,
		AnchorStartMs: row.AnchorStartMs,
		AnchorEndMs:   row.AnchorEndMs,
		Level:         int(row.ArtifactLevel),
		ParentIDs:     parentIDs,
	}, nil
}

type artifactMetadata struct {
	Coverage      []byte
	AnchorStartMs int64
	AnchorEndMs   int64
}

func artifactMetadataFor(items []CompactionCandidate, ids []pgtype.UUID) (artifactMetadata, error) {
	byID := make(map[pgtype.UUID]CompactionCandidate, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	covered := make([]CoveredSource, 0, len(ids))
	for _, id := range ids {
		item, ok := byID[id]
		if !ok {
			return artifactMetadata{}, fmt.Errorf("compaction artifact: marked id %s missing from candidates", formatUUID(id))
		}
		createdAtMs := int64(0)
		if !item.Record.CreatedAt.IsZero() {
			createdAtMs = item.Record.CreatedAt.UnixMilli()
		}
		covered = append(covered, CoveredSource{
			Ref:                    item.Record.Ref,
			ExternalMessageID:      item.Record.ExternalMessageID,
			SourceReplyToMessageID: item.Record.SourceReplyToMessageID,
			CreatedAtMs:            createdAtMs,
		})
	}
	encoded, err := json.Marshal(covered)
	if err != nil {
		return artifactMetadata{}, fmt.Errorf("encode compaction artifact coverage: %w", err)
	}
	metadata := artifactMetadata{Coverage: encoded}
	if len(covered) > 0 {
		metadata.AnchorStartMs = covered[0].CreatedAtMs
		metadata.AnchorEndMs = covered[len(covered)-1].CreatedAtMs
	}
	return metadata, nil
}

func DecodeArtifactCoverage(raw []byte) ([]CoveredSource, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var covered []CoveredSource
	if err := json.Unmarshal(raw, &covered); err != nil {
		return nil, fmt.Errorf("decode compaction artifact coverage: %w", err)
	}
	return covered, nil
}
