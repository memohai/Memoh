package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestBuildPipelineContextRestoresPreparedCurrentQueryWhenRenderedContextIsStale(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "external-old", time.Now().UTC().Add(-time.Minute).UnixMilli(), "old rendered message"))
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), pipeline: p}
	prepared := FormatUserHeader(UserMessageHeaderInput{
		MessageID:        "external-current",
		DisplayName:      "Alice",
		Channel:          "telegram",
		ConversationType: "group",
		Target:           "room",
		AttachmentPaths:  []string{"/data/report.pdf"},
		Time:             time.Unix(1_000, 0).UTC(),
	}, "current user query")

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "external-current",
		Query:             "current user query",
	}, 0, prepared)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 2 || !strings.Contains(built.Messages[1].TextContent(), `<attachment path="/data/report.pdf"/>`) {
		t.Fatalf("prepared current query metadata was lost: %#v", modelMessageTexts(built.Messages))
	}
}

func TestBuildPipelineContextReplacesRenderedCurrentMessageWithPreparedQuery(t *testing.T) {
	t.Parallel()

	const sessionID = "22222222-2222-2222-2222-222222222222"
	now := time.Now().UTC()
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "external-prior", now.Add(-2*time.Minute).UnixMilli(), "prior message"))
	current := pipelineMessageEvent(sessionID, "external-current", now.Add(-time.Minute).UnixMilli(), "[voice attachment]")
	current.ReplyToMessageID = "external-prior"
	current.ReplyToSender = "Bob"
	current.ReplyToPreview = "prior message"
	current.ForwardInfo = &pipelinepkg.ForwardInfo{MessageID: "forward-1", SenderName: "Source Room"}
	p.PushEvent(sessionID, current)
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), pipeline: p}
	prepared := FormatUserHeader(UserMessageHeaderInput{
		MessageID:          "external-current",
		DisplayName:        "Alice",
		Channel:            "telegram",
		ConversationType:   "group",
		Target:             "room",
		Time:               now.Add(-time.Minute),
		ReplyToMessageID:   "external-prior",
		ReplySender:        "Bob",
		ReplyPreview:       "prior message",
		ForwardedFrom:      "Source Room",
		ForwardedMessageID: "forward-1",
	}, "[Voice message transcription]\nship the fix")

	built, err := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{
		SessionID:         sessionID,
		ExternalMessageID: "external-current",
		Query:             "[Voice message transcription]\nship the fix",
	}, 0, prepared)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if len(built.Messages) != 1 {
		t.Fatalf("messages = %#v, want one merged rendered user message", modelMessageTexts(built.Messages))
	}
	got := built.Messages[0].TextContent()
	if !strings.Contains(got, "prior message") || !strings.Contains(got, "ship the fix") {
		t.Fatalf("prepared current query did not replace the rendered payload: %q", got)
	}
	if strings.Contains(got, "[voice attachment]") {
		t.Fatalf("stale rendered current payload survived replacement: %q", got)
	}
	for _, want := range []string{
		`<in-reply-to id="external-prior" sender="Bob">prior message</in-reply-to>`,
		`forwarded_from="Source Room"`,
		`forwarded_message_id="forward-1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prepared current query lost reply/forward context %q: %q", want, got)
		}
	}
}
