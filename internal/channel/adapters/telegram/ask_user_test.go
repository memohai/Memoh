package telegram

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/userinput"
)

func TestParseAskUserCallback(t *testing.T) {
	t.Parallel()

	cb, ok := parseAskUserCallback("aui~s~deadbeef~0~1")
	if !ok || cb.Op != "s" || cb.Token != "deadbeef" || cb.QIndex != 0 || cb.OIndex != 1 {
		t.Fatalf("select = %#v ok=%v", cb, ok)
	}
	cb, ok = parseAskUserCallback("aui~n~deadbeef~2")
	if !ok || cb.Op != "n" || cb.Page != 2 {
		t.Fatalf("nav = %#v ok=%v", cb, ok)
	}
	cb, ok = parseAskUserCallback("aui~go~deadbeef")
	if !ok || cb.Op != "go" {
		t.Fatalf("go = %#v ok=%v", cb, ok)
	}
	if _, ok := parseAskUserCallback("respond:input-1:q1.o1"); ok {
		t.Fatal("legacy respond callback must not parse as wizard")
	}
	if _, ok := parseAskUserCallback("m~noop"); ok {
		t.Fatal("interactive command callback must not parse as wizard")
	}
}

func TestAskUserTextPromptKeepsQuestionBinding(t *testing.T) {
	t.Parallel()

	store := newAskUserWizardStore()
	store.bindTextPrompt(10, 20, "wizard", "q1")
	store.bindTextPrompt(10, 21, "wizard", "q2")
	token, questionID, ok := store.takeTextPrompt(10, 20)
	if !ok || token != "wizard" || questionID != "q1" {
		t.Fatalf("prompt binding = token %q question %q ok=%v", token, questionID, ok)
	}
	store.byToken["wizard"] = &askUserWizard{Token: "wizard"}
	store.delete("wizard")
	if _, _, ok := store.takeTextPrompt(10, 21); ok {
		t.Fatal("wizard deletion must clear every pending text prompt")
	}
}

func TestAskUserWizardMultiQuestionFlow(t *testing.T) {
	t.Parallel()

	// Mirrors the real screenshot shape: multi_select then text.
	w := newAskUserWizard("input-1", []askUserQuestion{
		{
			ID:   "q1",
			Text: "你平时主要用哪些编程语言？",
			Kind: userinput.QuestionKindMultiSelect,
			Options: []askUserOption{
				{ID: "q1.o1", Label: "Python"},
				{ID: "q1.o2", Label: "JavaScript"},
				{ID: "q1.o3", Label: "Go"},
				{ID: "q1.o4", Label: "Rust"},
			},
		},
		{
			ID:   "q2",
			Text: "有没有最近在做的项目想聊聊？",
			Kind: userinput.QuestionKindText,
		},
	})

	text, actions := w.renderPage()
	if text != "你平时主要用哪些编程语言？" {
		t.Fatalf("page 1 text = %q", text)
	}
	if !navRowEquals(actions, "1/2", "跳过 →") {
		t.Fatalf("first-page nav = %v", actionLabels(actions))
	}
	if actionLabelsContain(actions, "多选") || actionLabelsContain(actions, "确认") {
		t.Fatalf("page must not repeat type or confirmation controls: %#v", actions)
	}

	// A selection is saved in place; the user decides when to advance.
	ready, needText, _ := applyAskUserCallback(w, askUserCallback{Op: "t", Token: w.Token, QIndex: 0, OIndex: 2})
	if ready || needText || w.Page != 0 {
		t.Fatalf("toggle: ready=%v needText=%v page=%d", ready, needText, w.Page)
	}
	text, actions = w.renderPage()
	if strings.Contains(text, "Go") {
		t.Fatalf("button selection must not be duplicated in body: %q", text)
	}
	if !actionLabelsContain(actions, "✓ Go") || !navRowEquals(actions, "1/2", "下一题 →") {
		t.Fatalf("selected page actions = %v", actionLabels(actions))
	}

	ready, needText, toast := applyAskUserCallback(w, askUserCallback{Op: "n", Token: w.Token, Page: 1})
	if toast != "" || ready || needText || w.Page != 1 {
		t.Fatalf("advance: ready=%v needText=%v page=%d toast=%q", ready, needText, w.Page, toast)
	}

	text, actions = w.renderPage()
	if text != "有没有最近在做的项目想聊聊？" {
		t.Fatalf("page 2 text = %q", text)
	}
	if !actionLabelsContain(actions, "填写答案") {
		t.Fatalf("text input button missing: %#v", actions)
	}
	if !navRowEquals(actions, "←", "2/2", "跳过并提交") {
		t.Fatalf("last-page nav = %v", actionLabels(actions))
	}

	// Replying to the last text question submits immediately; no second submit
	// action is required after the user has already sent their answer.
	w.TextPromptQ = "q2"
	ready, toast = applyAskUserTextAnswer(w, "Memoh Telegram 适配")
	if toast != "" || !ready || w.Page != 1 {
		t.Fatalf("text answer: ready=%v toast=%q", ready, toast)
	}
	answers := w.collectAnswers()
	if len(answers) != 2 {
		t.Fatalf("answers = %#v", answers)
	}
	ids, _ := answers[0]["option_ids"].([]any)
	if len(ids) != 1 || ids[0] != "q1.o3" {
		t.Fatalf("q1 = %#v", answers[0])
	}
	if answers[1]["text"] != "Memoh Telegram 适配" {
		t.Fatalf("q2 = %#v", answers[1])
	}
}

