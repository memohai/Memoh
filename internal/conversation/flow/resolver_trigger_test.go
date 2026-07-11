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

func (*triggerPromptProvider) DoStream(context.Context, sdk.GenerateParams) (*sdk.StreamResult, error) {
	return nil, errors.New("unexpected stream")
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

func TestResolvedContextWithoutReceiptCannotClaimCompaction(t *testing.T) {
	t.Parallel()

	if pressure, known, claimed := (resolvedContext{}).claimCompactionPressure(); claimed || known || pressure != 0 {
		t.Fatalf("claimCompactionPressure() = %d/%v/%v, want 0/false/false without receipt", pressure, known, claimed)
	}
}

func TestPromptReceiptConsumedAcrossSynchronousEntryPoints(t *testing.T) {
	providerFailure := errors.New("provider failed")
	for _, test := range []struct {
		name        string
		providerErr error
		run         func(*Resolver) error
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
			if test.providerErr != nil && !errors.Is(err, test.providerErr) {
				t.Fatalf("entry point error = %v, want %v", err, test.providerErr)
			}
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
		})
	}
}
