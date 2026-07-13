package flow

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/runtimefence"
)

func TestBuildInteractionMetadataIncludesForwardConversation(t *testing.T) {
	t.Parallel()

	meta := buildInteractionMetadata(conversation.ChatRequest{
		SourceReplyToMessageID:    "reply-1",
		ReplySender:               "Original Sender",
		ReplyPreview:              "quoted text",
		ForwardMessageID:          "forward-1",
		ForwardFromUserID:         "source-user",
		ForwardFromConversationID: "source-conversation",
		ForwardSender:             "Source Channel",
		ForwardDate:               1710000000,
	})

	reply, ok := meta["reply"].(map[string]any)
	if !ok || reply["message_id"] != "reply-1" || reply["sender"] != "Original Sender" || reply["preview"] != "quoted text" {
		t.Fatalf("unexpected reply metadata: %#v", meta["reply"])
	}
	forward, ok := meta["forward"].(map[string]any)
	if !ok {
		t.Fatalf("expected forward metadata: %#v", meta)
	}
	if forward["message_id"] != "forward-1" ||
		forward["from_user_id"] != "source-user" ||
		forward["from_conversation_id"] != "source-conversation" ||
		forward["sender"] != "Source Channel" ||
		forward["date"] != int64(1710000000) {
		t.Fatalf("unexpected forward metadata: %#v", forward)
	}
}

func TestBuildInteractionMetadataIncludesRequestedSkills(t *testing.T) {
	t.Parallel()

	meta := buildInteractionMetadata(conversation.ChatRequest{
		RequestedSkills: []conversation.RequestedSkillContext{
			{
				Name:           "writer",
				SourceKind:     "managed",
				OpaqueSourceID: "src-1",
				Content:        "raw content must not be persisted in metadata",
				ContentHash:    "hash-must-not-leak",
				Identity:       "managed:src-1:writer",
			},
			{
				Name:           "writer",
				SourceKind:     "managed",
				OpaqueSourceID: "src-1",
				Identity:       "managed:src-1:writer",
			},
			{
				Name:       "reviewer",
				SourceKind: "plugin",
			},
		},
	})

	raw, ok := meta["model_requested_skills"].([]map[string]any)
	if !ok {
		t.Fatalf("expected requested skill metadata: %#v", meta["model_requested_skills"])
	}
	if len(raw) != 2 {
		t.Fatalf("expected deduped requested skills, got %#v", raw)
	}
	if raw[0]["name"] != "writer" || raw[0]["source_kind"] != "managed" {
		t.Fatalf("unexpected first skill metadata: %#v", raw[0])
	}
	if _, ok := raw[0]["opaque_source_id"]; ok {
		t.Fatalf("requested skill metadata leaked opaque source id: %#v", raw[0])
	}
	if _, ok := raw[0]["content"]; ok {
		t.Fatalf("requested skill metadata leaked content: %#v", raw[0])
	}
	if _, ok := raw[0]["content_hash"]; ok {
		t.Fatalf("requested skill metadata leaked content_hash: %#v", raw[0])
	}
	if _, ok := raw[0]["ref"]; ok {
		t.Fatalf("requested skill metadata leaked ref: %#v", raw[0])
	}
	if raw[1]["name"] != "reviewer" || raw[1]["source_kind"] != "plugin" {
		t.Fatalf("unexpected second skill metadata: %#v", raw[1])
	}
}

func TestBuildInteractionMetadataIncludesPublicSkillActivation(t *testing.T) {
	t.Parallel()

	meta := buildInteractionMetadata(conversation.ChatRequest{
		UserMessageKind: conversation.UserMessageKindSkillActivation,
		SkillActivation: &conversation.SkillActivation{
			Prompt: "do it",
			Skills: []conversation.SkillActivationSkill{{
				Name:        "writer",
				DisplayName: "Writer",
				Description: "short safe description",
				SourceKind:  "managed",
				State:       "effective",
			}},
		},
		RequestedSkills: []conversation.RequestedSkillContext{{
			Name:           "writer",
			SourceKind:     "managed",
			OpaqueSourceID: "opaque-source",
			Content:        "raw content must not leak",
			ContentHash:    "hash-must-not-leak",
			Identity:       "writer|opaque-source|hash",
		}},
	})

	if meta["user_message_kind"] != conversation.UserMessageKindSkillActivation {
		t.Fatalf("user_message_kind = %#v", meta["user_message_kind"])
	}
	public, ok := meta["skill_activation"].(map[string]any)
	if !ok {
		t.Fatalf("expected public skill activation metadata: %#v", meta["skill_activation"])
	}
	if public["prompt"] != "do it" {
		t.Fatalf("prompt = %#v, want do it", public["prompt"])
	}
	skills, ok := public["skills"].([]map[string]any)
	if !ok || len(skills) != 1 {
		t.Fatalf("public skills = %#v, want one", public["skills"])
	}
	if skills[0]["name"] != "writer" || skills[0]["display_name"] != "Writer" {
		t.Fatalf("unexpected public skill: %#v", skills[0])
	}
	for _, key := range []string{"opaque_source_id", "content", "content_hash", "ref"} {
		if _, ok := skills[0][key]; ok {
			t.Fatalf("public skill leaked %s: %#v", key, skills[0])
		}
	}
	if _, ok := meta["audit_requested_skills"]; ok {
		t.Fatalf("audit metadata leaked into message metadata: %#v", meta["audit_requested_skills"])
	}
}

