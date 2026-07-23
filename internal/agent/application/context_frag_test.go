package application

import (
	"context"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	contextfrag "github.com/memohai/memoh/internal/agent/context/fragment"
	"github.com/memohai/memoh/internal/agent/runtime/native"
	"github.com/memohai/memoh/internal/agent/turn"
)

func TestBuildContextFragScopePreservesIMTopology(t *testing.T) {
	t.Parallel()

	scope := buildContextFragScope(ChatRequest{
		BotID:                     "bot-1",
		ChatID:                    "chat-1",
		ThreadID:                  "sess-1",
		SourceChannelIdentityID:   "identity-1",
		DisplayName:               "ignored",
		CurrentChannel:            "telegram",
		ConversationType:          turn.ConversationTypeGroup,
		ConversationName:          "Research Group",
		ReplyTarget:               "group-1",
		ExternalMessageID:         "msg-1",
		EventID:                   "evt-1",
		SourceReplyToMessageID:    "msg-0",
		ReplySender:               "Alice",
		MentionsBot:               true,
		RepliesToBot:              true,
		ForwardMessageID:          "fwd-1",
		ForwardFromUserID:         "user-2",
		ForwardFromConversationID: "source-chat",
		RawQuery:                  "/summarize this",
	}, "Bob", native.SessionContext{})

	if scope.BotID != "bot-1" || scope.ChatID != "chat-1" || scope.SessionID != "sess-1" {
		t.Fatalf("unexpected base scope: %#v", scope)
	}
	if scope.Platform != "telegram" || scope.ConversationType != turn.ConversationTypeGroup || scope.ConversationName != "Research Group" {
		t.Fatalf("unexpected conversation scope: %#v", scope)
	}
	if scope.CurrentMessageID != "msg-1" || scope.EventID != "evt-1" || scope.ReplyToMessageID != "msg-0" {
		t.Fatalf("unexpected message topology: %#v", scope)
	}
	if !scope.MentionsBot || !scope.RepliesToBot {
		t.Fatalf("expected structured directed-at-bot flags in scope: %#v", scope)
	}
	if scope.ForwardMessageID != "fwd-1" || scope.ForwardFromUserID != "user-2" || scope.ForwardFromConversationID != "source-chat" {
		t.Fatalf("unexpected forward topology: %#v", scope)
	}
	if !hasAttention(scope.Attention, contextfrag.AttentionReply) || !hasAttention(scope.Attention, contextfrag.AttentionCommand) {
		t.Fatalf("attention reasons = %#v, want reply and command", scope.Attention)
	}
	if !hasAttention(scope.Attention, contextfrag.AttentionMention) {
		t.Fatalf("attention reasons = %#v, want mention", scope.Attention)
	}
	if hasAttention(scope.Attention, contextfrag.AttentionPassive) {
		t.Fatalf("attention reasons should not include passive when reply/command are present: %#v", scope.Attention)
	}
}

func TestBuildContextFragScopeDoesNotInferDirectedReplyFromAnyReplyID(t *testing.T) {
	t.Parallel()

	scope := buildContextFragScope(ChatRequest{
		BotID:                  "bot-1",
		ChatID:                 "chat-1",
		ThreadID:               "sess-1",
		ConversationType:       turn.ConversationTypeGroup,
		SourceReplyToMessageID: "someone-elses-message",
		Query:                  "thread side comment",
	}, "Bob", native.SessionContext{})

	if scope.ReplyToMessageID != "someone-elses-message" {
		t.Fatalf("reply topology not preserved: %#v", scope)
	}
	if hasAttention(scope.Attention, contextfrag.AttentionReply) || hasAttention(scope.Attention, contextfrag.AttentionMention) {
		t.Fatalf("attention should not infer directed reply/mention without structured flags: %#v", scope.Attention)
	}
	if !hasAttention(scope.Attention, contextfrag.AttentionPassive) {
		t.Fatalf("group reply without directed flags should be passive attention: %#v", scope.Attention)
	}
}

func TestPrepareRunConfigDoesNotDoubleCountPipelineInlineImages(t *testing.T) {
	t.Parallel()

	image := sdk.ImagePart{Image: "data:image/png;base64,abc", MediaType: "image/png"}
	resolver := &Service{}
	cfg := native.RunConfig{
		Messages:     []sdk.Message{sdk.UserMessage("pipeline current user")},
		InlineImages: []sdk.ImagePart{image},
	}

	got := resolver.prepareRunConfig(context.Background(), cfg)

	if got.ContextManifest.Counts.Images != 1 {
		t.Fatalf("manifest image count = %d, want only image injected into SDK message: %#v", got.ContextManifest.Counts.Images, got.ContextManifest.Items)
	}
	rendered := contextfrag.Render(got.ContextFrags)
	if len(rendered.InlineImages) != 0 {
		t.Fatalf("rendered inline images = %#v, want images only inside pipeline SDK message", rendered.InlineImages)
	}
	if !messagesContainImage(got.Messages) {
		t.Fatalf("prepared messages do not contain injected image: %#v", got.Messages)
	}
}

func hasAttention(reasons []contextfrag.AttentionReason, want contextfrag.AttentionReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func messagesContainImage(messages []sdk.Message) bool {
	for _, message := range messages {
		for _, part := range message.Content {
			if _, ok := part.(sdk.ImagePart); ok {
				return true
			}
		}
	}
	return false
}
