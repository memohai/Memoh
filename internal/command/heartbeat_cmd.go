package command

func (h *Handler) buildHeartbeatGroup() *CommandGroup {
	g := newCommandGroup("heartbeat", "View heartbeat logs")
	g.DefaultAction = "logs"
	g.Register(SubCommand{
		Name:  "logs",
		Usage: "logs - List recent heartbeat logs",
		// UPSTREAM REPORT (backend, deferred): to offer the same --range time
		// window as /usage, heartbeatService.ListLogs + ListHeartbeatLogsByBot
		// need created_at From/To params. Pagination already covers "view all".
		ResultHandler: func(cc CommandContext) (*Result, error) {
			const pageSize = 10
			items, total, err := h.heartbeatService.ListLogs(cc.Ctx, cc.BotID, pageSize, cc.Page*pageSize)
			if err != nil {
				return nil, err
			}
			if total == 0 {
				return WithButtons(
					&Result{Text: cc.T("cmd.heartbeat.empty")},
					ListItem{Label: cc.T("cmd.heartbeat.section.settings"), Action: &ItemAction{Resource: "settings", Action: "get"}},
				), nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				dur := ""
				if item.CompletedAt != nil {
					dur = humanizeDuration(item.CompletedAt.Sub(item.StartedAt))
				}
				note := ""
				if item.ErrorMessage != "" {
					note = truncate(item.ErrorMessage, 80)
				}
				// Success is the common, expected outcome — flag only failures so
				// the eye lands on the run that needs attention.
				rec := []kv{{cc.T("cmd.heartbeat.fieldTime"), humanizeTimeT(cc, item.StartedAt)}}
				if !isSuccessStatus(item.Status) {
					rec = append(rec, kv{cc.T("cmd.common.fieldStatus"), humanizeStatusT(cc, item.Status)})
				}
				if dur != "" {
					rec = append(rec, kv{cc.T("cmd.heartbeat.fieldDuration"), dur})
				}
				records = append(records, listRecord{fields: rec, note: note})
			}
			return buildPagedListResult(cc.T("cmd.heartbeat.title"), "heartbeat", "logs", nil, records, cc.Page, pageSize, int(total), cc.T("cmd.heartbeat.olderHint"), cc.L), nil
		},
	})
	return g
}
