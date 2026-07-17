package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

type pipelineArtifactEventCursorReader interface {
	ListMessageEventCursorsByIDs(context.Context, sqlc.ListMessageEventCursorsByIDsParams) ([]sqlc.ListMessageEventCursorsByIDsRow, error)
}

const maxPipelineArtifactEventCursor = 1<<53 - 1

func (r *Resolver) LoadContextHistoryProjection(
	ctx context.Context,
	botID string,
	sessionID string,
) (pipelinepkg.ContextHistoryProjection, error) {
	build, err := r.loadPipelineHistoryProjection(ctx, compactionSummaryScope(
		botID,
		"",
		sessionID,
		"",
		"",
		"",
	), historyfrag.ScopeFallback{})
	return build.projection, err
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

	projectedArtifacts := make([]compaction.Artifact, 0, len(frontier.Artifacts))
	for _, artifact := range frontier.Artifacts {
		if _, conflict := blocked[artifact.ID]; conflict {
			continue
		}
		projected := artifact
		if len(projected.Coverage) == 0 {
			var hasDurableCoverageRows bool
			projected.Coverage, hasDurableCoverageRows, err = r.loadLegacyArtifactCoverage(ctx, projected, owner)
			if err != nil {
				return nil, nil, err
			}
			if len(projected.Coverage) == 0 && !hasDurableCoverageRows {
				projected.Coverage = legacyArtifactCoverage(catalog, projected, records, scope)
			}
			if len(projected.Coverage) == 0 {
				continue
			}
			projected.AnchorStartMs = projected.Coverage[0].CreatedAtMs
			projected.AnchorEndMs = projected.Coverage[len(projected.Coverage)-1].CreatedAtMs
		}
		projectedArtifacts = append(projectedArtifacts, projected)
	}
	projectedArtifacts = r.hydratePipelineArtifactEventCursors(ctx, owner, catalog, projectedArtifacts)

	artifacts := make([]pipelinepkg.CompactionArtifact, 0, len(projectedArtifacts))
	summaries := make([]historyfrag.HistoryRecord, 0, len(projectedArtifacts))
	for _, projected := range projectedArtifacts {
		sources := make([]pipelinepkg.CompactionSource, 0, len(projected.Coverage))
		for _, source := range projected.Coverage {
			sources = append(sources, pipelinepkg.CompactionSource{
				Ref:               source.Ref,
				HistoryMessageID:  source.Ref.ID,
				ExternalMessageID: source.ExternalMessageID,
				CreatedAtMs:       source.CreatedAtMs,
				EventCursor:       source.EventCursor,
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

func (r *Resolver) hydratePipelineArtifactEventCursors(
	ctx context.Context,
	owner compaction.ArtifactOwner,
	catalog *compaction.ArtifactCatalog,
	artifacts []compaction.Artifact,
) []compaction.Artifact {
	reader, ok := r.queries.(pipelineArtifactEventCursorReader)
	if !ok {
		return artifacts
	}
	botID, err := db.ParseUUID(owner.BotID)
	if err != nil {
		return artifacts
	}
	sessionID, err := db.ParseUUID(owner.SessionID)
	if err != nil {
		return artifacts
	}

	messageIDs := make([]pgtype.UUID, 0)
	seen := make(map[pgtype.UUID]struct{})
	for _, artifact := range artifacts {
		for _, source := range artifact.Coverage {
			if source.EventCursor != 0 {
				continue
			}
			expected, err := historyfrag.DBMessageIdentityRef(source.Ref.ID)
			if err != nil || !expected.EqualIdentity(source.Ref) {
				continue
			}
			messageID, err := db.ParseUUID(source.Ref.ID)
			if err != nil {
				continue
			}
			if _, exists := seen[messageID]; exists {
				continue
			}
			seen[messageID] = struct{}{}
			messageIDs = append(messageIDs, messageID)
		}
	}
	if len(messageIDs) == 0 {
		return artifacts
	}

	rows, err := reader.ListMessageEventCursorsByIDs(ctx, sqlc.ListMessageEventCursorsByIDsParams{
		MessageIds: messageIDs,
		BotID:      botID,
		SessionID:  sessionID,
	})
	if err != nil {
		if r.logger != nil {
			r.logger.Warn(
				"hydratePipelineArtifactEventCursors: failed to load source events",
				slog.Int("message_count", len(messageIDs)),
				slog.Any("error", err),
			)
		}
		return artifacts
	}
	rowsByID := make(map[string]sqlc.ListMessageEventCursorsByIDsRow, len(rows))
	for _, row := range rows {
		if row.EventID.Valid && row.EventCursor > 0 && row.EventCursor <= maxPipelineArtifactEventCursor {
			rowsByID[pgUUIDString(row.ID)] = row
		}
	}

	hydrated := append([]compaction.Artifact(nil), artifacts...)
	for artifactIndex := range hydrated {
		hydrated[artifactIndex].Coverage = append([]compaction.CoveredSource(nil), hydrated[artifactIndex].Coverage...)
		for sourceIndex := range hydrated[artifactIndex].Coverage {
			source := &hydrated[artifactIndex].Coverage[sourceIndex]
			if source.EventCursor != 0 {
				continue
			}
			row, exists := rowsByID[strings.TrimSpace(source.Ref.ID)]
			if !exists {
				continue
			}
			claimedArtifact, claimMatches := catalog.Resolve(owner, pgUUIDString(row.CompactID))
			if !claimMatches || claimedArtifact.ID != hydrated[artifactIndex].ID ||
				pgUUIDString(row.BotID) != owner.BotID ||
				pgUUIDString(row.SessionID) != owner.SessionID ||
				strings.TrimSpace(row.ExternalMessageID.String) != strings.TrimSpace(source.ExternalMessageID) ||
				strings.TrimSpace(row.SourceReplyToMessageID.String) != strings.TrimSpace(source.SourceReplyToMessageID) ||
				!row.CreatedAt.Valid || row.CreatedAt.Time.UnixMilli() != source.CreatedAtMs {
				continue
			}
			source.EventCursor = row.EventCursor
		}
	}
	return hydrated
}

func (r *Resolver) loadLegacyArtifactCoverage(
	ctx context.Context,
	artifact compaction.Artifact,
	owner compaction.ArtifactOwner,
) ([]compaction.CoveredSource, bool, error) {
	compactID, err := db.ParseUUID(artifact.ID)
	if err != nil {
		return nil, false, nil
	}
	rows, err := r.queries.ListMessageRefsByCompactID(ctx, compactID)
	if err != nil {
		return nil, false, fmt.Errorf("load legacy pipeline coverage for %s: %w", artifact.ID, err)
	}
	coverage := make([]compaction.CoveredSource, 0, len(rows))
	for _, row := range rows {
		if !row.CreatedAt.Valid ||
			(owner.BotID != "" && pgUUIDString(row.BotID) != owner.BotID) ||
			!owner.SessionIDKnown || pgUUIDString(row.SessionID) != owner.SessionID {
			return nil, true, nil
		}
		ref, err := historyfrag.DBMessageIdentityRef(pgUUIDString(row.ID))
		if err != nil {
			return nil, true, nil
		}
		coverage = append(coverage, compaction.CoveredSource{
			Ref:                    ref,
			ExternalMessageID:      strings.TrimSpace(row.ExternalMessageID.String),
			SourceReplyToMessageID: strings.TrimSpace(row.SourceReplyToMessageID.String),
			CreatedAtMs:            row.CreatedAt.Time.UnixMilli(),
		})
	}
	return coverage, len(rows) > 0, nil
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
