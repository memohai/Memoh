package compaction

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are a conversation summarizer. Given a conversation history, produce a concise summary that preserves:
- Key facts, decisions, and agreements
- User preferences and requests
- Important context needed for continuing the conversation
- Names, dates, numbers, and specific details
- Tool usage outcomes and their results

If <prior_context> is provided, it contains summaries of earlier conversation segments. Use them ONLY to understand the conversation flow and maintain continuity. Do NOT include, repeat, or rephrase any content from <prior_context> in your output.

For tool results, only include key outcomes; ignore intermediate steps or errors.

Output ONLY the summary of the new conversation segment. No preamble, no headers.`

type messageEntry struct {
	Role    string
	Content string
}

func buildUserPrompt(priorSummaries []string, messages []messageEntry) string {
	var sb strings.Builder
	if len(priorSummaries) > 0 {
		sb.WriteString("<prior_context>\n")
		sb.WriteString("The following are summaries of earlier parts of this conversation. They are provided ONLY as reference context to help you understand the conversation flow. Do NOT include or repeat any of this content in your output summary.\n\n")
		sb.WriteString(strings.Join(priorSummaries, "\n---\n"))
		sb.WriteString("\n</prior_context>\n\n")
		sb.WriteString("Now summarize the following conversation segment:\n")
	} else {
		sb.WriteString("Summarize the following conversation:\n")
	}
	for _, m := range messages {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	return sb.String()
}

// capPriorSummaries bounds the reference context fed to the summarizer.
// Summaries accumulate one per pass, so an unbounded <prior_context> would
// eventually dominate — or overflow — the compaction model's own window.
// The newest summaries carry the most continuity, so the budget keeps them
// (at least one) and drops from the oldest; order stays chronological. The
// cap is hard: when the forced newest summary alone exceeds it, its text is
// truncated so callers can rely on the returned total staying within budget.
func capPriorSummaries(summaries []string, maxTokens int) []string {
	if len(summaries) == 0 || maxTokens <= 0 {
		return summaries
	}
	accumulated := 0
	start := len(summaries)
	for i := len(summaries) - 1; i >= 0; i-- {
		cost := estimateBytesAsTokens(summaries[i])
		if start < len(summaries) && accumulated+cost > maxTokens {
			break
		}
		accumulated += cost
		start = i
	}
	kept := summaries[start:]
	if len(kept) == 1 && estimateBytesAsTokens(kept[0]) > maxTokens {
		kept = []string{truncateBytes(kept[0], maxTokens*4)}
	}
	return kept
}
