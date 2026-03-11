package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/schedule"
)

func (h *Handler) buildScheduleGroup() *CommandGroup {
	g := newCommandGroup("schedule", "Manage scheduled tasks")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all schedules",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.scheduleService.List(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No schedules found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				records = append(records, []kv{
					{"Name", item.Name},
					{"Pattern", item.Pattern},
					{"Enabled", boolStr(item.Enabled)},
					{"Description", truncate(item.Description, 30)},
				})
			}
			return formatItems(records), nil
		},
	})
	g.Register(SubCommand{
		Name:  "get",
		Usage: "get <name> - Get schedule details",
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /schedule get <name>", nil
			}
			item, err := h.findScheduleByName(cc, cc.Args[0])
			if err != nil {
				return "", err
			}
			maxCalls := "unlimited"
			if item.MaxCalls != nil {
				maxCalls = strconv.Itoa(*item.MaxCalls)
			}
			return formatKV([]kv{
				{"Name", item.Name},
				{"Description", item.Description},
				{"Pattern", item.Pattern},
				{"Command", item.Command},
				{"Enabled", boolStr(item.Enabled)},
				{"Max Calls", maxCalls},
				{"Current Calls", strconv.Itoa(item.CurrentCalls)},
				{"Created", item.CreatedAt.Format("2006-01-02 15:04:05")},
				{"Updated", item.UpdatedAt.Format("2006-01-02 15:04:05")},
			}), nil
		},
	})
	g.Register(SubCommand{
		Name:    "create",
		Usage:   "create <name> <pattern> <command> - Create a schedule",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 3 {
				return "Usage: /schedule create <name> <pattern> <command>\nExample: /schedule create daily-report \"0 9 * * *\" \"Send daily report\"", nil
			}
			name := cc.Args[0]
			pattern := cc.Args[1]
			command := strings.Join(cc.Args[2:], " ")
			item, err := h.scheduleService.Create(cc.Ctx, cc.BotID, schedule.CreateRequest{
				Name:        name,
				Description: name,
				Pattern:     pattern,
				Command:     command,
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Schedule %q created.", item.Name), nil
		},
	})
	g.Register(SubCommand{
		Name:    "update",
		Usage:   "update <name> [--pattern P] [--command C] - Update a schedule",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /schedule update <name> [--pattern P] [--command C]", nil
			}
			item, err := h.findScheduleByName(cc, cc.Args[0])
			if err != nil {
				return "", err
			}
			req := schedule.UpdateRequest{}
			args := cc.Args[1:]
			for i := 0; i < len(args); i++ {
				if i+1 >= len(args) {
					break
				}
				switch args[i] {
				case "--name":
					i++
					req.Name = &args[i]
				case "--pattern":
					i++
					req.Pattern = &args[i]
				case "--command":
					i++
					val := strings.Join(args[i:], " ")
					req.Command = &val
					i = len(args)
				case "--enabled":
					i++
					v := strings.ToLower(args[i]) == "true"
					req.Enabled = &v
				}
			}
			updated, err := h.scheduleService.Update(cc.Ctx, item.ID, req)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Schedule %q updated.", updated.Name), nil
		},
	})
	g.Register(SubCommand{
		Name:    "delete",
		Usage:   "delete <name> - Delete a schedule",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /schedule delete <name>", nil
			}
			item, err := h.findScheduleByName(cc, cc.Args[0])
			if err != nil {
				return "", err
			}
			if err := h.scheduleService.Delete(cc.Ctx, item.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf("Schedule %q deleted.", cc.Args[0]), nil
		},
	})
	g.Register(SubCommand{
		Name:    "enable",
		Usage:   "enable <name> - Enable a schedule",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /schedule enable <name>", nil
			}
			item, err := h.findScheduleByName(cc, cc.Args[0])
			if err != nil {
				return "", err
			}
			enabled := true
			_, err = h.scheduleService.Update(cc.Ctx, item.ID, schedule.UpdateRequest{Enabled: &enabled})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Schedule %q enabled.", cc.Args[0]), nil
		},
	})
	g.Register(SubCommand{
		Name:    "disable",
		Usage:   "disable <name> - Disable a schedule",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /schedule disable <name>", nil
			}
			item, err := h.findScheduleByName(cc, cc.Args[0])
			if err != nil {
				return "", err
			}
			enabled := false
			_, err = h.scheduleService.Update(cc.Ctx, item.ID, schedule.UpdateRequest{Enabled: &enabled})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Schedule %q disabled.", cc.Args[0]), nil
		},
	})
	return g
}

func (h *Handler) findScheduleByName(cc CommandContext, name string) (schedule.Schedule, error) {
	items, err := h.scheduleService.List(cc.Ctx, cc.BotID)
	if err != nil {
		return schedule.Schedule{}, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Name, name) {
			return item, nil
		}
	}
	return schedule.Schedule{}, fmt.Errorf("schedule %q not found", name)
}
