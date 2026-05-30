package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/schedule"
)

func (h *Handler) buildScheduleGroup() *CommandGroup {
	g := newCommandGroup("schedule", "Manage scheduled tasks")
	g.DefaultAction = "list" // bare /schedule lands on the live schedule list
	g.EnableActionMenu()     // bare /schedule shows its sub-actions as buttons
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all schedules",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			items, err := h.scheduleService.List(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No schedules yet.\n\nSchedules run a command on a recurring timer. Create one, for example:\n" + CmdRef(`schedule create daily "0 9 * * *" "Send the report"`)}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				// The cron phrase is the identifying fact, so it leads as the chip;
				// a status chip appears only when the schedule is paused (an
				// enabled schedule is the expected state and needs no flag).
				fields := []kv{
					{"Name", item.Name},
					{"", humanizeCron(item.Pattern)},
				}
				if !item.Enabled {
					fields = append(fields, kv{"", "paused"})
				}
				note := ""
				if d := strings.TrimSpace(item.Description); d != "" && !strings.EqualFold(d, strings.TrimSpace(item.Name)) {
					note = truncate(d, 60)
				}
				records = append(records, listRecord{
					fields: fields,
					note:   note,
					// Tap a schedule to open its details — no typing of /schedule get.
					action: &ItemAction{Resource: "schedule", Action: "get", Args: []string{item.Name}},
				})
			}
			return buildListResult("Schedules", "schedule", "list", nil, records, cc.Page, defaultListLimit, ""), nil
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
			status := "Active"
			if !item.Enabled {
				status = "Paused"
			}
			runs := strconv.Itoa(item.CurrentCalls)
			if item.MaxCalls != nil {
				runs = fmt.Sprintf("%d of %d", item.CurrentCalls, *item.MaxCalls)
			}
			desc := item.Description
			if d := strings.TrimSpace(desc); d == "" ||
				strings.EqualFold(d, strings.TrimSpace(item.Name)) ||
				strings.EqualFold(d, strings.TrimSpace(item.Command)) {
				desc = "" // don't echo the name/command back as a description
			}
			pairs := []kv{
				{"Description", desc},
				{"Schedule", humanizeCron(item.Pattern)},
				{"Command", item.Command},
				{"Status", status},
				{"Runs", runs},
				{"Created", humanizeTime(item.CreatedAt)},
			}
			if !item.UpdatedAt.Truncate(time.Second).Equal(item.CreatedAt.Truncate(time.Second)) {
				pairs = append(pairs, kv{"Updated", humanizeTime(item.UpdatedAt)})
			}
			return formatKVTitled(item.Name, pairs), nil
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
			// Echo the humanized cron + command so the user can confirm the
			// pattern was parsed as intended ("did 0 9 * * * mean 9am?").
			return fmt.Sprintf("✅ Schedule %s created.\n\n- Runs: %s\n- Command: %s",
				MdCode(item.Name), renderValue(humanizeCron(item.Pattern)), renderValue(item.Command)), nil
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
			return fmt.Sprintf("✅ Schedule %s updated.", MdCode(updated.Name)), nil
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
			return fmt.Sprintf("✅ Schedule %s deleted.", MdCode(item.Name)), nil
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
			return fmt.Sprintf("✅ Schedule %s enabled.", MdCode(item.Name)), nil
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
			return fmt.Sprintf("✅ Schedule %s paused.", MdCode(item.Name)), nil
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
