package prompts

import "fmt"

// Quote wraps content with backticks
func Quote(content string) string {
	return fmt.Sprintf("`%s`", content)
}

// Block wraps content in a code block with optional tag
func Block(content string, tag string) string {
	if tag != "" {
		return fmt.Sprintf("```%s\n%s\n```", tag, content)
	}
	return fmt.Sprintf("```\n%s\n```", content)
}
