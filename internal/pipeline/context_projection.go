package pipeline

import (
	"sort"
	"strings"
)

// HistoryPosition is the durable position of a message within session history.
type HistoryPosition struct {
	TurnPosition    int64 `json:"turn_position"`
	MessageSequence int64 `json:"message_sequence"`
}

func (position HistoryPosition) valid() bool {
	return position.TurnPosition > 0 && position.MessageSequence > 0
}

func (position HistoryPosition) before(other HistoryPosition) bool {
	if position.TurnPosition != other.TurnPosition {
		return position.TurnPosition < other.TurnPosition
	}
	return position.MessageSequence < other.MessageSequence
}

type positionedMergeEntry struct {
	entry    mergeEntry
	position HistoryPosition
	tier     int
}

const (
	positionTierArtifactPrefix = iota
	positionTierDurable
	positionTierProvisional
)

// ComposeContextProjection composes rendered and persisted context using
// durable history topology when it is available.
func ComposeContextProjection(rc RenderedContext, projection ContextHistoryProjection) *ComposeContextResult {
	if !projection.hasHistoryPositions() {
		return ComposeContextWithArtifacts(rc, projection.TurnResponses, projection.CompactionArtifacts)
	}

	coveredMessages := coveredExternalMessages(projection.CompactionArtifacts)
	activeRC := filterCoveredRenderedContextWithCoverage(rc, coveredMessages)
	activeTRs := filterCoveredTurnResponses(projection.TurnResponses, projection.CompactionArtifacts)
	entries := make([]positionedMergeEntry, 0, len(activeRC)+len(activeTRs)+len(projection.CompactionArtifacts))
	entries = appendPositionedRenderedContext(entries, activeRC, projection.ExternalMessagePositions)
	entries = appendPositionedArtifacts(entries, rc, projection, coveredMessages)
	entries = appendPositionedTurnResponses(entries, activeTRs)
	sort.SliceStable(entries, func(i, j int) bool {
		return positionedEntryBefore(entries[i], entries[j])
	})

	mergedEntries := make([]mergeEntry, 0, len(entries))
	for _, entry := range entries {
		mergedEntries = append(mergedEntries, entry.entry)
	}
	allMessages := materializeMergeEntries(mergedEntries)
	if len(allMessages) == 0 {
		return nil
	}
	return &ComposeContextResult{
		Messages:        allMessages,
		EstimatedTokens: estimateMessagesTokens(allMessages),
	}
}

func (projection ContextHistoryProjection) hasHistoryPositions() bool {
	if len(projection.HistoryMessagePositions) > 0 || len(projection.ExternalMessagePositions) > 0 {
		return true
	}
	for _, response := range projection.TurnResponses {
		if response.HistoryPosition.valid() {
			return true
		}
	}
	return false
}

func appendPositionedRenderedContext(
	entries []positionedMergeEntry,
	rc RenderedContext,
	positions map[string]HistoryPosition,
) []positionedMergeEntry {
	for _, segment := range rc {
		externalCursor := segment.eventCursor()
		if segment.IsMyself || segment.IsSelfSent {
			externalCursor = 0
		}
		position := positions[strings.TrimSpace(segment.MessageID)]
		tier := positionTierProvisional
		if position.valid() {
			tier = positionTierDurable
		}
		entries = append(entries, positionedMergeEntry{
			entry: mergeEntry{
				kind:                      "rc",
				time:                      segment.conversationAtMs(),
				step:                      segment.orderCursor(),
				rcContent:                 segment.Content,
				rcMessageID:               strings.TrimSpace(segment.MessageID),
				rcExternalCursor:          externalCursor,
				rcCurrentTrigger:          segment.CurrentTrigger,
				rcCurrentTriggerEvaluated: segment.CurrentTriggerEvaluated,
			},
			position: position,
			tier:     tier,
		})
	}
	return entries
}

func appendPositionedTurnResponses(entries []positionedMergeEntry, responses []TurnResponseEntry) []positionedMergeEntry {
	for i, response := range responses {
		tier := positionTierProvisional
		if response.HistoryPosition.valid() {
			tier = positionTierDurable
		}
		entries = append(entries, positionedMergeEntry{
			entry: mergeEntry{
				kind:         "tr",
				time:         response.RequestedAtMs,
				step:         int64(i),
				trRole:       response.Role,
				trContent:    response.Content,
				trRawContent: response.RawContent,
			},
			position: response.HistoryPosition,
			tier:     tier,
		})
	}
	return entries
}

func appendPositionedArtifacts(
	entries []positionedMergeEntry,
	rc RenderedContext,
	projection ContextHistoryProjection,
	coveredMessages map[string]externalMessageCoverage,
) []positionedMergeEntry {
	for i, artifact := range projection.CompactionArtifacts {
		if !artifact.usable() {
			continue
		}
		kind := "summary"
		summaryAtMs, precedesRenderedContext := artifactSummaryPlacement(artifact, rc, coveredMessages)
		if precedesRenderedContext {
			kind = "summary_before_rc"
		}
		position, positioned := artifactHistoryPosition(artifact, projection)
		tier := positionTierArtifactPrefix
		if positioned {
			tier = positionTierDurable
		}
		entries = append(entries, positionedMergeEntry{
			entry: mergeEntry{
				kind:              kind,
				time:              summaryAtMs,
				step:              int64(i),
				summaryContent:    "<summary>\n" + strings.TrimSpace(artifact.Summary) + "\n</summary>",
				summaryArtifactID: artifact.ID,
			},
			position: position,
			tier:     tier,
		})
	}
	return entries
}

func artifactHistoryPosition(artifact CompactionArtifact, projection ContextHistoryProjection) (HistoryPosition, bool) {
	if len(artifact.Sources) == 0 {
		return HistoryPosition{}, false
	}
	earliest := compactionSourceHistoryPosition(artifact.Sources[0], projection)
	if !earliest.valid() {
		return HistoryPosition{}, false
	}
	for _, source := range artifact.Sources[1:] {
		position := compactionSourceHistoryPosition(source, projection)
		if position.valid() && position.before(earliest) {
			earliest = position
		}
	}
	return earliest, earliest.valid()
}

func compactionSourceHistoryPosition(source CompactionSource, projection ContextHistoryProjection) HistoryPosition {
	position := projection.HistoryMessagePositions[strings.TrimSpace(source.HistoryMessageID)]
	if !position.valid() {
		position = projection.ExternalMessagePositions[strings.TrimSpace(source.ExternalMessageID)]
	}
	return position
}

func positionedEntryBefore(left, right positionedMergeEntry) bool {
	if left.tier != right.tier {
		return left.tier < right.tier
	}
	if left.tier == positionTierDurable {
		if left.position != right.position {
			return left.position.before(right.position)
		}
		if left.entry.kind != right.entry.kind {
			return mergeKindOrder(left.entry.kind) < mergeKindOrder(right.entry.kind)
		}
	}
	if left.entry.time != right.entry.time {
		return left.entry.time < right.entry.time
	}
	if left.entry.kind != right.entry.kind {
		return mergeKindOrder(left.entry.kind) < mergeKindOrder(right.entry.kind)
	}
	return left.entry.step < right.entry.step
}
