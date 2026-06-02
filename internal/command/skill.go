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
				return &Result{Text: cc.T("cmd.skill.unavailable")}, nil
			}
			items, err := h.skillLoader.LoadSkills(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return &Result{Text: cc.T("cmd.skill.empty")}, nil
			}
			records := make([]listRecord, 0, len(items))
			for _, item := range items {
				note := truncate(item.Description, 80)
				if strings.EqualFold(strings.TrimSpace(item.Description), strings.TrimSpace(item.Name)) {
					note = "" // description repeats the name; don't print it twice
				}
				records = append(records, listRecord{
					fields: []kv{{cc.T("cmd.common.fieldName"), item.Name}},
					note:   note,
				})
			}
			return buildListResult(cc.T("cmd.skill.title"), "skill", "list", nil, records, cc.Page, defaultListLimit, cc.L), nil
		},
	})
	return g
}
