package historyfrag

import (
	"encoding/json"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messagesource"
)

func TestDBMessageSourceHashIncludesMessageMetadata(t *testing.T) {
	t.Parallel()

	msg := messagepkg.Message{
		ID:       "row-1",
		BotID:    "bot-1",
		Role:     "user",
		Content:  conversation.NewTextContent("hello"),
		Metadata: map[string]any{"reply": map[string]any{"sender": "Alice"}},
	}
	changed := msg
	changed.Metadata = map[string]any{"reply": map[string]any{"sender": "Bob"}}

	if DBMessageSourceHash(msg).Value == DBMessageSourceHash(changed).Value {
		t.Fatal("message metadata change should affect DB source payload hash")
	}
}

func TestDBMessageSourceHashIncludesCanonicalSourceContext(t *testing.T) {
	t.Parallel()

	msg := messagepkg.Message{
		ID:      "row-1",
		Role:    "user",
		Content: conversation.NewTextContent("hello"),
		SourceContext: messagesource.NewV1(
			"Alice",
			"telegram",
			"group",
			"Dev Chat",
		),
	}
	wantDifferent := []messagesource.Context{
		messagesource.NewV1("Bob", "telegram", "group", "Dev Chat"),
		messagesource.NewV1("Alice", "slack", "group", "Dev Chat"),
		messagesource.NewV1("Alice", "telegram", "private", "Dev Chat"),
		messagesource.NewV1("Alice", "telegram", "group", "Renamed Chat"),
	}
	baseHash := DBMessageSourceHash(msg).Value
	for _, sourceContext := range wantDifferent {
		changed := msg
		changed.SourceContext = sourceContext
		if DBMessageSourceHash(changed).Value == baseHash {
			t.Fatalf("source context change did not affect hash: %+v", sourceContext)
		}
	}
}

func TestDBMessageSourceHashKeepsLegacyGolden(t *testing.T) {
	t.Parallel()

	msg := messagepkg.Message{
		ID:                      "row-legacy",
		BotID:                   "bot-1",
		SessionID:               "session-1",
		SenderChannelIdentityID: "identity-1",
		SenderUserID:            "user-1",
		SenderDisplayName:       "Alice",
		Platform:                "telegram",
		ExternalMessageID:       "message-1",
		SourceReplyToMessageID:  "message-0",
		Role:                    "user",
		Content:                 json.RawMessage(`{"role":"user","content":"hello"}`),
		Metadata:                map[string]any{"kind": "legacy"},
		Usage:                   json.RawMessage(`{"outputTokens":12}`),
		EventID:                 "event-1",
	}
	const want = "a11aa4b7b5355e7ba3ac3074af0dea6f75ad9a7f168f5a5ca2e6f89893956cc8"
	if got := DBMessageSourceHash(msg).Value; got != want {
		t.Fatalf("legacy source hash = %q, want %q", got, want)
	}
}

func TestDBMessageSourceHashRejectsUnknownSourceContextVersion(t *testing.T) {
	t.Parallel()

	for _, version := range []int{-1, 2} {
		msg := messagepkg.Message{
			ID:            "row-1",
			Role:          "user",
			Content:       conversation.NewTextContent("hello"),
			SourceContext: messagesource.Context{Version: version},
		}
		if hash := DBMessageSourceHash(msg); hash.Value != "" || hash.Algo != "" || hash.Scope != "" {
			t.Fatalf("version %d hash = %+v, want fail-closed zero hash", version, hash)
		}
		if _, err := FromDBMessage(msg, ScopeFallback{}); err == nil {
			t.Fatalf("version %d source context unexpectedly accepted", version)
		}
	}
}

func TestFromDBMessageKeepsLiveScopeOutOfCanonicalSourceHash(t *testing.T) {
	t.Parallel()

	msg := messagepkg.Message{
		ID:      "row-1",
		Role:    "user",
		Content: conversation.NewTextContent("hello"),
		SourceContext: messagesource.NewV1(
			"Historical Sender",
			"telegram",
			"group",
			"Historical Room",
		),
	}
	first, err := FromDBMessage(msg, ScopeFallback{
		ConversationType: "private",
		ConversationName: "Current Room",
		ReplyTarget:      "current-target-1",
	})
	if err != nil {
		t.Fatalf("first source record: %v", err)
	}
	second, err := FromDBMessage(msg, ScopeFallback{
		ConversationType: "channel",
		ConversationName: "Renamed Current Room",
		ReplyTarget:      "current-target-2",
	})
	if err != nil {
		t.Fatalf("second source record: %v", err)
	}
	if first.Ref.ContentHash != second.Ref.ContentHash {
		t.Fatal("live scope changed canonical source hash")
	}
	if first.SourceContext != msg.SourceContext || second.SourceContext != msg.SourceContext {
		t.Fatalf("source context drifted: first=%+v second=%+v", first.SourceContext, second.SourceContext)
	}
	if first.Scope.ReplyTarget == second.Scope.ReplyTarget {
		t.Fatal("test setup did not vary live placement scope")
	}
}

func TestFromDBMessageScopeFallbackDoesNotChangeDurableRefID(t *testing.T) {
	t.Parallel()

	msg := messagepkg.Message{
		ID:      "row-1",
		BotID:   "bot-1",
		Role:    "user",
		Content: persistedModelMessage(t, conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}),
	}
	first, err := FromDBMessage(msg, ScopeFallback{ChatID: "chat-1", ConversationType: "group"})
	if err != nil {
		t.Fatalf("first FromDBMessage failed: %v", err)
	}
	second, err := FromDBMessage(msg, ScopeFallback{ChatID: "chat-2", ConversationType: "private"})
	if err != nil {
		t.Fatalf("second FromDBMessage failed: %v", err)
	}

	if first.Ref.ID != second.Ref.ID {
		t.Fatalf("current-request fallback scope must not change DB row identity: first=%#v second=%#v", first.Ref, second.Ref)
	}
	if ToFrag(first).Ref.ID != ToFrag(second).Ref.ID {
		t.Fatalf("fallback scope changed durable frag identity: first=%#v second=%#v", ToFrag(first).Ref, ToFrag(second).Ref)
	}
	if ToFrag(first).Ref.ContentHash != ToFrag(second).Ref.ContentHash {
		t.Fatalf("fallback scope changed durable content hash: first=%#v second=%#v", ToFrag(first).Ref, ToFrag(second).Ref)
	}
}
