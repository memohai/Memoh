package command

func (h *Handler) buildSkillGroup() *CommandGroup {
	g := newCommandGroup("skill", "View bot skills")
	g.DefaultAction = "list"
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all skills",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.skillLoader == nil {
				return &Result{Text: "Skill loading is not available."}, nil
			}
			items, err := h.skillLoader.LoadSkills(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: "No skills found."}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				records = append(records, listRecord{fields: []kv{
					{"Name", item.Name},
					{"Description", truncate(item.Description, 60)},
				}})
			}
			return buildListResult("Skills", "skill", "list", nil, records, cc.Page, defaultListLimit, ""), nil
		},
	})
	return g
}
