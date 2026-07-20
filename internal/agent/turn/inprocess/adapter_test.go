package inprocess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/conversation"
)

type fakeRunner struct {
	gotReq conversation.ChatRequest
	chunks []string
	block  chan struct{} // when non-nil, stream waits before emitting
}

func (f *fakeRunner) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	f.gotReq = req
	ch := make(chan conversation.StreamChunk, len(f.chunks))
	errCh := make(chan error)
	go func() {
		defer close(ch)
		defer close(errCh)
		if f.block != nil {
			select {
			case <-f.block:
			case <-ctx.Done():
				return
			}
		}
		for _, c := range f.chunks {
			select {
			case ch <- conversation.StreamChunk(c):
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, errCh
}

func TestStartTurnRequiresTeamID(t *testing.T) {
	a := New(&fakeRunner{})
	_, err := a.StartTurn(context.Background(), turn.StartTurnCommand{Mode: turn.ModeChat})
	if err == nil {
		t.Fatal("want error for empty TeamID")
	}
}

func TestStartTurnStreamsEvents(t *testing.T) {
	r := &fakeRunner{chunks: []string{`{"type":"text_delta","text":"a"}`, `{"type":"done"}`}}
	a := New(r)
	h, err := a.StartTurn(context.Background(), turn.StartTurnCommand{
		TeamID: "team1", Mode: turn.ModeChat, BotID: "b", SessionID: "s", Query: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []turn.Event
	for e := range h.Events() {
		events = append(events, e)
	}
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	if events[0].Kind != "text_delta" || events[1].Kind != "done" {
		t.Fatalf("kinds = %q, %q", events[0].Kind, events[1].Kind)
	}
	if events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("seq not monotonic: %d, %d", events[0].Seq, events[1].Seq)
	}
	if string(events[0].Payload) != r.chunks[0] {
		t.Fatalf("payload mutated: %s", events[0].Payload)
	}
	if events[0].TeamID != "team1" || events[0].SessionID != "s" {
		t.Fatalf("event context missing: %+v", events[0])
	}
	if h.RunID() == "" || events[0].RunID != h.RunID() {
		t.Fatalf("run id mismatch: handle=%q event=%q", h.RunID(), events[0].RunID)
	}
	if r.gotReq.BotID != "b" || r.gotReq.Query != "hi" {
		t.Fatalf("ChatRequest not translated: %+v", r.gotReq)
	}
	for range h.Errs() {
	}
}

func TestInjectAndAssets(t *testing.T) {
	r := &fakeRunner{chunks: []string{`{"type":"done"}`}, block: make(chan struct{})}
	a := New(r)
	h, err := a.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "t", Mode: turn.ModeChat})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Inject(context.Background(), turn.InjectMessage{Text: "more"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-r.gotReq.InjectCh:
		if got.Text != "more" {
			t.Fatalf("inject text = %q", got.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("inject not delivered")
	}
	h.AddOutboundAssets([]turn.OutboundAssetRef{{ContentHash: "h1", Role: "attachment", Ordinal: 0}})
	refs := r.gotReq.OutboundAssetCollector()
	if len(refs) != 1 || refs[0].ContentHash != "h1" {
		t.Fatalf("assets = %+v", refs)
	}
	close(r.block)
	for range h.Events() {
	}
	for range h.Errs() {
	}
}

func TestCancelClosesEvents(t *testing.T) {
	r := &fakeRunner{chunks: []string{`{"type":"done"}`}, block: make(chan struct{})}
	a := New(r)
	h, err := a.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "t", Mode: turn.ModeChat})
	if err != nil {
		t.Fatal(err)
	}
	h.Cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-h.Events():
			if !ok {
				return // closed as expected
			}
		case <-deadline:
			t.Fatal("events channel not closed after cancel")
		}
	}
}