func TestAskUserWizardSingleSelectAutoAdvances(t *testing.T) {
	t.Parallel()

	w := newAskUserWizard("input-s", []askUserQuestion{
		{
			ID: "q1", Text: "速度？", Kind: userinput.QuestionKindSingleSelect,
			Options: []askUserOption{{ID: "q1.o1", Label: "快"}, {ID: "q1.o2", Label: "慢"}},
		},
		{ID: "q2", Text: "备注？", Kind: userinput.QuestionKindText},
	})

	text, actions := w.renderPage()
	if text != "速度？" || !navRowEquals(actions, "1/2", "跳过 →") {
		t.Fatalf("initial page: text=%q actions=%v", text, actionLabels(actions))
	}

	ready, needText, toast := applyAskUserCallback(w, askUserCallback{Op: "s", Token: w.Token, QIndex: 0, OIndex: 0})
	if toast != "" || ready || needText || w.Page != 1 {
		t.Fatalf("single select: ready=%v needText=%v page=%d toast=%q", ready, needText, w.Page, toast)
	}
	text, actions = w.renderPage()
	if text != "备注？" || !navRowEquals(actions, "←", "2/2", "跳过并提交") {
		t.Fatalf("next page: text=%q actions=%v", text, actionLabels(actions))
	}

	// Back still preserves the selection; tapping it again clears the answer.
	_, _, _ = applyAskUserCallback(w, askUserCallback{Op: "n", Token: w.Token, Page: 0})
	_, _, _ = applyAskUserCallback(w, askUserCallback{Op: "s", Token: w.Token, QIndex: 0, OIndex: 0})
	_, actions = w.renderPage()
	if !navRowEquals(actions, "1/2", "跳过 →") {
		t.Fatalf("cleared actions = %v", actionLabels(actions))
	}
}

func TestAskUserWizardCanSubmitSkippedQuestions(t *testing.T) {
	t.Parallel()

	w := newAskUserWizard("input-2", []askUserQuestion{
		{ID: "q1", Text: "Q1", Kind: userinput.QuestionKindText},
		{ID: "q2", Text: "Q2", Kind: userinput.QuestionKindText},
	})
	ready, _, toast := applyAskUserCallback(w, askUserCallback{Op: "n", Token: w.Token, Page: 1})
	if ready || toast != "" || w.Page != 1 {
		t.Fatalf("skip forward: ready=%v page=%d toast=%q", ready, w.Page, toast)
	}
	ready, _, toast = applyAskUserCallback(w, askUserCallback{Op: "go", Token: w.Token})
	if !ready || toast != "" {
		t.Fatalf("skip submit: ready=%v toast=%q", ready, toast)
	}
	answers := w.collectAnswers()
	if len(answers) != 2 || answers[0]["skipped"] != true || answers[1]["skipped"] != true {
		t.Fatalf("answers = %#v", answers)
	}
}

