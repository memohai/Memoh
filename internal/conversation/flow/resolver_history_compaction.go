package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/historyfrag"
)

func historyContextFragsForMessages(messages []conversation.ModelMessage, records []historyfrag.HistoryRecord) []contextfrag.ContextFrag {
	if len(messages) == 0 || len(records) == 0 {
		return nil
	}
	frags := make([]contextfrag.ContextFrag, 0)
	recordStart := 0
	for i, msg := range messages {
		if !looksLikeSummaryMessage(msg) {
			continue
		}
		for j := recordStart; j < len(records); j++ {
			record := records[j]
			if record.Kind != contextfrag.KindConversationSummary || record.Coverage == nil {
				continue
			}
			if !sameModelMessage(record.ModelMessage, msg) {
				continue
			}
			frag := historyfrag.ToFrag(record)
			frag.ID = fmt.Sprintf("message.%03d", i)
			frag.Provenance.Index = i
			frags = append(frags, frag)
			recordStart = j + 1
			break
		}
	}
	return frags
}

func looksLikeSummaryMessage(msg conversation.ModelMessage) bool {
	return strings.EqualFold(strings.TrimSpace(msg.Role), "user") &&
		strings.HasPrefix(strings.TrimSpace(msg.TextContent()), "<summary>")
}

func sameModelMessage(a conversation.ModelMessage, b conversation.ModelMessage) bool {
	return strings.EqualFold(strings.TrimSpace(a.Role), strings.TrimSpace(b.Role)) &&
		string(a.Content) == string(b.Content)
}

func (r *Resolver) replaceCompactedMessages(ctx context.Context, sessionID string, scope contextfrag.Scope, messages []historyfrag.HistoryRecord) ([]historyfrag.HistoryRecord, error) {
	if r.queries == nil {
		return messages, nil
	}
	if strings.TrimSpace(sessionID) == "" {
		// Sessionless (chat-scoped) loads have no session log list to draw from;
		// resolve each in-window compact group individually.
		return r.replaceRecentCompactedMessages(ctx, scope, messages)
	}
	frontier, err := r.loadActiveCompactionFrontier(ctx, scope.BotID, sessionID)
	if err != nil {
		return nil, err
	}
	if len(frontier.Artifacts) == 0 {
		return messages, nil
	}
	owner := compaction.ArtifactOwner{BotID: scope.BotID, SessionID: sessionID, SessionIDKnown: true}
	catalog := compaction.NewArtifactCatalog()
	catalog.Add(owner, frontier)
	resolve := func(record historyfrag.HistoryRecord) (compaction.Artifact, bool) {
		return resolveCatalogArtifact(catalog, recordArtifactOwner(record, scope), record)
	}
	messages = replaceCompactedHistoryRecordsWithResolver(messages, scope, resolve)
	sessionSummaries := summaryRecordsFromArtifacts(missingCompactionArtifacts(messages, frontier.Artifacts), scope)
	if len(sessionSummaries) > 0 {
		messages = prependMissingCompactionSummaries(messages, sessionSummaries)
	}
	return r.refreshCompactedSummaryCoverage(ctx, messages, resolve), nil
}

