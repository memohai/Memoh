package flow

import (
	"strings"
	"testing"
	"time"
)

func TestFormatUserHeaderIncludesAttachments(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	header := FormatUserHeader(UserMessageHeaderInput{
		MessageID:         "msg_1",
		ChannelIdentityID: "cid_1",
		DisplayName:       "Alice",
		Channel:           "feishu",
		ConversationType:  "group",
		ConversationName:  "Team Chat",
		AttachmentPaths:   []string{"/tmp/a.txt"},
		Time:              now,
		Timezone:          "UTC",
	}, "hello")

	if !strings.Contains(header, "<attachment path=\"/tmp/a.txt\"/>") {
		t.Fatalf("expected attachment tag in header: %s", header)
	}
}

func TestFormatUserHeaderWithoutAttachmentsUsesEmptyList(t *testing.T) {
	t.Parallel()

	header := FormatUserHeader(UserMessageHeaderInput{
		ChannelIdentityID: "cid_1",
		DisplayName:       "Alice",
		Channel:           "feishu",
		ConversationType:  "group",
		ConversationName:  "Team Chat",
		Time:              time.Now().UTC(),
	}, "hello")

	if strings.Contains(header, "<attachment ") {
		t.Fatalf("expected no attachment tag in header: %s", header)
	}
}

func TestFormatUserHeaderIncludesReplyAndForwardContext(t *testing.T) {
	t.Parallel()

	header := FormatUserHeader(UserMessageHeaderInput{
		MessageID:          "msg-1",
		DisplayName:        "Alice",
		Channel:            "telegram",
		ConversationType:   "group",
		Time:               time.Unix(1_000, 0).UTC(),
		ReplyToMessageID:   "reply-1",
		ReplySender:        `Bob & Carol`,
		ReplyPreview:       `<quoted & text>`,
		ForwardedFrom:      `Source "Room"`,
		ForwardedMessageID: "forward-1",
	}, "transcribed body")

	for _, want := range []string{
		`forwarded_from="Source &quot;Room&quot;"`,
		`forwarded_message_id="forward-1"`,
		`<in-reply-to id="reply-1" sender="Bob &amp; Carol">&lt;quoted &amp; text&gt;</in-reply-to>`,
		"transcribed body",
	} {
		if !strings.Contains(header, want) {
			t.Fatalf("header missing %q: %s", want, header)
		}
	}
}
