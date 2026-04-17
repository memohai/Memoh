package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	sdk "github.com/memohai/twilight-ai/sdk"
)

type SkillProvider struct {
	logger *slog.Logger
}

func NewSkillProvider(log *slog.Logger) *SkillProvider {
	if log == nil {
		log = slog.Default()
	}
	return &SkillProvider{logger: log.With(slog.String("tool", "skill"))}
}

func (*SkillProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || len(session.Skills) == 0 {
		return nil, nil
	}
	skills := session.Skills
	return []sdk.Tool{
		{
			Name:        "use_skill",
			Description: "Activate a skill to get its full instructions. Call this when you think a skill is relevant to the current task.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skillName": map[string]any{
						"type":        "string",
						"description": "The name of the skill to activate",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Why this skill is relevant to the current task",
					},
				},
				"required": []string{"skillName", "reason"},
			},
			Execute: func(_ *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				skillName := StringArg(args, "skillName")
				if skillName == "" {
					return nil, errors.New("skillName is required")
				}
				skill, ok := skills[skillName]
				if !ok {
					return map[string]any{
						"success": false,
						"error":   fmt.Sprintf("skill %q not found — check available skills in the system prompt", skillName),
					}, nil
				}
				return map[string]any{
					"success":     true,
					"skillName":   skillName,
					"description": skill.Description,
					"content":     skill.Content,
					"path":        skill.Path,
				}, nil
			},
		},
	}, nil
}
