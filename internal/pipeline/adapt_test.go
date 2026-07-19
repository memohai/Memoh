package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

func TestAdaptInbound_FallsBackToText_WhenPartsEmpty(t *testing.T) {
	msg := channel.InboundMessage{
		Channel:    channel.ChannelTypeTelegram,
		Message:    channel.Message{ID: "m1", Text: "plain hello"},
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
	}
	ev := AdaptInbound(msg, "sess", "ci", "Alice")
	me, ok := ev.(MessageEvent)
	if !ok {
		t.Fatalf("expected MessageEvent, got %T", ev)
	}
	if len(me.Content) != 1 || me.Content[0].Type != "text" || me.Content[0].Text != "plain hello" {
		t.Fatalf("expected single text node, got %+v", me.Content)
	}
}

func TestAdaptInbound_PreservesMention_FromParts(t *testing.T) {
	msg := channel.InboundMessage{
		Channel: channel.ChannelTypeTelegram,
		Message: channel.Message{
			ID:     "m1",
			Format: channel.MessageFormatRich,
			Text:   "hello @bot",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "hello "},
				{Type: channel.MessagePartMention, Text: "@bot", ChannelIdentityID: "ci-bot"},
			},
		},
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
	}
	me := AdaptInbound(msg, "sess", "ci", "Alice").(MessageEvent)
	if len(me.Content) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(me.Content), me.Content)
	}
	if me.Content[0].Type != "text" || me.Content[0].Text != "hello " {
		t.Fatalf("expected leading text node, got %+v", me.Content[0])
	}
	mention := me.Content[1]
	if mention.Type != "mention" || mention.UserID != "ci-bot" {
		t.Fatalf("expected mention with UserID=ci-bot, got %+v", mention)
	}
	if len(mention.Children) != 1 || mention.Children[0].Type != "text" || mention.Children[0].Text != "@bot" {
		t.Fatalf("expected mention to wrap text child '@bot', got %+v", mention.Children)
	}
}

func TestAdaptInbound_PreservesAddressingMetadata(t *testing.T) {
	msg := channel.InboundMessage{
		Channel: channel.ChannelTypeTelegram,
		Message: channel.Message{ID: "m1", Text: "@akazwz_bot hi"},
		Metadata: map[string]any{
			"is_mentioned":    true,
			"is_reply_to_bot": "true",
		},
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
	}
	me := AdaptInbound(msg, "sess", "ci-user", "Alice").(MessageEvent)
	if !me.MentionsMe {
		t.Fatal("expected is_mentioned metadata to set MessageEvent.MentionsMe")
	}
	if !me.RepliesToMe {
		t.Fatal("expected is_reply_to_bot metadata to set MessageEvent.RepliesToMe")
	}

	ic := Reduce(NewEmptyIC("sess"), me)
	rc := Render(ic, RenderParams{})
	if len(rc) != 1 {
		t.Fatalf("expected one rendered segment, got %d", len(rc))
	}
	if !rc[0].MentionsMe {
		t.Fatal("expected rendered segment to preserve MentionsMe without BotUserID")
	}
	if !rc[0].RepliesToMe {
		t.Fatal("expected rendered segment to preserve RepliesToMe without BotUserID")
	}
}

func TestAdaptInbound_EditPreservesEventIdentityAndAddressing(t *testing.T) {
	t.Parallel()

	event := AdaptInbound(channel.InboundMessage{
		Message:    channel.Message{ID: "message", Text: "edited @bot"},
		IsSelfSent: true,
		Metadata: map[string]any{
			"event_type":      "edit",
			"event_id":        "edit-delivery-1",
			"is_mentioned":    true,
			"is_reply_to_bot": false,
		},
	}, "sess", "sender", "Alice").(EditEvent)

	if event.EventID != "event_id:edit-delivery-1" {
		t.Fatalf("event id = %q, want stable metadata identity", event.EventID)
	}
	if !event.AddressingKnown || !event.MentionsMe || event.RepliesToMe {
		t.Fatalf("edit addressing = known:%t mention:%t reply:%t", event.AddressingKnown, event.MentionsMe, event.RepliesToMe)
	}
	if !event.IsSelfSent {
		t.Fatal("edit lost self-sent marker")
	}
}

