package command

import (
	"strings"

	"github.com/memohai/memoh/internal/i18n"
)

// Hint verbs name the shape of a typeable affordance when rendering the
// no-button fallback trailer. Setting ItemAction.Verb or ListView.HintVerb to
// one of these overrides the automatic inference in FallbackTrailer.
//
// The values double as the suffix of the i18n key (cmd.fallback.<verb>) so
// adding a verb is a two-step change: register the constant here and add the
// matching localized template in locales/*.json.
const (
	HintVerbSwitch  = "switch"
	HintVerbPick    = "pick"
	HintVerbToggle  = "toggle"
	HintVerbOpen    = "open"
	HintVerbDetails = "details"
	HintVerbRange   = "range"
	HintVerbMenu    = "menu"
)

// Typeable renders an ItemAction as the slash command a user would type to
// invoke it (e.g. "/memory set Alice"). Nil-safe.
//
// Unlike ParsedCallback.SyntheticCommand, this is designed for display in hint
// text — it does not append --page artifacts and does not round-trip through
// the callback encoder.
func (a *ItemAction) Typeable() string {
	if a == nil {
		return ""
	}
	resource := strings.TrimSpace(a.Resource)
	action := strings.TrimSpace(a.Action)
	if resource == "" || action == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("/")
	b.WriteString(resource)
	b.WriteString(" ")
	b.WriteString(action)
	for _, arg := range a.Args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		b.WriteString(" ")
		b.WriteString(arg)
	}
	return b.String()
}

// FallbackTrailer derives a human-readable list of typeable commands from an
// Interactive payload, intended for appending to Result.Text on channels that
// cannot render buttons. Returns "" when the payload offers no typeable
// affordance worth surfacing (display-only list with no extras, suppressed
// choices, nil view).
//
// The returned string carries Markdown markup (backticks via MdCode); the
// renderer's applyMessageFormat strips it for plain-text channels.
func FallbackTrailer(iv *Interactive, t *i18n.Localizer) string {
	if iv == nil {
		return ""
	}
	switch iv.Kind {
	case InteractiveList:
		return trailerForList(iv.List, t)
	case InteractiveChoices:
		return trailerForChoices(iv.Choices, t)
	case InteractiveModelPicker:
		return trailerForPicker(iv.Picker, t)
	case InteractiveRange:
		return trailerForRange(iv.Range, t)
	}
	return ""
}

func trailerForList(lv *ListView, t *i18n.Localizer) string {
	if lv == nil {
		return ""
	}

	// Explicit list-level override.
	if v := strings.TrimSpace(lv.HintVerb); v != "" {
		return listOverrideTrailer(lv, v, t)
	}

	// Walk actionable rows. Detect homogeneity by (Resource, Action) so a
	// memory/search/model-style list (every row's tap means "switch to this
	// one") collapses into a single switch line rather than enumerating every
	// row's typeable form.
	var actionable []*ItemAction
	resource, action := "", ""
	homogeneous := true
	for _, item := range lv.Items {
		if item.Action == nil {
			continue
		}
		if len(actionable) == 0 {
			resource = item.Action.Resource
			action = item.Action.Action
		} else if item.Action.Resource != resource || item.Action.Action != action {
			homogeneous = false
		}
		actionable = append(actionable, item.Action)
	}

	if len(actionable) > 0 {
		// Row-level Verb override applies when set on the first actionable row.
		if verb := strings.TrimSpace(actionable[0].Verb); verb != "" {
			return verbLine(verb, actionable, t)
		}
		if homogeneous {
			return t.T("cmd.fallback.switch", map[string]any{
				"command": MdCode("/" + resource + " " + action + " <name>"),
			})
		}
		// Heterogeneous actionable rows: list each typeable.
		return t.T("cmd.fallback.open", map[string]any{
			"commands": joinActionCmds(actionable),
		})
	}

	// No row-level actions; surface cross-nav extras if any.
	var extras []*ItemAction
	for _, ea := range lv.ExtraActions {
		if ea.Action != nil {
			extras = append(extras, ea.Action)
		}
	}
	if len(extras) > 0 {
		return t.T("cmd.fallback.open", map[string]any{
			"commands": joinActionCmds(extras),
		})
	}

	return ""
}

func listOverrideTrailer(lv *ListView, verb string, t *i18n.Localizer) string {
	switch verb {
	case HintVerbDetails:
		// Convention: list/get-paired groups (mcp, schedule, …) — the typeable
		// target is "/<Resource> get <name>".
		return t.T("cmd.fallback.details", map[string]any{
			"command": MdCode("/" + strings.TrimSpace(lv.Resource) + " get <name>"),
		})
	}
	return ""
}