func TestAskUserSubmittedSummaryIncludesEveryQuestion(t *testing.T) {
	t.Parallel()

	w := newAskUserWizard("input-summary", []askUserQuestion{
		{
			ID: "q1", Text: "Tea?", Kind: userinput.QuestionKindSingleSelect,
			Options: []askUserOption{{ID: "q1.o1", Label: "Oolong"}, {ID: "q1.o2", Label: "Green"}},
		},
		{ID: "q2", Text: "Notes?", Kind: userinput.QuestionKindText},
	})
	w.draftFor("q1").OptionIDs = []string{"q1.o1"}
	w.draftFor("q1").Answered = true
	w.draftFor("q2").Text = "Current answer"
	w.draftFor("q2").Answered = true
	w.Page = 1

	got := formatAskUserSubmittedSummary(w)
	want := "1. Tea?\nOolong\n\n2. Notes?\nCurrent answer"
	if got != want {
		t.Fatalf("submitted page = %q", got)
	}
}

func TestAskUserSingleQuestionSelectionSubmitsImmediately(t *testing.T) {
	t.Parallel()

	w := newAskUserWizard("single", []askUserQuestion{{
		ID: "q1", Text: "Tea?", Kind: userinput.QuestionKindSingleSelect,
		Options: []askUserOption{{ID: "q1.o1", Label: "Oolong"}, {ID: "q1.o2", Label: "Green"}},
	}})
	ready, needText, toast := applyAskUserCallback(w, askUserCallback{Op: "s", Token: w.Token, QIndex: 0, OIndex: 0})
	if !ready || needText || toast != "" {
		t.Fatalf("single question selection: ready=%v needText=%v toast=%q", ready, needText, toast)
	}
}

func TestAskUserFreeTextTransitionMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		questions  []askUserQuestion
		page       int
		wantReady  bool
		wantPage   int
		wantCustom bool
	}{
		{
			name: "text advances from middle",
			questions: []askUserQuestion{
				{ID: "q1", Text: "Notes?", Kind: userinput.QuestionKindText},
				{ID: "q2", Text: "Done?", Kind: userinput.QuestionKindText},
			},
			wantPage: 1,
		},
		{
			name:      "text submits on last page",
			questions: []askUserQuestion{{ID: "q1", Text: "Notes?", Kind: userinput.QuestionKindText}},
			wantReady: true,
		},
		{
			name: "single custom advances",
			questions: []askUserQuestion{
				{ID: "q1", Text: "Plan?", Kind: userinput.QuestionKindSingleSelect, AllowCustom: true},
				{ID: "q2", Text: "Done?", Kind: userinput.QuestionKindText},
			},
			wantPage:   1,
			wantCustom: true,
		},
		{
			name:       "single custom submits on last page",
			questions:  []askUserQuestion{{ID: "q1", Text: "Plan?", Kind: userinput.QuestionKindSingleSelect, AllowCustom: true}},
			wantReady:  true,
			wantCustom: true,
		},
		{
			name: "multi custom stays for more choices",
			questions: []askUserQuestion{{
				ID: "q1", Text: "Languages?（多选）", Kind: userinput.QuestionKindMultiSelect, AllowCustom: true,
				Options: []askUserOption{{ID: "q1.o1", Label: "Go"}, {ID: "q1.o2", Label: "Rust"}},
			}},
			wantCustom: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newAskUserWizard("matrix", tt.questions)
			w.Page = tt.page
			w.TextPromptQ = tt.questions[tt.page].ID
			ready, toast := applyAskUserTextAnswer(w, "custom answer")
			if toast != "" || ready != tt.wantReady || w.Page != tt.wantPage {
				t.Fatalf("ready=%v page=%d toast=%q", ready, w.Page, toast)
			}
			d := w.draftFor(tt.questions[tt.page].ID)
			if tt.wantCustom && d.CustomText != "custom answer" {
				t.Fatalf("custom answer = %q", d.CustomText)
			}
			if !tt.wantCustom && d.Text != "custom answer" {
				t.Fatalf("text answer = %q", d.Text)
			}
		})
	}
}

