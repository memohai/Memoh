package compaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type artifactProjectionQueries interface {
	GetCompactionLogByID(context.Context, pgtype.UUID) (sqlc.BotHistoryMessageCompact, error)
	ListCompactionArtifactLineageBySession(context.Context, pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error)
	ListCompactionArtifactLineageMetadataBySession(context.Context, pgtype.UUID) ([]sqlc.ListCompactionArtifactLineageMetadataBySessionRow, error)
	ListCompactionArtifactParentIDsBySuccessor(context.Context, sqlc.ListCompactionArtifactParentIDsBySuccessorParams) ([]pgtype.UUID, error)
	ListCompactionArtifactPayloadsByIDs(context.Context, []pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error)
	ListInvalidCompactionArtifactSeedsBySession(context.Context, sqlc.ListInvalidCompactionArtifactSeedsBySessionParams) ([]sqlc.ListInvalidCompactionArtifactSeedsBySessionRow, error)
}

type ArtifactProjection struct {
	queries artifactProjectionQueries
}

func NewArtifactProjection(queries artifactProjectionQueries) ArtifactProjection {
	return ArtifactProjection{queries: queries}
}

func buildArtifactFrontierExcludingInvalidSources(
	artifacts []Artifact,
	owner ArtifactOwner,
	invalidSources map[string][]CoveredSource,
) ArtifactFrontier {
	if len(invalidSources) == 0 {
		return buildArtifactFrontierForOwner(artifacts, owner)
	}
	nodes := make(map[string]Artifact, len(artifacts))
	successors := make(map[string][]string, len(artifacts))
	invalidCoverage := make(map[string][]CoveredSource)
	for _, sources := range invalidSources {
		for _, source := range sources {
			key := source.Ref.StableKey()
			invalidCoverage[key] = append(invalidCoverage[key], source)
		}
	}
	for _, artifact := range artifacts {
		nodes[artifact.ID] = artifact
		if artifact.SupersededBy != "" {
			successors[artifact.ID] = append(successors[artifact.ID], artifact.SupersededBy)
		}
		for _, parentID := range artifact.ParentIDs {
			successors[parentID] = append(successors[parentID], artifact.ID)
		}
	}
	queue := make([]string, 0, len(invalidSources))
	for id := range invalidSources {
		queue = append(queue, id)
	}
	for _, artifact := range artifacts {
		if artifact.Level > 0 && hasUnattributedInvalidCoverage(artifact, nodes, invalidSources, invalidCoverage) {
			queue = append(queue, artifact.ID)
		}
	}
	invalid := make(map[string]struct{}, len(queue))
	for next := 0; next < len(queue); next++ {
		id := queue[next]
		if _, seen := invalid[id]; seen {
			continue
		}
		invalid[id] = struct{}{}
		queue = append(queue, successors[id]...)
	}
	current := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if _, excluded := invalid[artifact.ID]; excluded {
			continue
		}
		if _, successorExcluded := invalid[artifact.SupersededBy]; successorExcluded {
			artifact.SupersededBy = ""
			artifact.SupersededAt = time.Time{}
		}
		current = append(current, artifact)
	}
	return buildArtifactFrontierForOwner(current, owner)
}

func hasUnattributedInvalidCoverage(
	artifact Artifact,
	nodes map[string]Artifact,
	invalidSources map[string][]CoveredSource,
	invalidCoverage map[string][]CoveredSource,
) bool {
	provided := make(map[string][]CoveredSource)
	for _, parentID := range artifact.ParentIDs {
		if _, invalid := invalidSources[parentID]; invalid {
			continue
		}
		for _, source := range nodes[parentID].Coverage {
			key := source.Ref.StableKey()
			provided[key] = append(provided[key], source)
		}
	}
	for _, source := range artifact.Coverage {
		key := source.Ref.StableKey()
		if coveredSourceMatchesAny(source, invalidCoverage[key]) && !coveredSourceMatchesAny(source, provided[key]) {
			return true
		}
	}
	return false
}

