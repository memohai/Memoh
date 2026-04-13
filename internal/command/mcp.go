package command

import (
	"fmt"
	"strings"
)

func (h *Handler) buildMCPGroup() *CommandGroup {
	g := newCommandGroup("mcp", "Manage MCP connections")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all MCP connections",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.mcpConnService.ListByBot(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No MCP connections found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				records = append(records, []kv{
					{"Name", item.Name},
					{"Type", item.Type},
					{"Active", boolStr(item.Active)},
					{"Status", item.Status},
				})
			}
			return formatLimitedItems(records, defaultListLimit, "Use /mcp get <name> for full details."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "get",
		Usage: "get <name> - Get MCP connection details",
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /mcp get <name>", nil
			}
			name := cc.Args[0]
			items, err := h.mcpConnService.ListByBot(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					toolNames := make([]string, 0, len(item.ToolsCache))
					for _, t := range item.ToolsCache {
						toolNames = append(toolNames, t.Name)
					}
					toolsStr := "none"
					if len(toolNames) > 0 {
						toolsStr = strings.Join(toolNames, ", ")
					}
					return formatKV([]kv{
						{"Name", item.Name},
						{"Type", item.Type},
						{"Active", boolStr(item.Active)},
						{"Status", item.Status},
						{"Status Message", item.StatusMessage},
						{"Auth Type", item.AuthType},
						{"Tools", toolsStr},
						{"Created", item.CreatedAt.Format("2006-01-02 15:04:05")},
						{"Updated", item.UpdatedAt.Format("2006-01-02 15:04:05")},
					}), nil
				}
			}
			return fmt.Sprintf("MCP connection %q not found.", name), nil
		},
	})
	g.Register(SubCommand{
		Name:    "delete",
		Usage:   "delete <name> - Delete an MCP connection",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /mcp delete <name>", nil
			}
			name := cc.Args[0]
			items, err := h.mcpConnService.ListByBot(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					if err := h.mcpConnService.Delete(cc.Ctx, cc.BotID, item.ID); err != nil {
						return "", err
					}
					return fmt.Sprintf("MCP connection %q deleted.", name), nil
				}
			}
			return fmt.Sprintf("MCP connection %q not found.", name), nil
		},
	})
	return g
}