func trailerForChoices(cv *ChoicesView, t *i18n.Localizer) string {
	if cv == nil || cv.SuppressFallback {
		return ""
	}
	var actionable []*ItemAction
	for _, ch := range cv.Choices {
		if ch.Action != nil {
			actionable = append(actionable, ch.Action)
		}
	}
	if len(actionable) == 0 {
		return ""
	}

	// Homogeneity by (Resource, Action). A homogeneous choice set is either a
	// pick (args are plain enum values like "low") or a flag-bearing toggle
	// (args have leading dashes like "--heartbeat_enabled true").
	resource := actionable[0].Resource
	action := actionable[0].Action
	homogeneous := true
	for _, a := range actionable[1:] {
		if a.Resource != resource || a.Action != action {
			homogeneous = false
			break
		}
	}

	if homogeneous {
		if verb := strings.TrimSpace(actionable[0].Verb); verb != "" {
			return verbLine(verb, actionable, t)
		}
		if isPickShape(actionable) {
			if hasAnyArgs(actionable) {
				return t.T("cmd.fallback.pick", map[string]any{
					"command": MdCode("/" + resource + " " + action + " " + pickValueClause(actionable)),
				})
			}
			// Single no-arg target (e.g. /mcp empty's "All commands ▸" → /help mcp):
			// surface it as a direct menu nudge rather than a templated pick.
			return t.T("cmd.fallback.menu", map[string]any{
				"command": MdCode(actionable[0].Typeable()),
			})
		}
		return t.T("cmd.fallback.toggle", map[string]any{
			"command": joinActionCmds(actionable),
		})
	}

	// Heterogeneous: list every typeable as a cross-nav opener.
	return t.T("cmd.fallback.open", map[string]any{
		"commands": joinActionCmds(actionable),
	})
}

// isPickShape reports whether the actions look like a value-pick (args are
// plain enum tokens) rather than a toggle (args carry leading-dash flags).
func isPickShape(actions []*ItemAction) bool {
	for _, a := range actions {
		for _, arg := range a.Args {
			if strings.HasPrefix(strings.TrimSpace(arg), "-") {
				return false
			}
		}
	}
	return true
}

// hasAnyArgs reports whether any of the actions carry at least one non-blank
// arg. A no-args homogeneous group renders as a direct menu nudge rather than
// a "<value>" template.
func hasAnyArgs(actions []*ItemAction) bool {
	for _, a := range actions {
		for _, arg := range a.Args {
			if strings.TrimSpace(arg) != "" {
				return true
			}
		}
	}
	return false
}

// pickValueClause builds the "<v1|v2|v3>" enumeration clause from a pick-shape
// choice set's first args, or "<value>" when no usable values are available.
// Duplicates are removed in encounter order so the catalog reads cleanly.
func pickValueClause(actions []*ItemAction) string {
	seen := make(map[string]bool, len(actions))
	var values []string
	for _, a := range actions {
		if len(a.Args) == 0 {
			continue
		}
		v := strings.TrimSpace(a.Args[0])
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		values = append(values, v)
	}
	if len(values) == 0 {
		return "<value>"
	}
	return "<" + strings.Join(values, "|") + ">"
}

func trailerForPicker(p *ModelPickerView, t *i18n.Localizer) string {
	if p == nil {
		return ""
	}
	switch p.Level {
	case LevelProviders:
		return t.T("cmd.fallback.menu", map[string]any{
			"command": MdCode("/model list <provider_name>"),
		})
	case LevelModels:
		return t.T("cmd.fallback.menu", map[string]any{
			"command": MdCode("/model set <name>"),
		})
	}
	return ""
}

func trailerForRange(rv *RangeView, t *i18n.Localizer) string {
	if rv == nil {
		return ""
	}
	resource := strings.TrimSpace(rv.Resource)
	action := strings.TrimSpace(rv.Action)
	if resource == "" || action == "" {
		return ""
	}
	return t.T("cmd.fallback.range", map[string]any{
		"command": MdCode("/" + resource + " " + action + " --range <preset>"),
		"presets": strings.Join(rv.Presets, " · "),
	})
}

func verbLine(verb string, actions []*ItemAction, t *i18n.Localizer) string {
	if len(actions) == 0 {
		return ""
	}
	switch verb {
	case HintVerbSwitch, HintVerbPick, HintVerbToggle, HintVerbDetails, HintVerbMenu, HintVerbRange:
		cmd := actions[0].Typeable()
		if cmd == "" {
			return ""
		}
		return t.T("cmd.fallback."+verb, map[string]any{"command": MdCode(cmd)})
	case HintVerbOpen:
		return t.T("cmd.fallback.open", map[string]any{"commands": joinActionCmds(actions)})
	}
	return ""
}

func joinActionCmds(actions []*ItemAction) string {
	parts := make([]string, 0, len(actions))
	for _, a := range actions {
		if cmd := a.Typeable(); cmd != "" {
			parts = append(parts, MdCode(cmd))
		}
	}
	// One-per-line for the "open" / heterogeneous trailer. A `·` separator
	// collapses into an unreadable wall on plain-text channels where each
	// command is many characters long (e.g. /settings's 7 cross-nav targets).
	// The trailer is only ever shown on no-button channels, so the extra
	// vertical space costs nothing on Telegram.
	if len(parts) <= 1 {
		return strings.Join(parts, "")
	}
	return "\n- " + strings.Join(parts, "\n- ")
}
