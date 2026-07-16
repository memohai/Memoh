package telegram

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/userinput"
)

// ask_user interactive callbacks use a short process-local wizard token so
// Telegram's 64-byte callback_data limit is never blown by UUIDs. Final submit
// still goes through the existing /respond continuation with structured
// Answers + user_input_id metadata — never by asking the human to type
// /respond.
const (
	askUserCallbackPrefix = "aui~"
	askUserWizardTTL      = 2 * time.Hour
)

type askUserOption struct {
	ID    string
	Label string
}

type askUserQuestion struct {
	ID          string
	Text        string
	Kind        string
	Options     []askUserOption
	AllowCustom bool
	Placeholder string
}

type askUserDraft struct {
	OptionIDs  []string
	CustomText string
	Text       string
	Answered   bool
}

type askUserWizard struct {
	Token        string
	UserInputID  string
	Questions    []askUserQuestion
	Page         int
	Drafts       map[string]*askUserDraft
	ChatID       int64
	MessageID    int
	CreatedAt    time.Time
	TextPromptID int    // force-reply prompt message id
	TextPromptQ  string // question id waiting for free-text
}

type askUserCallback struct {
	Op     string
	Token  string
	Page   int
	QIndex int
	OIndex int
}

func (a *TelegramAdapter) askUserStore() *askUserWizardStore {
	a.askUserOnce.Do(func() {
		a.askUserWizards = newAskUserWizardStore()
	})
	return a.askUserWizards
}

type askUserWizardStore struct {
	mu          sync.Mutex
	byToken     map[string]*askUserWizard
	textPrompts map[string]askUserTextPrompt // "chatID:msgID" → wizard/question
}

type askUserTextPrompt struct {
	Token      string
	QuestionID string
}

func newAskUserWizardStore() *askUserWizardStore {
	return &askUserWizardStore{
		byToken:     make(map[string]*askUserWizard),
		textPrompts: make(map[string]askUserTextPrompt),
	}
}

func (s *askUserWizardStore) put(w *askUserWizard) {
	if s == nil || w == nil || w.Token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(time.Now())
	s.byToken[w.Token] = w
}

func (s *askUserWizardStore) get(token string) *askUserWizard {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(time.Now())
	return s.byToken[strings.TrimSpace(token)]
}

func (s *askUserWizardStore) delete(token string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.byToken[token]
	delete(s.byToken, token)
	if w == nil {
		return
	}
	s.deleteTextPromptsLocked(token)
}

func (s *askUserWizardStore) bindTextPrompt(chatID int64, msgID int, token, questionID string) {
	if s == nil || chatID == 0 || msgID == 0 || token == "" || questionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.textPrompts[textPromptKey(chatID, msgID)] = askUserTextPrompt{Token: token, QuestionID: questionID}
}

func (s *askUserWizardStore) takeTextPrompt(chatID int64, msgID int) (token, questionID string, ok bool) {
	if s == nil {
		return "", "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := textPromptKey(chatID, msgID)
	prompt, ok := s.textPrompts[key]
	if ok {
		delete(s.textPrompts, key)
	}
	return prompt.Token, prompt.QuestionID, ok
}

func (s *askUserWizardStore) purgeExpiredLocked(now time.Time) {
	for token, w := range s.byToken {
		if w == nil || now.Sub(w.CreatedAt) > askUserWizardTTL {
			delete(s.byToken, token)
			s.deleteTextPromptsLocked(token)
		}
	}
}

func (s *askUserWizardStore) deleteTextPromptsLocked(token string) {
	for key, prompt := range s.textPrompts {
		if prompt.Token == token {
			delete(s.textPrompts, key)
		}
	}
}

func textPromptKey(chatID int64, msgID int) string {
	return strconv.FormatInt(chatID, 10) + ":" + strconv.Itoa(msgID)
}

func askUserToken(userInputID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(userInputID)))
	return fmt.Sprintf("%08x", h.Sum32())
}