func TestReduceEditRefreshesAddressingMetadata(t *testing.T) {
	t.Parallel()

	ic := Reduce(NewEmptyIC("sess"), MessageEvent{
		SessionID:   "sess",
		MessageID:   "message",
		EventCursor: 10,
		MentionsMe:  true,
		RepliesToMe: true,
		IsSelfSent:  false,
	})
	ic = Reduce(ic, EditEvent{
		SessionID:       "sess",
		MessageID:       "message",
		EventCursor:     20,
		AddressingKnown: true,
		MentionsMe:      false,
		RepliesToMe:     false,
	})

	rendered := Render(ic, RenderParams{})
	if rendered[0].MentionsMe || rendered[0].RepliesToMe {
		t.Fatalf("edited addressing remained mention:%t reply:%t", rendered[0].MentionsMe, rendered[0].RepliesToMe)
	}
}

func TestAdaptInbound_PreservesSelfSentMarker(t *testing.T) {
	t.Parallel()

	msg := channel.InboundMessage{
		Message:    channel.Message{ID: "echo", Text: "sent by this bot"},
		IsSelfSent: true,
	}

	event := AdaptInbound(msg, "sess", "bot-channel-identity", "Bot").(MessageEvent)
	if !event.IsSelfSent {
		t.Fatal("expected inbound self marker to set MessageEvent.IsSelfSent")
	}
	rendered := Render(Reduce(NewEmptyIC("sess"), event), RenderParams{})
	if got := LatestExternalEventCursor(rendered, 0); got != 0 {
		t.Fatalf("self-sent event activated external trigger at %d", got)
	}
}

func TestSelfSentRenameDoesNotActivateExternalTrigger(t *testing.T) {
	t.Parallel()

	ic := Reduce(NewEmptyIC("sess"), MessageEvent{
		SessionID:   "sess",
		MessageID:   "external",
		Sender:      &CanonicalUser{ID: "bot", DisplayName: "Old Bot"},
		EventCursor: 10,
	})
	ic = Reduce(ic, MessageEvent{
		SessionID:   "sess",
		MessageID:   "echo",
		Sender:      &CanonicalUser{ID: "bot", DisplayName: "New Bot"},
		EventCursor: 20,
		IsSelfSent:  true,
	})

	if got := LatestExternalEventCursor(Render(ic, RenderParams{}), 10); got != 0 {
		t.Fatalf("self-sent rename activated external trigger at %d", got)
	}
}

func TestAdaptInbound_PreservesSelfSentServiceMarker(t *testing.T) {
	t.Parallel()

	event := AdaptInbound(channel.InboundMessage{
		IsSelfSent: true,
		Metadata: map[string]any{
			"event_type":     "service",
			"service_action": string(ServiceChatRenamed),
			"new_title":      "new title",
		},
	}, "sess", "", "")

	if got := LatestExternalEventCursor(Render(Reduce(NewEmptyIC("sess"), event), RenderParams{}), 0); got != 0 {
		t.Fatalf("self-sent service event activated external trigger at %d", got)
	}
}

func TestAdaptInbound_PreservesLink_FromParts(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, Text: "Memoh", URL: "https://example.com"},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 1 || me.Content[0].Type != "link" {
		t.Fatalf("expected single link node, got %+v", me.Content)
	}
	if me.Content[0].URL != "https://example.com" {
		t.Fatalf("expected URL preserved, got %+v", me.Content[0])
	}
	kids := me.Content[0].Children
	if len(kids) != 1 || kids[0].Type != "text" || kids[0].Text != "Memoh" {
		t.Fatalf("expected link to wrap text 'Memoh', got %+v", kids)
	}
}

func TestAdaptInbound_CodeBlockIsLeafWithLanguage(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartCodeBlock, Text: "fn main(){}", Language: "rust"},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 1 {
		t.Fatalf("got %+v", me.Content)
	}
	got := me.Content[0]
	if got.Type != "pre" || got.Language != "rust" || got.Text != "fn main(){}" {
		t.Fatalf("expected leaf pre node with language=rust, got %+v", got)
	}
	if len(got.Children) != 0 {
		t.Fatalf("pre should be a leaf, got children %+v", got.Children)
	}
}

func TestAdaptInbound_InlineCodeStyleIsLeaf(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "x", Styles: []channel.MessageTextStyle{channel.MessageStyleCode}},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 1 || me.Content[0].Type != "code" || me.Content[0].Text != "x" {
		t.Fatalf("expected leaf code node, got %+v", me.Content)
	}
	if len(me.Content[0].Children) != 0 {
		t.Fatalf("inline code should be a leaf, got children %+v", me.Content[0].Children)
	}
}

