package command

import (
	"strings"
)

func (h *Handler) buildEmailGroup() *CommandGroup {
	g := newCommandGroup("email", "View email configuration")
	g.DefaultAction = "outbox" // bare /email lands on recent sends
	g.EnableActionMenu()       // bare /email shows its sub-actions as buttons
	g.Register(SubCommand{
		Name:  "providers",
		Usage: "providers - List email providers",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			items, err := h.emailService.ListProviders(cc.Ctx, "")
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No email providers yet.\n\nEmail providers let the bot send and receive mail. Add one in the web dashboard."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				fields := []kv{{"Name", item.Name}}
				if eng := distinctProviderEngine(item.Name, item.Provider); eng != "" {
					fields = append(fields, kv{"", eng})
				}
				records = append(records, listRecord{fields: fields})
			}
			return buildListResult("Email Providers", "email", "providers", nil, records, cc.Page, defaultListLimit, "Inspect access with "+CmdRef("email bindings")+"."), nil
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
				return &Result{Text: "No email bindings yet.\n\nA binding gives this bot an email address it can send from. Add one in the web dashboard."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				perms := buildPermString(item.CanRead, item.CanWrite, item.CanDelete)
				records = append(records, listRecord{fields: []kv{
					{"Address", item.EmailAddress},
					{"Permissions", perms},
				}})
			}
			return buildListResult("Email Bindings", "email", "bindings", nil, records, cc.Page, defaultListLimit, "See recent sends with "+CmdRef("email outbox")+"."), nil
		},
	})
	g.Register(SubCommand{
		Name:  "outbox",
		Usage: "outbox - List recently sent emails",
		// UPSTREAM REPORT (backend, deferred): to offer the same --range time
		// window as /usage, emailOutboxService.ListByBot + ListEmailOutboxByBot
		// need created_at From/To params. Pagination already covers "view all".
		ResultHandler: func(cc CommandContext) (*Result, error) {
			const pageSize = 10
			offset := cc.Page * pageSize
			items, total, err := h.emailOutboxService.ListByBot(cc.Ctx, cc.BotID, pageSize, int32(offset)) //nolint:gosec // offset is a small, bounded page index
			if err != nil {
				return nil, err
			}
			if total == 0 {
				return &Result{Text: "No emails sent yet.\n\nEmails the bot sends will appear here."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				to := strings.Join(item.To, ", ")
				// A failed send is the most actionable row — surface its reason.
				note := ""
				if item.Error != "" {
					note = truncate(item.Error, 80)
				}
				// "Sent" is the expected outcome; flag only failures, like heartbeat.
				fields := []kv{{"Subject", truncate(item.Subject, 40)}}
				if st := strings.ToLower(strings.TrimSpace(item.Status)); st != "sent" && !isSuccessStatus(item.Status) {
					fields = append(fields, kv{"Status", humanizeStatus(item.Status)})
				}
				fields = append(fields, kv{"To", truncate(to, 40)}, kv{"Sent", humanizeTime(item.SentAt)})
				records = append(records, listRecord{fields: fields, note: note})
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
