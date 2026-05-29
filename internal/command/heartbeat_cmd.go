package command

import (
	"fmt"
)

func (h *Handler) buildHeartbeatGroup() *CommandGroup {
	g := newCommandGroup("heartbeat", "View heartbeat logs")
	g.DefaultAction = "logs"
	g.Register(SubCommand{
		Name:  "logs",
		Usage: "logs - List recent heartbeat logs",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			const pageSize = 10
			items, total, err := h.heartbeatService.ListLogs(cc.Ctx, cc.BotID, pageSize, cc.Page*pageSize)
			if err != nil {
				return nil, err
			}
			if total == 0 {
				return &Result{Text: "No heartbeat logs found."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				dur := ""
				if item.CompletedAt != nil {
					dur = fmt.Sprintf("%.1fs", item.CompletedAt.Sub(item.StartedAt).Seconds())
				}
				errMsg := ""
				if item.ErrorMessage != "" {
					errMsg = truncate(item.ErrorMessage, 50)
				}
				rec := []kv{
					{"Time", item.StartedAt.Format("01-02 15:04:05")},
					{"Status", item.Status},
				}
				if dur != "" {
					rec = append(rec, kv{"Duration", dur})
				}
				if errMsg != "" {
					rec = append(rec, kv{"Error", errMsg})
				}
				records = append(records, listRecord{fields: rec})
			}
			return buildPagedListResult("Heartbeat Logs", "heartbeat", "logs", nil, records, cc.Page, pageSize, int(total), "Use the Web UI for older heartbeat logs."), nil
		},
	})
	return g
}
