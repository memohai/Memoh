package command

import "strings"

func (h *Handler) buildSkillGroup() *CommandGroup {
	g := newCommandGroup("skill", "View bot skills")
	g.DefaultAction = "list"
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all skills",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.skillLoader == nil {
				return &Result{Text: "Skills aren't available for this bot."}, nil
			}
			items, err := h.skillLoader.LoadSkills(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No skills yet.\n\nSkills are reusable tools the bot can call. Add them in the web dashboard."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				note := truncate(item.Description, 80)
				if strings.EqualFold(strings.TrimSpace(item.Description), strings.TrimSpace(item.Name)) {
					note = "" // description repeats the name; don't print it twice
				}
				records = append(records, listRecord{
					fields: []kv{{"Name", item.Name}},
					note:   note,
				})
			}
			return buildListResult("Skills", "skill", "list", nil, records, cc.Page, defaultListLimit, ""), nil
		},
	})
	return g
}
