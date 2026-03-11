package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/subagent"
)

func (h *Handler) buildSubagentGroup() *CommandGroup {
	g := newCommandGroup("subagent", "Manage subagents")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all subagents",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.subagentService.List(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No subagents found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				records = append(records, []kv{
					{"Name", item.Name},
					{"Description", truncate(item.Description, 40)},
				})
			}
			return formatItems(records), nil
		},
	})
	g.Register(SubCommand{
		Name:  "get",
		Usage: "get <name> - Get subagent details",
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /subagent get <name>", nil
			}
			name := cc.Args[0]
			item, err := h.subagentService.GetByBotAndName(cc.Ctx, cc.BotID, name)
			if err != nil {
				return "", fmt.Errorf("subagent %q not found", name)
			}
			return formatKV([]kv{
				{"Name", item.Name},
				{"Description", item.Description},
				{"Skills", fmt.Sprintf("%v", item.Skills)},
				{"Created", item.CreatedAt.Format("2006-01-02 15:04:05")},
				{"Updated", item.UpdatedAt.Format("2006-01-02 15:04:05")},
			}), nil
		},
	})
	g.Register(SubCommand{
		Name:    "create",
		Usage:   "create <name> <description> - Create a subagent",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 2 {
				return "Usage: /subagent create <name> <description>", nil
			}
			name := cc.Args[0]
			description := strings.Join(cc.Args[1:], " ")
			item, err := h.subagentService.Create(cc.Ctx, cc.BotID, subagent.CreateRequest{
				Name:        name,
				Description: description,
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Subagent %q created (ID: %s).", item.Name, item.ID), nil
		},
	})
	g.Register(SubCommand{
		Name:    "delete",
		Usage:   "delete <name> - Delete a subagent",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /subagent delete <name>", nil
			}
			name := cc.Args[0]
			item, err := h.subagentService.GetByBotAndName(cc.Ctx, cc.BotID, name)
			if err != nil {
				return "", fmt.Errorf("subagent %q not found", name)
			}
			if err := h.subagentService.Delete(cc.Ctx, item.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf("Subagent %q deleted.", name), nil
		},
	})
	return g
}
