package pipeline

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/messageconv"
)

// TurnResponseEntry represents an assistant or tool message from bot_history_messages,
// used as the "TR" stream in context composition.
type TurnResponseEntry struct {
	RequestedAtMs   int64           `json:"requested_at_ms"`
	Role            string          `json:"role"`
	Content         string          `json:"content"`
	RawContent      json.RawMessage `json:"raw_content,omitempty"`
	SourceMessageID string          `json:"source_message_id,omitempty"`
}

// ContextMessage is a unified message for LLM context, produced by MergeContext.
type ContextMessage struct {
	Role                 string          `json:"role"`
	Content              string          `json:"content"`
	RawContent           json.RawMessage `json:"raw_content,omitempty"`
	CompactionArtifactID string          `json:"compaction_artifact_id,omitempty"`
	RenderedMessageIDs   []string        `json:"rendered_message_ids,omitempty"`
	SourceMessageID      string          `json:"source_message_id,omitempty"`
	Current              bool            `json:"current,omitempty"`
}

// ComposeContextResult holds the output of ComposeContext.
type ComposeContextResult struct {
	Messages        []ContextMessage
	EstimatedTokens int
}

// CompactionSource identifies one durable history source covered by an active
// compaction artifact. ExternalMessageID projects that source onto the DCP
// rendered stream; HistoryMessageID projects it onto persisted turn responses.
type CompactionSource struct {
	Ref               contextfrag.ContextRef `json:"ref"`
	HistoryMessageID  string                 `json:"history_message_id,omitempty"`
	ExternalMessageID string                 `json:"external_message_id,omitempty"`
	CreatedAtMs       int64                  `json:"created_at_ms,omitempty"`
}

// CompactionArtifact is the pipeline-facing projection of one active durable
// artifact. Callers preserve frontier order; composition keeps each artifact
// separate so later restacks can supersede only the ranges they actually cover.
type CompactionArtifact struct {
	ID            string             `json:"id"`
	Summary       string             `json:"summary"`
	AnchorStartMs int64              `json:"anchor_start_ms,omitempty"`
	Sources       []CompactionSource `json:"sources,omitempty"`
}

// ContextHistoryProjection is the storage-independent history input consumed
// by pipeline orchestration.
type ContextHistoryProjection struct {
	TurnResponses          []TurnResponseEntry
	CompactionArtifacts    []CompactionArtifact
	LatestTurnResponseAtMs int64
}

// LatestExternalEventMs returns the latest external event timestamp after
// afterMs, or 0 if none found.
func LatestExternalEventMs(rc RenderedContext, afterMs int64) int64 {
	var latest int64
	for _, seg := range rc {
		eventAtMs := seg.eventAtMs()
		if eventAtMs > afterMs && !seg.IsMyself && !seg.IsSelfSent {
			if eventAtMs > latest {
				latest = eventAtMs
			}
		}
	}
	return latest
}

type mergeEntry struct {
	kind string // "rc", "summary", or "tr"
	time int64
	step int
	// For RC entries
	rcContent   []RenderedContentPiece
	rcMessageID string
	rcNative    bool
	current     bool
	// For summary entries
	summaryContent    string
	summaryArtifactID string
	// For TR entries
	trRole       string
	trContent    string
	trRawContent json.RawMessage
	trSourceID   string
}

// MergeContext interleaves RC segments and TR entries by timestamp.
// RC entries use their latest event time; TR entries use requestedAtMs.
// Tiebreaker: RC before TR on equal timestamp.
// Consecutive RC entries between TR entries are merged into one user message.
func MergeContext(rc RenderedContext, trs []TurnResponseEntry) []ContextMessage {
	entries := make([]mergeEntry, 0, len(rc)+len(trs))
	entries = appendRenderedContextEntries(entries, rc)
	entries = appendTurnResponseEntries(entries, trs)
	return mergeEntries(entries)
}

func appendRenderedContextEntries(entries []mergeEntry, rc RenderedContext) []mergeEntry {
	return appendRenderedContextEntriesAtCursor(entries, rc, nil)
}

func appendRenderedContextEntriesAtCursor(entries []mergeEntry, rc RenderedContext, afterMs *int64) []mergeEntry {
	for _, seg := range rc {
		entries = append(entries, mergeEntry{
			kind:        "rc",
			time:        seg.eventAtMs(),
			step:        -1,
			rcContent:   seg.Content,
			rcMessageID: strings.TrimSpace(seg.MessageID),
			rcNative:    renderedSegmentHasNativeContent(seg),
			current: afterMs != nil && seg.eventAtMs() > *afterMs &&
				!seg.IsMyself && !seg.IsSelfSent,
		})
	}
	return entries
}

