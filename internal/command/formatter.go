package command

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const defaultListLimit = 12

// formatItems renders a list of records as a Markdown-style list.
// Each record is rendered on a single line so long lists stay readable in IM.
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
		fmt.Fprintf(&b, "- %s", record[0].value)
		extras := make([]string, 0, len(record)-1)
		for _, pair := range record[1:] {
			if strings.TrimSpace(pair.value) == "" {
				continue
			}
			extras = append(extras, fmt.Sprintf("%s: %s", pair.key, pair.value))
		}
		if len(extras) > 0 {
			fmt.Fprintf(&b, " | %s", strings.Join(extras, " | "))
		}
	}
	return b.String()
}

func formatLimitedItems(items [][]kv, limit int, hint string) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	total := len(items)
	if total <= limit {
		return formatItems(items)
	}
	shown := items[:limit]
	result := formatItems(shown)
	suffix := fmt.Sprintf("Showing %d of %d items.", len(shown), total)
	if strings.TrimSpace(hint) != "" {
		suffix += " " + strings.TrimSpace(hint)
	}
	return result + "\n\n" + suffix
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

// truncate shortens a string to at most maxLen runes, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string([]rune(s)[:maxLen])
	}
	return string([]rune(s)[:maxLen-3]) + "..."
}

// boolStr returns "yes" or "no".
func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
