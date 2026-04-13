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
