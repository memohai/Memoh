package compaction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type artifactLineageMetadata struct {
	ID            string
	BotID         string
	SessionID     string
	Status        string
	HasSummary    bool
	CoverageCount int
	AnchorStartMs int64
	AnchorEndMs   int64
	Level         int
	ParentIDs     []string
	SupersededBy  string
	SupersededAt  time.Time
	StartedAt     time.Time
}

type artifactProjectionPlan struct {
	active  []artifactLineageMetadata
	aliases map[string]string
	issues  []LineageIssue
}

func artifactMetadataFromDBRow(row sqlc.ListCompactionArtifactLineageMetadataBySessionRow) (artifactLineageMetadata, error) {
	id := formatUUID(row.ID)
	if id == "" {
		return artifactLineageMetadata{}, errors.New("compaction artifact metadata: id is required")
	}
	parentIDs := make([]string, 0, len(row.ParentIds))
	for _, parentID := range row.ParentIds {
		if value := formatUUID(parentID); value != "" {
			parentIDs = append(parentIDs, value)
		}
	}
	return artifactLineageMetadata{
		ID:            id,
		BotID:         formatUUID(row.BotID),
		SessionID:     formatUUID(row.SessionID),
		Status:        strings.TrimSpace(row.Status),
		HasSummary:    row.HasSummary,
		CoverageCount: int(row.CoverageCount),
		AnchorStartMs: row.AnchorStartMs,
		AnchorEndMs:   row.AnchorEndMs,
		Level:         int(row.ArtifactLevel),
		ParentIDs:     parentIDs,
		SupersededBy:  formatUUID(row.SupersededBy),
		SupersededAt:  pgTime(row.SupersededAt.Time, row.SupersededAt.Valid),
		StartedAt:     pgTime(row.StartedAt.Time, row.StartedAt.Valid),
	}, nil
}

func buildArtifactProjectionPlan(
	metadata []artifactLineageMetadata,
	owner ArtifactOwner,
	invalidSources map[string][]CoveredSource,
) artifactProjectionPlan {
	current := excludeInvalidArtifactMetadata(metadata, invalidSources)
	nodes := make(map[string]artifactLineageMetadata, len(current))
	for _, artifact := range current {
		nodes[artifact.ID] = artifact
	}
	adjacent := metadataLineageAdjacency(nodes)
	aliases := make(map[string]string)
	active := make(map[string]artifactLineageMetadata)
	issues := make([]LineageIssue, 0)
	seenIssues := make(map[string]struct{})
	invalid := make(map[string]struct{})
	resolver := metadataLineageResolver{
		nodes: nodes,
		memo:  make(map[string]metadataLineageResolution, len(nodes)),
		state: make(map[string]lineageVisitState, len(nodes)),
	}
	for _, artifact := range current {
		if !metadataUsable(artifact) {
			continue
		}
		if artifact.CoverageCount < 0 {
			appendMetadataLineageIssue(
				LineageIssue{Kind: LineageIssueMalformedCoverage, ArtifactID: artifact.ID},
				&issues,
				seenIssues,
			)
			markMetadataConnectedLineage(artifact.ID, adjacent, invalid)
			continue
		}
		if !metadataMatchesOwner(artifact, owner) {
			ownerID := strings.TrimSpace(owner.BotID) + "/" + strings.TrimSpace(owner.SessionID)
			appendMetadataLineageIssue(
				LineageIssue{Kind: LineageIssueScopeMismatch, ArtifactID: artifact.ID, RelatedID: ownerID},
				&issues,
				seenIssues,
			)
			markMetadataConnectedLineage(artifact.ID, adjacent, invalid)
			continue
		}
		resolved := resolver.resolve(artifact.ID)
		if resolved.issue != nil {
			appendMetadataLineageIssue(*resolved.issue, &issues, seenIssues)
			markMetadataConnectedLineage(artifact.ID, adjacent, invalid)
			continue
		}
		active[resolved.terminal.ID] = resolved.terminal
		aliases[artifact.ID] = resolved.terminal.ID
	}
	for id := range invalid {
		delete(active, id)
		delete(aliases, id)
	}
	plan := artifactProjectionPlan{
		active:  make([]artifactLineageMetadata, 0, len(active)),
		aliases: aliases,
		issues:  issues,
	}
	for _, artifact := range active {
		plan.active = append(plan.active, artifact)
	}
	sort.Slice(plan.active, func(i, j int) bool {
		left, right := plan.active[i], plan.active[j]
		if left.AnchorStartMs != right.AnchorStartMs {
			return left.AnchorStartMs < right.AnchorStartMs
		}
		if !left.StartedAt.Equal(right.StartedAt) {
			return left.StartedAt.Before(right.StartedAt)
		}
		return left.ID < right.ID
	})
	sortLineageIssues(plan.issues)
	return plan
}

