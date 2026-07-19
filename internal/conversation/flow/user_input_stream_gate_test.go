package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/conversation"
	dbstore "github.com/memohai/memoh/internal/db/store"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/userinput"
)

func TestStreamContinuationGatesAskUserUntilTerminalPersistence(t *testing.T) {
	ambiguousErr := errors.New("ambiguous commit result")
	for _, tt := range []struct {
		name         string
		persistErr   error
		failFirst    error
		workspaceErr error
		wantPrompt   bool
		wantAttempts int32
		wantErr      error
	}{
		{name: "persisted response", wantPrompt: true, wantAttempts: 1},
		{name: "safe transient persistence failure", failFirst: dbstore.MarkPersistenceRetrySafe(errors.New("transient persist continuation")), wantPrompt: true, wantAttempts: 2},
		{name: "ambiguous persistence failure", failFirst: ambiguousErr, wantAttempts: 1, wantErr: ambiguousErr},
		{name: "persistence failure", persistErr: errors.New("persist continuation"), wantAttempts: 1},
		{name: "post commit workspace failure", workspaceErr: errors.New("persist workspace target"), wantPrompt: true, wantAttempts: 1},
		{name: "transient then post commit failure", failFirst: dbstore.MarkPersistenceRetrySafe(errors.New("transient persist continuation")), workspaceErr: errors.New("persist workspace target"), wantPrompt: true, wantAttempts: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.DiscardHandler)
			agent := agentpkg.New(agentpkg.Deps{Logger: logger})
			agent.SetToolProviders([]agenttools.ToolProvider{agenttools.NewAskUserProvider(logger)})
			messages := &modelStreamMessageService{
				recordingMessageService: &recordingMessageService{persistErr: tt.persistErr},
				failFirst:               tt.failFirst,
			}
			var releasePersistence func()
			if tt.persistErr == nil {
				started := make(chan struct{})
				release := make(chan struct{})
				var once sync.Once
				releasePersistence = func() { once.Do(func() { close(release) }) }
				defer releasePersistence()
				messages.persistStarted = started
				messages.persistRelease = release
			}
			resolver := &Resolver{agent: agent, messageService: messages, logger: logger}
			if tt.workspaceErr != nil {
				resolver.sessionService = &fakeBackgroundSessionService{
					getFn: func(context.Context, string) (sessionpkg.Session, error) {
						return sessionpkg.Session{Metadata: map[string]any{}}, nil
					},
					updateMetadataFn: func(context.Context, string, map[string]any) (sessionpkg.Session, error) {
						return sessionpkg.Session{}, tt.workspaceErr
					},
				}
			}
			pending := userinput.Request{
				ID: "00000000-0000-4000-8000-000000000731", BotID: "bot-1", SessionID: "session-1",
				ToolCallID: "ask-continuation", ToolName: userinput.ToolNameAskUser, ShortID: 1,
				Status: userinput.StatusPending, UIPayload: userinput.UIPayload{Version: userinput.PayloadVersion},
			}
			cfg := agentpkg.RunConfig{
				Model: &sdk.Model{ID: "continuation-model", Provider: &continuationAskUserProvider{}},
				Identity: agentpkg.SessionContext{
					BotID: "bot-1", ChatID: "bot-1", SessionID: "session-1",
				},
				SessionType:         "chat",
				SupportsToolCall:    true,
				CanRequestUserInput: true,
				ToolApprovalHandler: func(context.Context, sdk.ToolCall) (sdk.ToolApprovalResult, error) {
					return sdk.ToolApprovalResult{
						Decision: sdk.ToolApprovalDecisionDeferred, ApprovalID: pending.ID,
						Metadata: userinput.DeferredMetadata(pending),
					}, nil
				},
			}
			eventCh := make(chan WSStreamEvent)
			done := make(chan error, 1)
			go func() {
				req := conversation.ChatRequest{
					BotID: "bot-1", ChatID: "bot-1", SessionID: "session-1", UserMessagePersisted: true,
				}
				if tt.workspaceErr != nil {
					req.WorkspaceTarget = &conversation.WorkspaceTarget{TargetID: "workspace-1", Kind: "local"}
				}
				done <- resolver.streamContinuation(context.Background(), cfg, req, "continuation-model", eventCh)
				close(eventCh)
			}()

			var events []agentpkg.StreamEvent
			if messages.persistStarted != nil {
				deadline := time.NewTimer(time.Second)
				defer deadline.Stop()
			waitForPersistence:
				for {
					select {
					case <-messages.persistStarted:
						break waitForPersistence
					case data, ok := <-eventCh:
						if !ok {
							t.Fatal("continuation ended before terminal persistence")
						}
						event := decodeContinuationEvent(t, data)
						if event.Type == agentpkg.EventUserInputRequest || event.ToolName == userinput.ToolNameAskUser {
							t.Fatalf("ask_user event %q escaped before persistence", event.Type)
						}
						events = append(events, event)
					case <-deadline.C:
						t.Fatal("timed out waiting for continuation persistence")
					}
				}
				blocked := time.NewTimer(25 * time.Millisecond)
				defer blocked.Stop()
				select {
				case data, ok := <-eventCh:
					if !ok {
						t.Fatal("continuation ended while terminal persistence was blocked")
					}
					event := decodeContinuationEvent(t, data)
					t.Fatalf("event %q escaped while terminal persistence was blocked", event.Type)
				case <-blocked.C:
				}
				releasePersistence()
			}
			for data := range eventCh {
				event := decodeContinuationEvent(t, data)
				if (event.Type == agentpkg.EventUserInputRequest || event.ToolName == userinput.ToolNameAskUser) && !messages.persisted.Load() {
					t.Fatalf("ask_user event %q escaped before terminal persistence", event.Type)
				}
				events = append(events, event)
			}

			promptSeen := false
			for _, event := range events {
				if event.Type == agentpkg.EventUserInputRequest {
					promptSeen = true
				}
			}
			if promptSeen != tt.wantPrompt {
				t.Fatalf("prompt seen = %t, want %t; events=%#v", promptSeen, tt.wantPrompt, events)
			}
			if attempts := messages.persistAttempts.Load(); attempts != tt.wantAttempts {
				t.Fatalf("persistence attempts = %d, want %d", attempts, tt.wantAttempts)
			}
			err := <-done
			wantErr := tt.wantErr
			if wantErr == nil {
				wantErr = tt.persistErr
			}
			if wantErr == nil && err != nil {
				t.Fatalf("streamContinuation() error = %v", err)
			}
			if wantErr != nil && !errors.Is(err, wantErr) {
				t.Fatalf("streamContinuation() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func decodeContinuationEvent(t *testing.T, data WSStreamEvent) agentpkg.StreamEvent {
	t.Helper()
	var event agentpkg.StreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode continuation event: %v", err)
	}
	return event
}

type continuationAskUserProvider struct{}

func (*continuationAskUserProvider) Name() string { return "continuation-ask-user" }

func (*continuationAskUserProvider) ListModels(context.Context) ([]sdk.Model, error) { return nil, nil }

func (*continuationAskUserProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK}
}

func (*continuationAskUserProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true}, nil
}

func (*continuationAskUserProvider) DoGenerate(context.Context, sdk.GenerateParams) (*sdk.GenerateResult, error) {
	return &sdk.GenerateResult{FinishReason: sdk.FinishReasonToolCalls}, nil
}

func (*continuationAskUserProvider) DoStream(context.Context, sdk.GenerateParams) (*sdk.StreamResult, error) {
	stream := make(chan sdk.StreamPart, 6)
	stream <- &sdk.StartPart{}
	stream <- &sdk.StartStepPart{}
	stream <- &sdk.StreamToolCallPart{
		ToolCallID: "ask-continuation",
		ToolName:   userinput.ToolNameAskUser,
		Input: map[string]any{
			"questions": []any{map[string]any{"text": "Continue?", "kind": userinput.QuestionKindText}},
		},
	}
	stream <- &sdk.FinishStepPart{FinishReason: sdk.FinishReasonToolCalls}
	stream <- &sdk.FinishPart{FinishReason: sdk.FinishReasonToolCalls}
	close(stream)
	return &sdk.StreamResult{Stream: stream}, nil
}