type batchRecordingMessageService struct {
	recordingMessageService
	batchInputs []messagepkg.PersistInput
}

type atomicRecordingMessageService struct {
	batchRecordingMessageService
	roundInputs []messagepkg.PersistInput
	options     messagepkg.RoundPersistenceOptions
	fence       runtimefence.Fence
}

func (s *atomicRecordingMessageService) PersistRound(ctx context.Context, inputs []messagepkg.PersistInput, options messagepkg.RoundPersistenceOptions) ([]messagepkg.Message, bool, error) {
	s.roundInputs = append(s.roundInputs, inputs...)
	s.options = options
	s.fence, _ = runtimefence.FromContext(ctx)
	return recordedMessages(inputs), true, nil
}

func (s *batchRecordingMessageService) PersistToolTailRound(_ context.Context, inputs []messagepkg.PersistInput) ([]messagepkg.Message, bool, error) {
	s.batchInputs = append(s.batchInputs, inputs...)
	return recordedMessages(inputs), true, nil
}

func TestStoreMessagesUsesToolTailBatch(t *testing.T) {
	t.Parallel()

	messages := &batchRecordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	persisted, err := resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID:       storeRoundBotID,
		SessionID:   "33333333-3333-3333-3333-333333333333",
		Query:       "hello",
		SessionType: "chat",
		RuntimeType: "model",
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("call tool")},
		{Role: "tool", Content: conversation.NewTextContent("tool result")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}

	if len(messages.batchInputs) != 4 {
		t.Fatalf("batch inputs = %d, want 4", len(messages.batchInputs))
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("fallback Persist called %d times, want 0", len(messages.persisted))
	}
	if len(persisted) != 4 {
		t.Fatalf("persisted messages = %d, want 4", len(persisted))
	}
}

func TestStoreMessagesUsesAtomicRoundForRuntimeFence(t *testing.T) {
	t.Parallel()

	const sessionID = "33333333-3333-3333-3333-333333333333"
	messages := &atomicRecordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	fence := runtimefence.Fence{BotID: storeRoundBotID, SessionID: sessionID, Token: 7}
	ctx := runtimefence.WithContext(context.Background(), fence)

	persisted, err := resolver.storeMessages(ctx, conversation.ChatRequest{
		BotID:       storeRoundBotID,
		SessionID:   sessionID,
		Query:       "hello",
		SessionType: "chat",
		RuntimeType: "model",
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}
	if len(messages.roundInputs) != 2 || len(persisted) != 2 {
		t.Fatalf("atomic round inputs/persisted = %d/%d, want 2/2", len(messages.roundInputs), len(persisted))
	}
	if len(messages.batchInputs) != 0 || len(messages.persisted) != 0 {
		t.Fatalf("non-fenced fallback used: batch=%d persist=%d", len(messages.batchInputs), len(messages.persisted))
	}
	if messages.fence != fence {
		t.Fatalf("round fence = %#v, want %#v", messages.fence, fence)
	}
}

func TestStoreMessagesCommitsReplacementWithFencedRound(t *testing.T) {
	t.Parallel()

	const sessionID = "33333333-3333-3333-3333-333333333333"
	messages := &atomicRecordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	state := &replacementPersistenceState{
		oldTurnID:          "turn-old",
		requestMessageID:   "request-old",
		reason:             "retry",
		forkAnchor:         &forkAnchorUpdate{metadata: map[string]any{"forked_from": map[string]any{"fork_message_id": "assistant-parent"}}},
		forkAnchorPrepared: true,
	}
	fence := runtimefence.Fence{BotID: storeRoundBotID, SessionID: sessionID, Token: 8}
	ctx := runtimefence.WithContext(context.Background(), fence)
	ctx = context.WithValue(ctx, replacementPersistenceContextKey{}, state)

	_, err := resolver.storeMessages(ctx, conversation.ChatRequest{
		BotID: storeRoundBotID, SessionID: sessionID, ReusePersistedUserMessage: true,
		PersistedUserMessageID: "request-old", SkipHistoryTurn: true,
	}, []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("replacement")}}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}
	if !state.atomicCommitted {
		t.Fatal("replacement was not marked atomically committed")
	}
	replacement := messages.options.Replacement
	if replacement == nil || replacement.OldTurnID != "turn-old" || replacement.RequestMessageID != "request-old" || replacement.Reason != "retry" {
		t.Fatalf("replacement options = %#v", replacement)
	}
	if replacement.SessionMetadata == nil {
		t.Fatal("replacement fork metadata was not included in the transaction")
	}
}

func TestFindAssistantMessageForToolCall(t *testing.T) {
	t.Parallel()

	msgs := []messagepkg.Message{
		{ID: "m3", Role: "assistant", Content: []byte(`{"role":"assistant","content":[{"type":"text","text":"done, see call-1 above"}]}`)},
		{ID: "m2", Role: "tool", Content: []byte(`{"role":"tool","content":[{"type":"tool-result","toolCallId":"call-1"}]}`)},
		{ID: "m1", Role: "assistant", Content: []byte(`{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call-1","toolName":"generate_image"}]}`)},
	}

	if got := findAssistantMessageForToolCall(msgs, "call-1"); got != "m1" {
		t.Fatalf("findAssistantMessageForToolCall = %q, want m1 (assistant tool-call row, not tool row or echoed text)", got)
	}
	if got := findAssistantMessageForToolCall(msgs, "call-404"); got != "" {
		t.Fatalf("findAssistantMessageForToolCall unknown id = %q, want empty", got)
	}
}
