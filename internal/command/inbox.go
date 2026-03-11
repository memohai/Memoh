package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/inbox"
)

func (h *Handler) buildInboxGroup() *CommandGroup {
	g := newCommandGroup("inbox", "View bot inbox")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list [--unread] - List inbox items",
		Handler: func(cc CommandContext) (string, error) {
			filter := inbox.ListFilter{Limit: 20}
			for _, arg := range cc.Args {
				if strings.EqualFold(arg, "--unread") {
					unread := false
					filter.IsRead = &unread
				}
			}
			items, err := h.inboxService.List(cc.Ctx, cc.BotID, filter)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "Inbox is empty.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				status := "unread"
				if item.IsRead {
					status = "read"
				}
				records = append(records, []kv{
					{"Source", item.Source},
					{"Content", truncate(item.Content, 50)},
					{"Status", status},
					{"Time", item.CreatedAt.Format("01-02 15:04")},
				})
			}
			return formatItems(records), nil
		},
	})
	g.Register(SubCommand{
		Name:  "count",
		Usage: "count - Show inbox counts",
		Handler: func(cc CommandContext) (string, error) {
			result, err := h.inboxService.Count(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Unread: %d / Total: %d", result.Unread, result.Total), nil
		},
	})
	return g
}
