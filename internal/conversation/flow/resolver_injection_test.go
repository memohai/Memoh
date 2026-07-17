package flow

import (
	"context"
	"log/slog"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	agenttoolspkg "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/conversation"
)

func TestPrepareInjectedMessageRunRejectsUnconsumedDeliveryOnCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	injectCh := make(chan conversation.InjectMessage, 1)
	injectCh <- conversation.InjectMessage{
		Text: "never consumed",
		OnPersisted: func(err error) {
			result <- err
		},
	}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}
	agentCh, _, _ := resolver.prepareInjectedMessageRun(ctx, "bot", injectCh, false)
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("unconsumed injection reported successful persistence")
		}
	case <-time.After(time.Second):
		t.Fatal("unconsumed injection did not report cancellation")
	}
	select {
	case _, ok := <-agentCh:
		if ok {
			t.Fatal("canceled unconsumed injection reached the agent")
		}
	case <-time.After(time.Second):
		t.Fatal("agent injection channel did not close after cancellation")
	}
}

func TestNotifyInjectedPersistenceReportsEveryConsumedRecord(t *testing.T) {
	t.Parallel()

	results := make(chan error, 2)
	records := []conversation.InjectedMessageRecord{
		{Message: conversation.InjectMessage{OnPersisted: func(err error) { results <- err }}},
		{Message: conversation.InjectMessage{OnPersisted: func(err error) { results <- err }}},
	}
	notifyInjectedPersistence(&records, nil)
	for range records {
		if err := <-results; err != nil {
			t.Fatalf("persisted injection result = %v, want nil", err)
		}
	}
}

func TestPersistTerminalSnapshotStatusDoesNotAcknowledgeUnstoredInjection(t *testing.T) {
	t.Parallel()

	records := []conversation.InjectedMessageRecord{{
		Message: conversation.InjectMessage{Text: "consumed injection"},
	}}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}
	for _, tt := range []struct {
		name string
		snap terminalSnapshot
	}{
		{
			name: "no assistant output",
			snap: terminalSnapshot{sdkMessages: []sdk.Message{sdk.AssistantMessage("")}},
		},
		{
			name: "aborted before visible output",
			snap: terminalSnapshot{
				sdkMessages: []sdk.Message{sdk.AssistantMessage("partial")},
				aborted:     true,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			persisted, stored, err := resolver.persistTerminalSnapshotWithStatus(
				context.Background(),
				conversation.ChatRequest{BotID: "bot", ChatID: "chat"},
				resolvedContext{injectedRecords: &records},
				tt.snap,
			)
			if err != nil {
				t.Fatalf("persist terminal snapshot: %v", err)
			}
			if stored || len(persisted) != 0 {
				t.Fatalf("terminal persistence = stored:%t messages:%d, want false/0", stored, len(persisted))
			}
		})
	}
}

type injectionPrepareProvider struct {
	calls int
}

func (*injectionPrepareProvider) Name() string { return "injection-prepare" }

func (*injectionPrepareProvider) ListModels(context.Context) ([]sdk.Model, error) { return nil, nil }

func (*injectionPrepareProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK}
}

func (*injectionPrepareProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true}, nil
}

func (*injectionPrepareProvider) DoGenerate(context.Context, sdk.GenerateParams) (*sdk.GenerateResult, error) {
	return &sdk.GenerateResult{FinishReason: sdk.FinishReasonStop}, nil
}

func (p *injectionPrepareProvider) DoStream(context.Context, sdk.GenerateParams) (*sdk.StreamResult, error) {
	p.calls++
	stream := make(chan sdk.StreamPart, 8)
	stream <- &sdk.StartPart{}
	stream <- &sdk.StartStepPart{}
	finishReason := sdk.FinishReasonStop
	if p.calls == 1 {
		finishReason = sdk.FinishReasonToolCalls
		stream <- &sdk.StreamToolCallPart{
			ToolCallID: "call-1",
			ToolName:   "injection_noop",
			Input:      map[string]any{},
		}
	} else {
		stream <- &sdk.TextStartPart{ID: "answer"}
		stream <- &sdk.TextDeltaPart{ID: "answer", Text: "done"}
		stream <- &sdk.TextEndPart{ID: "answer"}
	}
	stream <- &sdk.FinishStepPart{FinishReason: finishReason}
	stream <- &sdk.FinishPart{FinishReason: finishReason}
	close(stream)
	return &sdk.StreamResult{Stream: stream}, nil
}

