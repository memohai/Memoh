package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

// reasoningChoices are the selectable reasoning levels. "off" disables thinking;
// the rest enable it at that effort. (The effort ladder is low/medium/high in
// the current backend.)
var reasoningChoices = []string{"off", "low", "medium", "high"}

func validEffort(v string) bool {
	switch v {
	case "low", "medium", "high":
		return true
	}
	return false
}

// buildReasoningGroup registers /reasoning — a first-class sibling of /model.
// Aliases /reason, /effort, /think all resolve here (see resourceAliases). It
// shows the current reasoning level and lets the user pick off/low/medium/high
// in one tap, reusing settingsService.UpsertBot (no backend changes).
func (h *Handler) buildReasoningGroup() *CommandGroup {
	g := newCommandGroup("reasoning", "View or set reasoning level (off/low/medium/high)")
	g.DefaultAction = "show"
	g.Register(SubCommand{
		Name:  "show",
		Usage: "show - Show the reasoning level and pick a new one",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			s, err := h.getBotSettings(cc)
			if err != nil {
				return nil, err
			}
			return reasoningResult(s.ReasoningEnabled, s.ReasoningEffort), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <off|low|medium|high> - Set the reasoning level",
		IsWrite: true,
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if len(cc.Args) < 1 {
				return &Result{Text: "Usage: /reasoning set <off|low|medium|high>"}, nil
			}
			level := strings.ToLower(strings.TrimSpace(cc.Args[0]))
			if h.settingsService == nil {
				return &Result{Text: "Reasoning isn't available right now."}, nil
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
				return &Result{Text: fmt.Sprintf("Unknown level %q — choose off, low, medium, or high.", cc.Args[0])}, nil
			}
			if _, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, req); err != nil {
				return nil, err
			}
			s, err := h.getBotSettings(cc)
			if err != nil {
				return nil, err
			}
			return reasoningResult(s.ReasoningEnabled, s.ReasoningEffort), nil
		},
	})
	return g
}

// reasoningResult builds the picker: a header with the current level plus one
// button per level (current marked ✓). Tapping re-dispatches "/reasoning set X"
// which edits the message in place.
func reasoningResult(enabled bool, effort string) *Result {
	effort = strings.ToLower(strings.TrimSpace(effort))
	current := "off"
	if enabled {
		current = effort
		if current == "" {
			current = "on"
		}
	}
	header := MdBold("🧠 Reasoning") + "\nCurrent: " + current
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
	// Button channels get the tradeoff + a tap prompt; text-only channels get a
	// copyable set command instead of a list that just restates the buttons.
	buttonTitle := header + "\n\nHigher effort means more careful thinking, but slower replies. Tap a level:"
	fallback := header + "\n\nLevels: " + strings.Join(reasoningChoices, " · ") + ".\nSet with " + CmdRef("reasoning set <level>") + "."
	return &Result{
		Text: fallback,
		Interactive: &Interactive{
			Kind:    InteractiveChoices,
			Choices: &ChoicesView{Title: buttonTitle, Choices: choices},
		},
	}
}