func parseAskUserCallback(data string) (askUserCallback, bool) {
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, askUserCallbackPrefix) {
		return askUserCallback{}, false
	}
	parts := strings.Split(strings.TrimPrefix(data, askUserCallbackPrefix), "~")
	if len(parts) < 2 {
		return askUserCallback{}, false
	}
	cb := askUserCallback{
		Op:    strings.TrimSpace(parts[0]),
		Token: strings.TrimSpace(parts[1]),
	}
	if cb.Op == "" || cb.Token == "" {
		return askUserCallback{}, false
	}
	switch cb.Op {
	case "n":
		if len(parts) != 3 {
			return askUserCallback{}, false
		}
		page, err := strconv.Atoi(parts[2])
		if err != nil || page < 0 {
			return askUserCallback{}, false
		}
		cb.Page = page
		return cb, true
	case "s", "t":
		if len(parts) != 4 {
			return askUserCallback{}, false
		}
		qi, errQ := strconv.Atoi(parts[2])
		oi, errO := strconv.Atoi(parts[3])
		if errQ != nil || errO != nil || qi < 0 || oi < 0 {
			return askUserCallback{}, false
		}
		cb.QIndex, cb.OIndex = qi, oi
		return cb, true
	case "ok", "x":
		if len(parts) != 3 {
			return askUserCallback{}, false
		}
		qi, err := strconv.Atoi(parts[2])
		if err != nil || qi < 0 {
			return askUserCallback{}, false
		}
		cb.QIndex = qi
		return cb, true
	case "go":
		if len(parts) != 2 {
			return askUserCallback{}, false
		}
		return cb, true
	default:
		return askUserCallback{}, false
	}
}

func encodeAskUserCallback(parts ...string) string {
	return askUserCallbackPrefix + strings.Join(parts, "~")
}