func appendTurnResponseEntries(entries []mergeEntry, trs []TurnResponseEntry) []mergeEntry {
	for i, tr := range trs {
		entries = append(entries, mergeEntry{
			kind:         "tr",
			time:         tr.RequestedAtMs,
			step:         i,
			trRole:       tr.Role,
			trContent:    tr.Content,
			trRawContent: tr.RawContent,
			trSourceID:   strings.TrimSpace(tr.SourceMessageID),
		})
	}
	return entries
}

func mergeEntries(entries []mergeEntry) []ContextMessage {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].time != entries[j].time {
			return entries[i].time < entries[j].time
		}
		if entries[i].kind != entries[j].kind {
			return mergeKindOrder(entries[i].kind) < mergeKindOrder(entries[j].kind)
		}
		return entries[i].step < entries[j].step
	})

	var messages []ContextMessage
	var pendingText strings.Builder
	var pendingMessageIDs []string
	pendingCurrent := false
	pendingRC := false
	pendingNative := false

	flushRC := func() {
		if pendingText.Len() > 0 || pendingNative {
			messages = append(messages, ContextMessage{
				Role:               "user",
				Content:            pendingText.String(),
				RenderedMessageIDs: append([]string(nil), pendingMessageIDs...),
				Current:            pendingCurrent,
			})
		}
		pendingText.Reset()
		pendingMessageIDs = pendingMessageIDs[:0]
		pendingCurrent = false
		pendingRC = false
		pendingNative = false
	}

	for _, entry := range entries {
		switch entry.kind {
		case "rc":
			hasText := false
			for _, piece := range entry.rcContent {
				if piece.Type == "text" && piece.Text != "" {
					hasText = true
					break
				}
			}
			if !hasText && !(entry.current && entry.rcNative) {
				continue
			}
			if pendingRC && pendingCurrent != entry.current {
				flushRC()
			}
			if !pendingRC {
				pendingCurrent = entry.current
				pendingRC = true
			}
			pendingNative = pendingNative || entry.rcNative
			if entry.rcMessageID != "" {
				pendingMessageIDs = append(pendingMessageIDs, entry.rcMessageID)
			}
			for _, piece := range entry.rcContent {
				if piece.Type == "text" {
					if pendingText.Len() > 0 {
						pendingText.WriteByte('\n')
					}
					pendingText.WriteString(piece.Text)
				}
			}
		case "summary":
			flushRC()
			messages = append(messages, ContextMessage{
				Role:                 "user",
				Content:              entry.summaryContent,
				CompactionArtifactID: entry.summaryArtifactID,
			})
		case "tr":
			flushRC()
			messages = append(messages, ContextMessage{
				Role:            entry.trRole,
				Content:         entry.trContent,
				RawContent:      entry.trRawContent,
				SourceMessageID: entry.trSourceID,
			})
		}
	}
	flushRC()

	return messages
}

func mergeKindOrder(kind string) int {
	switch kind {
	case "summary":
		return 0
	case "rc":
		return 1
	case "tr":
		return 2
	default:
		return 3
	}
}

// ComposeContext merges un-compacted RC and TR streams.
func ComposeContext(rc RenderedContext, trs []TurnResponseEntry) *ComposeContextResult {
	return ComposeContextWithArtifacts(rc, trs, nil)
}

// ComposeContextWithArtifacts replaces covered RC/TR sources with each active
// artifact at its own chronological anchor.
func ComposeContextWithArtifacts(rc RenderedContext, trs []TurnResponseEntry, artifacts []CompactionArtifact) *ComposeContextResult {
	return composeContextWithArtifacts(rc, trs, artifacts, 0, false)
}

func composeContextWithArtifactsAtCursor(rc RenderedContext, trs []TurnResponseEntry, artifacts []CompactionArtifact, afterMs int64) *ComposeContextResult {
	return composeContextWithArtifacts(rc, trs, artifacts, afterMs, true)
}

