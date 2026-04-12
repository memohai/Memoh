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
	Args              []string
	ChannelIdentityID string
	UserID            string
	ChannelType       string
	ConversationType  string
	ConversationID    string
	ThreadID          string
	RouteID           string
	SessionID         string
}

// SubCommand describes a single sub-command within a resource group.
type SubCommand struct {
	Name    string
	Usage   string
	IsWrite bool
	Handler func(cc CommandContext) (string, error)
}

// CommandGroup groups sub-commands under a resource name.
type CommandGroup struct {
	Name          string
	Description   string
	DefaultAction string
	commands      map[string]SubCommand
	order         []string // preserves registration order for help output
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
	fmt.Fprintf(&b, "/%s - %s\n", g.Name, g.Description)
	for _, name := range g.order {
		sub := g.commands[name]
		desc := subSummary(sub)
		if sub.IsWrite {
			desc += " [owner]"
		}
		fmt.Fprintf(&b, "- %s\n", desc)
	}
	fmt.Fprintf(&b, "\nUse /help %s <action> for details.", g.Name)
	return b.String()
}

func (g *CommandGroup) ActionHelp(action string) string {
	sub, ok := g.commands[action]
	if !ok {
		return fmt.Sprintf("Unknown action %q for /%s.\n\n%s", action, g.Name, g.Usage())
	}
	usage, summary := splitUsage(sub.Usage)
	var b strings.Builder
	fmt.Fprintf(&b, "/%s %s\n", g.Name, sub.Name)
	if summary != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", summary)
	}
	if usage == "" {
		usage = sub.Name
	}
	fmt.Fprintf(&b, "- Usage: /%s %s\n", g.Name, usage)
	if sub.IsWrite {
		b.WriteString("- Access: owner only\n")
	}
	fmt.Fprintf(&b, "- Tip: use /help %s to view sibling actions.", g.Name)
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

// GlobalHelp returns the top-level help text listing all commands.
func (r *Registry) GlobalHelp() string {
	var b strings.Builder
	b.WriteString("Available commands:\n\n")
	b.WriteString("/help - Show this help message\n")
	b.WriteString("/new - Start a new conversation (resets session context)\n")
	b.WriteString("/stop - Stop the current generation\n\n")
	for _, name := range r.order {
		group := r.groups[name]
		fmt.Fprintf(&b, "- /%s - %s\n", group.Name, group.Description)
	}
	b.WriteString("\nUse /help <group> to view actions, e.g. /help model")
	return strings.TrimRight(b.String(), "\n")
}

func (r *Registry) GroupHelp(name string) string {
	group, ok := r.groups[name]
	if !ok {
		return fmt.Sprintf("Unknown command group: /%s\n\n%s", name, r.GlobalHelp())
	}
	return group.Usage()
}

func (r *Registry) ActionHelp(groupName, action string) string {
	group, ok := r.groups[groupName]
	if !ok {
		return fmt.Sprintf("Unknown command group: /%s\n\n%s", groupName, r.GlobalHelp())
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

func subSummary(sub SubCommand) string {
	usage, summary := splitUsage(sub.Usage)
	if summary == "" {
		return usage
	}
	return fmt.Sprintf("%s - %s", sub.Name, summary)
}