func coveredSourceMatchesAny(source CoveredSource, candidates []CoveredSource) bool {
	for _, candidate := range candidates {
		if compatibleCoveredSource(source, candidate) {
			return true
		}
	}
	return false
}

func (p ArtifactProjection) LoadActiveSession(ctx context.Context, owner ArtifactOwner) (ArtifactFrontier, error) {
	if p.queries == nil {
		return ArtifactFrontier{}, nil
	}
	sessionUUID, err := db.ParseUUID(owner.SessionID)
	if err != nil {
		return ArtifactFrontier{}, err
	}
	metadataRows, err := p.queries.ListCompactionArtifactLineageMetadataBySession(ctx, sessionUUID)
	if err != nil {
		return ArtifactFrontier{}, err
	}
	botID := strings.TrimSpace(owner.BotID)
	if botID == "" && len(metadataRows) > 0 {
		botID = formatUUID(metadataRows[0].BotID)
	}
	invalidSources, err := p.loadInvalidSources(ctx, botID, sessionUUID)
	if err != nil {
		return ArtifactFrontier{}, err
	}
	metadata := make([]artifactLineageMetadata, 0, len(metadataRows))
	expected := make(map[string]artifactLineageMetadata, len(metadataRows))
	lineageValidated := true
	for _, row := range metadataRows {
		lineageValidated = lineageValidated && row.LineageValidated
		artifact, err := artifactMetadataFromDBRow(row)
		if err != nil {
			return ArtifactFrontier{}, err
		}
		metadata = append(metadata, artifact)
		expected[artifact.ID] = artifact
	}
	if !lineageValidated {
		return p.loadActiveSessionWithFullLineage(ctx, owner, invalidSources)
	}
	plan := buildArtifactProjectionPlan(metadata, owner, invalidSources)
	if len(plan.active) == 0 {
		return artifactFrontierFromProjectionPlan(plan, nil, owner), nil
	}
	requested := make(map[string]artifactLineageMetadata, len(plan.active))
	ids := make([]pgtype.UUID, 0, len(plan.active))
	for _, artifact := range plan.active {
		id, err := db.ParseUUID(artifact.ID)
		if err != nil {
			return ArtifactFrontier{}, err
		}
		requested[artifact.ID] = artifact
		ids = append(ids, id)
	}
	payloadRows, err := p.queries.ListCompactionArtifactPayloadsByIDs(ctx, ids)
	if err != nil {
		return ArtifactFrontier{}, err
	}
	payloads := make(map[string]Artifact, len(payloadRows))
	for _, row := range payloadRows {
		artifact, err := artifactFromDBRow(row)
		if err != nil {
			return ArtifactFrontier{}, err
		}
		effective, ok := requested[artifact.ID]
		if !ok {
			return ArtifactFrontier{}, fmt.Errorf("load compaction artifact payloads: unexpected artifact %s", artifact.ID)
		}
		if _, duplicate := payloads[artifact.ID]; duplicate {
			return ArtifactFrontier{}, fmt.Errorf("load compaction artifact payloads: duplicate artifact %s", artifact.ID)
		}
		if !artifactPayloadMatchesMetadata(artifact, expected[artifact.ID]) {
			return ArtifactFrontier{}, fmt.Errorf("load compaction artifact payloads: artifact %s changed after projection", artifact.ID)
		}
		artifact.ParentIDs = nil
		artifact.SupersededBy = effective.SupersededBy
		artifact.SupersededAt = effective.SupersededAt
		payloads[artifact.ID] = artifact
	}
	if len(payloads) != len(requested) {
		return ArtifactFrontier{}, fmt.Errorf("load compaction artifact payloads: got %d artifacts, want %d", len(payloads), len(requested))
	}
	hydrated := make([]Artifact, 0, len(plan.active))
	for _, artifact := range plan.active {
		hydrated = append(hydrated, payloads[artifact.ID])
	}
	return artifactFrontierFromProjectionPlan(plan, hydrated, owner), nil
}

