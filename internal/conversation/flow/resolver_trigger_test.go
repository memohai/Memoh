package flow

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/schedule"
)

type triggerPromptProvider struct {
	err error
}

func (*triggerPromptProvider) Name() string { return "trigger-prompt" }

func (*triggerPromptProvider) ListModels(context.Context) ([]sdk.Model, error) { return nil, nil }

func (*triggerPromptProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK}
}

func (*triggerPromptProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true}, nil
}

func (p *triggerPromptProvider) DoGenerate(context.Context, sdk.GenerateParams) (*sdk.GenerateResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &sdk.GenerateResult{
		Text:         "HEARTBEAT_OK",
		FinishReason: sdk.FinishReasonStop,
		Messages:     []sdk.Message{sdk.AssistantMessage("HEARTBEAT_OK")},
	}, nil
}

func (p *triggerPromptProvider) DoStream(context.Context, sdk.GenerateParams) (*sdk.StreamResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	stream := make(chan sdk.StreamPart, 8)
	stream <- &sdk.StartPart{}
	stream <- &sdk.StartStepPart{}
	stream <- &sdk.TextStartPart{ID: "text-1"}
	stream <- &sdk.TextDeltaPart{ID: "text-1", Text: "done"}
	stream <- &sdk.TextEndPart{ID: "text-1"}
	stream <- &sdk.FinishStepPart{FinishReason: sdk.FinishReasonStop}
	stream <- &sdk.FinishPart{FinishReason: sdk.FinishReasonStop}
	close(stream)
	return &sdk.StreamResult{Stream: stream}, nil
}

func TestFinishPromptCompactionConsumesKnownPressure(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation:      contextbudget.Allocation{CompactableTokens: 31},
	}, nil)
	resolved := resolvedContext{promptState: state}
	triggered := make(chan int, 1)
	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		promptCompactionFn: func(_ context.Context, _ conversation.ChatRequest, _ resolvedContext, pressure int) {
			triggered <- pressure
		},
	}

	resolver.finishPromptCompaction(context.Background(), conversation.ChatRequest{}, resolved)

	select {
	case pressure := <-triggered:
		if pressure != 31 {
			t.Fatalf("triggered pressure = %d, want 31", pressure)
		}
	case <-time.After(time.Second):
		t.Fatal("finishPromptCompaction() did not trigger compaction")
	}
	if _, _, claimed := resolved.claimCompactionPressure(); claimed {
		t.Fatal("finishPromptCompaction() left the receipt unconsumed")
	}
}

func TestFinishPromptCompactionConsumesUnknownPressureWithoutTriggering(t *testing.T) {
	t.Parallel()

	resolved := resolvedContext{promptState: &initialPromptState{}}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}

	resolver.finishPromptCompaction(context.Background(), conversation.ChatRequest{}, resolved)

	if _, _, claimed := resolved.claimCompactionPressure(); claimed {
		t.Fatal("finishPromptCompaction() left an unknown receipt unconsumed")
	}
}

