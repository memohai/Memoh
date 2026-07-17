package flow

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestTrimComposedPipelineMessagesKeepsRetainedArtifactIdentity(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	summaryA := historyfrag.SummaryRecord("artifact-a", "same summary text", []contextfrag.ContextRef{{Namespace: "source", ID: "a"}}, scope)
	summaryB := historyfrag.SummaryRecord("artifact-b", "same summary text", []contextfrag.ContextRef{{Namespace: "source", ID: "b"}}, scope)
	composed := &pipelinepkg.ComposeContextResult{Messages: []pipelinepkg.ContextMessage{
		{Role: "user", Content: summaryA.ModelMessage.TextContent(), CompactionArtifactID: "artifact-a"},
		{Role: "user", Content: strings.Repeat("old raw context", 200)},
		{Role: "user", Content: summaryB.ModelMessage.TextContent(), CompactionArtifactID: "artifact-b"},
		{Role: "user", Content: "latest raw context"},
	}}
	entries := composedPipelineMessages(composed, []historyfrag.HistoryRecord{summaryA, summaryB})
	budget := estimateMessageTokens(summaryB.ModelMessage) + estimateMessageTokens(conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("latest raw context"),
	})

	built := trimComposedPipelineMessages(nil, entries, budget)
	if len(built.Messages) != 4 || !strings.HasPrefix(built.Messages[0].TextContent(), "[System Notice]") {
		t.Fatalf("trimmed messages = %#v, want notice plus both summaries and latest raw", modelMessageTexts(built.Messages))
	}
	if got, want := pipelineSummaryIDs(built.HistoryRecords), []string{"artifact-a", "artifact-b"}; !equalStrings(got, want) {
		t.Fatalf("retained summary identities = %#v, want %#v", got, want)
	}
	frags := historyContextFragsForMessages(built.Messages, built.HistoryRecords)
	if len(frags) != 2 || frags[0].Ref.ID != "artifact-a" || frags[1].Ref.ID != "artifact-b" {
		t.Fatalf("retained summary frags = %#v, want artifact-a and artifact-b", frags)
	}
}

func TestTrimMessagesAndRecordsPreservesEveryActiveSummary(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	summaryA := historyfrag.SummaryRecord("artifact-a", "summary a", nil, scope)
	summaryB := historyfrag.SummaryRecord("artifact-b", "summary b", nil, scope)
	oldRaw := historyfrag.HistoryRecord{ModelMessage: conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(strings.Repeat("old raw context", 200)),
	}}
	latest := historyfrag.HistoryRecord{ModelMessage: conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("latest raw context"),
	}}
	budget := estimateMessageTokens(summaryB.ModelMessage) + estimateMessageTokens(latest.ModelMessage)

	messages, retained, estimate := trimMessagesAndRecordsByTokens(nil, []historyfrag.HistoryRecord{
		summaryA,
		oldRaw,
		summaryB,
		latest,
	}, budget)
	if got, want := pipelineSummaryIDs(retained), []string{"artifact-a", "artifact-b", ""}; !equalStrings(got, want) {
		t.Fatalf("retained records = %#v, want both summaries plus latest raw", got)
	}
	if len(messages) != 4 || !strings.HasPrefix(messages[0].TextContent(), "[System Notice]") {
		t.Fatalf("trimmed messages = %#v, want notice plus both summaries and latest raw", modelMessageTexts(messages))
	}
	wantEstimate := 0
	for _, message := range messages {
		wantEstimate += estimateMessageTokens(message)
	}
	if estimate != wantEstimate {
		t.Fatalf("estimated tokens = %d, want emitted-message total %d", estimate, wantEstimate)
	}
}
