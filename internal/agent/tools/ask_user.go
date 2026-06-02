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
		Name:        userinput.ToolNameAskUser,
		Description: "Pause the run and ask the user one concise question. Use this whenever the user must choose a plan, select an option, answer a multiple-choice quiz, provide open text input, or decide the next step. For quizzes or multiple-choice questions, put only the question text in `question` and put every answer choice in `options`; do not duplicate choices inside `question` or write the choices as normal assistant text. If one choice should let the user type a custom answer, include that choice in `options` with `input_type` set to `text` and omit `value`. For open text input without choices, set top-level `input_type` to `text` and omit `options`. Wait for this tool's result before grading, explaining the answer, or continuing. Ask only one question per call.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The single question to ask the user. For a quiz, include only the quiz question text here; do not include A/B/C choices or any option list.",
				},
				"options": map[string]any{
					"type":        "array",
					"description": "Choices to show. Required for multiple-choice questions, quizzes, polls, and plan selection.",
					"minItems":    1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string", "description": "Stable option identifier, such as A, B, C, plan_a, or custom."},
							"label":       map[string]any{"type": "string", "description": "Short user-facing option label. For quizzes, include the answer text."},
							"description": map[string]any{"type": "string", "description": "Optional one-sentence tradeoff or detail."},
							"value":       map[string]any{"type": "string", "description": "Value to return to the model when this fixed option is selected. Omit for text-input options."},
							"input_type": map[string]any{
								"type":        "string",
								"enum":        []string{"text"},
								"description": "Set to text when this option should reveal a custom text input after the user selects it. Omit for normal fixed options.",
							},
							"placeholder": map[string]any{
								"type":        "string",
								"description": "Optional placeholder for this option's text input. Only used when input_type is text.",
							},
						},
						"required":             []string{"id", "label"},
						"additionalProperties": false,
					},
				},
				"input_type": map[string]any{
					"type":        "string",
					"enum":        []string{"text"},
					"description": "Set to text only for open text input without choices. Omit for normal choice requests.",
				},
				"placeholder": map[string]any{
					"type":        "string",
					"description": "Optional placeholder for open text input. For a custom text option, put placeholder on that option instead.",
				},
			},
			"required":             []string{"question"},
			"additionalProperties": false,
		},
		RequireApproval: true,
		Execute: func(_ *sdk.ToolExecContext, input any) (any, error) {
			if err := userinput.ValidateAskUserInput(input); err != nil {
				return map[string]any{
					"status":      "invalid_arguments",
					"error":       err.Error(),
					"instruction": "Call ask_user again with a valid JSON object containing a non-empty question. For multiple-choice or quiz questions, include all choices in options.",
				}, nil
			}
			return nil, errors.New("ask_user must be resolved through user input before execution")
		},
	}}, nil
}
