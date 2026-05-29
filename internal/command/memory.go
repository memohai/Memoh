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
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.memProvService == nil {
				return &Result{Text: "Memory provider service is not available."}, nil
			}
			items, err := h.memProvService.List(cc.Ctx)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No memory providers found."}, nil
			}
			settingsResp, _ := h.getBotSettings(cc)
			currentRecords := make([]listRecord, 0, 1)
			otherRecords := make([]listRecord, 0, len(items))
			for _, item := range items {
				def := ""
				if item.IsDefault {
					def = " (default)"
				}
				label := item.Name + def
				if item.ID == settingsResp.MemoryProviderID {
					currentRecords = append(currentRecords, listRecord{
						selected: true,
						fields:   []kv{{"Name", label + " [current]"}, {"Provider", item.Provider}},
					})
					continue
				}
				otherRecords = append(otherRecords, listRecord{
					fields: []kv{{"Name", label}, {"Provider", item.Provider}},
				})
			}
			currentRecords = append(currentRecords, otherRecords...)
			return buildListResult("Memory Providers", "memory", "list", nil, currentRecords, cc.Page, defaultListLimit, "Use /memory current to inspect the active provider."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "current",
		Usage: "current - Show the current memory provider",
		Handler: func(cc CommandContext) (string, error) {
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			settingsResp, err := h.getBotSettings(cc)
			if err != nil {
				return "", err
			}
			return formatKV([]kv{{"Memory Provider", h.resolveMemoryProviderName(cc, settingsResp.MemoryProviderID)}}), nil
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
				return "Settings service is not available.", nil
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
			return fmt.Sprintf("Memory provider %q not found.", name), nil
		},
	})
	return g
}
