package tools

import (
	"context"
	"errors"
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
	if session.IsSubagent {
		return nil, nil
	}
	return []sdk.Tool{
		{
			Name:        "use_skill",
			Description: "Use a skill if you think it is relevant to the current task",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skillName": map[string]any{
						"type":        "string",
						"description": "The name of the skill to use",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "The reason why you think this skill is relevant to the current task",
					},
				},
				"required": []string{"skillName", "reason"},
			},
			Execute: func(_ *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				skillName := StringArg(args, "skillName")
				reason := StringArg(args, "reason")
				if skillName == "" {
					return nil, errors.New("skillName is required")
				}
				return map[string]any{
					"success":   true,
					"skillName": skillName,
					"reason":    reason,
				}, nil
			},
		},
	}, nil
}
