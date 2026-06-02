package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/i18n"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

// reasoningChoices are the selectable reasoning levels. "off" disables thinking;
// the rest enable it at that effort.
var reasoningChoices = []string{
	"off",
	models.ReasoningEffortNone,
	models.ReasoningEffortLow,
	models.ReasoningEffortMedium,
	models.ReasoningEffortHigh,
	models.ReasoningEffortXHigh,
}

func validEffort(v string) bool {
	switch v {
	case models.ReasoningEffortNone, models.ReasoningEffortLow, models.ReasoningEffortMedium, models.ReasoningEffortHigh, models.ReasoningEffortXHigh:
		return true
	}
	return false
}

// buildReasoningGroup registers /reasoning — a first-class sibling of /model.
// Aliases /reason, /effort, /think all resolve here (see resourceAliases). It
// shows the current reasoning level and lets the user pick the reasoning effort
// in one tap, reusing settingsService.UpsertBot (no backend changes).
func (h *Handler) buildReasoningGroup() *CommandGroup {
	g := newCommandGroup("reasoning", "View or set reasoning level")
	g.DefaultAction = "show"
	g.Register(SubCommand{
		Name:  "show",
		Usage: "show - Show the reasoning level and pick a new one",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			s, err := h.getBotSettings(cc)
			if err != nil {
				return nil, err
			}
			return reasoningResult(cc.L, s.ReasoningEnabled, s.ReasoningEffort), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <off|none|low|medium|high|xhigh> - Set the reasoning level",
		IsWrite: true,
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if len(cc.Args) < 1 {
				return &Result{Text: cc.T("cmd.reasoning.setUsage")}, nil
			}
			level := strings.ToLower(strings.TrimSpace(cc.Args[0]))
			if h.settingsService == nil {
				return &Result{Text: cc.T("cmd.reasoning.unavailable")}, nil
			}
			req := settings.UpsertRequest{}
			switch {
			case level == "off":
				off := false
				req.ReasoningEnabled = &off
			case validEffort(level):
				on := true
				req.ReasoningEnabled = &on
				req.ReasoningEffort = &level
			default:
				return &Result{Text: cc.T("cmd.reasoning.unknownLevel", map[string]any{"level": fmt.Sprintf("%q", cc.Args[0])})}, nil
			}
			if _, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, req); err != nil {
				return nil, err
			}
			s, err := h.getBotSettings(cc)
			if err != nil {
				return nil, err
			}
			return reasoningResult(cc.L, s.ReasoningEnabled, s.ReasoningEffort), nil
		},
	})
	return g
}

// reasoningResult builds the picker: a header with the current level plus one
// button per level (current marked ✓). Tapping re-dispatches "/reasoning set X"
// which edits the message in place. Level tokens (off/none/low/…) are canonical
// args and stay untranslated; only the surrounding prose is localized via t.
func reasoningResult(t *i18n.Localizer, enabled bool, effort string) *Result {
	effort = strings.ToLower(strings.TrimSpace(effort))
	current := t.T("cmd.common.off")
	if enabled {
		current = effort
		if current == "" {
			current = t.T("cmd.common.on")
		}
	}
	header := MdBold(t.T("cmd.reasoning.header")) + "\n" + t.T("cmd.reasoning.current", map[string]any{"level": current})
	choices := make([]ListItem, 0, len(reasoningChoices))
	for _, lvl := range reasoningChoices {
		selected := false
		if lvl == "off" {
			selected = !enabled
		} else {
			selected = enabled && lvl == effort
		}
		choices = append(choices, ListItem{
			Label:    lvl,
			Selected: selected,
			Action:   &ItemAction{Resource: "reasoning", Action: "set", Args: []string{lvl}},
		})
	}
	// Button channels see header + "Choose a level:" via Choices.Title; no-button
	// channels see header alone — the renderer appends the auto-derived
	// "Pick with /reasoning set <off|none|low|medium|high|xhigh>." trailer so the
	// level list and the typeable form arrive together without manual baking.
	buttonTitle := header + "\n\n" + t.T("cmd.reasoning.tapPrompt")
	return &Result{
		Text: header,
		Interactive: &Interactive{
			Kind:    InteractiveChoices,
			Choices: &ChoicesView{Title: buttonTitle, Choices: choices},
		},
	}
}
