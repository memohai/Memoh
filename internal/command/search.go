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
			if h.searchProvService == nil {
				return "Search provider service is not available.", nil
			}
			items, err := h.searchProvService.List(cc.Ctx, "")
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No search providers found.", nil
			}
			settingsResp, _ := h.getBotSettings(cc)
			currentRecords := make([][]kv, 0, 1)
			otherRecords := make([][]kv, 0, len(items))
			for _, item := range items {
				label := item.Name
				record := []kv{
					{"Name", label},
					{"Provider", item.Provider},
				}
				if item.ID == settingsResp.SearchProviderID {
					label += " [current]"
					record[0].value = label
					currentRecords = append(currentRecords, record)
					continue
				}
				otherRecords = append(otherRecords, record)
			}
			currentRecords = append(currentRecords, otherRecords...)
			records := currentRecords
			return formatLimitedItems(records, defaultListLimit, "Use /search current to inspect the active provider."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "current",
		Usage: "current - Show the current search provider",
		Handler: func(cc CommandContext) (string, error) {
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			settingsResp, err := h.getBotSettings(cc)
			if err != nil {
				return "", err
			}
			return formatKV([]kv{{"Search Provider", h.resolveSearchProviderName(cc, settingsResp.SearchProviderID)}}), nil
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
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			name := cc.Args[0]
			before, _ := h.getBotSettings(cc)
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
					return formatChangedValue("Search provider", h.resolveSearchProviderName(cc, before.SearchProviderID), item.Name), nil
				}
			}
			return fmt.Sprintf("Search provider %q not found.", name), nil
		},
	})
	return g
}
