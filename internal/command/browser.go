package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildBrowserGroup() *CommandGroup {
	g := newCommandGroup("browser", "Manage browser context")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all browser contexts",
		Handler: func(cc CommandContext) (string, error) {
			if h.browserCtxService == nil {
				return "Browser context service is not available.", nil
			}
			items, err := h.browserCtxService.List(cc.Ctx)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No browser contexts found.", nil
			}
			settingsResp, _ := h.getBotSettings(cc)
			currentRecords := make([][]kv, 0, 1)
			otherRecords := make([][]kv, 0, len(items))
			for _, item := range items {
				label := item.Name
				record := []kv{{"Name", label}}
				if item.ID == settingsResp.BrowserContextID {
					label += " [current]"
					record[0].value = label
					currentRecords = append(currentRecords, record)
					continue
				}
				otherRecords = append(otherRecords, record)
			}
			currentRecords = append(currentRecords, otherRecords...)
			records := currentRecords
			return formatLimitedItems(records, defaultListLimit, "Use /browser current to inspect the active context."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "current",
		Usage: "current - Show the current browser context",
		Handler: func(cc CommandContext) (string, error) {
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			settingsResp, err := h.getBotSettings(cc)
			if err != nil {
				return "", err
			}
			return formatKV([]kv{{"Browser Context", h.resolveBrowserContextName(cc, settingsResp.BrowserContextID)}}), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <name> - Set the browser context for this bot",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /browser set <name>", nil
			}
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			name := cc.Args[0]
			before, _ := h.getBotSettings(cc)
			items, err := h.browserCtxService.List(cc.Ctx)
			if err != nil {
				return "", err
			}
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					_, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
						BrowserContextID: item.ID,
					})
					if err != nil {
						return "", err
					}
					return formatChangedValue("Browser context", h.resolveBrowserContextName(cc, before.BrowserContextID), item.Name), nil
				}
			}
			return fmt.Sprintf("Browser context %q not found.", name), nil
		},
	})
	return g
}
