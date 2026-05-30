package inbound

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/command"
)

const actionTypeCallback = "callback"

// friendlyOps renders a blame-free, recovery-oriented line for an operational
// failure on a user-command path. The raw cause belongs in logs; the user never
// sees infra nouns ("route resolver", "session service", etc.).
func friendlyOps(verb string) string {
	return "⚠️ Couldn't " + verb + " right now. Try again in a moment."
}

// formatNewSessionMessage builds the /new confirmation card. A fresh start is an
// orientation moment, so it confirms the full setup the user is departing with —
// which model (and its provider) will answer, whether reasoning is on, and how
// much context budget they have. These are not "defaults to hide"; on this
// surface they reassure and inform. Markdown markers are authored unconditionally
// and stripped later for non-markdown channels.
func formatNewSessionMessage(modeLabel string, cc command.CurrentContext) string {
	reasoning := "off"
	if cc.ReasoningEnabled {
		reasoning = strings.TrimSpace(cc.ReasoningEffort)
		if reasoning == "" {
			reasoning = "on"
		}
	}
	var b strings.Builder
	b.WriteString(command.MdBold(fmt.Sprintf("✨ New %s started.", modeLabel)))
	// Display names / enums read better as plain text than monospace.
	fmt.Fprintf(&b, "\n\n- Model: %s", cc.ChatModel)
	if hb := strings.TrimSpace(cc.HeartbeatModel); hb != "" && hb != "(none)" {
		fmt.Fprintf(&b, "\n- Heartbeat: %s", hb)
	}
	fmt.Fprintf(&b, "\n- Reasoning: %s", reasoning)
	if cw := strings.TrimSpace(cc.ContextWindow); cw != "" {
		fmt.Fprintf(&b, "\n- Context: %s tokens", cw)
	}
	// One guiding line: how to change the setup they're departing with.
	fmt.Fprintf(&b, "\n\nTip: adjust anytime with %s or %s.", command.CmdRef("model"), command.CmdRef("reasoning"))
	return b.String()
}

// renderResult converts a neutral command.Result into a channel.Message,
// upgrading to interactive inline-keyboard buttons when the channel advertises
// button support. Channels without button support (or results without
// structured data) degrade to the complete fallback Text. The final message
// format (Markdown vs Plain) is decided once, capability-gated.
func renderResult(result *command.Result, caps channel.ChannelCapabilities) channel.Message {
	if result == nil {
		return channel.Message{}
	}
	var msg channel.Message
	if result.Interactive == nil || !caps.Buttons {
		msg = channel.Message{Text: result.Text}
	} else {
		switch result.Interactive.Kind {
		case command.InteractiveList:
			msg = renderListView(result.Text, result.Interactive.List)
		case command.InteractiveModelPicker:
			msg = renderModelPicker(result.Interactive.Picker)
		case command.InteractiveChoices:
			msg = renderChoicesView(result.Interactive.Choices)
		case command.InteractiveRange:
			msg = renderRangeView(result.Text, result.Interactive.Range)
		default:
			msg = channel.Message{Text: result.Text}
		}
	}
	return applyMessageFormat(msg, caps)
}

// applyMessageFormat sets the message format from the channel's capabilities:
// Markdown when supported (Telegram renders it, others degrade client-side),
// otherwise the inline markup authored upstream is stripped so text-only
// channels stay clean. This is the single place command-reply format is decided.
func applyMessageFormat(msg channel.Message, caps channel.ChannelCapabilities) channel.Message {
	if caps.Markdown || caps.RichText {
		msg.Format = channel.MessageFormatMarkdown
	} else {
		msg.Text = stripInlineMarkup(msg.Text)
	}
	return msg
}

