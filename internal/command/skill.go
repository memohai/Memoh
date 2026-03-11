package command

func (h *Handler) buildSkillGroup() *CommandGroup {
	g := newCommandGroup("skill", "View bot skills")
	g.DefaultAction = "list"
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all skills",
		Handler: func(cc CommandContext) (string, error) {
			if h.skillLoader == nil {
				return "Skill loading is not available.", nil
			}
			items, err := h.skillLoader.LoadSkills(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No skills found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				records = append(records, []kv{
					{"Name", item.Name},
					{"Description", truncate(item.Description, 60)},
				})
			}
			return formatItems(records), nil
		},
	})
	return g
}
