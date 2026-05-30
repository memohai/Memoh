package command

import (
	"fmt"
	"strings"
)

func (h *Handler) buildMCPGroup() *CommandGroup {
	g := newCommandGroup("mcp", "Manage MCP connections")
	g.DefaultAction = "list" // bare /mcp lands on the connection list
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all MCP connections",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			items, err := h.mcpConnService.ListByBot(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return WithButtons(
					&Result{Text: "No MCP connections yet.\n\nMCP connections give the bot extra tools from external servers. Add one in the web dashboard."},
					ListItem{Label: "All commands ▸", Action: &ItemAction{Resource: "help", Action: "mcp"}},
				), nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				records = append(records, listRecord{
					fields: []kv{
						{"Name", item.Name},
						{"Status", humanizeStatus(item.Status)},
						{"Type", item.Type},
					},
					// Tap a connection to open its details — no typing of /mcp get.
					action: &ItemAction{Resource: "mcp", Action: "get", Args: []string{item.Name}},
				})
			}
			result := buildListResult("MCP Connections", "mcp", "list", nil, records, cc.Page, defaultListLimit, "See full details with "+CmdRef("mcp get <name>")+".")
			return WithExtraActions(result,
				ListItem{Label: "All commands ▸", Action: &ItemAction{Resource: "help", Action: "mcp"}},
			), nil
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
					authType := item.AuthType
					if strings.EqualFold(strings.TrimSpace(authType), "none") {
						authType = "" // default for every server; omit rather than print noise
					}
					// Lead with Status (what the user opened this view to check); show
					// Reason only on failure (formatKV skips blank values).
					return formatKVTitled(item.Name, []kv{
						{"Status", humanizeStatus(item.Status)},
						{"Reason", item.StatusMessage},
						{"Type", item.Type},
						{"Active", boolStr(item.Active)},
						{"Auth", authType},
						{"Tools", toolsStr},
						{"Created", humanizeTime(item.CreatedAt)},
						{"Updated", humanizeTime(item.UpdatedAt)},
					}), nil
				}
			}
			return fmt.Sprintf("No MCP connection named %q. Run /mcp list to see connections.", name), nil
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
					return fmt.Sprintf("✅ MCP connection %s deleted.", MdCode(item.Name)), nil
				}
			}
			return fmt.Sprintf("No MCP connection named %q. Run /mcp list to see connections.", name), nil
		},
	})
	return g
}