// missingCompactionArtifacts filters the active frontier down to summaries not
// already represented in the loaded records.
func missingCompactionArtifacts(messages []historyfrag.HistoryRecord, artifacts []compaction.Artifact) []compaction.Artifact {
	seen := make(map[string]struct{}, len(messages))
	for _, record := range messages {
		if record.SourceKind == historyfrag.SourceCompactionLog {
			if id := strings.TrimSpace(record.Ref.ID); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	missing := make([]compaction.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if _, ok := seen[artifact.ID]; ok {
			continue
		}
		missing = append(missing, artifact)
	}
	return missing
}

func (r *Resolver) replaceRecentCompactedMessages(ctx context.Context, scope contextfrag.Scope, messages []historyfrag.HistoryRecord) ([]historyfrag.HistoryRecord, error) {
	if r.queries == nil {
		return messages, nil
	}

	compactGroups := make(map[string][]int) // compact_id -> indices
	for i, m := range messages {
		if m.CompactID != "" {
			compactGroups[m.CompactID] = append(compactGroups[m.CompactID], i)
		}
	}
	catalog := compaction.NewArtifactCatalog()
	loadedFrontiers := make(map[compaction.ArtifactOwner]struct{})
	projection := compaction.NewArtifactProjection(r.queries)
	for _, record := range messages {
		owner := recordArtifactOwner(record, scope)
		if owner.SessionID == "" {
			continue
		}
		if _, loaded := loadedFrontiers[owner]; loaded {
			continue
		}
		frontier, err := r.loadActiveCompactionFrontier(ctx, owner.BotID, owner.SessionID)
		if err != nil {
			return nil, err
		}
		catalog.Add(owner, frontier)
		loadedFrontiers[owner] = struct{}{}
	}
	for compactID := range compactGroups {
		owner, consistent := compactGroupOwner(messages, compactGroups[compactID], scope)
		if !consistent {
			if r.logger != nil {
				r.logger.Warn("replaceCompactedMessages: compact group spans owners", slog.String("compact_id", compactID))
			}
			continue
		}
		if _, loaded := loadedFrontiers[owner]; loaded {
			continue
		}
		artifact, err := projection.LoadActiveByID(ctx, compactID, owner)
		if err != nil {
			var lineageErr *compaction.LineageError
			if !errors.As(err, &lineageErr) {
				return nil, err
			}
			if r.logger != nil {
				r.logger.Warn("replaceCompactedMessages: failed to load compaction artifact", slog.String("compact_id", compactID), slog.Any("error", err))
			}
			continue
		}
		merged := catalog.Add(owner, compaction.NewArtifactAliasFrontier(compactID, artifact))
		for _, issue := range merged.Issues {
			if r.logger != nil && (issue.Kind == compaction.LineageIssueCoverageOverlap || issue.Kind == compaction.LineageIssueAliasConflict) {
				r.logger.Warn("replaceCompactedMessages: owner frontier conflict", slog.String("issue", issue.Error()))
			}
		}
	}
	resolve := func(record historyfrag.HistoryRecord) (compaction.Artifact, bool) {
		return resolveCatalogArtifact(catalog, recordArtifactOwner(record, scope), record)
	}

	replaced := replaceCompactedHistoryRecordsWithResolver(messages, scope, resolve)
	return r.refreshCompactedSummaryCoverage(ctx, replaced, resolve), nil
}

func resolveCatalogArtifact(catalog *compaction.ArtifactCatalog, owner compaction.ArtifactOwner, record historyfrag.HistoryRecord) (compaction.Artifact, bool) {
	if compactID := strings.TrimSpace(record.CompactID); compactID != "" {
		if artifact, ok := catalog.Resolve(owner, compactID); ok {
			isSummary := record.SourceKind == historyfrag.SourceCompactionLog && record.Kind == contextfrag.KindConversationSummary
			if isSummary || len(artifact.Coverage) == 0 || artifact.Covers(record.Ref) {
				return artifact, true
			}
		}
	}
	return catalog.ResolveCoveredRef(owner, record.Ref)
}

func (r *Resolver) loadActiveCompactionFrontier(ctx context.Context, botID, sessionID string) (compaction.ArtifactFrontier, error) {
	frontier, err := compaction.NewArtifactProjection(r.queries).LoadActiveSession(ctx, compaction.ArtifactOwner{BotID: botID, SessionID: sessionID, SessionIDKnown: true})
	if err != nil {
		return compaction.ArtifactFrontier{}, err
	}
	for _, issue := range frontier.Issues {
		if r.logger != nil {
			r.logger.Warn("loadActiveCompactionFrontier: ignored invalid lineage", slog.String("issue", issue.Error()))
		}
	}
	for _, artifact := range frontier.Artifacts {
		if artifact.CoverageMalformed && r.logger != nil {
			r.logger.Warn("loadActiveCompactionFrontier: malformed coverage requires legacy backfill", slog.String("compact_id", artifact.ID))
		}
	}
	return frontier, nil
}

func recordArtifactOwner(record historyfrag.HistoryRecord, fallback contextfrag.Scope) compaction.ArtifactOwner {
	sessionID := strings.TrimSpace(record.SessionID)
	sessionIDKnown := record.SessionIDKnown || sessionID != ""
	if !sessionIDKnown {
		sessionID = strings.TrimSpace(fallback.SessionID)
		sessionIDKnown = sessionID != ""
	}
	return compaction.ArtifactOwner{
		BotID:          firstNonEmpty(strings.TrimSpace(record.BotID), strings.TrimSpace(fallback.BotID)),
		SessionID:      sessionID,
		SessionIDKnown: sessionIDKnown,
	}
}

func compactGroupOwner(messages []historyfrag.HistoryRecord, indices []int, fallback contextfrag.Scope) (compaction.ArtifactOwner, bool) {
	owner := compaction.ArtifactOwner{
		BotID:          strings.TrimSpace(fallback.BotID),
		SessionID:      strings.TrimSpace(fallback.SessionID),
		SessionIDKnown: strings.TrimSpace(fallback.SessionID) != "",
	}
	for _, index := range indices {
		recordOwner := recordArtifactOwner(messages[index], contextfrag.Scope{})
		if recordOwner.BotID != "" {
			if owner.BotID != "" && owner.BotID != recordOwner.BotID {
				return compaction.ArtifactOwner{}, false
			}
			owner.BotID = recordOwner.BotID
		}
		if recordOwner.SessionIDKnown {
			if owner.SessionIDKnown && owner.SessionID != recordOwner.SessionID {
				return compaction.ArtifactOwner{}, false
			}
			owner.SessionID = recordOwner.SessionID
			owner.SessionIDKnown = true
		}
	}
	return owner, true
}

func summaryRecordsFromArtifacts(artifacts []compaction.Artifact, scope contextfrag.Scope) []historyfrag.HistoryRecord {
	if len(artifacts) == 0 {
		return nil
	}
	records := make([]historyfrag.HistoryRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		records = append(records, artifact.HistoryRecord(scope))
	}
	return records
}

// refreshCompactedSummaryCoverage backfills legacy summaries that predate
// persisted artifact coverage. New artifacts already carry immutable refs and
// avoid this per-summary query.
func (r *Resolver) refreshCompactedSummaryCoverage(ctx context.Context, messages []historyfrag.HistoryRecord, resolve func(historyfrag.HistoryRecord) (compaction.Artifact, bool)) []historyfrag.HistoryRecord {
	if r.queries == nil {
		return messages
	}
	for i := range messages {
		record := &messages[i]
		if record.SourceKind != historyfrag.SourceCompactionLog || record.Kind != contextfrag.KindConversationSummary {
			continue
		}
		if artifact, ok := resolve(*record); ok && len(artifact.Coverage) > 0 {
			continue
		}
		compactID, err := db.ParseUUID(record.CompactID)
		if err != nil {
			continue
		}
		coverage := contextfrag.NewSummaryCoverage(record.Ref, r.coveredRefsForCompact(ctx, compactID))
		record.Coverage = &coverage
	}
	return messages
}

func (r *Resolver) coveredRefsForCompact(ctx context.Context, compactID pgtype.UUID) []contextfrag.ContextRef {
	if r.queries == nil || !compactID.Valid {
		return nil
	}
	rows, err := r.queries.ListMessageRefsByCompactID(ctx, compactID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("coveredRefsForCompact: failed to load compacted message refs", slog.String("compact_id", pgUUIDString(compactID)), slog.Any("error", err))
		}
		return nil
	}
	refs := make([]contextfrag.ContextRef, 0, len(rows))
	for _, row := range rows {
		ref, err := historyfrag.DBMessageIdentityRef(pgUUIDString(row.ID))
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("coveredRefsForCompact: skipped compacted message ref", slog.String("compact_id", pgUUIDString(compactID)), slog.Any("error", err))
			}
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

func prependMissingCompactionSummaries(messages []historyfrag.HistoryRecord, summaries []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if len(summaries) == 0 {
		return messages
	}
	seen := make(map[string]struct{}, len(messages))
	for _, record := range messages {
		if id := strings.TrimSpace(record.CompactID); id != "" {
			seen[id] = struct{}{}
		}
		if record.SourceKind == historyfrag.SourceCompactionLog {
			if id := strings.TrimSpace(record.Ref.ID); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	missing := make([]historyfrag.HistoryRecord, 0, len(summaries))
	for _, summary := range summaries {
		id := strings.TrimSpace(summary.CompactID)
		if id == "" {
			id = strings.TrimSpace(summary.Ref.ID)
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		missing = append(missing, summary)
	}
	if len(missing) == 0 {
		return messages
	}
	out := make([]historyfrag.HistoryRecord, 0, len(missing)+len(messages))
	out = append(out, missing...)
	out = append(out, messages...)
	return out
}

func pgUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}

func replaceCompactedHistoryRecords(messages []historyfrag.HistoryRecord, summaries map[string]string, scope contextfrag.Scope) []historyfrag.HistoryRecord {
	artifacts := make(map[string]compaction.Artifact, len(summaries))
	for compactID, summary := range summaries {
		artifacts[compactID] = compaction.Artifact{ID: compactID, Summary: summary}
	}
	return replaceCompactedHistoryRecordsWithArtifacts(messages, artifacts, scope)
}

func replaceCompactedHistoryRecordsWithArtifacts(messages []historyfrag.HistoryRecord, artifacts map[string]compaction.Artifact, scope contextfrag.Scope) []historyfrag.HistoryRecord {
	return replaceCompactedHistoryRecordsWithResolver(messages, scope, func(record historyfrag.HistoryRecord) (compaction.Artifact, bool) {
		artifact, ok := artifacts[strings.TrimSpace(record.CompactID)]
		return artifact, ok
	})
}

func replaceCompactedHistoryRecordsWithResolver(
	messages []historyfrag.HistoryRecord,
	scope contextfrag.Scope,
	resolve func(historyfrag.HistoryRecord) (compaction.Artifact, bool),
) []historyfrag.HistoryRecord {
	type sourceGroupKey struct {
		compactID string
		index     int
	}
	type recordAssignment struct {
		artifactID string
		source     sourceGroupKey
	}
	type artifactGroup struct {
		artifact compaction.Artifact
		indices  []int
	}
	sourceGroupFor := func(record historyfrag.HistoryRecord, index int) sourceGroupKey {
		if compactID := strings.TrimSpace(record.CompactID); compactID != "" {
			return sourceGroupKey{compactID: compactID, index: -1}
		}
		return sourceGroupKey{index: index}
	}
	requiredGroups := make(map[sourceGroupKey]struct{})
	for i, record := range messages {
		if record.Required {
			requiredGroups[sourceGroupFor(record, i)] = struct{}{}
		}
	}

	assignments := make([]recordAssignment, len(messages))
	groups := make(map[string]*artifactGroup)
	for i, record := range messages {
		artifact, ok := resolve(record)
		if !ok || artifact.ID == "" || strings.TrimSpace(artifact.Summary) == "" {
			continue
		}
		assignments[i] = recordAssignment{artifactID: artifact.ID, source: sourceGroupFor(record, i)}
		group := groups[artifact.ID]
		if group == nil {
			group = &artifactGroup{artifact: artifact}
			groups[artifact.ID] = group
		}
		group.indices = append(group.indices, i)
	}
	if len(groups) == 0 {
		return messages
	}

	result := make([]historyfrag.HistoryRecord, 0, len(messages))
	emitted := make(map[string]struct{}, len(groups))
	for i, record := range messages {
		assignment := assignments[i]
		artifactID := assignment.artifactID
		if artifactID == "" {
			result = append(result, record)
			continue
		}
		group := groups[artifactID]
		if _, seen := emitted[artifactID]; !seen {
			emitted[artifactID] = struct{}{}
			artifact := group.artifact
			if len(artifact.Coverage) == 0 {
				artifact.Coverage = make([]compaction.CoveredSource, 0, len(group.indices))
				for _, index := range group.indices {
					artifact.Coverage = append(artifact.Coverage, compaction.CoveredSource{Ref: messages[index].Ref})
				}
			}
			result = append(result, artifact.HistoryRecord(scope))
		}
		if _, required := requiredGroups[assignment.source]; required {
			result = append(result, record)
		}
	}
	return result
}
