package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildSearchGroup() *CommandGroup {
	g := newCommandGroup("search", "Manage search provider")
	g.DefaultAction = "list" // bare /search lands on the provider list (current marked)
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all search providers",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.searchProvService == nil {
				return &Result{Text: "Search isn't available right now."}, nil
			}
			items, err := h.searchProvService.List(cc.Ctx, "")
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No search providers yet.\n\nSearch providers let the bot look things up on the web. Add one in the web dashboard."}, nil
			}
			settingsResp, _ := h.getBotSettings(cc)
			currentRecords := make([]listRecord, 0, 1)
			otherRecords := make([]listRecord, 0, len(items))
			for _, item := range items {
				rec := providerListRecord(item.Name, item.Provider, false, item.ID == settingsResp.SearchProviderID)
				if item.ID == settingsResp.SearchProviderID {
					currentRecords = append(currentRecords, rec)
					continue
				}
				otherRecords = append(otherRecords, rec)
			}
			currentRecords = append(currentRecords, otherRecords...)
			return buildListResult("Search Providers", "search", "list", nil, currentRecords, cc.Page, defaultListLimit, "Switch with "+CmdRef("search set <name>")+"."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "current",
		Usage: "current - Show the current search provider",
		Handler: func(cc CommandContext) (string, error) {
			if h.settingsService == nil {
				return "Search isn't available right now.", nil
			}
			settingsResp, err := h.getBotSettings(cc)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(settingsResp.SearchProviderID) == "" {
				return "No search provider is set. See options with " + CmdRef("search list") + ", then choose one with " + CmdRef("search set <name>") + ".", nil
			}
			return "Active search provider: " + h.resolveSearchProviderName(cc, settingsResp.SearchProviderID), nil
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
				return "Search isn't available right now.", nil
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
			return fmt.Sprintf("No search provider named %q. See options with %s.", name, CmdRef("search list")), nil
		},
	})
	return g
}
