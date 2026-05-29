package command

import (
	"strings"
)

func (h *Handler) buildEmailGroup() *CommandGroup {
	g := newCommandGroup("email", "View email configuration")
	g.Register(SubCommand{
		Name:  "providers",
		Usage: "providers - List email providers",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			items, err := h.emailService.ListProviders(cc.Ctx, "")
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No email providers found."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				records = append(records, listRecord{fields: []kv{
					{"Name", item.Name},
					{"Provider", item.Provider},
				}})
			}
			return buildListResult("Email Providers", "email", "providers", nil, records, cc.Page, defaultListLimit, "Use /email bindings to inspect bot bindings."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "bindings",
		Usage: "bindings - List bot email bindings",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			items, err := h.emailService.ListBindings(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No email bindings found."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				perms := buildPermString(item.CanRead, item.CanWrite, item.CanDelete)
				records = append(records, listRecord{fields: []kv{
					{"Address", item.EmailAddress},
					{"Permissions", perms},
				}})
			}
			return buildListResult("Email Bindings", "email", "bindings", nil, records, cc.Page, defaultListLimit, "Use /email outbox to inspect recent sends."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "outbox",
		Usage: "outbox - List recently sent emails",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			const pageSize = 10
			offset := cc.Page * pageSize
			items, total, err := h.emailOutboxService.ListByBot(cc.Ctx, cc.BotID, pageSize, int32(offset)) //nolint:gosec // offset is a small, bounded page index
			if err != nil {
				return nil, err
			}
			if total == 0 {
				return &Result{Text: "Outbox is empty."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				to := strings.Join(item.To, ", ")
				records = append(records, listRecord{fields: []kv{
					{"Subject", truncate(item.Subject, 40)},
					{"To", truncate(to, 40)},
					{"Status", item.Status},
					{"Sent", item.SentAt.Format("01-02 15:04")},
				}})
			}
			return buildPagedListResult("Outbox", "email", "outbox", nil, records, cc.Page, pageSize, int(total), "Use the Web UI for older outbox entries."), nil
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
