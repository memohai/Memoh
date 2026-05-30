package command

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const defaultListLimit = 12

// formatItems renders records in the compact list layout (see formatRow),
// without per-record notes. Retained for direct callers and tests.
func formatItems(items [][]kv) string {
	records := make([]listRecord, 0, len(items))
	for _, fields := range items {
		records = append(records, listRecord{fields: fields})
	}
	return formatRecords(records)
}

// formatRecords renders list rows in the compact layout:
//
//   - label — chip · chip
//     note
//
// The first field is the row label; remaining non-empty fields become a
// " · "-separated run of chips after an em dash; an optional record note flows
// onto an indented second line. Field keys are intentionally dropped — the
// values carry the meaning and omitting "Key:" prefixes keeps rows scannable on
// narrow IM screens. Code-spanning is per-value via renderValue.
func formatRecords(records []listRecord) string {
	var b strings.Builder
	first := true
	for _, r := range records {
		if len(r.fields) == 0 {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString(formatRow(r))
	}
	return b.String()
}

func formatRow(r listRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s", renderValue(r.fields[0].value))
	chips := make([]string, 0, len(r.fields)-1)
	for _, pair := range r.fields[1:] {
		if strings.TrimSpace(pair.value) == "" {
			continue
		}
		chips = append(chips, renderValue(pair.value))
	}
	if len(chips) > 0 {
		fmt.Fprintf(&b, " — %s", strings.Join(chips, " · "))
	}
	if note := strings.TrimSpace(r.note); note != "" {
		fmt.Fprintf(&b, "\n  %s", note)
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
		if strings.TrimSpace(p.value) == "" {
			continue // omit blank fields rather than print a dangling "Key: "
		}
		fmt.Fprintf(&b, "- %s: %s\n", p.key, renderValue(p.value))
	}
	return b.String()
}

// formatKVTitled prefixes a bold title above a key-value detail block, giving
// detail views the same bold header that list views carry ("Title (N)").
func formatKVTitled(title string, pairs []kv) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return formatKV(pairs)
	}
	return MdBold(t) + "\n\n" + formatKV(pairs)
}

type kv struct {
	key   string
	value string
}

// renderValue formats a value for Markdown text, wrapping it in a code span
// only when it reads as a machine token (see isMachineToken). The same rule
// feeds formatItems and formatKV so every text surface is styled consistently:
// short words, enums, booleans, and humanized scalars stay plain; IDs, paths,
// cron, slugs, and markup-bearing values become monospace.
func renderValue(value string) string {
	if isMachineToken(value) {
		return MdCode(value)
	}
	return value
}

// isMachineToken reports whether a value reads as a machine token (ID, path,
// cron, slug, handle) that benefits from a monospace code span — rather than a
// human word, enum, number, or phrase that reads better as plain text.
//
// It is also a correctness guard: the Telegram Markdown→HTML pass runs an italic
// regex after bold, so a bare value containing * _ ` [ ] would be mangled (e.g.
// cron "0 9 * * *" -> "0 9 <i> </i> *"). Such values are always code-wrapped.
func isMachineToken(v string) bool {
	s := strings.TrimSpace(v)
	if s == "" {
		return false
	}
	// Inline-markdown metachars must live inside a code span to survive the
	// Telegram renderer verbatim (also a strong "this is a token" signal). This
	// stays first so cron ("0 9 * * *") is always code-wrapped.
	if strings.ContainsAny(s, "*_`[]") {
		return true
	}
	// Whitespace => human phrase / prose / ratio ("12.4K / 1.0M"), not a token.
	// Checked before the slash rule so a spaced ratio is not mistaken for a path.
	if strings.ContainsAny(s, " \t\n\r") {
		return false
	}
	// Paths and namespaced slugs ("anthropic/claude-opus", "/srv/data").
	if strings.Contains(s, "/") {
		return true
	}
	// Email addresses read as tokens.
	if strings.Contains(s, "@") {
		return true
	}
	// Known no-space human words (booleans, enums, placeholders) stay plain.
	if isHumanWord(s) {
		return false
	}
	// Long opaque identifiers (UUIDs, hashes, long slugs) read as tokens; short
	// bare words and humanized scalars ("42", "12.4K", "820ms") stay plain.
	return utf8.RuneCountInString(s) >= 12
}

// isHumanWord matches single-token, space-free values that are human-facing
// words (booleans, status enums, roles, placeholders) and must stay plain.
func isHumanWord(s string) bool {
	switch strings.ToLower(s) {
	case "yes", "no", "on", "off", "none", "(none)", "true", "false", "unlimited", "unknown",
		"ok", "success", "failed", "fail", "error", "errored", "active", "inactive",
		"enabled", "disabled", "connected", "disconnected", "pending", "running", "stopped",
		"idle", "ready", "allow", "deny", "allowed", "denied", "owner", "admin", "member",
		"guest", "read", "write", "delete", "stdio", "http", "https", "sse",
		"sent", "queued", "sending", "bounced", "draft", "default":
		return true
	}
	return false
}

// listRecord is one row destined for a paginated list: fields drives both the
// text rendering (via formatRecords) and the structured ListItem (fields[0] is
// the label). note is optional prose shown on an indented second line in text
// output. selected and action are optional enrichments for interactive renderers.
type listRecord struct {
	fields   []kv
	note     string
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
	items := make([]ListItem, 0, len(pageRecords))
	for _, r := range pageRecords {
		items = append(items, listItemFromRecord(r))
	}

	text := formatRecords(pageRecords)
	// Footer: pagination count (only when paginated) + the cross-reference hint
	// (always, when set) — previously the hint only rendered inside the
	// pagination branch, so small config lists never showed their next-step.
	var suffixParts []string
	if total > len(pageRecords) {
		suffixParts = append(suffixParts, fmt.Sprintf("Showing %d of %d items.", len(pageRecords), total))
	}
	if h := strings.TrimSpace(hint); h != "" {
		suffixParts = append(suffixParts, h)
	}
	if len(suffixParts) > 0 {
		if text != "" {
			text += "\n\n"
		}
		text += strings.Join(suffixParts, " ")
	}
	if t := strings.TrimSpace(title); t != "" && len(pageRecords) > 0 {
		text = fmt.Sprintf("%s\n\n%s", MdBold(fmt.Sprintf("%s (%d)", t, total)), text)
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

// WithExtraActions attaches contextual entry buttons below the list rows of a
// Result (e.g. "All commands", "Create new"). Only meaningful when the Result
// carries an InteractiveList. Nil/non-list Results pass through unchanged.
func WithExtraActions(r *Result, extras ...ListItem) *Result {
	if r == nil || r.Interactive == nil || r.Interactive.List == nil {
		return r
	}
	r.Interactive.List.ExtraActions = append(r.Interactive.List.ExtraActions, extras...)
	return r
}

// WithButtons attaches tappable action buttons to any Result (including plain
// text / empty states). Button channels render a ChoicesView; text-only channels
// see only the text. Use this for empty-state guidance buttons ("All commands ▸")
// where there is no list to attach ExtraActions to.
func WithButtons(r *Result, buttons ...ListItem) *Result {
	if r == nil || len(buttons) == 0 {
		return r
	}
	r.Interactive = &Interactive{
		Kind:    InteractiveChoices,
		Choices: &ChoicesView{Title: r.Text, Choices: buttons},
	}
	return r
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

// onOff returns "on" or "off" — preferred over boolStr for enable/active flags
// in compact list rows.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
