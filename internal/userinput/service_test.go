package userinput

import (
	"errors"
	"testing"
)

func selectPayload() UIPayload {
	return UIPayload{
		Version: PayloadVersion,
		Questions: []UIQuestion{
			{
				ID:   "q1",
				Text: "Which plan?",
				Kind: QuestionKindSingleSelect,
				Options: []UIOption{
					{ID: "q1.o1", Label: "Plan A"},
					{ID: "q1.o2", Label: "Plan B"},
				},
				AllowCustom: true,
			},
			{
				ID:   "q2",
				Text: "Which features?",
				Kind: QuestionKindMultiSelect,
				Options: []UIOption{
					{ID: "q2.o1", Label: "Search"},
					{ID: "q2.o2", Label: "Sync"},
				},
			},
			{
				ID:   "q3",
				Text: "Anything else?",
				Kind: QuestionKindText,
			},
		},
	}
}

func TestSubmittedResultValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		answers []QuestionAnswer
	}{
		{"missing answer", []QuestionAnswer{
			{QuestionID: "q1", OptionIDs: []string{"q1.o1"}},
			{QuestionID: "q2", OptionIDs: []string{"q2.o1"}},
		}},
		{"unknown question", []QuestionAnswer{
			{QuestionID: "q9", OptionIDs: []string{"q1.o1"}},
		}},
		{"duplicate answer", []QuestionAnswer{
			{QuestionID: "q1", OptionIDs: []string{"q1.o1"}},
			{QuestionID: "q1", OptionIDs: []string{"q1.o2"}},
			{QuestionID: "q2", OptionIDs: []string{"q2.o1"}},
			{QuestionID: "q3", Text: "ok"},
		}},
		{"unknown option", []QuestionAnswer{
			{QuestionID: "q1", OptionIDs: []string{"nope"}},
			{QuestionID: "q2", OptionIDs: []string{"q2.o1"}},
			{QuestionID: "q3", Text: "ok"},
		}},
		{"multiple options on single select", []QuestionAnswer{
			{QuestionID: "q1", OptionIDs: []string{"q1.o1", "q1.o2"}},
			{QuestionID: "q2", OptionIDs: []string{"q2.o1"}},
			{QuestionID: "q3", Text: "ok"},
		}},
		{"custom text without allow_custom", []QuestionAnswer{
			{QuestionID: "q1", OptionIDs: []string{"q1.o1"}},
			{QuestionID: "q2", CustomText: "extra"},
			{QuestionID: "q3", Text: "ok"},
		}},
		{"empty selection", []QuestionAnswer{
			{QuestionID: "q1"},
			{QuestionID: "q2", OptionIDs: []string{"q2.o1"}},
			{QuestionID: "q3", Text: "ok"},
		}},
		{"text on select question", []QuestionAnswer{
			{QuestionID: "q1", Text: "Plan A"},
			{QuestionID: "q2", OptionIDs: []string{"q2.o1"}},
			{QuestionID: "q3", Text: "ok"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := submittedResult(selectPayload(), tc.answers); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestServiceCanRespond(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	plain := Request{ID: "plain-1", Status: StatusPending}
	if !svc.CanRespond(plain) {
		t.Fatal("plain pending request should be answerable")
	}

	acp := Request{
		ID:               "acp-1",
		Status:           StatusPending,
		ProviderMetadata: map[string]any{"source": ProviderSourceACPMCP},
	}
	if svc.CanRespond(acp) {
		t.Fatal("ACP/MCP request without a live waiter should not be answerable")
	}
	release := svc.RegisterWaiter(acp.ID)
	if !svc.CanRespond(acp) {
		t.Fatal("ACP/MCP request with a live waiter should be answerable")
	}
	release()
	if svc.CanRespond(acp) {
		t.Fatal("ACP/MCP request should stop being answerable after waiter release")
	}

	terminal := acp
	terminal.Status = StatusSubmitted
	release = svc.RegisterWaiter(terminal.ID)
	defer release()
	if svc.CanRespond(terminal) {
		t.Fatal("terminal request should not be answerable even with a waiter")
	}
}

func TestParseAskUserPayloadRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	twoOptions := []any{
		map[string]any{"label": "A"},
		map[string]any{"label": "B"},
	}
	cases := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"empty object", map[string]any{}},
		{"legacy v1 shape", map[string]any{"question": "Which plan?", "options": []any{"A", "B"}}},
		{"missing text", map[string]any{"questions": []any{
			map[string]any{"kind": "text"},
		}}},
		{"missing kind", map[string]any{"questions": []any{
			map[string]any{"text": "Which plan?", "options": twoOptions},
		}}},
		{"select without options", map[string]any{"questions": []any{
			map[string]any{"text": "Which plan?", "kind": "single_select"},
		}}},
		{"option without label", map[string]any{"questions": []any{
			map[string]any{"text": "Which plan?", "kind": "single_select", "options": []any{map[string]any{"label": "A"}, map[string]any{"description": "no label"}}},
		}}},
		{"options on text question", map[string]any{"questions": []any{
			map[string]any{"text": "Say more", "kind": "text", "options": twoOptions},
		}}},
		{"unknown top-level field", map[string]any{
			"questions": []any{map[string]any{"text": "Say more", "kind": "text"}},
			"multiple":  true,
		}},
		{"unknown question field", map[string]any{"questions": []any{
			map[string]any{"text": "Which plan?", "kind": "single_select", "options": twoOptions, "input_type": "text"},
		}}},
		{"unknown option field", map[string]any{"questions": []any{
			map[string]any{"text": "Which plan?", "kind": "single_select", "options": []any{
				map[string]any{"label": "A", "value": "a"}, map[string]any{"label": "B"},
			}},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseAskUserPayload(tc.input)
			if err == nil {
				t.Fatalf("expected error for %#v", tc.input)
			}
			if !errors.Is(err, ErrInvalidAskUserInput) {
				t.Fatalf("expected ErrInvalidAskUserInput, got %v", err)
			}
		})
	}
}

func TestPayloadFromStoredUpgradesLegacySingleSelect(t *testing.T) {
	t.Parallel()

	payload := PayloadFromStored(map[string]any{
		"question": "Which plan?",
		"options": []any{
			map[string]any{"id": "a", "label": "Plan A", "value": "A"},
			map[string]any{"id": "custom", "label": "Custom answer", "input_type": "text", "placeholder": "Type an answer"},
		},
	})
	if len(payload.Questions) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	q := payload.Questions[0]
	if q.Kind != QuestionKindSingleSelect || q.Text != "Which plan?" {
		t.Fatalf("unexpected question: %#v", q)
	}
	// The v1 custom-text option becomes question-level allow_custom.
	if !q.AllowCustom || q.Placeholder != "Type an answer" {
		t.Fatalf("expected custom option upgrade: %#v", q)
	}
	if len(q.Options) != 1 || q.Options[0].ID != "a" || q.Options[0].Label != "Plan A" {
		t.Fatalf("unexpected options: %#v", q.Options)
	}
}

func TestPayloadFromStoredUpgradesLegacyMultipleAndText(t *testing.T) {
	t.Parallel()

	multi := PayloadFromStored(map[string]any{
		"question": "Pick plans",
		"multiple": true,
		"options": []any{
			map[string]any{"id": "a", "label": "Plan A"},
			map[string]any{"id": "b", "label": "Plan B"},
		},
	})
	if multi.Questions[0].Kind != QuestionKindMultiSelect {
		t.Fatalf("expected multi_select upgrade: %#v", multi.Questions[0])
	}

	text := PayloadFromStored(map[string]any{
		"question":   "What do you think?",
		"input_type": "text",
	})
	if text.Questions[0].Kind != QuestionKindText || text.Questions[0].AllowCustom {
		t.Fatalf("expected text upgrade: %#v", text.Questions[0])
	}
}
