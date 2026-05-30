package command

import (
	"context"
	"fmt"
	"strings"
)

// CommandContext carries execution context for a sub-command.
type CommandContext struct {
	Ctx               context.Context
	BotID             string
	Role              string // "owner", "admin", "member", or "" (guest)
	WriteAccess       bool
	Args              []string
	ChannelIdentityID string
	UserID            string
	ChannelType       string
	ConversationType  string
	ConversationID    string
	ThreadID          string
	RouteID           string
	SessionID         string
	Page              int    // zero-based page offset for paginated list commands
	Prov              int    // provider index for the model picker (-1 if absent)
	Flat              int    // flat model index for picker selection (-1 if absent)
	Range             string // time-window key for time-series commands ("" = default)
}

// SubCommand describes a single sub-command within a resource group.
//
// A sub-command provides either Handler (plain text) or ResultHandler
// (structured Result for rich rendering). When both are set, ResultHandler
// takes precedence.
type SubCommand struct {
	Name          string
	Usage         string
	IsWrite       bool
	Handler       func(cc CommandContext) (string, error)
	ResultHandler func(cc CommandContext) (*Result, error)
}

// CommandGroup groups sub-commands under a resource name.
type CommandGroup struct {
	Name          string
	Description   string
	DefaultAction string
	actionMenu    bool // bare invocation shows sub-actions as buttons (see EnableActionMenu)
	commands      map[string]SubCommand
	order         []string // preserves registration order for help output
}

// EnableActionMenu makes a bare invocation of this group (e.g. /schedule) render
// its sub-actions as tappable buttons instead of running a default action — for
// groups with several actions worth discovering by tapping rather than typing.
func (g *CommandGroup) EnableActionMenu() { g.actionMenu = true }

// actionMenuResult builds the bare-invocation action menu: one button per
// sub-action (tap re-dispatches "/group action"), with the textual Usage as the
// fallback for non-button channels.
func (g *CommandGroup) actionMenuResult() *Result {
	title := MdBold("/"+g.Name) + " — " + g.Description + "\n\nChoose an action:"
	choices := make([]ListItem, 0, len(g.order))
	for _, name := range g.order {
		choices = append(choices, ListItem{
			Label:  name,
			Action: &ItemAction{Resource: g.Name, Action: name},
		})
	}
	return &Result{
		Text:        g.Usage(),
		Interactive: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{Title: title, Choices: choices}},
	}
}

func newCommandGroup(name, description string) *CommandGroup {
	return &CommandGroup{
		Name:        name,
		Description: description,
		commands:    make(map[string]SubCommand),
	}
}

func (g *CommandGroup) Register(sub SubCommand) {
	g.commands[sub.Name] = sub
	g.order = append(g.order, sub.Name)
}

// Usage returns the usage text for this resource group.
func (g *CommandGroup) Usage() string {
	var b strings.Builder
	b.WriteString(MdBold("/"+g.Name) + " — " + g.Description + "\n\n")
	for _, name := range g.order {
		sub := g.commands[name]
		_, summary := splitUsage(sub.Usage)
		line := CmdRef(g.Name + " " + sub.Name)
		if summary != "" {
			line += " — " + summary
		}
		if sub.IsWrite {
			line += " (owner)"
		}
		fmt.Fprintf(&b, "- %s\n", line)
	}
	fmt.Fprintf(&b, "\nRun %s for details.", CmdRef("help "+g.Name+" <action>"))
	return strings.TrimRight(b.String(), "\n")
}

func (g *CommandGroup) ActionHelp(action string) string {
	sub, ok := g.commands[action]
	if !ok {
		return fmt.Sprintf("Unknown action %s for %s. Run %s to see its actions.", MdCode(action), CmdRef(g.Name), CmdRef("help "+g.Name))
	}
	usage, summary := splitUsage(sub.Usage)
	var b strings.Builder
	b.WriteString(MdBold("/"+g.Name+" "+sub.Name) + "\n")
	if summary != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", summary)
	}
	if usage == "" {
		usage = sub.Name
	}
	fmt.Fprintf(&b, "- Usage: %s\n", CmdRef(g.Name+" "+usage))
	if sub.IsWrite {
		b.WriteString("- Access: owner only\n")
	}
	fmt.Fprintf(&b, "- See sibling actions: %s", CmdRef("help "+g.Name))
	return strings.TrimRight(b.String(), "\n")
}

// Registry holds all registered command groups.
type Registry struct {
	groups map[string]*CommandGroup
	order  []string
}

func newRegistry() *Registry {
	return &Registry{
		groups: make(map[string]*CommandGroup),
	}
}

func (r *Registry) RegisterGroup(group *CommandGroup) {
	r.groups[group.Name] = group
	r.order = append(r.order, group.Name)
}

// GlobalHelp returns the top-level help text listing all commands. Single-token
// commands are rendered as plain "/cmd" (not code spans) so Telegram linkifies
// them as tap-to-send; multi-word sub-actions stay tap-to-copy in GroupHelp.
func (r *Registry) GlobalHelp() string {
	var b strings.Builder
	b.WriteString(MdBold("Available commands") + "\n\n")
	b.WriteString("- /help — show this help\n")
	b.WriteString("- /new — start a new conversation\n")
	b.WriteString("- /stop — stop the current reply\n")
	for _, name := range r.order {
		group := r.groups[name]
		fmt.Fprintf(&b, "- /%s — %s\n", group.Name, group.Description)
	}
	b.WriteString("\nTap any command to run it. For a group's actions, send /help <group> — e.g. /help model.")
	return strings.TrimRight(b.String(), "\n")
}

func (r *Registry) GroupHelp(name string) string {
	group, ok := r.groups[name]
	if !ok {
		return fmt.Sprintf("Unknown command %s. Run %s to see all commands.", CmdRef(name), CmdRef("help"))
	}
	return group.Usage()
}

func (r *Registry) ActionHelp(groupName, action string) string {
	group, ok := r.groups[groupName]
	if !ok {
		return fmt.Sprintf("Unknown command %s. Run %s to see all commands.", CmdRef(groupName), CmdRef("help"))
	}
	return group.ActionHelp(action)
}

func splitUsage(usage string) (commandUsage string, summary string) {
	usage = strings.TrimSpace(usage)
	if usage == "" {
		return "", ""
	}
	parts := strings.SplitN(usage, " - ", 2)
	commandUsage = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		summary = strings.TrimSpace(parts[1])
	}
	return commandUsage, summary
}
