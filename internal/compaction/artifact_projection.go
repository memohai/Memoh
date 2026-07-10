package compaction

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type LineageIssueKind string

const (
	LineageIssueCycle                  LineageIssueKind = "cycle"
	LineageIssueMissingSuccessor       LineageIssueKind = "missing_successor"
	LineageIssueInactiveSuccessor      LineageIssueKind = "inactive_successor"
	LineageIssueInconsistentMarker     LineageIssueKind = "inconsistent_supersession_marker"
	LineageIssueParentMismatch         LineageIssueKind = "parent_mismatch"
	LineageIssueScopeMismatch          LineageIssueKind = "scope_mismatch"
	LineageIssueMissingDerivedCoverage LineageIssueKind = "missing_derived_coverage"
)

type LineageIssue struct {
	Kind       LineageIssueKind
	ArtifactID string
	RelatedID  string
}

func (i LineageIssue) Error() string {
	if i.RelatedID == "" {
		return fmt.Sprintf("compaction artifact %s: lineage %s", i.ArtifactID, i.Kind)
	}
	return fmt.Sprintf("compaction artifact %s: lineage %s (%s)", i.ArtifactID, i.Kind, i.RelatedID)
}

type LineageError struct {
	Issue LineageIssue
}

func (e *LineageError) Error() string {
	return e.Issue.Error()
}

type ArtifactFrontier struct {
	Artifacts []Artifact
	Issues    []LineageIssue
	aliases   map[string]string
	byID      map[string]Artifact
}

func (f ArtifactFrontier) Resolve(lineageID string) (Artifact, bool) {
	activeID, ok := f.aliases[strings.TrimSpace(lineageID)]
	if !ok {
		return Artifact{}, false
	}
	artifact, ok := f.byID[activeID]
	return artifact, ok
}

type ArtifactProjection struct {
	queries dbstore.Queries
}

func NewArtifactProjection(queries dbstore.Queries) ArtifactProjection {
	return ArtifactProjection{queries: queries}
}

func (p ArtifactProjection) LoadActiveSession(ctx context.Context, sessionID string) (ArtifactFrontier, error) {
	if p.queries == nil {
		return ArtifactFrontier{}, nil
	}
	sessionUUID, err := db.ParseUUID(sessionID)
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
	return buildArtifactFrontier(artifacts), nil
}

func (p ArtifactProjection) LoadActiveByID(ctx context.Context, id string) (Artifact, error) {
	if p.queries == nil {
		return Artifact{}, errors.New("compaction artifact projection: queries are required")
	}
	startID := strings.TrimSpace(id)
	if _, err := db.ParseUUID(startID); err != nil {
		return Artifact{}, err
	}
	cache := make(map[string]Artifact)
	load := func(id string) (Artifact, error) {
		if artifact, ok := cache[id]; ok {
			return artifact, nil
		}
		artifactID, err := db.ParseUUID(id)
		if err != nil {
			return Artifact{}, err
		}
		row, err := p.queries.GetCompactionLogByID(ctx, artifactID)
		if err != nil {
			return Artifact{}, err
		}
		artifact, err := artifactFromDBRow(row)
		if err != nil {
			return Artifact{}, err
		}
		cache[id] = artifact
		return artifact, nil
	}

	current, err := load(startID)
	if err != nil {
		return Artifact{}, err
	}
	botID, sessionID := current.BotID, current.SessionID
	visited := make(map[string]struct{})
	for {
		if _, ok := visited[current.ID]; ok {
			return Artifact{}, lineageError(LineageIssueCycle, startID, current.ID)
		}
		visited[current.ID] = struct{}{}
		if !artifactUsable(current) {
			return Artifact{}, lineageError(LineageIssueInactiveSuccessor, startID, current.ID)
		}
		if issue, ok := validateSupersessionMarker(current); ok {
			return Artifact{}, &LineageError{Issue: issue}
		}
		if !sameArtifactScope(current, botID, sessionID) {
			return Artifact{}, lineageError(LineageIssueScopeMismatch, startID, current.ID)
		}
		for _, parentID := range current.ParentIDs {
			parent, err := load(parentID)
			if err != nil || !artifactUsable(parent) || parent.SupersededBy != current.ID {
				return Artifact{}, lineageError(LineageIssueParentMismatch, current.ID, parentID)
			}
			if markerIssue, invalid := validateSupersessionMarker(parent); invalid {
				return Artifact{}, &LineageError{Issue: markerIssue}
			}
			if !sameArtifactScope(parent, botID, sessionID) {
				return Artifact{}, lineageError(LineageIssueScopeMismatch, current.ID, parentID)
			}
		}
		if current.SupersededBy == "" {
			if derivedCoverageMissing(current) {
				return Artifact{}, lineageError(LineageIssueMissingDerivedCoverage, current.ID, "")
			}
			return current, nil
		}
		next, err := load(current.SupersededBy)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Artifact{}, lineageError(LineageIssueMissingSuccessor, current.ID, current.SupersededBy)
			}
			return Artifact{}, fmt.Errorf("load compaction successor %s: %w", current.SupersededBy, err)
		}
		if !artifactUsable(next) {
			return Artifact{}, lineageError(LineageIssueInactiveSuccessor, current.ID, next.ID)
		}
		if !sameArtifactScope(next, botID, sessionID) {
			return Artifact{}, lineageError(LineageIssueScopeMismatch, current.ID, next.ID)
		}
		if !containsID(next.ParentIDs, current.ID) {
			return Artifact{}, lineageError(LineageIssueParentMismatch, current.ID, next.ID)
		}
		current = next
	}
}

