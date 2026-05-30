package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

// providerListRecord builds a compact provider row: the name as the label, then
// chips for current/default and the engine slug — but the engine chip is shown
// only when it adds information the name doesn't already convey, so e.g.
// "Built-in Memory" does not get a redundant "builtin" chip.
func providerListRecord(name, provider string, isDefault, isCurrent bool) listRecord {
	fields := []kv{{"Name", name}}
	if isCurrent {
		fields = append(fields, kv{"", "current"})
	}
	if isDefault {
		fields = append(fields, kv{"", "default"})
	}
	if engine := distinctProviderEngine(name, provider); engine != "" {
		fields = append(fields, kv{"", engine})
	}
	return listRecord{selected: isCurrent, fields: fields}
}

// distinctProviderEngine returns the provider engine slug only when it is not
// already implied by the name (comparing alphanumerics only); otherwise "".
func distinctProviderEngine(name, provider string) string {
	p := strings.TrimSpace(provider)
	if p == "" {
		return ""
	}
	if n, pn := alnumLower(name), alnumLower(p); pn == "" || strings.Contains(n, pn) {
		return ""
	}
	return p
}

func alnumLower(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (h *Handler) buildMemoryGroup() *CommandGroup {
	g := newCommandGroup("memory", "Manage memory provider")
	g.DefaultAction = "list" // bare /memory lands on the provider list (current marked)
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all memory providers",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.memProvService == nil {
				return &Result{Text: "Memory isn't available right now."}, nil
			}
			items, err := h.memProvService.List(cc.Ctx)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No memory providers yet.\n\nMemory providers give the bot long-term memory across conversations. Add one in the web dashboard."}, nil
			}
			settingsResp, _ := h.getBotSettings(cc)
			currentRecords := make([]listRecord, 0, 1)
			otherRecords := make([]listRecord, 0, len(items))
			for _, item := range items {
				rec := providerListRecord(item.Name, item.Provider, item.IsDefault, item.ID == settingsResp.MemoryProviderID)
				// Tap a provider to switch to it — no typing of /memory set.
				rec.action = &ItemAction{Resource: "memory", Action: "set", Args: []string{item.Name}}
				if item.ID == settingsResp.MemoryProviderID {
					currentRecords = append(currentRecords, rec)
					continue
				}
				otherRecords = append(otherRecords, rec)
			}
			currentRecords = append(currentRecords, otherRecords...)
			return buildListResult("Memory Providers", "memory", "list", nil, currentRecords, cc.Page, defaultListLimit, "Switch with "+CmdRef("memory set <name>")+"."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "current",
		Usage: "current - Show the current memory provider",
		Handler: func(cc CommandContext) (string, error) {
			if h.settingsService == nil {
				return "Memory isn't available right now.", nil
			}
			settingsResp, err := h.getBotSettings(cc)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(settingsResp.MemoryProviderID) == "" {
				return "No memory provider is set. See options with " + CmdRef("memory list") + ", then choose one with " + CmdRef("memory set <name>") + ".", nil
			}
			return "Active memory provider: " + h.resolveMemoryProviderName(cc, settingsResp.MemoryProviderID), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <name> - Set the memory provider for this bot",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /memory set <name>", nil
			}
			if h.settingsService == nil {
				return "Memory isn't available right now.", nil
			}
			name := cc.Args[0]
			before, _ := h.getBotSettings(cc)
			items, err := h.memProvService.List(cc.Ctx)
			if err != nil {
				return "", err
			}
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					_, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
						MemoryProviderID: item.ID,
					})
					if err != nil {
						return "", err
					}
					return formatChangedValue("Memory provider", h.resolveMemoryProviderName(cc, before.MemoryProviderID), item.Name), nil
				}
			}
			return fmt.Sprintf("No memory provider named %q. See options with %s.", name, CmdRef("memory list")), nil
		},
	})
	return g
}
