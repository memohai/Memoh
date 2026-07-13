package pipeline

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestHandleReplyWithAgentACPDoesNotConsumeReceiptWithoutRuntimeAttempt(t *testing.T) {
	t.Parallel()

	receipt := &countingDirectDiscussReceipt{}
	preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
		RunConfig: agentpkg.RunConfig{Messages: []sdk.Message{sdk.UserMessage("prepared")}},
		Receipt:   receipt,
	}}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
		RuntimeType:                 sessionpkg.RuntimeACPAgent,
		DirectDiscussPromptPreparer: preparer,
	}}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot",
			SessionID:        "session",
			ConversationType: channel.ConversationTypeGroup,
		},
		lastProcessedMs: 50,
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{{
		MessageID:    "current",
		ReceivedAtMs: 100,
		MentionsMe:   true,
		Content:      []RenderedContentPiece{{Type: "text", Text: "current"}},
	}}, driver.logger, &fakeDiscussStreamer{})

	if preparer.calls != 1 {
		t.Fatalf("ACP prepare calls = %d, want 1", preparer.calls)
	}
	if receipt.calls.Load() != 0 {
		t.Fatalf("receipt consumed without runtime attempt %d times", receipt.calls.Load())
	}
	if sess.lastProcessedMs != 50 {
		t.Fatalf("cursor advanced without runtime attempt to %d", sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgentACPFinishesReceiptAcrossAttemptExits(t *testing.T) {
	t.Parallel()

	abortEvent, err := json.Marshal(agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort})
	if err != nil {
		t.Fatalf("marshal abort event: %v", err)
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name       string
		ctx        context.Context
		runtime    discussRuntimeStreamer
		wantCursor int64
	}{
		{
			name:       "closed without terminal",
			ctx:        context.Background(),
			runtime:    &scriptedDiscussRuntimeStreamer{},
			wantCursor: 50,
		},
		{
			name:       "terminal abort",
			ctx:        context.Background(),
			runtime:    &scriptedDiscussRuntimeStreamer{chunks: []conversation.StreamChunk{abortEvent}},
			wantCursor: 100,
		},
		{
			name:       "cancelled",
			ctx:        cancelledCtx,
			runtime:    &blockingDiscussRuntimeStreamer{},
			wantCursor: 50,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			receipt := &countingDirectDiscussReceipt{}
			preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
				RunConfig: agentpkg.RunConfig{Messages: []sdk.Message{sdk.UserMessage("prepared late binding")}},
				Receipt:   receipt,
			}}
			resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
				RuntimeType:                 sessionpkg.RuntimeACPAgent,
				DirectDiscussPromptPreparer: preparer,
			}}
			driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: test.runtime})
			sess := &discussSession{
				config: DiscussSessionConfig{
					BotID:            "bot",
					SessionID:        "session",
					ConversationType: channel.ConversationTypeGroup,
				},
				lastProcessedMs: 50,
			}

			driver.handleReplyWithAgent(test.ctx, sess, RenderedContext{{
				MessageID:    "current",
				ReceivedAtMs: 100,
				MentionsMe:   true,
				Content:      []RenderedContentPiece{{Type: "text", Text: "current"}},
			}}, driver.logger, &fakeDiscussStreamer{})

			if receipt.calls.Load() != 1 {
				t.Fatalf("receipt finish calls = %d, want 1", receipt.calls.Load())
			}
			if sess.lastProcessedMs != test.wantCursor {
				t.Fatalf("cursor = %d, want %d", sess.lastProcessedMs, test.wantCursor)
			}
		})
	}
}