func TestAskUserSubmittedSummaryMarksSkippedQuestions(t *testing.T) {
	t.Parallel()

	w := newAskUserWizard("skipped-summary", []askUserQuestion{
		{ID: "q1", Text: "First?", Kind: userinput.QuestionKindText},
		{ID: "q2", Text: "Second?", Kind: userinput.QuestionKindText},
	})
	w.draftFor("q2").Text = "Answer"
	w.draftFor("q2").Answered = true
	if got, want := formatAskUserSubmittedSummary(w), "1. First?\n跳过\n\n2. Second?\nAnswer"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestPrepareTelegramAskUserBuildsWizardActions(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	tc := &channel.StreamToolCall{
		Name: "ask_user",
		Input: map[string]any{
			"user_input_id": "input-9",
			"payload": map[string]any{
				"questions": []any{
					map[string]any{
						"id":   "q1",
						"text": "Speed?",
						"kind": "single_select",
						"options": []any{
							map[string]any{"id": "q1.o1", "label": "Fast"},
							map[string]any{"id": "q1.o2", "label": "Slow"},
						},
						"allow_custom": true,
					},
				},
			},
		},
	}
	text, actions, token, ok := adapter.prepareTelegramAskUser(tc)
	if !ok || token == "" {
		t.Fatalf("prepare ok=%v token=%q", ok, token)
	}
	if text != "Speed?" {
		t.Fatalf("text = %q", text)
	}
	foundOther := false
	for _, a := range actions {
		if strings.Contains(a.Label, "其他") {
			foundOther = true
			if !strings.HasPrefix(a.Value, "aui~x~") {
				t.Fatalf("Other callback = %q, want aui~x~", a.Value)
			}
		}
		if strings.Contains(a.Value, "/respond") {
			t.Fatalf("action leaked /respond: %#v", a)
		}
	}
	if !foundOther {
		t.Fatalf("missing 其他 action: %#v", actions)
	}
	if adapter.askUserStore().get(token) == nil {
		t.Fatal("wizard should be stored")
	}
}

func TestRenderAskUserChoiceShowsOnlyQuestion(t *testing.T) {
	t.Parallel()

	tc := &channel.StreamToolCall{
		Name: "ask_user",
		Input: map[string]any{
			"user_input_id": "input-1",
			"payload": map[string]any{
				"questions": []any{
					map[string]any{
						"text": "你更喜欢哪种编程语言？",
						"kind": "single_select",
						"options": []any{
							map[string]any{"id": "q1.o1", "label": "Python"},
							map[string]any{"id": "q1.o2", "label": "Go"},
						},
					},
				},
			},
		},
		Actions: []channel.Action{
			{Type: "user_input", Label: "Python", Value: "respond:input-1:q1.o1"},
			{Type: "user_input", Label: "Go", Value: "respond:input-1:q1.o2"},
		},
	}
	text, _ := renderToolCallPresentation(tc, channel.BuildToolCallStart(tc))
	if text != "你更喜欢哪种编程语言？" {
		t.Fatalf("rendered ask_user prompt = %q", text)
	}
}

func actionLabelsContain(actions []channel.Action, want string) bool {
	for _, a := range actions {
		if a.Label == want || strings.Contains(a.Label, want) {
			return true
		}
	}
	return false
}

func actionLabels(actions []channel.Action) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Label)
	}
	return out
}

// navRowEquals checks that the highest-numbered action row is exactly the
// expected left-to-right labels (page chrome).
func navRowEquals(actions []channel.Action, want ...string) bool {
	if len(actions) == 0 || len(want) == 0 {
		return false
	}
	maxRow := actions[0].Row
	for _, a := range actions {
		if a.Row > maxRow {
			maxRow = a.Row
		}
	}
	var got []string
	for _, a := range actions {
		if a.Row == maxRow {
			// Skip non-nav controls that share the last row only if labels
			// match Submit — page chrome is always the final nav row and
			// Submit sits on its own row after.
			got = append(got, a.Label)
		}
	}
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestAskUserWizardNavChromeLayout(t *testing.T) {
	t.Parallel()

	w := newAskUserWizard("nav", []askUserQuestion{
		{ID: "q1", Text: "Q1", Kind: userinput.QuestionKindText},
		{ID: "q2", Text: "Q2", Kind: userinput.QuestionKindText},
		{ID: "q3", Text: "Q3", Kind: userinput.QuestionKindText},
	})

	// first
	w.Page = 0
	_, actions := w.renderPage()
	if !navRowEquals(actions, "1/3", "跳过 →") {
		t.Fatalf("first = %v", actionLabels(actions))
	}

	// middle
	w.Page = 1
	w.draftFor("q1").Text = "a"
	w.draftFor("q1").Answered = true
	_, actions = w.renderPage()
	if !navRowEquals(actions, "←", "2/3", "跳过 →") {
		t.Fatalf("middle = %v", actionLabels(actions))
	}

	// last
	w.Page = 2
	w.draftFor("q2").Text = "b"
	w.draftFor("q2").Answered = true
	_, actions = w.renderPage()
	if !navRowEquals(actions, "←", "3/3", "跳过并提交") {
		t.Fatalf("last = %v", actionLabels(actions))
	}
}