// stripInlineMarkup removes the inline Markdown markers (** and `) authored for
// capable channels, leaving clean text for plain-text-only channels.
func stripInlineMarkup(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

// renderListView renders a paginated list. The list content lives in the
// message text; buttons are added only for navigation (Prev/Next + Close) when
// there is more than one page, or for rows that carry an explicit ItemAction.
// A single-page, action-free list renders as plain text (no keyboard), matching
// prior behavior.
func renderListView(text string, lv *command.ListView) channel.Message {
	msg := channel.Message{Text: text}
	if lv == nil {
		return msg
	}

	pageSize := lv.PageSize
	if pageSize <= 0 {
		pageSize = 12
	}
	totalPages := 1
	if pageSize > 0 {
		totalPages = (lv.Total + pageSize - 1) / pageSize
	}

	var actions []channel.Action
	row := 0

	for _, item := range lv.Items {
		if item.Action == nil {
			continue
		}
		label := item.Label
		if item.Selected {
			label = "✓ " + label
		}
		actions = append(actions, channel.Action{
			Type:  actionTypeCallback,
			Label: label,
			Value: command.EncodeListCallback(item.Action.Resource, item.Action.Action, item.Action.Args, 0),
			Row:   row,
		})
		row++
	}

	if totalPages <= 1 && len(actions) == 0 {
		return msg
	}

	if totalPages > 1 {
		navRow := row
		if lv.Page > 0 {
			actions = append(actions, channel.Action{
				Type:  actionTypeCallback,
				Label: "◀ Prev",
				Value: command.EncodeListCallback(lv.Resource, lv.Action, lv.Args, lv.Page-1),
				Row:   navRow,
			})
		}
		actions = append(actions, channel.Action{
			Type:  actionTypeCallback,
			Label: fmt.Sprintf("%d/%d", lv.Page+1, totalPages),
			Value: command.NoopCallback(),
			Row:   navRow,
		})
		if lv.Page < totalPages-1 {
			actions = append(actions, channel.Action{
				Type:  actionTypeCallback,
				Label: "Next ▶",
				Value: command.EncodeListCallback(lv.Resource, lv.Action, lv.Args, lv.Page+1),
				Row:   navRow,
			})
		}
		row++
	}

	actions = append(actions, channel.Action{
		Type:  actionTypeCallback,
		Label: "✕ Close",
		Value: command.DismissCallback(),
		Row:   row,
	})

	msg.Actions = actions
	return msg
}

// renderModelPicker renders the two-level model picker. On button channels the
// message body is a compact status header (current model + reasoning, or the
// provider + page range) — not the flat fallback list. The provider level shows
// a 2-column grid (● marks the provider holding the current model, with its
// model count); the model level shows one model per row (✓ marks the selected
// model) with a back button. Both levels paginate and carry a Close button.
func renderModelPicker(p *command.ModelPickerView) channel.Message {
	if p == nil {
		return channel.Message{}
	}
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = 8
	}
	totalPages := 1
	if p.Total > 0 {
		totalPages = (p.Total + pageSize - 1) / pageSize
	}
	page := p.Page
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * pageSize
	end := start + pageSize

	msg := channel.Message{Text: modelPickerHeader(p, start, end)}

	var actions []channel.Action
	row := 0

	switch p.Level {
	case command.LevelProviders:
		if end > len(p.Providers) {
			end = len(p.Providers)
		}
		col := 0
		for i := start; i < end; i++ {
			prov := p.Providers[i]
			label := fmt.Sprintf("%s (%d)", prov.Name, prov.Count)
			if prov.HasCurrent {
				label += " ●"
			}
			actions = append(actions, channel.Action{
				Type:  actionTypeCallback,
				Label: truncateButtonLabel(label),
				Value: command.EncodeModelProviderCallback(prov.Index, 0),
				Row:   row,
			})
			col++
			if col == 2 {
				col = 0
				row++
			}
		}
		if col != 0 {
			row++
		}
	case command.LevelModels:
		if end > len(p.Models) {
			end = len(p.Models)
		}
		for i := start; i < end; i++ {
			m := p.Models[i]
			label := m.Name
			if m.Selected {
				label = "✓ " + label
			}
			actions = append(actions, channel.Action{
				Type:  actionTypeCallback,
				Label: truncateButtonLabel(label),
				Value: command.EncodeModelSelectCallback(m.FlatIndex),
				Row:   row,
			})
			row++
		}
	}

	if totalPages > 1 {
		navRow := row
		if page > 0 {
			actions = append(actions, channel.Action{
				Type: actionTypeCallback, Label: "◀ Prev",
				Value: pickerPageCallback(p, page-1), Row: navRow,
			})
		}
		actions = append(actions, channel.Action{
			Type: actionTypeCallback, Label: fmt.Sprintf("%d/%d", page+1, totalPages),
			Value: command.NoopCallback(), Row: navRow,
		})
		if page < totalPages-1 {
			actions = append(actions, channel.Action{
				Type: actionTypeCallback, Label: "Next ▶",
				Value: pickerPageCallback(p, page+1), Row: navRow,
			})
		}
		row++
	}

	if p.Level == command.LevelModels {
		actions = append(actions, channel.Action{
			Type: actionTypeCallback, Label: "◀ Providers",
			Value: command.EncodeListCallback("model", "list", nil, 0), Row: row,
		})
		row++
	}
	actions = append(actions, channel.Action{
		Type: actionTypeCallback, Label: "✕ Close",
		Value: command.DismissCallback(), Row: row,
	})

	msg.Actions = actions
	return msg
}

