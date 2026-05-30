package command

import (
	"strings"
)

func (h *Handler) buildEmailGroup() *CommandGroup {
	g := newCommandGroup("email", "View email configuration")
	g.DefaultAction = "outbox" // bare /email lands on recent sends
	g.Register(SubCommand{
		Name:  "providers",
		Usage: "providers - List email providers",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			items, err := h.emailService.ListProviders(cc.Ctx, "")
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return WithButtons(
					&Result{Text: "No email providers yet.\n\nEmail providers let the bot send and receive mail. Add one in the web dashboard."},
					ListItem{Label: "Bindings", Action: &ItemAction{Resource: "email", Action: "bindings"}},
					ListItem{Label: "Outbox", Action: &ItemAction{Resource: "email", Action: "outbox"}},
				), nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				fields := []kv{{"Name", item.Name}}
				if eng := distinctProviderEngine(item.Name, item.Provider); eng != "" {
					fields = append(fields, kv{"", eng})
				}
				records = append(records, listRecord{fields: fields})
			}
			result := buildListResult("Email Providers", "email", "providers", nil, records, cc.Page, defaultListLimit, "")
			return WithExtraActions(result,
				ListItem{Label: "Bindings", Action: &ItemAction{Resource: "email", Action: "bindings"}},
				ListItem{Label: "Outbox", Action: &ItemAction{Resource: "email", Action: "outbox"}},
			), nil
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
				return WithButtons(
					&Result{Text: "No email bindings yet.\n\nA binding gives this bot an email address it can send from. Add one in the web dashboard."},
					ListItem{Label: "Providers", Action: &ItemAction{Resource: "email", Action: "providers"}},
					ListItem{Label: "Outbox", Action: &ItemAction{Resource: "email", Action: "outbox"}},
				), nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				perms := buildPermString(item.CanRead, item.CanWrite, item.CanDelete)
				records = append(records, listRecord{fields: []kv{
					{"Address", item.EmailAddress},
					{"Permissions", perms},
				}})
			}
			result := buildListResult("Email Bindings", "email", "bindings", nil, records, cc.Page, defaultListLimit, "")
			return WithExtraActions(result,
				ListItem{Label: "Providers", Action: &ItemAction{Resource: "email", Action: "providers"}},
				ListItem{Label: "Outbox", Action: &ItemAction{Resource: "email", Action: "outbox"}},
			), nil
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
				return WithButtons(
					&Result{Text: "No emails sent yet.\n\nEmails the bot sends will appear here."},
					ListItem{Label: "Providers", Action: &ItemAction{Resource: "email", Action: "providers"}},
					ListItem{Label: "Bindings", Action: &ItemAction{Resource: "email", Action: "bindings"}},
				), nil
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
			result := buildPagedListResult("Outbox", "email", "outbox", nil, records, cc.Page, pageSize, int(total), "")
			return WithExtraActions(result,
				ListItem{Label: "Providers", Action: &ItemAction{Resource: "email", Action: "providers"}},
				ListItem{Label: "Bindings", Action: &ItemAction{Resource: "email", Action: "bindings"}},
			), nil
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
