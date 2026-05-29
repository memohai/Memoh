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

// listRecord is one row destined for a paginated list: the kv fields drive both
// the text fallback (via formatItems) and the structured ListItem. selected and
// action are optional enrichments for interactive renderers.
type listRecord struct {
	fields   []kv
	selected bool
	action   *ItemAction
}

// buildListResult slices an in-memory record set for the requested page and
// produces a Result carrying complete fallback Text (preserving the existing
// "Showing N of M items." wording) plus a structured ListView. Text-only
// channels only ever see page 0, matching prior behavior.
func buildListResult(title, resource, action string, args []string, records []listRecord, page, pageSize int, hint string) *Result {
	if pageSize <= 0 {
		pageSize = defaultListLimit
	}
	total := len(records)
	if page < 0 {
		page = 0
	}
	start := page * pageSize
	if total > 0 && start >= total {
		page = (total - 1) / pageSize
		start = page * pageSize
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	var pageRecords []listRecord
	if start < total {
		pageRecords = records[start:end]
	}
	return assembleListResult(title, resource, action, args, pageRecords, page, pageSize, total, hint)
}

// buildPagedListResult builds a list Result when the caller has already fetched
// a single page from a server-paginated source (records IS the page, total is
// the full count). Used by commands whose service supports limit/offset.
func buildPagedListResult(title, resource, action string, args []string, pageRecords []listRecord, page, pageSize, total int, hint string) *Result {
	if pageSize <= 0 {
		pageSize = defaultListLimit
	}
	if page < 0 {
		page = 0
	}
	return assembleListResult(title, resource, action, args, pageRecords, page, pageSize, total, hint)
}

func assembleListResult(title, resource, action string, args []string, pageRecords []listRecord, page, pageSize, total int, hint string) *Result {
	textItems := make([][]kv, 0, len(pageRecords))
	items := make([]ListItem, 0, len(pageRecords))
	for _, r := range pageRecords {
		textItems = append(textItems, r.fields)
		items = append(items, listItemFromRecord(r))
	}

	text := formatItems(textItems)
	if total > len(pageRecords) {
		suffix := fmt.Sprintf("Showing %d of %d items.", len(pageRecords), total)
		if strings.TrimSpace(hint) != "" {
			suffix += " " + strings.TrimSpace(hint)
		}
		if text != "" {
			text += "\n\n"
		}
		text += suffix
	}
	if t := strings.TrimSpace(title); t != "" && len(pageRecords) > 0 {
		text = fmt.Sprintf("%s (%d)\n\n%s", t, total, text)
	}

	return &Result{
		Text: text,
		Interactive: &Interactive{
			Kind: InteractiveList,
			List: &ListView{
				Title:    title,
				Resource: resource,
				Action:   action,
				Args:     args,
				Items:    items,
				Total:    total,
				Page:     page,
				PageSize: pageSize,
			},
		},
	}
}

func listItemFromRecord(r listRecord) ListItem {
	item := ListItem{Selected: r.selected, Action: r.action}
	if len(r.fields) == 0 {
		return item
	}
	item.Label = r.fields[0].value
	extras := make([]string, 0, len(r.fields)-1)
	for _, pair := range r.fields[1:] {
		if strings.TrimSpace(pair.value) == "" {
			continue
		}
		extras = append(extras, fmt.Sprintf("%s: %s", pair.key, pair.value))
	}
	item.Detail = strings.Join(extras, " | ")
	return item
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