type injectionToolProvider struct{}

func (injectionToolProvider) Tools(context.Context, agenttoolspkg.SessionContext) ([]sdk.Tool, error) {
	return []sdk.Tool{{
		Name: "injection_noop",
		Execute: func(*sdk.ToolExecContext, any) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}}, nil
}

func TestAgentInjectionRecordsSameOutputBoundaryInArrivalOrder(t *testing.T) {
	t.Parallel()

	injections := make(chan agentpkg.InjectMessage, 2)
	injections <- agentpkg.InjectMessage{HeaderifiedText: "first injection"}
	injections <- agentpkg.InjectMessage{HeaderifiedText: "second injection"}
	close(injections)

	var positions []int
	agent := agentpkg.New(agentpkg.Deps{})
	agent.SetToolProviders([]agenttoolspkg.ToolProvider{injectionToolProvider{}})
	for range agent.Stream(context.Background(), agentpkg.RunConfig{
		Model: &sdk.Model{
			ID:       "injection-model",
			Provider: &injectionPrepareProvider{},
		},
		Messages:         []sdk.Message{sdk.UserMessage("original")},
		InjectCh:         injections,
		SupportsToolCall: true,
		InjectedRecorder: func(_ string, insertAfter int) {
			positions = append(positions, insertAfter)
		},
	}) {
	}

	if len(positions) != 2 || positions[0] != 2 || positions[1] != 2 {
		t.Fatalf("injection positions = %#v, want [2 2]", positions)
	}
}

func TestPrepareInjectedMessageRunDeduplicatesDurableEventsPerRun(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}
	message := conversation.InjectMessage{
		Text:            "redelivered",
		HeaderifiedText: "redelivered",
		Source: conversation.InjectedMessageSource{
			EventID: "session-event",
		},
	}

	records := collectInjectedRun(t, resolver, []conversation.InjectMessage{message, message})
	if len(records) != 1 || records[0].Message.Source.EventID != "session-event" {
		t.Fatalf("first run records = %#v, want one durable event", records)
	}

	records = collectInjectedRun(t, resolver, []conversation.InjectMessage{message})
	if len(records) != 1 {
		t.Fatalf("recovery run records = %#v, want event accepted again", records)
	}
}

func TestPrepareInjectedMessageRunDoesNotDeduplicateWithoutDurableEventID(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}
	message := conversation.InjectMessage{Text: "untracked", HeaderifiedText: "untracked"}
	records := collectInjectedRun(t, resolver, []conversation.InjectMessage{message, message})
	if len(records) != 2 {
		t.Fatalf("records without event id = %#v, want both forwarded", records)
	}
}

func collectInjectedRun(t *testing.T, resolver *Resolver, messages []conversation.InjectMessage) []conversation.InjectedMessageRecord {
	t.Helper()

	injectCh := make(chan conversation.InjectMessage, len(messages))
	for _, message := range messages {
		injectCh <- message
	}
	close(injectCh)
	agentCh, records, recorder := resolver.prepareInjectedMessageRun(context.Background(), "bot", injectCh, false)
	for message := range agentCh {
		recorder(message.HeaderifiedText, 0)
	}
	return *records
}