func (p ArtifactProjection) loadActiveSessionWithFullLineage(
	ctx context.Context,
	owner ArtifactOwner,
	invalidSources map[string][]CoveredSource,
) (ArtifactFrontier, error) {
	sessionUUID, err := db.ParseUUID(owner.SessionID)
	if err != nil {
		return ArtifactFrontier{}, err
	}
	rows, err := p.queries.ListCompactionArtifactLineageBySession(ctx, sessionUUID)
	if err != nil {
		return ArtifactFrontier{}, err
	}
	artifacts := make([]Artifact, 0, len(rows))
	for _, row := range rows {
		artifact, err := artifactFromDBRow(row)
		if err != nil {
			return ArtifactFrontier{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	return buildArtifactFrontierExcludingInvalidSources(artifacts, owner, invalidSources), nil
}

func artifactFrontierFromProjectionPlan(
	plan artifactProjectionPlan,
	hydrated []Artifact,
	owner ArtifactOwner,
) ArtifactFrontier {
	frontier := buildArtifactFrontierForOwner(hydrated, owner)
	aliases := make(map[string]string, len(plan.aliases))
	for alias, activeID := range plan.aliases {
		if _, ok := frontier.byID[activeID]; ok {
			aliases[alias] = activeID
		}
	}
	frontier.aliases = aliases
	frontier.Issues = append(plan.issues, frontier.Issues...)
	sortLineageIssues(frontier.Issues)
	return frontier
}

func artifactPayloadMatchesMetadata(artifact Artifact, metadata artifactLineageMetadata) bool {
	return artifact.ID == metadata.ID &&
		artifact.BotID == metadata.BotID &&
		artifact.SessionID == metadata.SessionID &&
		artifact.Status == metadata.Status &&
		(strings.TrimSpace(artifact.Summary) != "") == metadata.HasSummary &&
		(artifact.CoverageMalformed || len(artifact.Coverage) == metadata.CoverageCount) &&
		artifact.AnchorStartMs == metadata.AnchorStartMs &&
		artifact.AnchorEndMs == metadata.AnchorEndMs &&
		artifact.Level == metadata.Level &&
		equalArtifactIDSequence(artifact.ParentIDs, metadata.ParentIDs) &&
		artifact.SupersededBy == metadata.SupersededBy &&
		artifact.SupersededAt.Equal(metadata.SupersededAt) &&
		artifact.StartedAt.Equal(metadata.StartedAt)
}

func equalArtifactIDSequence(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (p ArtifactProjection) LoadActiveByID(ctx context.Context, id string, owner ArtifactOwner) (Artifact, error) {
	if p.queries == nil {
		return Artifact{}, errors.New("compaction artifact projection: queries are required")
	}
	startID := strings.TrimSpace(id)
	if _, err := db.ParseUUID(startID); err != nil {
		return Artifact{}, err
	}
	artifacts, err := p.loadConnectedLineage(ctx, startID)
	if err != nil {
		return Artifact{}, err
	}
	var sessionID pgtype.UUID
	if len(artifacts) > 0 && artifacts[0].SessionID != "" {
		sessionID, err = db.ParseUUID(artifacts[0].SessionID)
		if err != nil {
			return Artifact{}, err
		}
	}
	botID := ""
	if len(artifacts) > 0 {
		botID = artifacts[0].BotID
	}
	invalidSources, err := p.loadInvalidSources(ctx, botID, sessionID)
	if err != nil {
		return Artifact{}, err
	}
	frontier := buildArtifactFrontierExcludingInvalidSources(artifacts, owner, invalidSources)
	if artifact, ok := frontier.Resolve(startID); ok {
		return artifact, nil
	}
	if len(frontier.Issues) > 0 {
		return Artifact{}, &LineageError{Issue: frontier.Issues[0]}
	}
	return Artifact{}, &LineageError{Issue: LineageIssue{Kind: LineageIssueInactiveSuccessor, ArtifactID: startID}}
}

func (p ArtifactProjection) loadInvalidSources(ctx context.Context, botID string, sessionID pgtype.UUID) (map[string][]CoveredSource, error) {
	var botUUID pgtype.UUID
	var err error
	if strings.TrimSpace(botID) != "" {
		botUUID, err = db.ParseUUID(botID)
		if err != nil {
			return nil, err
		}
	}
	seeds, err := p.queries.ListInvalidCompactionArtifactSeedsBySession(ctx, sqlc.ListInvalidCompactionArtifactSeedsBySessionParams{
		BotID:     botUUID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("load invalid compaction artifacts: %w", err)
	}
	invalid := make(map[string][]CoveredSource, len(seeds))
	for _, seed := range seeds {
		if id := formatUUID(seed.ID); id != "" {
			coverage, _ := DecodeArtifactCoverage(seed.Coverage)
			invalid[id] = coverage
		}
	}
	return invalid, nil
}

func (p ArtifactProjection) loadConnectedLineage(ctx context.Context, startID string) ([]Artifact, error) {
	queue := []string{startID}
	requested := make(map[string]struct{})
	artifacts := make([]Artifact, 0, 2)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := requested[id]; seen {
			continue
		}
		requested[id] = struct{}{}
		artifactID, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		row, err := p.queries.GetCompactionLogByID(ctx, artifactID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) && id != startID {
				continue
			}
			return nil, fmt.Errorf("load compaction artifact %s: %w", id, err)
		}
		artifact, err := artifactFromDBRow(row)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
		incoming, err := p.queries.ListCompactionArtifactParentIDsBySuccessor(ctx, sqlc.ListCompactionArtifactParentIDsBySuccessorParams{
			SuccessorID: row.ID,
			BotID:       row.BotID,
			SessionID:   row.SessionID,
		})
		if err != nil {
			return nil, fmt.Errorf("load parents for compaction artifact %s: %w", id, err)
		}
		for _, parentID := range incoming {
			if parent := formatUUID(parentID); parent != "" {
				queue = append(queue, parent)
			}
		}
		if artifact.SupersededBy != "" {
			queue = append(queue, artifact.SupersededBy)
		}
		queue = append(queue, artifact.ParentIDs...)
	}
	return artifacts, nil
}

func artifactFromDBRow(row sqlc.BotHistoryMessageCompact) (Artifact, error) {
	id := formatUUID(row.ID)
	if id == "" {
		return Artifact{}, errors.New("compaction artifact: id is required")
	}
	coverage, coverageErr := DecodeArtifactCoverage(row.Coverage)
	coverageMalformed := coverageErr != nil
	if !coverageMalformed && len(coverage) > 0 {
		coverageMalformed = row.AnchorStartMs != coverage[0].CreatedAtMs ||
			row.AnchorEndMs != coverage[len(coverage)-1].CreatedAtMs
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
		ID:                id,
		BotID:             formatUUID(row.BotID),
		SessionID:         formatUUID(row.SessionID),
		Status:            strings.TrimSpace(row.Status),
		Summary:           row.Summary,
		Version:           version,
		Coverage:          coverage,
		AnchorStartMs:     row.AnchorStartMs,
		AnchorEndMs:       row.AnchorEndMs,
		Level:             int(row.ArtifactLevel),
		ParentIDs:         parentIDs,
		SupersededBy:      formatUUID(row.SupersededBy),
		SupersededAt:      pgTime(row.SupersededAt.Time, row.SupersededAt.Valid),
		StartedAt:         pgTime(row.StartedAt.Time, row.StartedAt.Valid),
		CoverageMalformed: coverageMalformed,
	}, nil
}

func pgTime(value time.Time, valid bool) time.Time {
	if !valid {
		return time.Time{}
	}
	return value.UTC()
}
