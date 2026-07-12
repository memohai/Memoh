package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messagesource"
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

type decliningBatchMessageService struct {
	recordingMessageService
	batchInputs []messagepkg.PersistInput
}

func (s *decliningBatchMessageService) PersistToolTailRound(_ context.Context, inputs []messagepkg.PersistInput) ([]messagepkg.Message, bool, error) {
	s.batchInputs = append(s.batchInputs, inputs...)
	return nil, false, nil
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

	persisted := resolver.storeMessages(context.Background(), conversation.ChatRequest{
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

func TestStoreMessagesPreservesReceiptWhenToolTailBatchDeclines(t *testing.T) {
	t.Parallel()

	messages := &decliningBatchMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	sourceContext := messagesource.NewV1("Sender", "telegram", "private", "Chat")
	receipt := &conversation.UserMessageReceipt{
		DisplayText:       "hello",
		ExternalMessageID: "external-user",
		EventID:           "33333333-3333-3333-3333-333333333333",
		SourceContext:     sourceContext,
		Metadata:          map[string]any{"platform": "telegram"},
	}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID, SessionID: "44444444-4444-4444-4444-444444444444",
		Query: "hello", SessionType: "chat", RuntimeType: "model",
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello"), UserReceipt: receipt},
		{Role: "assistant", Content: conversation.NewTextContent("call")},
		{Role: "tool", Content: conversation.NewTextContent("result")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})

	if len(messages.batchInputs) != 4 || len(messages.persisted) != 4 {
		t.Fatalf("batch attempt=%d sequential=%d, want 4/4", len(messages.batchInputs), len(messages.persisted))
	}
	for _, inputs := range [][]messagepkg.PersistInput{messages.batchInputs, messages.persisted} {
		if inputs[0].ExternalMessageID != "external-user" || inputs[0].EventID != receipt.EventID ||
			inputs[0].SourceContext != sourceContext || inputs[0].Metadata["platform"] != "telegram" ||
			inputs[1].SourceReplyToMessageID != "external-user" {
			t.Fatalf("batch fallback changed receipt provenance: %#v", inputs)
		}
	}
}

func TestStoreMessagesUsesPerMessageInjectionReceipt(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	sourceContext := messagesource.NewV1("Injected Sender", "telegram", "group", "Injected Room")
	receipt := &conversation.UserMessageReceipt{
		ID:                      "receipt-b",
		DisplayText:             "raw injected text",
		SenderChannelIdentityID: "11111111-1111-1111-1111-111111111111",
		SenderUserID:            "22222222-2222-2222-2222-222222222222",
		ExternalMessageID:       "external-b",
		SourceReplyToMessageID:  "reply-b",
		EventID:                 "33333333-3333-3333-3333-333333333333",
		SourceContext:           sourceContext,
		Metadata:                map[string]any{"route_id": "route-b", "platform": "telegram"},
		Attachments: []conversation.ChatAttachment{{
			ContentHash: "asset-b",
			Mime:        "image/png",
			Name:        "b.png",
			Size:        42,
			Metadata:    map[string]any{"storage_key": "assets/b.png", "source": "injected"},
		}},
	}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID:             storeRoundBotID,
		SessionID:         "44444444-4444-4444-4444-444444444444",
		Query:             "initial",
		ExternalMessageID: "external-a",
		RouteID:           "route-a",
		CurrentChannel:    "slack",
		SessionType:       "chat",
		RuntimeType:       "model",
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("initial")},
		{Role: "assistant", Content: conversation.NewTextContent("working")},
		{Role: "user", Content: conversation.NewTextContent("<message>raw injected text</message>"), UserReceipt: receipt},
		{Role: "user", Content: conversation.NewTextContent("initial")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})

	if len(messages.persisted) != 5 {
		t.Fatalf("persisted inputs = %d, want 5", len(messages.persisted))
	}
	injected := messages.persisted[2]
	if injected.SenderChannelIdentityID != receipt.SenderChannelIdentityID ||
		injected.SenderUserID != receipt.SenderUserID ||
		injected.ExternalMessageID != receipt.ExternalMessageID ||
		injected.SourceReplyToMessageID != receipt.SourceReplyToMessageID ||
		injected.EventID != receipt.EventID || injected.SourceContext != sourceContext ||
		injected.DisplayText != receipt.DisplayText || injected.Metadata["route_id"] != "route-b" ||
		injected.Metadata["platform"] != "telegram" {
		t.Fatalf("injected provenance = %#v, want receipt %#v", injected, receipt)
	}
	if len(injected.Assets) != 1 || injected.Assets[0].ContentHash != "asset-b" ||
		injected.Assets[0].Role != "attachment" || injected.Assets[0].Ordinal != 0 ||
		injected.Assets[0].Mime != "image/png" || injected.Assets[0].SizeBytes != 42 ||
		injected.Assets[0].Name != "b.png" || injected.Assets[0].StorageKey != "assets/b.png" ||
		injected.Assets[0].Metadata["source"] != "injected" {
		t.Fatalf("injected assets = %#v", injected.Assets)
	}
	var storedModelMessage conversation.ModelMessage
	if err := json.Unmarshal(injected.Content, &storedModelMessage); err != nil {
		t.Fatalf("decode injected model content: %v", err)
	}
	if storedModelMessage.TextContent() != "<message>raw injected text</message>" || storedModelMessage.UserReceipt != nil {
		t.Fatalf("injected model content = %#v", storedModelMessage)
	}
	synthetic := messages.persisted[3]
	if synthetic.SenderChannelIdentityID != "" || synthetic.SenderUserID != "" ||
		synthetic.ExternalMessageID != "" || synthetic.EventID != "" ||
		synthetic.SourceReplyToMessageID != "" || synthetic.SourceContext != (messagesource.Context{}) ||
		len(synthetic.Metadata) != 0 || len(synthetic.Assets) != 0 {
		t.Fatalf("synthetic user inherited provenance: %#v", synthetic)
	}
	if messages.persisted[1].SourceReplyToMessageID != "external-a" ||
		messages.persisted[4].SourceReplyToMessageID != "external-b" {
		t.Fatalf("assistant source replies did not follow the latest real user: before=%q after=%q",
			messages.persisted[1].SourceReplyToMessageID, messages.persisted[4].SourceReplyToMessageID)
	}

	withoutExternal := &recordingMessageService{}
	resolver.messageService = withoutExternal
	receiptWithoutExternal := *receipt
	receiptWithoutExternal.ExternalMessageID = ""
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID, SessionID: "44444444-4444-4444-4444-444444444444",
		Query: "initial", ExternalMessageID: "external-a", SessionType: "chat", RuntimeType: "model",
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("initial")},
		{Role: "user", Content: conversation.NewTextContent("injected"), UserReceipt: &receiptWithoutExternal},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if got := withoutExternal.persisted[2].SourceReplyToMessageID; got != "" {
		t.Fatalf("assistant after receipt without external ID replied to %q, want empty", got)
	}
}