func composeContextWithArtifacts(
	rc RenderedContext,
	trs []TurnResponseEntry,
	artifacts []CompactionArtifact,
	afterMs int64,
	markCurrent bool,
) *ComposeContextResult {
	activeRC := filterCoveredRenderedContext(rc, artifacts)
	activeTRs := filterCoveredTurnResponses(trs, artifacts)
	entries := make([]mergeEntry, 0, len(activeRC)+len(activeTRs)+len(artifacts))
	if markCurrent {
		entries = appendRenderedContextEntriesAtCursor(entries, activeRC, &afterMs)
	} else {
		entries = appendRenderedContextEntries(entries, activeRC)
	}
	for i, artifact := range artifacts {
		if !artifact.usable() {
			continue
		}
		entries = append(entries, mergeEntry{
			kind:              "summary",
			time:              artifactSummaryAnchor(artifact),
			step:              i,
			summaryContent:    "<summary>\n" + strings.TrimSpace(artifact.Summary) + "\n</summary>",
			summaryArtifactID: artifact.ID,
		})
	}
	entries = appendTurnResponseEntries(entries, activeTRs)
	allMessages := mergeEntries(entries)
	if len(allMessages) == 0 {
		return nil
	}

	return &ComposeContextResult{
		Messages:        allMessages,
		EstimatedTokens: estimateMessagesTokens(allMessages),
	}
}

func renderedSegmentHasNativeContent(segment RenderedSegment) bool {
	return len(segment.ImageRefs) > 0
}

const earliestMergeTime int64 = -1 << 63

func filterCoveredRenderedContext(rc RenderedContext, artifacts []CompactionArtifact) RenderedContext {
	cutoffs := coveredExternalMessageCutoffs(artifacts)
	if len(cutoffs) == 0 {
		return rc
	}
	filtered := make(RenderedContext, 0, len(rc))
	for _, segment := range rc {
		cutoffMs, covered := cutoffs[strings.TrimSpace(segment.MessageID)]
		if covered && cutoffMs > 0 && segment.eventAtMs() <= cutoffMs {
			continue
		}
		filtered = append(filtered, segment)
	}
	return filtered
}

// ActiveRenderedContext removes only sources covered by usable artifacts.
func ActiveRenderedContext(rc RenderedContext, artifacts []CompactionArtifact) RenderedContext {
	return filterCoveredRenderedContext(rc, artifacts)
}

func filterCoveredTurnResponses(trs []TurnResponseEntry, artifacts []CompactionArtifact) []TurnResponseEntry {
	covered := make(map[string]struct{})
	for _, artifact := range artifacts {
		if !artifact.usable() {
			continue
		}
		for _, source := range artifact.Sources {
			if id := strings.TrimSpace(source.HistoryMessageID); id != "" {
				covered[id] = struct{}{}
			}
		}
	}
	if len(covered) == 0 {
		return trs
	}
	filtered := make([]TurnResponseEntry, 0, len(trs))
	for _, tr := range trs {
		if _, ok := covered[strings.TrimSpace(tr.SourceMessageID)]; ok {
			continue
		}
		filtered = append(filtered, tr)
	}
	return filtered
}

func coveredExternalMessageCutoffs(artifacts []CompactionArtifact) map[string]int64 {
	cutoffs := make(map[string]int64)
	for _, artifact := range artifacts {
		if !artifact.usable() {
			continue
		}
		for _, source := range artifact.Sources {
			id := strings.TrimSpace(source.ExternalMessageID)
			if id != "" && source.CreatedAtMs > cutoffs[id] {
				cutoffs[id] = source.CreatedAtMs
			}
		}
	}
	return cutoffs
}

func (artifact CompactionArtifact) usable() bool {
	return strings.TrimSpace(artifact.ID) != "" && strings.TrimSpace(artifact.Summary) != ""
}

func artifactSummaryAnchor(artifact CompactionArtifact) int64 {
	if artifact.AnchorStartMs <= 0 {
		return earliestMergeTime
	}
	return artifact.AnchorStartMs
}

func (seg RenderedSegment) eventAtMs() int64 {
	if seg.LastEventAtMs > 0 {
		return seg.LastEventAtMs
	}
	return seg.ReceivedAtMs
}

func estimateMessagesTokens(messages []ContextMessage) int {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m)
	}
	return total
}

func estimateMessageTokens(m ContextMessage) int {
	if len(m.RawContent) > 0 {
		return messageconv.EstimateCanonicalContentTokens(m.RawContent)
	}
	return contextbudget.EstimateTextTokens(m.Content)
}
