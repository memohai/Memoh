package pipeline

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
)

const charsPerToken = 2

// TurnResponseEntry represents an assistant or tool message from bot_history_messages,
// used as the "TR" stream in context composition.
type TurnResponseEntry struct {
	RequestedAtMs     int64           `json:"requested_at_ms"`
	Role              string          `json:"role"`
	Content           string          `json:"content"`
	RawContent        json.RawMessage `json:"raw_content,omitempty"`
	SourceMessageID   string          `json:"source_message_id,omitempty"`
	ExternalMessageID string          `json:"external_message_id,omitempty"`
	CompactID         string          `json:"compact_id,omitempty"`
}

// ContextMessage is a unified message for LLM context, produced by MergeContext.
type ContextMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	RawContent json.RawMessage `json:"raw_content,omitempty"`
}

// ComposeContextResult holds the output of ComposeContext.
type ComposeContextResult struct {
	Messages        []ContextMessage
	EstimatedTokens int
}

type CompactSummary struct {
	Text                   string           `json:"text,omitempty"`
	CoveredMessageIDs      []string         `json:"covered_message_ids,omitempty"`
	CoveredMessageCutoffMs map[string]int64 `json:"covered_message_cutoff_ms,omitempty"`

	CoveredHistoryMessageIDs []string `json:"covered_history_message_ids,omitempty"`
}