func TestStoreRoundRemapsMetadataAcrossSyntheticToolClosure(t *testing.T) {
	t.Parallel()

	messages := &batchRecordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	call := sdkMessagesToModelMessages([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-1",
			ToolName:   "lookup",
			Input:      map[string]any{},
		}},
	}})[0]
	failureMetadata := map[string]any{"error": "runtime failed", "stop_reason": "error"}
	sourceContext := messagesource.NewV1("Original Sender", "telegram", "private", "Original Chat")

	_, err := resolver.storeRoundWithOptionsResult(context.Background(), conversation.ChatRequest{
		BotID:       storeRoundBotID,
		SessionID:   "33333333-3333-3333-3333-333333333333",
		Query:       "hello",
		SessionType: "chat",
		RuntimeType: "acp_agent",
	}, []conversation.ModelMessage{
		{
			Role:    "user",
			Content: conversation.NewTextContent("hello"),
			UserReceipt: &conversation.UserMessageReceipt{
				ExternalMessageID: "external-original",
				DisplayText:       "hello",
				SourceContext:     sourceContext,
			},
		},
		call,
		{Role: "assistant", Content: conversation.NewTextContent("runtime failed")},
	}, "", storeRoundOptions{
		SkipMemory:             true,
		MessageMetadataByIndex: map[int]map[string]any{2: failureMetadata},
	})
	if err != nil {
		t.Fatalf("storeRoundWithOptionsResult() error = %v", err)
	}
	if len(messages.batchInputs) != 4 {
		t.Fatalf("persisted inputs = %d, want user, call, synthetic result, failure", len(messages.batchInputs))
	}
	if messages.batchInputs[2].Role != "tool" {
		t.Fatalf("input[2] role = %q, want synthetic tool", messages.batchInputs[2].Role)
	}
	if _, leaked := messages.batchInputs[2].Metadata["error"]; leaked {
		t.Fatalf("synthetic tool inherited failure metadata: %#v", messages.batchInputs[2].Metadata)
	}
	if messages.batchInputs[3].Metadata["error"] != "runtime failed" || messages.batchInputs[3].Metadata["stop_reason"] != "error" {
		t.Fatalf("failure metadata moved or disappeared: %#v", messages.batchInputs[3].Metadata)
	}
	if messages.batchInputs[0].ExternalMessageID != "external-original" ||
		messages.batchInputs[0].SourceContext != sourceContext {
		t.Fatalf("tool closure repair lost user receipt: %#v", messages.batchInputs[0])
	}
}
