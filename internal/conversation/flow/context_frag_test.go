package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

func TestBuildContextFragScopePreservesIMTopology(t *testing.T) {
	t.Parallel()

	scope := buildContextFragScope(conversation.ChatRequest{
		BotID:                     "bot-1",
		ChatID:                    "chat-1",
		SessionID:                 "sess-1",
		SourceChannelIdentityID:   "identity-1",
		DisplayName:               "ignored",
		CurrentChannel:            "telegram",
		ConversationType:          conversation.KindGroup,
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
	}, "Bob", agent.SessionContext{})

	if scope.BotID != "bot-1" || scope.ChatID != "chat-1" || scope.SessionID != "sess-1" {
		t.Fatalf("unexpected base scope: %#v", scope)
	}
	if scope.Platform != "telegram" || scope.ConversationType != conversation.KindGroup || scope.ConversationName != "Research Group" {
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

	scope := buildContextFragScope(conversation.ChatRequest{
		BotID:                  "bot-1",
		ChatID:                 "chat-1",
		SessionID:              "sess-1",
		ConversationType:       conversation.KindGroup,
		SourceReplyToMessageID: "someone-elses-message",
		Query:                  "thread side comment",
	}, "Bob", agent.SessionContext{})

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

func hasAttention(reasons []contextfrag.AttentionReason, want contextfrag.AttentionReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
