package command

import (
	"strings"
)

func (h *Handler) buildEmailGroup() *CommandGroup {
	g := newCommandGroup("email", "View email configuration")
	g.Register(SubCommand{
		Name:  "providers",
		Usage: "providers - List email providers",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.emailService.ListProviders(cc.Ctx, "")
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No email providers found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				records = append(records, []kv{
					{"Name", item.Name},
					{"Provider", item.Provider},
				})
			}
			return formatLimitedItems(records, defaultListLimit, "Use /email bindings to inspect bot bindings."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "bindings",
		Usage: "bindings - List bot email bindings",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.emailService.ListBindings(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No email bindings found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				perms := buildPermString(item.CanRead, item.CanWrite, item.CanDelete)
				records = append(records, []kv{
					{"Address", item.EmailAddress},
					{"Permissions", perms},
				})
			}
			return formatLimitedItems(records, defaultListLimit, "Use /email outbox to inspect recent sends."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "outbox",
		Usage: "outbox - List recently sent emails",
		Handler: func(cc CommandContext) (string, error) {
			items, _, err := h.emailOutboxService.ListByBot(cc.Ctx, cc.BotID, 10, 0)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "Outbox is empty.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				to := strings.Join(item.To, ", ")
				records = append(records, []kv{
					{"Subject", truncate(item.Subject, 40)},
					{"To", truncate(to, 40)},
					{"Status", item.Status},
					{"Sent", item.SentAt.Format("01-02 15:04")},
				})
			}
			return formatLimitedItems(records, 10, "Use the Web UI for older outbox entries."), nil
		},
	})
	return g
}

func buildPermString(read, write, del bool) string {
	var parts []string
	if read {
		parts = append(parts, "read")
	}
	if write {
		parts = append(parts, "write")
	}
	if del {
		parts = append(parts, "delete")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