func TestAdaptInbound_NestsCompoundStyles(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{
					Type:   channel.MessagePartText,
					Text:   "shout",
					Styles: []channel.MessageTextStyle{channel.MessageStyleBold, channel.MessageStyleItalic},
				},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 1 || me.Content[0].Type != "bold" {
		t.Fatalf("expected outer bold, got %+v", me.Content)
	}
	inner := me.Content[0].Children
	if len(inner) != 1 || inner[0].Type != "italic" {
		t.Fatalf("expected inner italic, got %+v", inner)
	}
	leaf := inner[0].Children
	if len(leaf) != 1 || leaf[0].Type != "text" || leaf[0].Text != "shout" {
		t.Fatalf("expected leaf text 'shout', got %+v", leaf)
	}
}

func TestAdaptInbound_StrikethroughStyleWraps(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "gone", Styles: []channel.MessageTextStyle{channel.MessageStyleStrikethrough}},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 1 || me.Content[0].Type != "strikethrough" {
		t.Fatalf("expected strikethrough wrapper, got %+v", me.Content)
	}
	kids := me.Content[0].Children
	if len(kids) != 1 || kids[0].Type != "text" || kids[0].Text != "gone" {
		t.Fatalf("expected text child 'gone', got %+v", kids)
	}
}

func TestAdaptInbound_EmojiBecomesText(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartEmoji, Emoji: "👍"},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 1 || me.Content[0].Type != "text" || me.Content[0].Text != "👍" {
		t.Fatalf("expected emoji rendered as text node, got %+v", me.Content)
	}
}

func TestAdaptInbound_SkipsLiterallyEmptyTextParts(t *testing.T) {
	// Only truly empty text parts are dropped; whitespace-only spans (newline
	// separators emitted by structured-post adapters like Feishu) must survive
	// so that line breaks between rich spans are preserved end-to-end.
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID: "m1",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: ""},
				{Type: channel.MessagePartText, Text: "kept"},
				{Type: channel.MessagePartText, Text: "\n"},
				{Type: channel.MessagePartText, Text: "next"},
			},
		},
	}
	me := AdaptInbound(msg, "sess", "", "").(MessageEvent)
	if len(me.Content) != 3 {
		t.Fatalf("expected 3 nodes (kept, newline separator, next), got %+v", me.Content)
	}
	if me.Content[0].Text != "kept" {
		t.Fatalf("part 0 wrong: %+v", me.Content[0])
	}
	if me.Content[1].Type != "text" || me.Content[1].Text != "\n" {
		t.Fatalf("expected newline separator preserved, got %+v", me.Content[1])
	}
	if me.Content[2].Text != "next" {
		t.Fatalf("part 2 wrong: %+v", me.Content[2])
	}
}

func TestAdaptInbound_EditEvent_PreservesParts(t *testing.T) {
	msg := channel.InboundMessage{
		Message: channel.Message{
			ID:   "m1",
			Text: "fallback",
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartMention, Text: "@bot", ChannelIdentityID: "ci-bot"},
			},
		},
		Metadata:   map[string]any{"event_type": "edit"},
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
	}
	e := AdaptInbound(msg, "sess", "ci", "Alice").(EditEvent)
	if len(e.Content) != 1 || e.Content[0].Type != "mention" || e.Content[0].UserID != "ci-bot" {
		t.Fatalf("expected edit event to preserve mention, got %+v", e.Content)
	}
}

func TestAdaptInbound_RoundTripsThroughRenderer(t *testing.T) {
	msg := channel.InboundMessage{
		Channel: channel.ChannelTypeTelegram,
		Message: channel.Message{
			ID:     "m1",
			Format: channel.MessageFormatRich,
			Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "hi "},
				{Type: channel.MessagePartMention, Text: "@bot", ChannelIdentityID: "ci-bot"},
				{Type: channel.MessagePartText, Text: " see "},
				{Type: channel.MessagePartLink, Text: "Memoh", URL: "https://example.com"},
			},
		},
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
	}
	me := AdaptInbound(msg, "sess", "ci-sender", "Alice").(MessageEvent)
	icMsg := &ICMessage{
		MessageID:    me.MessageID,
		Sender:       me.Sender,
		ReceivedAtMs: me.ReceivedAtMs,
		TimestampSec: me.TimestampSec,
		UTCOffsetMin: me.UTCOffsetMin,
		Content:      me.Content,
		Conversation: me.Conversation,
	}
	seg := renderMessage(icMsg, RenderParams{})
	if len(seg.Content) == 0 {
		t.Fatalf("expected rendered content")
	}
	xml := seg.Content[0].Text
	if !strings.Contains(xml, `<mention uid="ci-bot">@bot</mention>`) {
		t.Fatalf("expected rendered mention tag, got: %s", xml)
	}
	if !strings.Contains(xml, `<a href="https://example.com">Memoh</a>`) {
		t.Fatalf("expected rendered link tag, got: %s", xml)
	}
}