func buildArtifactFrontier(artifacts []Artifact) ArtifactFrontier {
	nodes := make(map[string]Artifact, len(artifacts))
	for _, artifact := range artifacts {
		nodes[artifact.ID] = artifact
	}
	adjacent := lineageAdjacency(nodes)
	aliases := make(map[string]string)
	active := make(map[string]Artifact)
	issues := make([]LineageIssue, 0)
	seenIssues := make(map[string]struct{})
	invalid := make(map[string]struct{})
	resolver := loadedLineageResolver{
		nodes: nodes,
		memo:  make(map[string]loadedLineageResolution, len(nodes)),
		state: make(map[string]lineageVisitState, len(nodes)),
	}
	for _, artifact := range artifacts {
		if !artifactUsable(artifact) {
			continue
		}
		resolved := resolver.resolve(artifact.ID)
		if resolved.issue != nil {
			key := fmt.Sprintf("%s:%s:%s", resolved.issue.Kind, resolved.issue.ArtifactID, resolved.issue.RelatedID)
			if _, seen := seenIssues[key]; !seen {
				seenIssues[key] = struct{}{}
				issues = append(issues, *resolved.issue)
			}
			markConnectedLineage(artifact.ID, adjacent, invalid)
			continue
		}
		active[resolved.terminal.ID] = resolved.terminal
		aliases[artifact.ID] = resolved.terminal.ID
	}
	for id := range invalid {
		delete(active, id)
		delete(aliases, id)
	}

	frontier := ArtifactFrontier{
		Artifacts: make([]Artifact, 0, len(active)),
		Issues:    issues,
		aliases:   aliases,
		byID:      active,
	}
	for _, artifact := range active {
		frontier.Artifacts = append(frontier.Artifacts, artifact)
	}
	sort.Slice(frontier.Artifacts, func(i, j int) bool {
		left, right := frontier.Artifacts[i], frontier.Artifacts[j]
		if left.AnchorStartMs != right.AnchorStartMs {
			return left.AnchorStartMs < right.AnchorStartMs
		}
		if !left.StartedAt.Equal(right.StartedAt) {
			return left.StartedAt.Before(right.StartedAt)
		}
		return left.ID < right.ID
	})
	sort.Slice(frontier.Issues, func(i, j int) bool {
		left, right := frontier.Issues[i], frontier.Issues[j]
		if left.ArtifactID != right.ArtifactID {
			return left.ArtifactID < right.ArtifactID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.RelatedID < right.RelatedID
	})
	return frontier
}

func lineageAdjacency(nodes map[string]Artifact) map[string][]string {
	adjacent := make(map[string][]string, len(nodes))
	connect := func(left, right string) {
		if _, ok := nodes[left]; !ok {
			return
		}
		if _, ok := nodes[right]; !ok {
			return
		}
		adjacent[left] = append(adjacent[left], right)
		adjacent[right] = append(adjacent[right], left)
	}
	for _, artifact := range nodes {
		if artifact.SupersededBy != "" {
			connect(artifact.ID, artifact.SupersededBy)
		}
		for _, parentID := range artifact.ParentIDs {
			connect(artifact.ID, parentID)
		}
	}
	return adjacent
}

func markConnectedLineage(startID string, adjacent map[string][]string, marked map[string]struct{}) {
	queue := []string{startID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := marked[id]; seen {
			continue
		}
		marked[id] = struct{}{}
		queue = append(queue, adjacent[id]...)
	}
}

type lineageVisitState uint8

const (
	lineageVisiting lineageVisitState = iota + 1
	lineageVisited
)

type loadedLineageResolution struct {
	terminal Artifact
	issue    *LineageIssue
}

type loadedLineageResolver struct {
	nodes map[string]Artifact
	memo  map[string]loadedLineageResolution
	state map[string]lineageVisitState
}

func (r *loadedLineageResolver) resolve(id string) loadedLineageResolution {
	if resolved, ok := r.memo[id]; ok {
		return resolved
	}
	if r.state[id] == lineageVisiting {
		return loadedLineageResolution{issue: issue(LineageIssueCycle, id, id)}
	}
	r.state[id] = lineageVisiting
	current, ok := r.nodes[id]
	if !ok {
		return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueMissingSuccessor, id, id)})
	}
	if !artifactUsable(current) {
		return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueInactiveSuccessor, id, id)})
	}
	if markerIssue, invalid := validateSupersessionMarker(current); invalid {
		return r.finish(id, loadedLineageResolution{issue: &markerIssue})
	}
	for _, parentID := range current.ParentIDs {
		parent, exists := r.nodes[parentID]
		if !exists || !artifactUsable(parent) || parent.SupersededBy != current.ID {
			return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueParentMismatch, current.ID, parentID)})
		}
		if markerIssue, invalid := validateSupersessionMarker(parent); invalid {
			return r.finish(id, loadedLineageResolution{issue: &markerIssue})
		}
		if !sameArtifactScope(parent, current.BotID, current.SessionID) {
			return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueScopeMismatch, current.ID, parentID)})
		}
	}
	if current.SupersededBy == "" {
		if derivedCoverageMissing(current) {
			return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueMissingDerivedCoverage, current.ID, "")})
		}
		return r.finish(id, loadedLineageResolution{terminal: current})
	}
	next, exists := r.nodes[current.SupersededBy]
	if !exists {
		return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueMissingSuccessor, current.ID, current.SupersededBy)})
	}
	if !artifactUsable(next) {
		return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueInactiveSuccessor, current.ID, next.ID)})
	}
	if !sameArtifactScope(next, current.BotID, current.SessionID) {
		return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueScopeMismatch, current.ID, next.ID)})
	}
	if !containsID(next.ParentIDs, current.ID) {
		return r.finish(id, loadedLineageResolution{issue: issue(LineageIssueParentMismatch, current.ID, next.ID)})
	}
	return r.finish(id, r.resolve(next.ID))
}

