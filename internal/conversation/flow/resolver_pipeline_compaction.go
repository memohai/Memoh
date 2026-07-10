package flow

import (
	"context"
	"strings"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func (r *Resolver) LoadCompactionArtifacts(
	ctx context.Context,
	botID string,
	sessionID string,
	messages []messagepkg.Message,
) ([]pipelinepkg.CompactionArtifact, error) {
	records := make([]historyfrag.HistoryRecord, 0, len(messages))
	for _, message := range messages {
		record, err := historyfrag.FromDBMessageWithLogger(r.logger, message, historyfrag.ScopeFallback{})
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	artifacts, _, err := r.loadPipelineCompactionArtifacts(ctx, compactionSummaryScope(
		botID,
		"",
		sessionID,
		"",
		"",
		"",
	), records)
	return artifacts, err
}

func (r *Resolver) loadPipelineCompactionArtifacts(
	ctx context.Context,
	scope contextfrag.Scope,
	records []historyfrag.HistoryRecord,
) ([]pipelinepkg.CompactionArtifact, []historyfrag.HistoryRecord, error) {
	if r.queries == nil || strings.TrimSpace(scope.SessionID) == "" {
		return nil, nil, nil
	}
	owner := compaction.ArtifactOwner{
		BotID:          strings.TrimSpace(scope.BotID),
		SessionID:      strings.TrimSpace(scope.SessionID),
		SessionIDKnown: true,
	}
	frontier, err := r.loadActiveCompactionFrontier(ctx, owner.BotID, owner.SessionID)
	if err != nil {
		return nil, nil, err
	}
	catalog := compaction.NewArtifactCatalog()
	catalog.Add(owner, frontier)
	blocked := conflictingArtifactIDs(catalog, records, scope)

	artifacts := make([]pipelinepkg.CompactionArtifact, 0, len(frontier.Artifacts))
	summaries := make([]historyfrag.HistoryRecord, 0, len(frontier.Artifacts))
	for _, artifact := range frontier.Artifacts {
		if _, conflict := blocked[artifact.ID]; conflict {
			continue
		}
		projected := artifact
		if len(projected.Coverage) == 0 {
			projected.Coverage = legacyArtifactCoverage(catalog, projected, records, scope)
			if len(projected.Coverage) == 0 {
				continue
			}
			projected.AnchorStartMs = projected.Coverage[0].CreatedAtMs
			projected.AnchorEndMs = projected.Coverage[len(projected.Coverage)-1].CreatedAtMs
		}
		sources := make([]pipelinepkg.CompactionSource, 0, len(projected.Coverage))
		for _, source := range projected.Coverage {
			sources = append(sources, pipelinepkg.CompactionSource{
				Ref:               source.Ref,
				HistoryMessageID:  source.Ref.ID,
				ExternalMessageID: source.ExternalMessageID,
				CreatedAtMs:       source.CreatedAtMs,
			})
		}
		artifacts = append(artifacts, pipelinepkg.CompactionArtifact{
			ID:            projected.ID,
			Summary:       projected.Summary,
			AnchorStartMs: projected.AnchorStartMs,
			Sources:       sources,
		})
		summaries = append(summaries, projected.HistoryRecord(scope))
	}
	return artifacts, summaries, nil
}

func legacyArtifactCoverage(
	catalog *compaction.ArtifactCatalog,
	artifact compaction.Artifact,
	records []historyfrag.HistoryRecord,
	scope contextfrag.Scope,
) []compaction.CoveredSource {
	covered := make([]compaction.CoveredSource, 0)
	seen := make(map[string]struct{})
	for _, record := range records {
		compactID := strings.TrimSpace(record.CompactID)
		if compactID == "" {
			continue
		}
		resolved, ok := catalog.Resolve(recordArtifactOwner(record, scope), compactID)
		if !ok || resolved.ID != artifact.ID {
			continue
		}
		key := record.Ref.StableKey()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		createdAtMs := int64(0)
		if !record.CreatedAt.IsZero() {
			createdAtMs = record.CreatedAt.UnixMilli()
		}
		covered = append(covered, compaction.CoveredSource{
			Ref:                    record.Ref,
			ExternalMessageID:      record.ExternalMessageID,
			SourceReplyToMessageID: record.SourceReplyToMessageID,
			CreatedAtMs:            createdAtMs,
		})
	}
	return covered
}
