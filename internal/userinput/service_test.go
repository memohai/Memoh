package userinput

import "testing"

func TestSubmittedResultRequiresUserResponse(t *testing.T) {
	if _, err := submittedResult(SubmitInput{}); err == nil {
		t.Fatal("expected empty submit to fail")
	}

	result, err := submittedResult(SubmitInput{
		OptionID:    "a",
		OptionValue: "A",
	})
	if err != nil {
		t.Fatalf("submitted result: %v", err)
	}
	if result["status"] != StatusSubmitted {
		t.Fatalf("status = %#v", result["status"])
	}
	response, ok := result["user_response"].(map[string]any)
	if !ok {
		t.Fatalf("missing user_response: %#v", result)
	}
	if response["option_id"] != "a" || response["value"] != "A" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if result["instruction"] == "" {
		t.Fatalf("missing instruction: %#v", result)
	}
}

func TestCanceledResultIsToolResultPayload(t *testing.T) {
	result := canceledResult("")
	if result["status"] != StatusCanceled {
		t.Fatalf("status = %#v", result["status"])
	}
	response, ok := result["user_response"].(map[string]any)
	if !ok {
		t.Fatalf("missing user_response: %#v", result)
	}
	if response["canceled"] != true || response["reason"] != "user_canceled" {
		t.Fatalf("unexpected cancel response: %#v", response)
	}
	if result["instruction"] == "" {
		t.Fatalf("missing instruction: %#v", result)
	}
}

func TestNormalizeUIPayload(t *testing.T) {
	payload := normalizeUIPayload(map[string]any{
		"question":     "Choose one",
		"allow_custom": true,
		"options": []any{
			map[string]any{"id": "a", "label": "Plan A", "value": "A"},
			map[string]any{"id": "custom", "label": "Custom answer", "input_type": "text", "placeholder": "Type an answer"},
			"Plan B",
		},
	})

	if payload.Question != "Choose one" || !payload.AllowCustom {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if len(payload.Options) != 3 {
		t.Fatalf("expected three options: %#v", payload.Options)
	}
	if payload.Options[0].ID != "a" || payload.Options[0].Value != "A" {
		t.Fatalf("unexpected first option: %#v", payload.Options[0])
	}
	if payload.Options[1].ID != "custom" || payload.Options[1].InputType != "text" || payload.Options[1].Placeholder != "Type an answer" {
		t.Fatalf("unexpected second option: %#v", payload.Options[1])
	}
	if payload.Options[1].Value != nil {
		t.Fatalf("text input option should not default value: %#v", payload.Options[1])
	}
	if payload.Options[2].ID != "Plan B" || payload.Options[2].Value != "Plan B" {
		t.Fatalf("unexpected third option: %#v", payload.Options[2])
	}
}

func TestValidateAskUserInputRejectsMissingQuestion(t *testing.T) {
	t.Parallel()

	for _, input := range []any{nil, map[string]any{}, map[string]any{"options": []any{"A"}}} {
		if err := ValidateAskUserInput(input); err == nil {
			t.Fatalf("expected invalid input error for %#v", input)
		}
	}
}

func TestNormalizeUIPayloadDefaultsTextInputWithoutOptions(t *testing.T) {
	t.Parallel()

	payload := normalizeUIPayload(map[string]any{"question": "What do you think?"})

	if payload.InputType != "text" {
		t.Fatalf("expected text input type, got %q", payload.InputType)
	}
	if !payload.AllowCustom {
		t.Fatal("expected custom input to be allowed when no options are present")
	}
}
