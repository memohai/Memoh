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
		sources := make([]pipelinepkg.CompactionSource, 0, len(artifact.Coverage))
		for _, source := range artifact.Coverage {
			sources = append(sources, pipelinepkg.CompactionSource{
				HistoryMessageID:  source.Ref.ID,
				ExternalMessageID: source.ExternalMessageID,
				CreatedAtMs:       source.CreatedAtMs,
			})
		}
		artifacts = append(artifacts, pipelinepkg.CompactionArtifact{
			ID:            artifact.ID,
			Summary:       artifact.Summary,
			AnchorStartMs: artifact.AnchorStartMs,
			Sources:       sources,
		})
		summaries = append(summaries, artifact.HistoryRecord(scope))
	}
	return artifacts, summaries, nil
}
