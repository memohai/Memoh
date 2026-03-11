package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildSearchGroup() *CommandGroup {
	g := newCommandGroup("search", "Manage search provider")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all search providers",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.searchProvService.List(cc.Ctx, "")
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No search providers found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				records = append(records, []kv{
					{"Name", item.Name},
					{"Provider", item.Provider},
				})
			}
			return formatItems(records), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <name> - Set the search provider for this bot",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /search set <name>", nil
			}
			name := cc.Args[0]
			items, err := h.searchProvService.List(cc.Ctx, "")
			if err != nil {
				return "", err
			}
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					_, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
						SearchProviderID: item.ID,
					})
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("Search provider set to %q.", item.Name), nil
				}
			}
			return fmt.Sprintf("Search provider %q not found.", name), nil
		},
	})
	return g
}