// LatestExternalEventMs returns the latest non-self segment event timestamp
// after afterMs, or 0 if none found.
func LatestExternalEventMs(rc RenderedContext, afterMs int64) int64 {
	var latest int64
	for _, seg := range rc {
		eventAtMs := seg.eventAtMs()
		if eventAtMs > afterMs && isExternalSegment(seg) {
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
	rcContent []RenderedContentPiece
	// For summary entries
	summaryContent string
	// For TR entries
	trRole       string
	trContent    string
	trRawContent json.RawMessage
}

type budgetSourceGroup struct {
	rcIndices []int
	trIndices []int
	tokens    int
	priority  int
	time      int64
	forceKeep bool
}

// MergeContext interleaves RC segments and TR entries by timestamp.
// RC entries use receivedAtMs; TR entries use requestedAtMs.
// Tiebreaker: RC before TR on equal timestamp.
// Consecutive RC entries between TR entries are merged into one user message.
func MergeContext(rc RenderedContext, trs []TurnResponseEntry) []ContextMessage {
	entries := make([]mergeEntry, 0, len(rc)+len(trs))
	entries = appendRenderedContextEntries(entries, rc)
	entries = appendTurnResponseEntries(entries, trs)
	return mergeEntries(entries)
}

func appendRenderedContextEntries(entries []mergeEntry, rc RenderedContext) []mergeEntry {
	for _, seg := range rc {
		entries = append(entries, mergeEntry{
			kind:      "rc",
			time:      seg.ReceivedAtMs,
			step:      -1,
			rcContent: seg.Content,
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

	flushRC := func() {
		if pendingText.Len() > 0 {
			messages = append(messages, ContextMessage{Role: "user", Content: pendingText.String()})
			pendingText.Reset()
		}
	}

	for _, entry := range entries {
		switch entry.kind {
		case "rc":
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
				Role:    "user",
				Content: entry.summaryContent,
			})
		case "tr":
			flushRC()
			messages = append(messages, ContextMessage{
				Role:       entry.trRole,
				Content:    entry.trContent,
				RawContent: entry.trRawContent,
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

// ComposeContext merges RC and TRs, optionally prepends a compaction summary.
func ComposeContext(rc RenderedContext, trs []TurnResponseEntry, compactSummary string) *ComposeContextResult {
	return ComposeContextWithSummary(rc, trs, CompactSummary{Text: compactSummary})
}

func ComposeContextWithSummary(rc RenderedContext, trs []TurnResponseEntry, compactSummary CompactSummary) *ComposeContextResult {
	summaryText := strings.TrimSpace(compactSummary.Text)
	summaryAnchorMs, hasSummaryAnchor := compactSummary.summaryAnchorMs(rc, trs)
	rc = filterCoveredRenderedContext(rc, compactSummary)
	trs = filterCoveredTurnResponses(trs, compactSummary)
	entries := make([]mergeEntry, 0, len(rc)+len(trs)+1)
	entries = appendRenderedContextEntries(entries, rc)
	if summaryText != "" {
		summaryTime := summaryAnchorMs
		if !hasSummaryAnchor {
			summaryTime = earliestMergeTime
		}
		entries = append(entries, mergeEntry{
			kind:           "summary",
			time:           summaryTime,
			step:           -1,
			summaryContent: "[Conversation summary]\n" + summaryText,
		})
	}
	entries = appendTurnResponseEntries(entries, trs)
	allMessages := mergeEntries(entries)
	if len(allMessages) == 0 && summaryText == "" {
		return nil
	}

	return &ComposeContextResult{
		Messages:        allMessages,
		EstimatedTokens: estimateMessagesTokens(allMessages),
	}
}

const earliestMergeTime int64 = -1 << 63

func TrimContextSourcesByBudget(rc RenderedContext, trs []TurnResponseEntry, compactSummary CompactSummary, maxTokens int) (RenderedContext, []TurnResponseEntry) {
	sourceRC := rc
	sourceTRs := trs
	budgetRC := filterCoveredRenderedContext(rc, compactSummary)
	budgetTRs := filterCoveredTurnResponses(trs, compactSummary)
	if maxTokens <= 0 {
		return sourceRC, sourceTRs
	}
	composed := ComposeContextWithSummary(sourceRC, sourceTRs, compactSummary)
	if composed == nil || composed.EstimatedTokens <= maxTokens {
		return sourceRC, sourceTRs
	}

	selectedRC := make(map[int]bool, len(budgetRC))
	selectedTR := make(map[int]bool, len(budgetTRs))
	usedTokens := compactSummaryBudgetTokens(compactSummary)
	groups := budgetSourceGroups(budgetRC, budgetTRs, compactSummary)
	for _, group := range groups {
		if !group.forceKeep {
			continue
		}
		for _, idx := range group.rcIndices {
			selectedRC[idx] = true
		}
		for _, idx := range group.trIndices {
			selectedTR[idx] = true
		}
		usedTokens += group.tokens
	}

	remaining := maxTokens - usedTokens
	if remaining < 0 {
		remaining = 0
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].priority != groups[j].priority {
			return groups[i].priority > groups[j].priority
		}
		return groups[i].time > groups[j].time
	})

	for _, group := range groups {
		if group.forceKeep {
			continue
		}
		if group.tokens > remaining {
			continue
		}
		for _, idx := range group.rcIndices {
			selectedRC[idx] = true
		}
		for _, idx := range group.trIndices {
			selectedTR[idx] = true
		}
		remaining -= group.tokens
	}

	outRC := make(RenderedContext, 0, len(budgetRC))
	for i, seg := range budgetRC {
		if selectedRC[i] {
			outRC = append(outRC, seg)
		}
	}
	outTRs := make([]TurnResponseEntry, 0, len(budgetTRs))
	for i, tr := range budgetTRs {
		if selectedTR[i] {
			outTRs = append(outTRs, tr)
		}
	}
	outRC = appendCoveredRenderedAnchors(outRC, sourceRC, compactSummary)
	outTRs = dropOrphanTurnResponseTools(outTRs)
	outTRs = appendCoveredTurnResponseAnchors(outTRs, sourceTRs, compactSummary)
	return outRC, outTRs
}

func appendCoveredRenderedAnchors(out RenderedContext, source RenderedContext, summary CompactSummary) RenderedContext {
	covered := stringSet(summary.CoveredMessageIDs)
	if len(covered) == 0 {
		return out
	}
	for _, seg := range source {
		messageID := strings.TrimSpace(seg.MessageID)
		if _, ok := covered[messageID]; ok && summary.coversRenderedMessage(messageID, seg.eventAtMs()) {
			out = append(out, seg)
		}
	}
	return out
}

func appendCoveredTurnResponseAnchors(out []TurnResponseEntry, source []TurnResponseEntry, summary CompactSummary) []TurnResponseEntry {
	covered := stringSet(summary.CoveredHistoryMessageIDs)
	if len(covered) == 0 {
		return out
	}
	for _, tr := range source {
		if _, ok := covered[strings.TrimSpace(tr.SourceMessageID)]; ok {
			out = append(out, tr)
		}
	}
	return out
}

func budgetSourceGroups(rc RenderedContext, trs []TurnResponseEntry, compactSummary CompactSummary) []budgetSourceGroup {
	latestTriggerIndex := latestExternalRenderedSegmentIndex(rc)
	groups := make([]budgetSourceGroup, 0, len(rc)+len(trs))
	for i, seg := range rc {
		groups = append(groups, budgetSourceGroup{
			rcIndices: []int{i},
			tokens:    estimateRenderedSegmentTokens(seg),
			priority:  renderedSegmentBudgetPriority(seg, compactSummary),
			time:      seg.eventAtMs(),
			forceKeep: isPostCompactRenderedSegment(seg, compactSummary) || i == latestTriggerIndex,
		})
	}
	groupedToolIndices := make(map[int]struct{})
	for i := 0; i < len(trs); i++ {
		if _, ok := groupedToolIndices[i]; ok {
			continue
		}
		group := budgetSourceGroup{
			trIndices: []int{i},
			tokens:    estimateTurnResponseTokens(trs[i]),
			priority:  turnResponseBudgetPriority(trs[i]),
			time:      trs[i].RequestedAtMs,
		}
		if callIDs := turnResponseToolCallIDs(trs[i]); len(callIDs) > 0 {
			for j := i + 1; j < len(trs) && len(callIDs) > 0; j++ {
				if !strings.EqualFold(strings.TrimSpace(trs[j].Role), "tool") {
					continue
				}
				matchedIDs := turnResponseMatchingToolResultIDs(trs[j], callIDs)
				if len(matchedIDs) == 0 {
					continue
				}
				group.trIndices = append(group.trIndices, j)
				group.tokens += estimateTurnResponseTokens(trs[j])
				group.time = max(group.time, trs[j].RequestedAtMs)
				groupedToolIndices[j] = struct{}{}
				for _, id := range matchedIDs {
					delete(callIDs, id)
				}
			}
		}
		groups = append(groups, group)
	}
	return groups
}

func latestExternalRenderedSegmentIndex(rc RenderedContext) int {
	latestIndex := -1
	var latestTime int64
	for i, seg := range rc {
		if seg.IsMyself || seg.IsSelfSent {
			continue
		}
		eventAt := seg.eventAtMs()
		if latestIndex == -1 || eventAt >= latestTime {
			latestIndex = i
			latestTime = eventAt
		}
	}
	return latestIndex
}

func compactSummaryBudgetTokens(summary CompactSummary) int {
	text := strings.TrimSpace(summary.Text)
	if text == "" {
		return 0
	}
	return estimateMessageTokens(ContextMessage{Role: "user", Content: "[Conversation summary]\n" + text})
}

func estimateRenderedSegmentTokens(seg RenderedSegment) int {
	total := 0
	for _, piece := range seg.Content {
		total += len(piece.Text) + len(piece.URL)
	}
	if total == 0 {
		return 0
	}
	return int(math.Ceil(float64(total+1) / charsPerToken))
}

func estimateTurnResponseTokens(tr TurnResponseEntry) int {
	return estimateMessageTokens(ContextMessage{
		Role:       tr.Role,
		Content:    tr.Content,
		RawContent: tr.RawContent,
	})
}

func renderedSegmentBudgetPriority(seg RenderedSegment, summary CompactSummary) int {
	if isPostCompactRenderedSegment(seg, summary) {
		return 90
	}
	if seg.MentionsMe || seg.RepliesToMe {
		return 85
	}
	if seg.IsMyself || seg.IsSelfSent {
		return 60
	}
	return 70
}

func isPostCompactRenderedSegment(seg RenderedSegment, summary CompactSummary) bool {
	messageID := strings.TrimSpace(seg.MessageID)
	if messageID == "" {
		return false
	}
	_, ok := stringSet(summary.CoveredMessageIDs)[messageID]
	return ok && !summary.coversRenderedMessage(messageID, seg.eventAtMs())
}

func turnResponseBudgetPriority(tr TurnResponseEntry) int {
	if strings.EqualFold(strings.TrimSpace(tr.Role), "tool") {
		return 55
	}
	return 70
}

func turnResponseToolCallIDs(tr TurnResponseEntry) map[string]struct{} {
	if !strings.EqualFold(strings.TrimSpace(tr.Role), "assistant") {
		return nil
	}
	ids := make(map[string]struct{})
	var parts []turnResponsePart
	if err := json.Unmarshal(tr.RawContent, &parts); err != nil {
		return nil
	}
	for _, part := range parts {
		partType := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(part.Type)), "_", "-")
		if strings.Contains(partType, "tool-call") {
			if id := strings.TrimSpace(part.ToolCallID); id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func turnResponseMatchingToolResultIDs(tr TurnResponseEntry, callIDs map[string]struct{}) []string {
	if len(callIDs) == 0 || !strings.EqualFold(strings.TrimSpace(tr.Role), "tool") {
		return nil
	}
	var parts []turnResponsePart
	if err := json.Unmarshal(tr.RawContent, &parts); err != nil {
		return nil
	}
	var matched []string
	for _, part := range parts {
		partType := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(part.Type)), "_", "-")
		if !strings.Contains(partType, "tool-result") {
			continue
		}
		id := strings.TrimSpace(part.ToolCallID)
		if _, ok := callIDs[id]; ok {
			matched = append(matched, id)
		}
	}
	return matched
}

// dropOrphanTurnResponseTools mirrors the flow path's dropOrphanToolRecords:
// call ids are collected across the whole slice so a result separated from its
// call by other rows is still recognized, and duplicate results for an already
// consumed call id are dropped.
func dropOrphanTurnResponseTools(trs []TurnResponseEntry) []TurnResponseEntry {
	out := trs[:0]
	assistantCallIDs := make(map[string]struct{})
	for _, tr := range trs {
		for id := range turnResponseToolCallIDs(tr) {
			assistantCallIDs[id] = struct{}{}
		}
	}
	keptResultIDs := make(map[string]struct{})
	for _, tr := range trs {
		if strings.EqualFold(strings.TrimSpace(tr.Role), "tool") {
			keep := false
			for _, id := range turnResponseMatchingToolResultIDs(tr, assistantCallIDs) {
				if _, consumed := keptResultIDs[id]; consumed {
					continue
				}
				keptResultIDs[id] = struct{}{}
				keep = true
			}
			if keep {
				out = append(out, tr)
			}
			continue
		}
		out = append(out, tr)
	}
	return out
}

func filterCoveredRenderedContext(rc RenderedContext, compactSummary CompactSummary) RenderedContext {
	covered := stringSet(compactSummary.CoveredMessageIDs)
	if len(covered) == 0 {
		return rc
	}
	filtered := make(RenderedContext, 0, len(rc))
	for _, seg := range rc {
		messageID := strings.TrimSpace(seg.MessageID)
		if _, ok := covered[messageID]; ok && compactSummary.coversRenderedMessage(messageID, seg.eventAtMs()) {
			continue
		}
		filtered = append(filtered, seg)
	}
	return filtered
}

func (seg RenderedSegment) eventAtMs() int64 {
	if seg.LastEventAtMs > 0 {
		return seg.LastEventAtMs
	}
	return seg.ReceivedAtMs
}

func isExternalSegment(seg RenderedSegment) bool {
	return !seg.IsMyself && !seg.IsSelfSent
}

func (s CompactSummary) coversRenderedMessage(messageID string, receivedAtMs int64) bool {
	cutoffMs, ok := s.CoveredMessageCutoffMs[messageID]
	if !ok || cutoffMs <= 0 {
		return false
	}
	return receivedAtMs <= cutoffMs
}

func (s CompactSummary) summaryAnchorMs(rc RenderedContext, trs []TurnResponseEntry) (int64, bool) {
	var anchor int64
	hasAnchor := false
	update := func(value int64) {
		if value <= 0 {
			return
		}
		if !hasAnchor || value < anchor {
			anchor = value
			hasAnchor = true
		}
	}

	coveredMessages := stringSet(s.CoveredMessageIDs)
	if len(coveredMessages) > 0 {
		for _, seg := range rc {
			messageID := strings.TrimSpace(seg.MessageID)
			if _, ok := coveredMessages[messageID]; !ok {
				continue
			}
			if s.coversRenderedMessage(messageID, seg.eventAtMs()) {
				update(seg.eventAtMs())
				continue
			}
			if cutoffMs, ok := s.CoveredMessageCutoffMs[messageID]; ok && cutoffMs > 0 && seg.ReceivedAtMs <= cutoffMs {
				update(seg.ReceivedAtMs)
			}
		}
	}

	coveredHistory := stringSet(s.CoveredHistoryMessageIDs)
	if len(coveredHistory) > 0 {
		for _, tr := range trs {
			if _, ok := coveredHistory[strings.TrimSpace(tr.SourceMessageID)]; ok {
				update(tr.RequestedAtMs)
			}
		}
	}

	return anchor, hasAnchor
}

func filterCoveredTurnResponses(trs []TurnResponseEntry, compactSummary CompactSummary) []TurnResponseEntry {
	coveredHistory := stringSet(compactSummary.CoveredHistoryMessageIDs)
	if len(coveredHistory) == 0 {
		return trs
	}
	filtered := make([]TurnResponseEntry, 0, len(trs))
	for _, tr := range trs {
		if _, ok := coveredHistory[strings.TrimSpace(tr.SourceMessageID)]; ok {
			continue
		}
		filtered = append(filtered, tr)
	}
	return filtered
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
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
		return int(math.Ceil(float64(len(m.RawContent)) / charsPerToken))
	}
	return int(math.Ceil(float64(len(m.Content)) / charsPerToken))
}