func TestCommandFieldTranslation(t *testing.T) {
	r := &fakeRunner{}
	a := New(r)
	cmd := turn.StartTurnCommand{
		TeamID: "t", Mode: turn.ModeChat,
		BotID: "bot", ChatID: "chat", SessionID: "sess", RouteID: "route",
		Token: "tok", ChatToken: "ctok", UserID: "u", SourceChannelIdentityID: "ci",
		DisplayName: "dn", ExternalMessageID: "ext", EventID: "ev",
		Query: "q", ModelQuery: "mq", UserMessageKind: "kind", UserVisibleText: "uvt",
		Attachments: []turn.Attachment{{Type: "image", ContentHash: "ch1", Name: "a.png"}},
		ReplyTarget: "rt", ConversationType: "group", ConversationName: "cn",
		SourceReplyToMessageID: "srm", ReplySender: "rs", ReplyPreview: "rp",
		ReplyAttachments: []turn.Attachment{{Type: "file"}},
		MentionsBot:      true, RepliesToBot: true,
		ForwardMessageID: "fm", ForwardFromUserID: "fu", ForwardFromConversationID: "fc",
		ForwardSender: "fs", ForwardDate: 42,
		CurrentChannel: "telegram", Channels: []string{"telegram"},
		Model: "m1", ReasoningEffort: "high", WorkspaceTargetID: "wt",
		SkillActivation:      &turn.SkillActivation{Prompt: "p", Skills: []turn.SkillActivationSkill{{Name: "sk"}}},
		RequestedSkills:      []turn.RequestedSkillContext{{Name: "rs1", ContentHash: "rh"}},
		SkipMemoryExtraction: true, SkipTitleGeneration: true, UserMessagePersisted: true,
	}
	h, err := a.StartTurn(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	for range h.Events() {
	}
	got := r.gotReq
	checks := map[string][2]string{
		"BotID":                     {got.BotID, "bot"},
		"ChatID":                    {got.ChatID, "chat"},
		"SessionID":                 {got.SessionID, "sess"},
		"RouteID":                   {got.RouteID, "route"},
		"Token":                     {got.Token, "tok"},
		"ChatToken":                 {got.ChatToken, "ctok"},
		"UserID":                    {got.UserID, "u"},
		"SourceChannelIdentityID":   {got.SourceChannelIdentityID, "ci"},
		"DisplayName":               {got.DisplayName, "dn"},
		"ExternalMessageID":         {got.ExternalMessageID, "ext"},
		"EventID":                   {got.EventID, "ev"},
		"Query":                     {got.Query, "q"},
		"ModelQuery":                {got.ModelQuery, "mq"},
		"UserMessageKind":           {got.UserMessageKind, "kind"},
		"UserVisibleText":           {got.UserVisibleText, "uvt"},
		"ReplyTarget":               {got.ReplyTarget, "rt"},
		"ConversationType":          {got.ConversationType, "group"},
		"ConversationName":          {got.ConversationName, "cn"},
		"SourceReplyToMessageID":    {got.SourceReplyToMessageID, "srm"},
		"ReplySender":               {got.ReplySender, "rs"},
		"ReplyPreview":              {got.ReplyPreview, "rp"},
		"ForwardMessageID":          {got.ForwardMessageID, "fm"},
		"ForwardFromUserID":         {got.ForwardFromUserID, "fu"},
		"ForwardFromConversationID": {got.ForwardFromConversationID, "fc"},
		"ForwardSender":             {got.ForwardSender, "fs"},
		"CurrentChannel":            {got.CurrentChannel, "telegram"},
		"Model":                     {got.Model, "m1"},
		"ReasoningEffort":           {got.ReasoningEffort, "high"},
		"WorkspaceTargetID":         {got.WorkspaceTargetID, "wt"},
	}
	for name, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if !got.MentionsBot || !got.RepliesToBot || !got.SkipMemoryExtraction || !got.SkipTitleGeneration || !got.UserMessagePersisted {
		t.Error("bool fields not translated")
	}
	if got.ForwardDate != 42 {
		t.Errorf("ForwardDate = %d", got.ForwardDate)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].ContentHash != "ch1" || got.Attachments[0].Name != "a.png" {
		t.Errorf("Attachments = %+v", got.Attachments)
	}
	if len(got.ReplyAttachments) != 1 || got.ReplyAttachments[0].Type != "file" {
		t.Errorf("ReplyAttachments = %+v", got.ReplyAttachments)
	}
	if got.SkillActivation == nil || got.SkillActivation.Prompt != "p" || len(got.SkillActivation.Skills) != 1 {
		t.Errorf("SkillActivation = %+v", got.SkillActivation)
	}
	if len(got.RequestedSkills) != 1 || got.RequestedSkills[0].Name != "rs1" || got.RequestedSkills[0].ContentHash != "rh" {
		t.Errorf("RequestedSkills = %+v", got.RequestedSkills)
	}
	if len(got.Channels) != 1 || got.Channels[0] != "telegram" {
		t.Errorf("Channels = %+v", got.Channels)
	}
}

func TestStartTurnRejectsForeignTeam(t *testing.T) {
	a := New(&fakeRunner{}, WithAllowedTeam("team-home"))
	_, err := a.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "team-other", Mode: turn.ModeChat})
	if !errors.Is(err, turn.ErrTeamNotServed) {
		t.Fatalf("err = %v, want ErrTeamNotServed", err)
	}
	if _, err := a.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "team-home", Mode: turn.ModeChat}); err != nil {
		t.Fatalf("home team rejected: %v", err)
	}
}