// modelPickerHeader builds the compact status header shown above the keyboard.
func modelPickerHeader(p *command.ModelPickerView, start, end int) string {
	var b strings.Builder
	b.WriteString(command.MdBold("⚙ Model Configuration") + "\n\n")
	if p.Level == command.LevelModels {
		provider := p.ProviderName
		if provider == "" {
			provider = "models"
		}
		if p.Total > 0 && end > p.Total {
			end = p.Total
		}
		if p.Total > end-start {
			fmt.Fprintf(&b, "Provider: %s (%d–%d of %d)\n\nSelect a model:", provider, start+1, end, p.Total)
		} else {
			fmt.Fprintf(&b, "Provider: %s\n\nSelect a model:", provider)
		}
		return b.String()
	}
	current := p.CurrentDisplay
	if strings.TrimSpace(current) == "" {
		current = "(none)"
	}
	// Display names / enums read better plain than monospace.
	fmt.Fprintf(&b, "Current model: %s\n", current)
	if r := strings.TrimSpace(p.Reasoning); r != "" {
		fmt.Fprintf(&b, "Reasoning: %s\n", r)
	}
	b.WriteString("\nSelect a provider:")
	return b.String()
}

// pickerPageCallback builds the callback_data for paginating within the current
// picker level.
func pickerPageCallback(p *command.ModelPickerView, page int) string {
	if p.Level == command.LevelModels {
		return command.EncodeModelProviderCallback(p.ProviderIndex, page)
	}
	return command.EncodeListCallback("model", "list", nil, page)
}

// truncateButtonLabel keeps inline-keyboard labels within Telegram's practical
// length so long model names don't overflow the button.
func truncateButtonLabel(s string) string {
	const maxLen = 60
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-1]) + "…"
}

// renderChoicesView renders a flat set of one-tap choices (e.g. /effort levels,
// /think on/off, /settings toggles): choice buttons laid out 2 per row (current
// marked ✓) plus a Close row. Each tap re-dispatches "/{resource} {action}
// {args}" and edits in place.
func renderChoicesView(cv *command.ChoicesView) channel.Message {
	if cv == nil {
		return channel.Message{}
	}
	msg := channel.Message{Text: cv.Title}
	var actions []channel.Action
	col, row := 0, 0
	for _, item := range cv.Choices {
		if item.Action == nil {
			continue
		}
		label := item.Label
		if item.Selected {
			label = "✓ " + label
		}
		actions = append(actions, channel.Action{
			Type:  actionTypeCallback,
			Label: label,
			Value: command.EncodeListCallback(item.Action.Resource, item.Action.Action, item.Action.Args, 0),
			Row:   row,
		})
		col++
		if col == 2 {
			col = 0
			row++
		}
	}
	if col != 0 {
		row++
	}
	actions = append(actions, channel.Action{
		Type: actionTypeCallback, Label: "✕ Close", Value: command.DismissCallback(), Row: row,
	})
	msg.Actions = actions
	return msg
}

// renderRangeView renders a time-window selector: one row of preset buttons
// (the active preset marked ●) plus Close. Tapping a preset re-runs the command
// with that --range and edits the message in place.
func renderRangeView(text string, rv *command.RangeView) channel.Message {
	msg := channel.Message{Text: text}
	if rv == nil {
		return msg
	}
	var actions []channel.Action
	for _, preset := range rv.Presets {
		label := rangePresetLabel(preset)
		if preset == rv.Current {
			label += " ●"
		}
		actions = append(actions, channel.Action{
			Type:  actionTypeCallback,
			Label: label,
			Value: command.EncodeRangeCallback(rv.Resource, rv.Action, preset),
			Row:   0,
		})
	}
	actions = append(actions, channel.Action{
		Type: actionTypeCallback, Label: "✕ Close",
		Value: command.DismissCallback(), Row: 1,
	})
	msg.Actions = actions
	return msg
}

func rangePresetLabel(preset string) string {
	if preset == "all" {
		return "All"
	}
	return preset
}
