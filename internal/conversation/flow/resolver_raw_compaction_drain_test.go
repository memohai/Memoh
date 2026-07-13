package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type rawDrainHistoryService struct {
	recordingMessageService
	history []messagepkg.Message
}

func (s *rawDrainHistoryService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return append([]messagepkg.Message(nil), s.history...), nil
}

func TestDrainRawHistoryContextContinuesOnNextPromptAfterAttemptCap(t *testing.T) {
	t.Parallel()

	current := persistedHistoryMessage(t, "current", "assistant", "current answer")
	history := &rawDrainHistoryService{}
	setOldHistory := func(words int) {
		history.history = []messagepkg.Message{
			persistedHistoryMessage(t, "old", "user", strings.Repeat("old ", words)),
			current,
		}
	}
	setOldHistory(8_000)
	attempts := 0
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: history,
		syncCompactionFn: func(context.Context, conversation.ChatRequest, int, int) (compaction.Result, error) {
			attempts++
			switch attempts {
			case 1:
				setOldHistory(6_000)
			case 2:
				setOldHistory(4_000)
			case 3:
				setOldHistory(2_000)
			default:
				history.history = []messagepkg.Message{current}
			}
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
	}
	req := conversation.ChatRequest{BotID: "bot", SessionID: "session"}
	fallback := historyfrag.ScopeFallback{}
	initial, err := resolver.buildRawHistoryContext(context.Background(), req, fallback)
	if err != nil {
		t.Fatalf("buildRawHistoryContext() error = %v", err)
	}

	first, attempted, err := resolver.drainRawHistoryContext(context.Background(), req, fallback, initial, 2_000)
	if err != nil {
		t.Fatalf("first drainRawHistoryContext() error = %v", err)
	}
	threshold := preSendCompactionThreshold(2_000)
	if !attempted || attempts != maxPreSendCompactionAttempts || first.Allocation.CompactableTokens < threshold {
		t.Fatalf("first raw drain = attempted:%t attempts:%d pressure:%d threshold:%d", attempted, attempts, first.Allocation.CompactableTokens, threshold)
	}

	second, attempted, err := resolver.drainRawHistoryContext(context.Background(), req, fallback, first, 2_000)
	if err != nil {
		t.Fatalf("second drainRawHistoryContext() error = %v", err)
	}
	if !attempted || attempts != maxPreSendCompactionAttempts+1 || second.Allocation.CompactableTokens >= threshold {
		t.Fatalf("second raw drain = attempted:%t attempts:%d pressure:%d threshold:%d", attempted, attempts, second.Allocation.CompactableTokens, threshold)
	}
	texts := modelMessageTexts(second.Messages)
	if len(texts) != 1 || texts[0] != "current answer" {
		t.Fatalf("second raw drain messages = %#v", texts)
	}
}
