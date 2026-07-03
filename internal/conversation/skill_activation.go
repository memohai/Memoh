package conversation

import (
	"strings"
)

func NewSkillActivation(items []RequestedSkillContext, prompt string) *SkillActivation {
	activation := &SkillActivation{Prompt: strings.TrimSpace(prompt)}
	seen := map[string]struct{}{}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		key := strings.TrimSpace(item.Identity)
		if key == "" {
			key = name + "\x00" + strings.TrimSpace(item.SourceKind)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		activation.Skills = append(activation.Skills, SkillActivationSkill{
			Name:        name,
			DisplayName: name,
			Description: strings.TrimSpace(item.Description),
			SourceKind:  strings.TrimSpace(item.SourceKind),
			State:       "effective",
		})
	}
	if len(activation.Skills) == 0 && activation.Prompt == "" {
		return nil
	}
	return activation
}

func SkillActivationModelQuery(activation *SkillActivation) string {
	if activation == nil {
		return ""
	}
	if prompt := strings.TrimSpace(activation.Prompt); prompt != "" {
		return prompt
	}
	names := make([]string, 0, len(activation.Skills))
	for _, skill := range activation.Skills {
		name := strings.TrimSpace(skill.DisplayName)
		if name == "" {
			name = strings.TrimSpace(skill.Name)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "The user activated the following skill for this turn without an additional prompt: " + strings.Join(names, ", ") + "."
}