func parseAskUserToolCall(tc *channel.StreamToolCall) (userInputID string, questions []askUserQuestion, ok bool) {
	if tc == nil || !strings.EqualFold(strings.TrimSpace(tc.Name), "ask_user") {
		return "", nil, false
	}
	in, _ := tc.Input.(map[string]any)
	if in == nil {
		return "", nil, false
	}
	userInputID = strings.TrimSpace(asString(in["user_input_id"]))
	payload, _ := in["payload"].(map[string]any)
	if payload == nil {
		payload, _ = in["payload"].(map[string]interface{})
	}
	// payload may be nested as map from JSON round-trip; also accept direct questions.
	rawQuestions := payload["questions"]
	if rawQuestions == nil {
		return userInputID, nil, userInputID != ""
	}
	list, _ := rawQuestions.([]any)
	if list == nil {
		if typed, ok := rawQuestions.([]map[string]any); ok {
			for _, item := range typed {
				list = append(list, item)
			}
		}
	}
	for i, raw := range list {
		qMap, _ := raw.(map[string]any)
		if qMap == nil {
			continue
		}
		q := askUserQuestion{
			ID:          strings.TrimSpace(asString(qMap["id"])),
			Text:        strings.TrimSpace(asString(qMap["text"])),
			Kind:        strings.TrimSpace(asString(qMap["kind"])),
			AllowCustom: asBool(qMap["allow_custom"]),
			Placeholder: strings.TrimSpace(asString(qMap["placeholder"])),
		}
		if q.ID == "" {
			q.ID = fmt.Sprintf("q%d", i+1)
		}
		if q.Kind == "" {
			q.Kind = userinput.QuestionKindText
		}
		opts, _ := qMap["options"].([]any)
		if opts == nil {
			if typed, ok := qMap["options"].([]map[string]any); ok {
				for _, item := range typed {
					opts = append(opts, item)
				}
			}
		}
		for j, rawOpt := range opts {
			oMap, _ := rawOpt.(map[string]any)
			if oMap == nil {
				continue
			}
			opt := askUserOption{
				ID:    strings.TrimSpace(asString(oMap["id"])),
				Label: strings.TrimSpace(asString(oMap["label"])),
			}
			if opt.Label == "" {
				continue
			}
			if opt.ID == "" {
				opt.ID = fmt.Sprintf("%s.o%d", q.ID, j+1)
			}
			q.Options = append(q.Options, opt)
		}
		questions = append(questions, q)
	}
	return userInputID, questions, userInputID != "" && len(questions) > 0
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func newAskUserWizard(userInputID string, questions []askUserQuestion) *askUserWizard {
	w := &askUserWizard{
		Token:       askUserToken(userInputID),
		UserInputID: strings.TrimSpace(userInputID),
		Questions:   questions,
		Drafts:      make(map[string]*askUserDraft, len(questions)),
		CreatedAt:   time.Now(),
	}
	for _, q := range questions {
		w.Drafts[q.ID] = &askUserDraft{}
	}
	return w
}

func (w *askUserWizard) questionAt(page int) (askUserQuestion, bool) {
	if w == nil || page < 0 || page >= len(w.Questions) {
		return askUserQuestion{}, false
	}
	return w.Questions[page], true
}

func (w *askUserWizard) draftFor(qID string) *askUserDraft {
	if w == nil {
		return &askUserDraft{}
	}
	if d, ok := w.Drafts[qID]; ok && d != nil {
		return d
	}
	d := &askUserDraft{}
	if w.Drafts == nil {
		w.Drafts = make(map[string]*askUserDraft)
	}
	w.Drafts[qID] = d
	return d
}

func (w *askUserWizard) collectAnswers() []map[string]any {
	out := make([]map[string]any, 0, len(w.Questions))
	for _, q := range w.Questions {
		d := w.draftFor(q.ID)
		entry := map[string]any{"question_id": q.ID}
		if !d.Answered {
			entry["skipped"] = true
			out = append(out, entry)
			continue
		}
		if len(d.OptionIDs) > 0 {
			ids := make([]any, 0, len(d.OptionIDs))
			for _, id := range d.OptionIDs {
				ids = append(ids, id)
			}
			entry["option_ids"] = ids
		}
		if strings.TrimSpace(d.CustomText) != "" {
			entry["custom_text"] = d.CustomText
		}
		if strings.TrimSpace(d.Text) != "" {
			entry["text"] = d.Text
		}
		out = append(out, entry)
	}
	return out
}

// Each question is one page. Choices update the draft in place; navigation is
// always explicit so users can review, change, or skip any question. The card
// only renders the current question and its current answer.
func (w *askUserWizard) renderPage() (text string, actions []channel.Action) {
	if w == nil || len(w.Questions) == 0 {
		return "Input requested", nil
	}
	if w.Page < 0 {
		w.Page = 0
	}
	if w.Page >= len(w.Questions) {
		w.Page = len(w.Questions) - 1
	}
	q, ok := w.questionAt(w.Page)
	if !ok {
		return "Input requested", nil
	}
	var b strings.Builder
	b.WriteString(q.Text)
	d := w.draftFor(q.ID)
	// Choice buttons already show their selection state. Free text and custom
	// values stay in the body because they cannot fit in a button label.
	if ans := formatAskUserBodyAnswer(q, d); ans != "" {
		b.WriteString("\n\n")
		b.WriteString(ans)
	}

	row := 0
	switch q.Kind {
	case userinput.QuestionKindSingleSelect:
		for oi, opt := range q.Options {
			label := opt.Label
			if d.Answered && len(d.OptionIDs) == 1 && d.OptionIDs[0] == opt.ID && d.CustomText == "" {
				label = "✓ " + label
			}
			actions = append(actions, channel.Action{
				Type:  "user_input",
				Label: truncateAskUserLabel(label),
				Value: encodeAskUserCallback("s", w.Token, strconv.Itoa(w.Page), strconv.Itoa(oi)),
				Row:   row,
			})
			if (oi+1)%2 == 0 {
				row++
			}
		}
		if len(q.Options)%2 != 0 {
			row++
		}
		if q.AllowCustom {
			label := "其他…"
			if d.Answered && strings.TrimSpace(d.CustomText) != "" {
				label = "✓ 其他…"
			}
			actions = append(actions, channel.Action{
				Type:  "user_input",
				Label: label,
				Value: encodeAskUserCallback("x", w.Token, strconv.Itoa(w.Page)),
				Row:   row,
			})
			row++
		}
	case userinput.QuestionKindMultiSelect:
		for oi, opt := range q.Options {
			label := opt.Label
			if containsString(d.OptionIDs, opt.ID) {
				label = "✓ " + label
			}
			actions = append(actions, channel.Action{
				Type:  "user_input",
				Label: truncateAskUserLabel(label),
				Value: encodeAskUserCallback("t", w.Token, strconv.Itoa(w.Page), strconv.Itoa(oi)),
				Row:   row,
			})
			if (oi+1)%2 == 0 {
				row++
			}
		}
		if len(q.Options)%2 != 0 {
			row++
		}
		if q.AllowCustom {
			label := "其他…"
			if strings.TrimSpace(d.CustomText) != "" {
				label = "✓ 其他…"
			}
			actions = append(actions, channel.Action{
				Type:  "user_input",
				Label: label,
				Value: encodeAskUserCallback("x", w.Token, strconv.Itoa(w.Page)),
				Row:   row,
			})
			row++
		}
	default: // text
		label := "填写答案"
		if d.Answered {
			label = "重新填写"
		}
		actions = append(actions, channel.Action{
			Type:  "user_input",
			Label: label,
			Value: encodeAskUserCallback("x", w.Token, strconv.Itoa(w.Page)),
			Row:   row,
		})
		row++
	}

	if len(w.Questions) == 1 {
		label := "提交"
		if !d.Answered {
			label = "跳过"
		}
		actions = append(actions, channel.Action{
			Type:  "user_input",
			Label: label,
			Value: encodeAskUserCallback("go", w.Token),
			Row:   row,
		})
		return b.String(), actions
	}

	if w.Page > 0 {
		actions = append(actions, channel.Action{
			Type:  "user_input",
			Label: "←",
			Value: encodeAskUserCallback("n", w.Token, strconv.Itoa(w.Page-1)),
			Row:   row,
		})
	}
	actions = append(actions, channel.Action{
		Type:  "user_input",
		Label: fmt.Sprintf("%d/%d", w.Page+1, len(w.Questions)),
		Value: encodeAskUserCallback("n", w.Token, strconv.Itoa(w.Page)),
		Row:   row,
	})
	if w.Page < len(w.Questions)-1 {
		label := "下一题 →"
		if !d.Answered {
			label = "跳过 →"
		}
		actions = append(actions, channel.Action{
			Type:  "user_input",
			Label: label,
			Value: encodeAskUserCallback("n", w.Token, strconv.Itoa(w.Page+1)),
			Row:   row,
		})
	} else {
		label := "提交"
		if !d.Answered {
			label = "跳过并提交"
		}
		actions = append(actions, channel.Action{
			Type:  "user_input",
			Label: label,
			Value: encodeAskUserCallback("go", w.Token),
			Row:   row,
		})
	}

	return b.String(), actions
}

func formatAskUserBodyAnswer(q askUserQuestion, d *askUserDraft) string {
	if d == nil || !d.Answered {
		return ""
	}
	if q.Kind == userinput.QuestionKindText {
		return strings.TrimSpace(d.Text)
	}
	if custom := strings.TrimSpace(d.CustomText); custom != "" {
		return "其他：" + custom
	}
	return ""
}

// formatDraftAnswer returns the saved answer for the current page.
func formatDraftAnswer(q askUserQuestion, d *askUserDraft) string {
	if d == nil || !d.Answered {
		return ""
	}
	switch q.Kind {
	case userinput.QuestionKindText:
		return strings.TrimSpace(d.Text)
	case userinput.QuestionKindSingleSelect:
		if custom := strings.TrimSpace(d.CustomText); custom != "" {
			return custom
		}
		if len(d.OptionIDs) == 1 {
			for _, opt := range q.Options {
				if opt.ID == d.OptionIDs[0] {
					return opt.Label
				}
			}
			return d.OptionIDs[0]
		}
		return ""
	case userinput.QuestionKindMultiSelect:
		parts := make([]string, 0, len(d.OptionIDs)+1)
		for _, id := range d.OptionIDs {
			label := id
			for _, opt := range q.Options {
				if opt.ID == id {
					label = opt.Label
					break
				}
			}
			parts = append(parts, label)
		}
		if custom := strings.TrimSpace(d.CustomText); custom != "" {
			parts = append(parts, custom)
		}
		return strings.Join(parts, ", ")
	default:
		if s := strings.TrimSpace(d.Text); s != "" {
			return s
		}
		return strings.TrimSpace(d.CustomText)
	}
}

func truncateAskUserLabel(label string) string {
	label = strings.TrimSpace(label)
	// Telegram button labels are soft-limited; keep readable.
	const maxRunes = 40
	runes := []rune(label)
	if len(runes) <= maxRunes {
		return label
	}
	return string(runes[:maxRunes-1]) + "…"
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func toggleString(list []string, target string) []string {
	out := make([]string, 0, len(list)+1)
	found := false
	for _, item := range list {
		if item == target {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		out = append(out, target)
	}
	return out
}

// applyAskUserCallback mutates the wizard for a callback. ready means the
// wizard has a complete answer set and should be submitted. needText means
// the adapter should prompt for free-text (force-reply). toast is a short
// callback answer shown as a Telegram alert/toast.
func applyAskUserCallback(w *askUserWizard, cb askUserCallback) (ready, needText bool, toast string) {
	if w == nil {
		return false, false, "已过期"
	}
	switch cb.Op {
	case "n":
		if cb.Page < 0 || cb.Page >= len(w.Questions) {
			return false, false, ""
		}
		w.Page = cb.Page
		return false, false, ""
	case "s":
		q, ok := w.questionAt(cb.QIndex)
		if !ok || cb.OIndex < 0 || cb.OIndex >= len(q.Options) {
			return false, false, "无效操作"
		}
		if w.Page != cb.QIndex {
			w.Page = cb.QIndex
		}
		d := w.draftFor(q.ID)
		selectedID := q.Options[cb.OIndex].ID
		if d.Answered && len(d.OptionIDs) == 1 && d.OptionIDs[0] == selectedID && d.CustomText == "" {
			d.OptionIDs = nil
			d.Answered = false
			return false, false, ""
		}
		d.OptionIDs = []string{selectedID}
		d.CustomText = ""
		d.Text = ""
		d.Answered = true
		if w.Page < len(w.Questions)-1 {
			w.Page++
			return false, false, ""
		}
		return true, false, ""
	case "t":
		q, ok := w.questionAt(cb.QIndex)
		if !ok || cb.OIndex < 0 || cb.OIndex >= len(q.Options) {
			return false, false, "无效操作"
		}
		if w.Page != cb.QIndex {
			w.Page = cb.QIndex
		}
		d := w.draftFor(q.ID)
		d.OptionIDs = toggleString(d.OptionIDs, q.Options[cb.OIndex].ID)
		d.Answered = len(d.OptionIDs) > 0 || strings.TrimSpace(d.CustomText) != ""
		return false, false, ""
	case "x":
		q, ok := w.questionAt(cb.QIndex)
		if !ok {
			return false, false, "无效操作"
		}
		if w.Page != cb.QIndex {
			w.Page = cb.QIndex
		}
		w.TextPromptQ = q.ID
		return false, true, ""
	case "go":
		return true, false, ""
	default:
		return false, false, "未知操作"
	}
}

func applyAskUserTextAnswer(w *askUserWizard, text string) (ready bool, toast string) {
	if w == nil {
		return false, "已过期"
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false, "答案不能为空"
	}
	qID := strings.TrimSpace(w.TextPromptQ)
	if qID == "" {
		// Fall back to current page.
		if q, ok := w.questionAt(w.Page); ok {
			qID = q.ID
		}
	}
	var q askUserQuestion
	found := false
	for i, item := range w.Questions {
		if item.ID == qID {
			q = item
			found = true
			w.Page = i
			break
		}
	}
	if !found {
		return false, "无效操作"
	}
	d := w.draftFor(q.ID)
	switch q.Kind {
	case userinput.QuestionKindText:
		d.Text = text
		d.OptionIDs = nil
		d.CustomText = ""
		d.Answered = true
	case userinput.QuestionKindSingleSelect:
		if !q.AllowCustom {
			return false, "本题不支持自定义"
		}
		d.CustomText = text
		d.OptionIDs = nil
		d.Text = ""
		d.Answered = true
	case userinput.QuestionKindMultiSelect:
		if !q.AllowCustom {
			return false, "本题不支持自定义"
		}
		d.CustomText = text
		// Custom alone (or with toggled options) is a complete multi answer.
		d.Answered = true
	default:
		d.Text = text
		d.Answered = true
	}
	w.TextPromptQ = ""
	w.TextPromptID = 0
	// Text and single-select custom answers are complete actions: advance to the
	// next question, or submit immediately on the last page. Multi-select custom
	// text stays on the page because the user may still add regular options.
	if q.Kind != userinput.QuestionKindMultiSelect {
		if w.Page < len(w.Questions)-1 {
			w.Page++
			return false, ""
		}
		return true, ""
	}
	return false, ""
}

// prepareTelegramAskUser builds wizard presentation for an outbound ask_user
// tool card. Returns ok=false when the tool call is not an ask_user prompt.
func (a *TelegramAdapter) prepareTelegramAskUser(tc *channel.StreamToolCall) (text string, actions []channel.Action, token string, ok bool) {
	userInputID, questions, parsed := parseAskUserToolCall(tc)
	if !parsed {
		return "", nil, "", false
	}
	w := newAskUserWizard(userInputID, questions)
	// Single-question single_select without multi-step needs still benefits
	// from wizard (Other → force-reply, no /respond). Always use wizard.
	text, actions = w.renderPage()
	if a != nil {
		a.askUserStore().put(w)
	}
	return text, actions, w.Token, true
}

func (a *TelegramAdapter) bindAskUserMessage(token string, chatID int64, msgID int) {
	if a == nil || token == "" {
		return
	}
	w := a.askUserStore().get(token)
	if w == nil {
		return
	}
	w.ChatID = chatID
	w.MessageID = msgID
	a.askUserStore().put(w)
}