func TestStartTurnClaimsIdempotencyKey(t *testing.T) {
	a := New(&fakeRunner{})
	first, err := a.StartTurn(context.Background(), turn.StartTurnCommand{
		TeamID: "t", Mode: turn.ModeChat, IdempotencyKey: "msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range first.Events() {
	}
	// Redelivery of the same platform message must be rejected even after
	// the first run completed.
	if _, err := a.StartTurn(context.Background(), turn.StartTurnCommand{
		TeamID: "t", Mode: turn.ModeChat, IdempotencyKey: "msg-1",
	}); !errors.Is(err, turn.ErrDuplicateTurn) {
		t.Fatalf("err = %v, want ErrDuplicateTurn", err)
	}
	// Same key under another team is a distinct claim.
	if _, err := a.StartTurn(context.Background(), turn.StartTurnCommand{
		TeamID: "t2", Mode: turn.ModeChat, IdempotencyKey: "msg-1",
	}); err != nil {
		t.Fatalf("cross-team claim rejected: %v", err)
	}
	// Empty keys are never deduplicated.
	for range 2 {
		if _, err := a.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "t", Mode: turn.ModeChat}); err != nil {
			t.Fatalf("empty key rejected: %v", err)
		}
	}
}

func TestFailedStartDoesNotClaimIdempotencyKey(t *testing.T) {
	a := New(&fakeRunner{})
	cmd := turn.StartTurnCommand{
		TeamID: "t", Mode: turn.ModeDiscuss, IdempotencyKey: "msg-1",
	}
	for range 2 {
		_, err := a.StartTurn(context.Background(), cmd)
		if err == nil {
			t.Fatal("expected unconfigured discuss runtime error")
		}
		if errors.Is(err, turn.ErrDuplicateTurn) {
			t.Fatalf("failed start claimed idempotency key: %v", err)
		}
	}
}

// TestCancelUnblocksFullEventBuffer reproduces the reviewer's 32-event
// burst: with no consumer and a full buffer, Cancel must still unblock
// the pump and close both channels.
func TestCancelUnblocksFullEventBuffer(t *testing.T) {
	chunks := make([]string, 40)
	for i := range chunks {
		chunks[i] = `{"type":"text_delta","delta":"x"}`
	}
	r := &fakeRunner{chunks: chunks}
	a := New(r)
	h, err := a.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "t", Mode: turn.ModeChat})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // let the pump fill the buffer
	h.Cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-h.Events():
			if !ok {
				for range h.Errs() {
				}
				return
			}
		case <-deadline:
			t.Fatal("events channel not closed after cancel with full buffer")
		}
	}
}
