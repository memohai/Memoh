package command

import (
	"fmt"
	"strings"
)

// formatItems renders a list of records as a Markdown-style list.
// Each record is a slice of kv pairs; the first pair's value is used as the
// bullet title, and subsequent pairs are indented beneath it.
//
// Example output:
//
//   - mybot
//     Description: A helpful assistant
//     ID: abc123
//
//   - another
//     Description: Something else
//     ID: def456
func formatItems(items [][]kv) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, record := range items {
		if len(record) == 0 {
			continue
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "- %s\n", record[0].value)
		for _, pair := range record[1:] {
			fmt.Fprintf(&b, "  %s: %s\n", pair.key, pair.value)
		}
	}
	return b.String()
}

// formatKV renders key-value pairs as a simple Markdown list.
//
// Example output:
//
//   - ID: abc123
//   - Name: mybot
func formatKV(pairs []kv) string {
	var b strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&b, "- %s: %s\n", p.key, p.value)
	}
	return b.String()
}

type kv struct {
	key   string
	value string
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// boolStr returns "yes" or "no".
func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
