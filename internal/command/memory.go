package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildMemoryGroup() *CommandGroup {
	g := newCommandGroup("memory", "Manage memory provider")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all memory providers",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.memProvService.List(cc.Ctx)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No memory providers found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				def := ""
				if item.IsDefault {
					def = " (default)"
				}
				records = append(records, []kv{
					{"Name", item.Name + def},
					{"Provider", item.Provider},
				})
			}
			return formatItems(records), nil
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
			name := cc.Args[0]
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
					return fmt.Sprintf("Memory provider set to %q.", item.Name), nil
				}
			}
			return fmt.Sprintf("Memory provider %q not found.", name), nil
		},
	})
	return g
}
