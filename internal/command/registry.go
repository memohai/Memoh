package command

import (
	"context"
	"fmt"
	"strings"
)

// CommandContext carries execution context for a sub-command.
type CommandContext struct {
	Ctx   context.Context
	BotID string
	Role  string // "owner", "admin", "member", or "" (guest)
	Args  []string
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
		perm := ""
		if sub.IsWrite {
			perm = " [owner]"
		}
		fmt.Fprintf(&b, "- %s%s\n", sub.Usage, perm)
	}
	return b.String()
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
	b.WriteString("/help - Show this help message\n\n")
	for i, name := range r.order {
		if i > 0 {
			b.WriteByte('\n')
		}
		group := r.groups[name]
		b.WriteString(group.Usage())
	}
	return strings.TrimRight(b.String(), "\n")
}
