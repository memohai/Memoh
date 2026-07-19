package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestStoreRoundBindsDiscussOutputToPersistedUserMessage(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.StoreRound(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		[]sdk.Message{sdk.AssistantMessage("reply")},
		"",
	)
	if err != nil {
		t.Fatalf("StoreRound() error = %v", err)
	}
	if len(messages.persisted) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messages.persisted))
	}
	if got := messages.persisted[0].TurnRequestMessageID; got != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("turn request message id = %q, want triggering user message", got)
	}
}

func TestStoreMessagesDoesNotContinueHistoryTurnWhenHistoryIsSkipped(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	_, err := resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID:           storeRoundBotID,
		SessionID:       "33333333-3333-3333-3333-333333333333",
		SkipHistoryTurn: true,
	}, []conversation.ModelMessage{{
		Role:    "user",
		Content: json.RawMessage(`[{"type":"image","url":"data:image/png;base64,aW1hZ2U="}]`),
	}}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}
	if len(messages.persisted) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messages.persisted))
	}
	if !messages.persisted[0].SkipHistoryTurn || messages.persisted[0].ContinueHistoryTurn {
		t.Fatalf("history flags = skip:%t continue:%t, want true/false", messages.persisted[0].SkipHistoryTurn, messages.persisted[0].ContinueHistoryTurn)
	}
}

func TestStoreRoundRejectsMissingPersistedUserMessage(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.StoreRound(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"",
		[]sdk.Message{sdk.AssistantMessage("reply")},
		"",
	)
	if err == nil {
		t.Fatal("StoreRound() error = nil, want missing trigger message error")
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("persisted messages = %d, want 0 without an explicit turn binding", len(messages.persisted))
	}
}

func TestStoreRoundReturnsMessagePersistenceFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist failed")
	messages := &recordingMessageService{persistErr: wantErr}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.StoreRound(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		[]sdk.Message{sdk.AssistantMessage("reply")},
		"",
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("StoreRound() error = %v, want %v", err, wantErr)
	}
}

func TestStoreRoundDoesNotPartiallyPersistExistingUserTail(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist second tail message")
	messages := &failingTurnResponseTailService{persistErr: wantErr, failAt: 2}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.StoreRound(
		context.Background(),
		storeRoundBotID,
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"telegram",
		"55555555-5555-5555-5555-555555555555",
		[]sdk.Message{
			{
				Role: sdk.MessageRoleAssistant,
				Content: []sdk.MessagePart{
					sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "exec"},
				},
			},
			sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "exec", Result: "ok"}),
			sdk.AssistantMessage("done"),
		},
		"",
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("StoreRound() error = %v, want %v", err, wantErr)
	}
	if messages.tailCalls != 1 {
		t.Fatalf("atomic tail calls = %d, want 1", messages.tailCalls)
	}
	if len(messages.durable) != 0 {
		t.Fatalf("durable tail messages = %d, want 0 after rollback", len(messages.durable))
	}
}

func TestStoreMessagesRoutesSkippedExistingUserTailThroughRound(t *testing.T) {
	t.Parallel()

	messages := &failingTurnResponseTailService{persistErr: errors.New("unexpected turn response tail")}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	persisted, err := resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID:                     storeRoundBotID,
		SessionID:                 "33333333-3333-3333-3333-333333333333",
		ReusePersistedUserMessage: true,
		PersistedUserMessageID:    "55555555-5555-5555-5555-555555555555",
		SkipHistoryTurn:           true,
	}, []conversation.ModelMessage{
		{Role: "assistant", Content: conversation.NewTextContent("call tool")},
		{Role: "tool", Content: conversation.NewTextContent("tool result")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeMessages() error = %v", err)
	}
	if messages.tailCalls != 0 {
		t.Fatalf("atomic tail calls = %d, want 0 for skipped history", messages.tailCalls)
	}
	if len(messages.persisted) != 3 || len(persisted) != 3 {
		t.Fatalf("persisted messages = %d/%d, want 3/3", len(messages.persisted), len(persisted))
	}
	for i, input := range messages.persisted {
		if !input.SkipHistoryTurn {
			t.Fatalf("persisted message %d did not preserve skipped history", i)
		}
	}
}

func TestStoreMessagesRejectsNonAtomicMixedRound(t *testing.T) {
	t.Parallel()

	messages := &nonAtomicMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, err := resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID:     storeRoundBotID,
		SessionID: "33333333-3333-3333-3333-333333333333",
		Query:     "first",
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("first")},
		{Role: "assistant", Content: conversation.NewTextContent("first answer")},
		{Role: "user", Content: conversation.NewTextContent("injected")},
		{Role: "assistant", Content: conversation.NewTextContent("final")},
	}, "", storeRoundOptions{})
	if err == nil {
		t.Fatal("storeMessages() error = nil, want missing atomic round capability")
	}
	if messages.persistCalls != 0 {
		t.Fatalf("fallback Persist calls = %d, want 0", messages.persistCalls)
	}
}

type nonAtomicMessageService struct {
	messagepkg.Service
	persistCalls int
}

func (s *nonAtomicMessageService) Persist(context.Context, messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persistCalls++
	return messagepkg.Message{}, nil
}

type failingTurnResponseTailService struct {
	recordingMessageService
	persistErr   error
	failAt       int
	persistCalls int
	tailCalls    int
	durable      []messagepkg.PersistInput
}

func (s *failingTurnResponseTailService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persistCalls++
	if s.persistCalls == s.failAt {
		return messagepkg.Message{}, s.persistErr
	}
	s.durable = append(s.durable, input)
	return messagepkg.Message{ID: "message-id", Role: input.Role}, nil
}

func (s *failingTurnResponseTailService) PersistTurnResponseTail(
	_ context.Context,
	_ []messagepkg.PersistInput,
) ([]messagepkg.Message, error) {
	s.tailCalls++
	return nil, s.persistErr
}

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
