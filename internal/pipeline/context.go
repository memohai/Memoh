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

// LatestExternalEventMs returns the receivedAtMs of the latest non-self segment
// after afterMs, or 0 if none found.
func LatestExternalEventMs(rc RenderedContext, afterMs int64) int64 {
	var latest int64
	for _, seg := range rc {
		eventAtMs := seg.eventAtMs()
		if eventAtMs > afterMs && !seg.IsMyself {
			if eventAtMs > latest {
				latest = eventAtMs
			}
		}
	}
	return latest
}

type mergeEntry struct {
	kind string // "rc" or "tr"
	time int64
	step int
	// For RC entries
	rcContent []RenderedContentPiece
	// For TR entries
	trRole       string
	trContent    string
	trRawContent json.RawMessage
}

// MergeContext interleaves RC segments and TR entries by timestamp.
// RC entries use receivedAtMs; TR entries use requestedAtMs.
// Tiebreaker: RC before TR on equal timestamp.
// Consecutive RC entries between TR entries are merged into one user message.
func MergeContext(rc RenderedContext, trs []TurnResponseEntry) []ContextMessage {
	entries := make([]mergeEntry, 0, len(rc)+len(trs))

	for _, seg := range rc {
		entries = append(entries, mergeEntry{
			kind:      "rc",
			time:      seg.ReceivedAtMs,
			step:      -1,
			rcContent: seg.Content,
		})
	}

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

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].time != entries[j].time {
			return entries[i].time < entries[j].time
		}
		if entries[i].kind != entries[j].kind {
			return entries[i].kind == "rc"
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
		if entry.kind == "rc" {
			for _, piece := range entry.rcContent {
				if piece.Type == "text" {
					if pendingText.Len() > 0 {
						pendingText.WriteByte('\n')
					}
					pendingText.WriteString(piece.Text)
				}
			}
		} else {
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

// ComposeContext merges RC and TRs, optionally prepends a compaction summary.
func ComposeContext(rc RenderedContext, trs []TurnResponseEntry, compactSummary string) *ComposeContextResult {
	return ComposeContextWithSummary(rc, trs, CompactSummary{Text: compactSummary})
}

func ComposeContextWithSummary(rc RenderedContext, trs []TurnResponseEntry, compactSummary CompactSummary) *ComposeContextResult {
	rc = filterCoveredRenderedContext(rc, compactSummary)
	trs = filterCoveredTurnResponses(trs, compactSummary)
	allMessages := MergeContext(rc, trs)
	summaryText := strings.TrimSpace(compactSummary.Text)
	if len(allMessages) == 0 && summaryText == "" {
		return nil
	}

	if summaryText != "" {
		summary := ContextMessage{Role: "user", Content: "[Conversation summary]\n" + summaryText}
		allMessages = append([]ContextMessage{summary}, allMessages...)
	}

	return &ComposeContextResult{
		Messages:        allMessages,
		EstimatedTokens: estimateMessagesTokens(allMessages),
	}
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

func (s CompactSummary) coversRenderedMessage(messageID string, receivedAtMs int64) bool {
	if len(s.CoveredMessageCutoffMs) == 0 {
		return true
	}
	cutoffMs, ok := s.CoveredMessageCutoffMs[messageID]
	if !ok || cutoffMs <= 0 {
		return true
	}
	return receivedAtMs <= cutoffMs
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