func excludeInvalidArtifactMetadata(
	metadata []artifactLineageMetadata,
	invalidSources map[string][]CoveredSource,
) []artifactLineageMetadata {
	if len(invalidSources) == 0 {
		return metadata
	}
	successors := make(map[string][]string, len(metadata))
	for _, artifact := range metadata {
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
	invalid := make(map[string]struct{}, len(queue))
	for next := 0; next < len(queue); next++ {
		id := queue[next]
		if _, seen := invalid[id]; seen {
			continue
		}
		invalid[id] = struct{}{}
		queue = append(queue, successors[id]...)
	}
	current := make([]artifactLineageMetadata, 0, len(metadata))
	for _, artifact := range metadata {
		if _, excluded := invalid[artifact.ID]; excluded {
			continue
		}
		if _, successorExcluded := invalid[artifact.SupersededBy]; successorExcluded {
			artifact.SupersededBy = ""
			artifact.SupersededAt = time.Time{}
		}
		current = append(current, artifact)
	}
	return current
}

type metadataLineageResolution struct {
	terminal artifactLineageMetadata
	issue    *LineageIssue
}

type metadataLineageResolver struct {
	nodes map[string]artifactLineageMetadata
	memo  map[string]metadataLineageResolution
	state map[string]lineageVisitState
}

func (r *metadataLineageResolver) resolve(id string) metadataLineageResolution {
	if resolved, ok := r.memo[id]; ok {
		return resolved
	}
	if r.state[id] == lineageVisiting {
		return metadataLineageResolution{issue: issue(LineageIssueCycle, id, id)}
	}
	r.state[id] = lineageVisiting
	current, ok := r.nodes[id]
	if !ok {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueMissingSuccessor, id, id)})
	}
	if !metadataUsable(current) {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueInactiveSuccessor, id, id)})
	}
	if current.CoverageCount < 0 {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueMalformedCoverage, current.ID, "")})
	}
	if markerIssue, invalid := validateMetadataSupersessionMarker(current); invalid {
		return r.finish(id, metadataLineageResolution{issue: &markerIssue})
	}
	if metadataLineageCoverageMissing(current) {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueMissingDerivedCoverage, current.ID, "")})
	}
	for _, parentID := range current.ParentIDs {
		parent, exists := r.nodes[parentID]
		if !exists || !metadataUsable(parent) || parent.SupersededBy != current.ID {
			return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueParentMismatch, current.ID, parentID)})
		}
		if parent.CoverageCount < 0 {
			return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueMalformedCoverage, parent.ID, "")})
		}
		if markerIssue, invalid := validateMetadataSupersessionMarker(parent); invalid {
			return r.finish(id, metadataLineageResolution{issue: &markerIssue})
		}
		if !sameMetadataScope(parent, current.BotID, current.SessionID) {
			return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueScopeMismatch, current.ID, parentID)})
		}
		if metadataLineageCoverageMissing(parent) {
			return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueMissingDerivedCoverage, parent.ID, "")})
		}
	}
	if current.SupersededBy == "" {
		return r.finish(id, metadataLineageResolution{terminal: current})
	}
	next, exists := r.nodes[current.SupersededBy]
	if !exists {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueMissingSuccessor, current.ID, current.SupersededBy)})
	}
	if !metadataUsable(next) {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueInactiveSuccessor, current.ID, next.ID)})
	}
	if !sameMetadataScope(next, current.BotID, current.SessionID) {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueScopeMismatch, current.ID, next.ID)})
	}
	if !containsID(next.ParentIDs, current.ID) {
		return r.finish(id, metadataLineageResolution{issue: issue(LineageIssueParentMismatch, current.ID, next.ID)})
	}
	return r.finish(id, r.resolve(next.ID))
}

func (r *metadataLineageResolver) finish(id string, resolved metadataLineageResolution) metadataLineageResolution {
	r.state[id] = lineageVisited
	r.memo[id] = resolved
	return resolved
}

func metadataUsable(artifact artifactLineageMetadata) bool {
	return artifact.Status == "ok" && artifact.HasSummary
}

func validateMetadataSupersessionMarker(artifact artifactLineageMetadata) (LineageIssue, bool) {
	if (artifact.SupersededBy == "") != artifact.SupersededAt.IsZero() {
		return LineageIssue{Kind: LineageIssueInconsistentMarker, ArtifactID: artifact.ID, RelatedID: artifact.SupersededBy}, true
	}
	return LineageIssue{}, false
}

func metadataLineageCoverageMissing(artifact artifactLineageMetadata) bool {
	participatesInLineage := len(artifact.ParentIDs) > 0 || artifact.SupersededBy != ""
	return participatesInLineage && artifact.CoverageCount <= 0
}

func metadataMatchesOwner(artifact artifactLineageMetadata, owner ArtifactOwner) bool {
	botID := strings.TrimSpace(owner.BotID)
	if botID != "" && artifact.BotID != botID {
		return false
	}
	sessionID := strings.TrimSpace(owner.SessionID)
	return (!owner.SessionIDKnown && sessionID == "") || artifact.SessionID == sessionID
}

func sameMetadataScope(artifact artifactLineageMetadata, botID, sessionID string) bool {
	return artifact.BotID == botID && artifact.SessionID == sessionID
}

func metadataLineageAdjacency(nodes map[string]artifactLineageMetadata) map[string][]string {
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

func markMetadataConnectedLineage(startID string, adjacent map[string][]string, marked map[string]struct{}) {
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

func appendMetadataLineageIssue(issue LineageIssue, issues *[]LineageIssue, seen map[string]struct{}) {
	key := fmt.Sprintf("%s:%s:%s", issue.Kind, issue.ArtifactID, issue.RelatedID)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*issues = append(*issues, issue)
}

func sortLineageIssues(issues []LineageIssue) {
	sort.Slice(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		if left.ArtifactID != right.ArtifactID {
			return left.ArtifactID < right.ArtifactID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.RelatedID < right.RelatedID
	})
}