func TestFinishPromptCompactionDoesNotRetriggerPreSendAttempt(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation:      contextbudget.Allocation{CompactableTokens: 31},
	}, nil)
	if !state.ClaimCompaction() {
		t.Fatal("ClaimCompaction() = false, want pre-send owner")
	}
	triggered := make(chan struct{}, 1)
	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		promptCompactionFn: func(context.Context, conversation.ChatRequest, resolvedContext, int) {
			triggered <- struct{}{}
		},
	}

	resolver.finishPromptCompaction(context.Background(), conversation.ChatRequest{}, resolvedContext{promptState: state})

	select {
	case <-triggered:
		t.Fatal("finishPromptCompaction() retriggered a pre-send attempt")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestResolvedContextWithoutReceiptCannotClaimCompaction(t *testing.T) {
	t.Parallel()

	if pressure, known, claimed := (resolvedContext{}).claimCompactionPressure(); claimed || known || pressure != 0 {
		t.Fatalf("claimCompactionPressure() = %d/%v/%v, want 0/false/false without receipt", pressure, known, claimed)
	}
}

func TestPromptReceiptConsumedAcrossSynchronousEntryPoints(t *testing.T) {
	providerFailure := errors.New("provider failed")
	for _, test := range []struct {
		name                  string
		providerErr           error
		suppressProviderError bool
		skipPromptCompaction  bool
		run                   func(*Resolver) error
	}{
		{
			name: "chat success",
			run: func(resolver *Resolver) error {
				_, err := resolver.Chat(context.Background(), conversation.ChatRequest{
					BotID:                "bot",
					ChatID:               "chat",
					SessionID:            "session",
					Query:                "hello",
					UserMessagePersisted: true,
					SkipTitleGeneration:  true,
				})
				return err
			},
		},
		{
			name:        "chat provider error",
			providerErr: providerFailure,
			run: func(resolver *Resolver) error {
				_, err := resolver.Chat(context.Background(), conversation.ChatRequest{
					BotID:                "bot",
					ChatID:               "chat",
					SessionID:            "session",
					Query:                "hello",
					UserMessagePersisted: true,
					SkipTitleGeneration:  true,
				})
				return err
			},
		},
		{
			name:                 "stream chat success",
			skipPromptCompaction: true,
			run: func(resolver *Resolver) error {
				chunks, errs := resolver.StreamChat(context.Background(), conversation.ChatRequest{
					BotID:                "bot",
					ChatID:               "chat",
					SessionID:            "session",
					Query:                "hello",
					UserMessagePersisted: true,
					SkipTitleGeneration:  true,
				})
				for range chunks {
				}
				var streamErr error
				for err := range errs {
					if err != nil {
						streamErr = err
					}
				}
				return streamErr
			},
		},
		{
			name:                  "stream chat provider error",
			providerErr:           providerFailure,
			suppressProviderError: true,
			skipPromptCompaction:  true,
			run: func(resolver *Resolver) error {
				chunks, errs := resolver.StreamChat(context.Background(), conversation.ChatRequest{
					BotID:                "bot",
					ChatID:               "chat",
					SessionID:            "session",
					Query:                "hello",
					UserMessagePersisted: true,
					SkipTitleGeneration:  true,
				})
				for range chunks {
				}
				var streamErr error
				for err := range errs {
					if err != nil {
						streamErr = err
					}
				}
				return streamErr
			},
		},
		{
			name: "schedule success",
			run: func(resolver *Resolver) error {
				_, err := resolver.TriggerSchedule(context.Background(), "bot", schedule.TriggerPayload{
					SessionID: "session",
					Command:   "run",
				}, "")
				return err
			},
		},
		{
			name:        "schedule provider error",
			providerErr: providerFailure,
			run: func(resolver *Resolver) error {
				_, err := resolver.TriggerSchedule(context.Background(), "bot", schedule.TriggerPayload{
					SessionID: "session",
					Command:   "run",
				}, "")
				return err
			},
		},
		{
			name: "heartbeat success",
			run: func(resolver *Resolver) error {
				_, err := resolver.TriggerHeartbeat(context.Background(), "bot", heartbeat.TriggerPayload{
					SessionID: "session",
				}, "")
				return err
			},
		},
		{
			name:        "heartbeat provider error",
			providerErr: providerFailure,
			run: func(resolver *Resolver) error {
				_, err := resolver.TriggerHeartbeat(context.Background(), "bot", heartbeat.TriggerPayload{
					SessionID: "session",
				}, "")
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bridgeCtx, cancelBridge := context.WithCancel(context.Background())
			bridgeDone := make(chan struct{})
			go func() {
				<-bridgeCtx.Done()
				close(bridgeDone)
			}()
			t.Cleanup(cancelBridge)
			state := &initialPromptState{}
			state.Store(initialPromptResult{
				AccountingReady: true,
				Allocation:      contextbudget.Allocation{CompactableTokens: 47},
			}, nil)
			resolved := resolvedContext{
				runConfig: agentpkg.RunConfig{Model: &sdk.Model{
					ID:       "trigger-model",
					Provider: &triggerPromptProvider{err: test.providerErr},
					Type:     sdk.ModelTypeChat,
				}},
				model: models.GetResponse{
					ID:    "model-id",
					Model: models.Model{ModelID: "trigger-model"},
				},
				provider:    sqlc.Provider{ClientType: "trigger-prompt"},
				promptState: state,
				injectionBridge: &injectionBridge{
					cancel: cancelBridge,
					done:   bridgeDone,
				},
			}
			triggered := make(chan int, 1)
			resolver := &Resolver{
				agent:          agentpkg.New(agentpkg.Deps{Logger: slog.New(slog.DiscardHandler)}),
				logger:         slog.New(slog.DiscardHandler),
				messageService: &recordingMessageService{},
				resolveContextFn: func(context.Context, conversation.ChatRequest) (resolvedContext, error) {
					return resolved, nil
				},
				promptCompactionFn: func(_ context.Context, _ conversation.ChatRequest, _ resolvedContext, pressure int) {
					triggered <- pressure
				},
			}

			err := test.run(resolver)
			if test.providerErr == nil && err != nil {
				t.Fatalf("entry point error = %v", err)
			}
			if test.providerErr != nil && !test.suppressProviderError && !errors.Is(err, test.providerErr) {
				t.Fatalf("entry point error = %v, want %v", err, test.providerErr)
			}
			if test.suppressProviderError && err != nil {
				t.Fatalf("stream entry point error = %v, want nil", err)
			}
			if !test.skipPromptCompaction {
				select {
				case pressure := <-triggered:
					if pressure != 47 {
						t.Fatalf("triggered pressure = %d, want 47", pressure)
					}
				case <-time.After(time.Second):
					t.Fatal("entry point did not trigger prompt compaction")
				}
				if _, _, claimed := resolved.claimCompactionPressure(); claimed {
					t.Fatal("entry point left the receipt unconsumed")
				}
			}
			select {
			case <-bridgeDone:
			case <-time.After(time.Second):
				t.Fatal("entry point did not close injection bridge")
			}
		})
	}
}
