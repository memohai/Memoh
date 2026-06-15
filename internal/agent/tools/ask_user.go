package tools

import (
	"context"
	"errors"
	"log/slog"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/userinput"
)

type AskUserProvider struct{}

func NewAskUserProvider(_ *slog.Logger) *AskUserProvider {
	return &AskUserProvider{}
}

func (*AskUserProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent {
		return nil, nil
	}
	return []sdk.Tool{{
		Name:        ToolAskUser.String(),
		Description: "Pause the run and ask the user one or more questions (a quiz question, a plan choice, a decision, or open text input). Use this whenever the user asks you to quiz them, test them, or pose a multiple-choice question, and whenever the user must make a choice before you continue. Put the question text in `text` and every answer choice in `options`; never write the choices as ordinary assistant text or simulate the interaction yourself. Wait for this tool's result before grading, explaining answers, or continuing. If the latest user message asks for another question, quiz, or choice, create it with this tool — do not treat that request itself as the user's answer.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questions": map[string]any{
					"type":        "array",
					"minItems":    1,
					"maxItems":    userinput.MaxQuestionsPerRequest,
					"description": "Questions to ask in this single pause. Use one element unless the user must answer several related questions at once.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{
								"type":        "string",
								"description": "The question text. Do not embed answer choices here.",
							},
							"kind": map[string]any{
								"type":        "string",
								"enum":        []string{userinput.QuestionKindSingleSelect, userinput.QuestionKindMultiSelect, userinput.QuestionKindText},
								"description": "single_select: the user picks exactly one option. multi_select: the user picks one or more options (select-all-that-apply, multi-answer quizzes). text: free-form text answer without options.",
							},
							"options": map[string]any{
								"type":        "array",
								"minItems":    userinput.MinOptionsPerQuestion,
								"maxItems":    userinput.MaxOptionsPerQuestion,
								"description": "Answer choices. Required for single_select and multi_select; forbidden for text.",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"label": map[string]any{
											"type":        "string",
											"description": "Short user-facing answer text.",
										},
										"description": map[string]any{
											"type":        "string",
											"description": "Optional one-sentence tradeoff or detail.",
										},
									},
									"required":             []string{"label"},
									"additionalProperties": false,
								},
							},
							"allow_custom": map[string]any{
								"type":        "boolean",
								"description": "Select kinds only: also offer an \"Other\" free-text entry alongside the options.",
							},
							"placeholder": map[string]any{
								"type":        "string",
								"description": "Optional placeholder for the text input (kind text) or the custom entry (allow_custom).",
							},
						},
						"required":             []string{"text", "kind"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"questions"},
			"additionalProperties": false,
		},
		RequireApproval: true,
		Execute: func(_ *sdk.ToolExecContext, input any) (any, error) {
			if err := userinput.ValidateAskUserInput(input); err != nil {
				return map[string]any{
					"status":      "invalid_arguments",
					"error":       err.Error(),
					"instruction": "Call " + toolRef(ToolAskUser) + " again with a valid `questions` array. Every question needs `text` and a `kind` of single_select, multi_select, or text; select kinds need `options` with labels.",
				}, nil
			}
			return nil, errors.New(ToolAskUser.String() + " must be resolved through user input before execution")
		},
	}}, nil
}