func TestPersistTerminalSnapshotPreservesInjectedSourcesAndOrder(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	records := []conversation.InjectedMessageRecord{
		{
			Message: conversation.InjectMessage{
				Text:            "first injected display",
				HeaderifiedText: "first injected model text",
				Attachments: []conversation.ChatAttachment{{
					ContentHash: "hash-1",
					Mime:        "image/png",
					Name:        "first.png",
				}},
				Source: conversation.InjectedMessageSource{
					ExternalMessageID: "injected-external-1",
					EventID:           "11111111-1111-1111-1111-111111111111",
					DeliveryClaim: &conversation.DeliveryClaim{
						EventID:    "11111111-1111-1111-1111-111111111111",
						ClaimToken: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					},
					EventCursor:               101,
					ReceivedAtMs:              1_010,
					SenderChannelIdentityID:   "injected-identity-1",
					SenderUserID:              "injected-user-1",
					RouteID:                   "injected-route",
					Platform:                  "telegram",
					SourceReplyToMessageID:    "reply-1",
					ReplySender:               "quoted sender",
					ReplyPreview:              "quoted text",
					ForwardMessageID:          "forward-1",
					ForwardFromUserID:         "forward-user",
					ForwardFromConversationID: "forward-conversation",
					ForwardSender:             "forward sender",
					ForwardDate:               1_700_000_000,
				},
			},
			AfterOutput: 1,
			Sequence:    0,
		},
		{
			Message: conversation.InjectMessage{
				Text:            "second injected display",
				HeaderifiedText: "second injected model text",
				Source: conversation.InjectedMessageSource{
					ExternalMessageID: "injected-external-2",
					EventID:           "22222222-2222-2222-2222-222222222222",
					DeliveryClaim: &conversation.DeliveryClaim{
						EventID:    "22222222-2222-2222-2222-222222222222",
						ClaimToken: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
					},
					EventCursor:             102,
					ReceivedAtMs:            1_020,
					SenderChannelIdentityID: "injected-identity-2",
					SenderUserID:            "injected-user-2",
					RouteID:                 "injected-route",
					Platform:                "telegram",
				},
			},
			AfterOutput: 1,
			Sequence:    1,
		},
	}

	err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:             "bot-1",
			SessionID:         "session-1",
			Query:             "original model text",
			RawQuery:          "original display",
			ExternalMessageID: "original-external",
			EventID:           "33333333-3333-3333-3333-333333333333",
			EventDeliveryClaim: &conversation.DeliveryClaim{
				EventID:    "33333333-3333-3333-3333-333333333333",
				ClaimToken: "cccccccc-cccc-cccc-cccc-cccccccccccc",
			},
			SourceChannelIdentityID: "original-identity",
			UserID:                  "original-user",
			RouteID:                 "original-route",
			CurrentChannel:          "telegram",
		},
		resolvedContext{injectedRecords: &records},
		terminalSnapshot{
			sdkMessages: []sdk.Message{
				sdk.AssistantMessage("before injections"),
				sdk.AssistantMessage("after injections"),
			},
			visibleOutput: true,
		},
	)
	if err != nil {
		t.Fatalf("persistTerminalSnapshot() error = %v", err)
	}

	if len(messages.persisted) != 5 {
		t.Fatalf("persisted messages = %d, want 5: %#v", len(messages.persisted), messages.persisted)
	}
	if len(messages.roundOptions) != 1 || len(messages.roundOptions[0].DeliveryClaims) != 3 {
		t.Fatalf("round delivery claims = %#v, want original and two injected claims", messages.roundOptions)
	}
	wantClaims := []conversation.DeliveryClaim{
		{EventID: "33333333-3333-3333-3333-333333333333", ClaimToken: "cccccccc-cccc-cccc-cccc-cccccccccccc"},
		{EventID: "11111111-1111-1111-1111-111111111111", ClaimToken: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
		{EventID: "22222222-2222-2222-2222-222222222222", ClaimToken: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"},
	}
	for i, want := range wantClaims {
		got := messages.roundOptions[0].DeliveryClaims[i]
		if got.EventID != want.EventID || got.ClaimToken != want.ClaimToken {
			t.Fatalf("round delivery claim %d = %#v, want %#v", i, got, want)
		}
	}
	wantRoles := []string{"user", "assistant", "user", "user", "assistant"}
	for i, want := range wantRoles {
		if got := messages.persisted[i].Role; got != want {
			t.Fatalf("persisted role %d = %q, want %q", i, got, want)
		}
	}

	first := messages.persisted[2]
	if first.SenderChannelIdentityID != "injected-identity-1" || first.SenderUserID != "injected-user-1" {
		t.Fatalf("first injected sender = %q/%q", first.SenderChannelIdentityID, first.SenderUserID)
	}
	if first.ExternalMessageID != "injected-external-1" || first.EventID != "11111111-1111-1111-1111-111111111111" || first.SourceReplyToMessageID != "reply-1" {
		t.Fatalf("first injected provenance = %#v", first)
	}
	if first.DisplayText != "first injected display" || persistedTextContent(t, first.Content) != "first injected model text" {
		t.Fatalf("first injected display/model = %q/%q", first.DisplayText, persistedTextContent(t, first.Content))
	}
	if len(first.Assets) != 1 || first.Assets[0].ContentHash != "hash-1" || first.Assets[0].Name != "first.png" {
		t.Fatalf("first injected assets = %#v", first.Assets)
	}
	if first.Metadata["route_id"] != "injected-route" || first.Metadata["platform"] != "telegram" {
		t.Fatalf("first injected route metadata = %#v", first.Metadata)
	}
	reply, _ := first.Metadata["reply"].(map[string]any)
	if reply["message_id"] != "reply-1" || reply["sender"] != "quoted sender" || reply["preview"] != "quoted text" {
		t.Fatalf("first injected reply metadata = %#v", reply)
	}
	forward, _ := first.Metadata["forward"].(map[string]any)
	if forward["message_id"] != "forward-1" || forward["from_user_id"] != "forward-user" || forward["from_conversation_id"] != "forward-conversation" {
		t.Fatalf("first injected forward metadata = %#v", forward)
	}

	second := messages.persisted[3]
	if second.ExternalMessageID != "injected-external-2" || second.EventID != "22222222-2222-2222-2222-222222222222" || second.SenderChannelIdentityID != "injected-identity-2" {
		t.Fatalf("second injected source = %#v", second)
	}
	if messages.persisted[1].SourceReplyToMessageID != "original-external" {
		t.Fatalf("pre-injection assistant source reply = %q", messages.persisted[1].SourceReplyToMessageID)
	}
	if messages.persisted[4].SourceReplyToMessageID != "injected-external-2" {
		t.Fatalf("post-injection assistant source reply = %q", messages.persisted[4].SourceReplyToMessageID)
	}
}

func TestPersistTerminalSnapshotPreservesWorkspaceTargetOnInjectedMessages(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	records := []conversation.InjectedMessageRecord{{
		Message: conversation.InjectMessage{
			Text:            "injected display",
			HeaderifiedText: "injected model text",
			Source: conversation.InjectedMessageSource{
				ExternalMessageID: "injected-external",
				RouteID:           "injected-route",
				Platform:          "telegram",
			},
		},
	}}

	err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:          "bot-1",
			SessionID:      "session-1",
			Query:          "original query",
			RouteID:        "original-route",
			CurrentChannel: "telegram",
			WorkspaceTarget: &conversation.WorkspaceTarget{
				TargetID:      "computer-b",
				Kind:          "remote",
				Name:          "Computer B",
				WorkspacePath: "/workspace/project",
			},
		},
		resolvedContext{injectedRecords: &records},
		terminalSnapshot{
			sdkMessages:   []sdk.Message{sdk.AssistantMessage("response")},
			visibleOutput: true,
		},
	)
	if err != nil {
		t.Fatalf("persistTerminalSnapshot() error = %v", err)
	}

	for _, message := range messages.persisted {
		if message.ExternalMessageID != "injected-external" {
			continue
		}
		location, _ := message.Metadata["execution_location"].(map[string]any)
		if location["target_id"] != "computer-b" || location["workspace_path"] != "/workspace/project" {
			t.Fatalf("injected execution location = %#v", location)
		}
		if message.Metadata["route_id"] != "injected-route" {
			t.Fatalf("injected route metadata = %#v", message.Metadata)
		}
		return
	}
	t.Fatal("injected message was not persisted")
}
