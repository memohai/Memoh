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
		Handler: func(cc CommandContext) (string, error) {
			items, _, err := h.heartbeatService.ListLogs(cc.Ctx, cc.BotID, 10, 0)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No heartbeat logs found.", nil
			}
			records := make([][]kv, 0, len(items))
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
				records = append(records, rec)
			}
			return formatItems(records), nil
		},
	})
	return g
}
