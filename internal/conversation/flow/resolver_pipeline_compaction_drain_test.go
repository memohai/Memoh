package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestDrainPipelineHistoryContextRebuildsFromCompactionArtifacts(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		artifact  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	base := time.UnixMilli(1_000).UTC()
	covered := pipelineHistoryMessage(t, "row-old", botID, sessionID, "external-old", base.Add(100*time.Millisecond), "user", strings.Repeat("old raw context ", 200))
	messages := &pipelineContextMessageService{rows: []messagepkg.Message{covered}}
	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "external-old", 1_000, strings.Repeat("old raw context ", 200)))
	p.PushEvent(sessionID, pipelineMessageEvent(sessionID, "external-current", 2_000, "current"))
	queries := &recordingCompactionLogQueries{}
	attempts := 0
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: messages,
		queries:        queries,
		syncCompactionFn: func(context.Context, conversation.ChatRequest, int, int) (compaction.Result, error) {
			attempts++
			covered.CompactID = artifact
			messages.rows[0] = covered
			queries.logs = []sqlc.BotHistoryMessageCompact{
				pipelineArtifactRow(t, artifact, botID, sessionID, "condensed old context", []messagepkg.Message{covered}, base.Add(time.Minute)),
			}
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
	}
	req := conversation.ChatRequest{
		BotID:             botID,
		ChatID:            "chat",
		SessionID:         sessionID,
		ExternalMessageID: "external-current",
		Query:             "current",
	}
	initial, err := resolver.buildPipelineContext(context.Background(), req, 0)
	if err != nil {
		t.Fatalf("buildPipelineContext() error = %v", err)
	}
	if initial.Allocation.CompactableTokens < preSendCompactionThreshold(200) {
		t.Fatalf("initial pressure = %d, want backstop pressure", initial.Allocation.CompactableTokens)
	}

	drained, attempted, err := resolver.drainPipelineHistoryContext(context.Background(), req, initial, 200)
	if err != nil {
		t.Fatalf("drainPipelineHistoryContext() error = %v", err)
	}
	if !attempted || attempts != 1 || drained.Allocation.CompactableTokens >= preSendCompactionThreshold(200) {
		t.Fatalf("pipeline drain = attempted:%t attempts:%d pressure:%d", attempted, attempts, drained.Allocation.CompactableTokens)
	}
	texts := modelMessageTexts(drained.Messages)
	if len(texts) != 2 || !strings.Contains(texts[0], "condensed old context") || !strings.Contains(texts[1], "current") || strings.Contains(strings.Join(texts, "\n"), "old raw context") {
		t.Fatalf("drained pipeline messages = %#v", texts)
	}
}
