package channel

import (
	"regexp"
	"strings"
)

// ContainsMarkdown returns true if the text contains common Markdown constructs.
func ContainsMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	patterns := []string{
		`\*\*[^*]+\*\*`,
		`\*[^*]+\*`,
		`~~[^~]+~~`,
		"`[^`]+`",
		"```[\\s\\S]*```",
		`\[.+\]\(.+\)`,
		`(?m)^#{1,6}\s`,
		`(?m)^[-*]\s`,
		`(?m)^\d+\.\s`,
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}
