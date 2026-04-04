package pipeline

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	charsPerToken = 2
	imageTokens   = 100
)

// TurnResponseEntry represents an assistant or tool message from bot_history_messages,
// used as the "TR" stream in context composition.
type TurnResponseEntry struct {
	RequestedAtMs int64  `json:"requested_at_ms"`
	Role          string `json:"role"`
	Content       string `json:"content"`
}

// ContextMessage is a unified message for LLM context, produced by MergeContext.
type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ComposeContextResult holds the output of ComposeContext.
type ComposeContextResult struct {
	Messages           []ContextMessage
	EstimatedTokens    int
	RawEstimatedTokens int
}

// LatestExternalEventMs returns the receivedAtMs of the latest non-self segment
// after afterMs, or 0 if none found.
func LatestExternalEventMs(rc RenderedContext, afterMs int64) int64 {
	var latest int64
	for _, seg := range rc {
		if seg.ReceivedAtMs > afterMs && !seg.IsMyself {
			if seg.ReceivedAtMs > latest {
				latest = seg.ReceivedAtMs
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
	trRole    string
	trContent string
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
			kind:      "tr",
			time:      tr.RequestedAtMs,
			step:      i,
			trRole:    tr.Role,
			trContent: tr.Content,
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
				Role:    entry.trRole,
				Content: entry.trContent,
			})
		}
	}
	flushRC()

	return messages
}

// ComposeContext merges RC and TRs, optionally prepends a compaction summary,
// and trims to fit within maxTokens.
func ComposeContext(rc RenderedContext, trs []TurnResponseEntry, maxTokens int, compactSummary string) *ComposeContextResult {
	allMessages := MergeContext(rc, trs)
	if len(allMessages) == 0 && compactSummary == "" {
		return nil
	}

	if compactSummary != "" {
		summary := ContextMessage{Role: "user", Content: "[Conversation summary]\n" + compactSummary}
		allMessages = append([]ContextMessage{summary}, allMessages...)
	}

	rawEstimatedTokens := estimateMessagesTokens(allMessages)
	trimmed, estimatedTokens := TrimContext(allMessages, maxTokens)

	return &ComposeContextResult{
		Messages:           trimmed,
		EstimatedTokens:    estimatedTokens,
		RawEstimatedTokens: rawEstimatedTokens,
	}
}

// TrimContext drops messages from the front (oldest) to fit within maxTokens.
// For user messages, drops individual content. For assistant messages with tool
// context, also drops following tool messages. Returns trimmed messages and
// their estimated token count.
func TrimContext(messages []ContextMessage, maxTokens int) ([]ContextMessage, int) {
	totalTokens := estimateMessagesTokens(messages)
	if totalTokens <= maxTokens {
		return messages, totalTokens
	}

	result := make([]ContextMessage, len(messages))
	copy(result, messages)

	for totalTokens > maxTokens && len(result) > 1 {
		dropped := result[0]
		droppedTokens := estimateMessageTokens(dropped)
		result = result[1:]
		totalTokens -= droppedTokens

		// If dropped an assistant, also drop following tool messages
		if dropped.Role == "assistant" {
			for len(result) > 0 && result[0].Role == "tool" {
				totalTokens -= estimateMessageTokens(result[0])
				result = result[1:]
			}
		}
	}

	// Don't start with orphaned tool messages
	for len(result) > 1 && result[0].Role == "tool" {
		totalTokens -= estimateMessageTokens(result[0])
		result = result[1:]
	}

	return result, totalTokens
}

// FindWorkingWindowCursor walks backward from the newest entries (RC + TRs),
// accumulating token cost until the budget is exceeded. Returns the timestamp
// of the cutoff point.
func FindWorkingWindowCursor(rc RenderedContext, trs []TurnResponseEntry, budgetTokens int) int64 {
	type entry struct {
		timeMs int64
		tokens int
	}

	var entries []entry
	for _, seg := range rc {
		tokens := 0
		for _, p := range seg.Content {
			if p.Type == "text" {
				tokens += int(math.Ceil(float64(len(p.Text)) / charsPerToken))
			} else {
				tokens += imageTokens
			}
		}
		entries = append(entries, entry{timeMs: seg.ReceivedAtMs, tokens: tokens})
	}

	for _, tr := range trs {
		tokens := int(math.Ceil(float64(len(tr.Content)) / charsPerToken))
		entries = append(entries, entry{timeMs: tr.RequestedAtMs, tokens: tokens})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timeMs > entries[j].timeMs
	})

	accum := 0
	for _, e := range entries {
		accum += e.tokens
		if accum > budgetTokens {
			return e.timeMs
		}
	}

	if len(entries) > 0 {
		return entries[len(entries)-1].timeMs
	}
	return 0
}

func estimateMessagesTokens(messages []ContextMessage) int {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m)
	}
	return total
}

func estimateMessageTokens(m ContextMessage) int {
	return int(math.Ceil(float64(len(m.Content)) / charsPerToken))
}

// EstimateTokens estimates the token count for a string using the chars-per-token heuristic.
func EstimateTokens(s string) int {
	return int(math.Ceil(float64(len(s)) / charsPerToken))
}

// FormatLastProcessedMs returns the lastProcessedMs as a string for metadata storage.
func FormatLastProcessedMs(ms int64) string {
	return strconv.FormatInt(ms, 10)
}