func (r *loadedLineageResolver) finish(id string, resolved loadedLineageResolution) loadedLineageResolution {
	r.state[id] = lineageVisited
	r.memo[id] = resolved
	return resolved
}

func artifactFromDBRow(row sqlc.BotHistoryMessageCompact) (Artifact, error) {
	id := formatUUID(row.ID)
	if id == "" {
		return Artifact{}, errors.New("compaction artifact: id is required")
	}
	coverage, coverageErr := DecodeArtifactCoverage(row.Coverage)
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
		CoverageMalformed: coverageErr != nil,
	}, nil
}

func artifactUsable(artifact Artifact) bool {
	return artifact.Status == "ok" && strings.TrimSpace(artifact.Summary) != ""
}

func validateSupersessionMarker(artifact Artifact) (LineageIssue, bool) {
	if (artifact.SupersededBy == "") != artifact.SupersededAt.IsZero() {
		return LineageIssue{Kind: LineageIssueInconsistentMarker, ArtifactID: artifact.ID, RelatedID: artifact.SupersededBy}, true
	}
	return LineageIssue{}, false
}

func derivedCoverageMissing(artifact Artifact) bool {
	return len(artifact.ParentIDs) > 0 && (artifact.CoverageMalformed || len(artifact.Coverage) == 0)
}

func sameArtifactScope(artifact Artifact, botID, sessionID string) bool {
	return (botID == "" || artifact.BotID == "" || artifact.BotID == botID) &&
		(sessionID == "" || artifact.SessionID == "" || artifact.SessionID == sessionID)
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func lineageError(kind LineageIssueKind, artifactID, relatedID string) error {
	return &LineageError{Issue: LineageIssue{Kind: kind, ArtifactID: artifactID, RelatedID: relatedID}}
}

func issue(kind LineageIssueKind, artifactID, relatedID string) *LineageIssue {
	return &LineageIssue{Kind: kind, ArtifactID: artifactID, RelatedID: relatedID}
}

func pgTime(value time.Time, valid bool) time.Time {
	if !valid {
		return time.Time{}
	}
	return value.UTC()
}
