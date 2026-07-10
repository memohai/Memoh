package compaction

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/contextfrag"
)

const ArtifactVersion = 1

type CoveredSource struct {
	Ref                    contextfrag.ContextRef `json:"ref"`
	ExternalMessageID      string                 `json:"external_message_id,omitempty"`
	SourceReplyToMessageID string                 `json:"source_reply_to_message_id,omitempty"`
	CreatedAtMs            int64                  `json:"created_at_ms,omitempty"`
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
