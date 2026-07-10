package flow

import (
	"reflect"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/messageconv"
)

func TestBudgetItemsAdaptersProduceIdenticalToolOccurrences(t *testing.T) {
	t.Parallel()

	messages := messageconv.SDKMessagesToModelMessages([]sdk.Message{
		sdk.UserMessage("question"),
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.ToolCallPart{ToolCallID: "call-a", ToolName: "first", Input: map[string]any{}},
				sdk.ToolCallPart{ToolCallID: "call-b", ToolName: "second", Input: map[string]any{}},
			},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-a", ToolName: "first", Result: "a"}),
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-b", ToolName: "second", Result: "b"}),
		sdk.AssistantMessage("done"),
	})
	records := make([]historyfrag.HistoryRecord, len(messages))
	entries := make([]composedPipelineMessage, len(messages))
	for i, message := range messages {
		records[i] = historyfrag.HistoryRecord{
			Ref:          contextfrag.ContextRef{Namespace: "history", ID: string(rune('a' + i))},
			ModelMessage: message,
		}
		entries[i] = composedPipelineMessage{message: message}
	}

	historyProjection := budgetItemsForHistoryRecords(records)
	pipelineProjection := budgetItemsForPipelineEntries(entries)
	if len(historyProjection.items) != len(pipelineProjection.items) {
		t.Fatalf("item counts differ: history=%d pipeline=%d", len(historyProjection.items), len(pipelineProjection.items))
	}
	for i := range historyProjection.items {
		historyItem := historyProjection.items[i]
		pipelineItem := pipelineProjection.items[i]
		if historyItem.Tokens != pipelineItem.Tokens || historyItem.Group != pipelineItem.Group || historyItem.Retention != pipelineItem.Retention {
			t.Fatalf("item[%d] differs: history=%#v pipeline=%#v", i, historyItem, pipelineItem)
		}
	}
	if !reflect.DeepEqual(historyProjection.toolAnalysis, pipelineProjection.toolAnalysis) {
		t.Fatalf("tool analyses differ:\nhistory  %#v\npipeline %#v", historyProjection.toolAnalysis, pipelineProjection.toolAnalysis)
	}
}

func TestBudgetItemsForHistoryRecordsMakesRequiredClosureAtomic(t *testing.T) {
	t.Parallel()

	messages := messageconv.SDKMessagesToModelMessages([]sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "call-a",
				ToolName:   "lookup",
				Input:      map[string]any{"query": "weather"},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-a", ToolName: "lookup", Result: "sunny"}),
	})
	projection := budgetItemsForHistoryRecords([]historyfrag.HistoryRecord{
		{Ref: contextfrag.ContextRef{Namespace: "history", ID: "call"}, ModelMessage: messages[0], Required: true},
		{Ref: contextfrag.ContextRef{Namespace: "history", ID: "result"}, ModelMessage: messages[1]},
	})

	if projection.items[0].Group == "" || projection.items[0].Group != projection.items[1].Group {
		t.Fatalf("tool closure groups = %#v, want one group", projection.items)
	}
	if !projection.items[0].Compactable {
		t.Fatalf("required raw source was removed from compactable pressure: %#v", projection.items[0])
	}
	result := contextbudget.Allocate(contextbudget.Request{SourceLimit: 1, Items: projection.items})
	if len(result.Kept) != 2 || result.SourcesFit || result.SourceOverflowTokens <= 0 {
		t.Fatalf("required closure allocation = %#v, want both kept with explicit overflow", result)
	}
}

func TestBudgetItemsForHistoryRecordsRequiresEveryActiveArtifactByIdentity(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	first := historyfrag.SummaryRecord("artifact-a", "same text", nil, scope)
	second := historyfrag.SummaryRecord("artifact-b", "same text", nil, scope)
	projection := budgetItemsForHistoryRecords([]historyfrag.HistoryRecord{first, second})

	if projection.items[0].ID == projection.items[1].ID {
		t.Fatalf("distinct artifacts share identity %q", projection.items[0].ID)
	}
	for i, item := range projection.items {
		if item.Retention != contextbudget.RetentionRequired || item.Compactable {
			t.Fatalf("artifact item[%d] = %#v, want required and non-compactable", i, item)
		}
	}
	result := contextbudget.Allocate(contextbudget.Request{SourceLimit: 1, Items: projection.items})
	if len(result.Kept) != 2 || result.SourcesFit || len(result.Dropped) != 0 {
		t.Fatalf("artifact overflow was hidden: %#v", result)
	}
}

func TestBudgetItemsForPipelineEntriesRequiresSummaryAndCurrentSource(t *testing.T) {
	t.Parallel()

	summary := historyfrag.SummaryRecord("artifact-a", "summary", nil, contextfrag.Scope{})
	projection := budgetItemsForPipelineEntries([]composedPipelineMessage{
		{message: summary.ModelMessage, summaryRecord: summary, hasSummary: true},
		{message: sdkModelMessage(t, sdk.UserMessage("raw history"))},
		{message: sdkModelMessage(t, sdk.UserMessage("current")), forceKeep: true},
	})

	if projection.items[0].Retention != contextbudget.RetentionRequired || projection.items[0].Compactable {
		t.Fatalf("summary item = %#v, want required artifact", projection.items[0])
	}
	if projection.items[1].Retention != contextbudget.RetentionCandidate || !projection.items[1].Compactable {
		t.Fatalf("raw item = %#v, want compactable candidate", projection.items[1])
	}
	if projection.items[2].Retention != contextbudget.RetentionRequired || !projection.items[2].Compactable {
		t.Fatalf("current item = %#v, want required raw source", projection.items[2])
	}
}

func TestBudgetItemsExcludeReasoningFromProjectedPayload(t *testing.T) {
	t.Parallel()

	message := sdkModelMessage(t, sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ReasoningPart{Text: "private reasoning", ProviderMetadata: map[string]any{"signature": "secret"}},
			sdk.TextPart{Text: "visible"},
		},
	})
	projection := budgetItemsForHistoryRecords([]historyfrag.HistoryRecord{{ModelMessage: message}})

	if projection.items[0].Tokens != 2 {
		t.Fatalf("visible token cost = %d, want 2", projection.items[0].Tokens)
	}
	if len(projection.messages[0].Content) != 1 {
		t.Fatalf("projected content = %#v, want one visible part", projection.messages[0].Content)
	}
	text, ok := projection.messages[0].Content[0].(sdk.TextPart)
	if !ok || text.Text != "visible" {
		t.Fatalf("projected visible content = %#v", projection.messages[0].Content[0])
	}

	reasoningOnly := sdkModelMessage(t, sdk.Message{
		Role:    sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private only"}},
	})
	projection = budgetItemsForHistoryRecords([]historyfrag.HistoryRecord{{ModelMessage: reasoningOnly}})
	if projection.items[0].Tokens != 0 || len(projection.messages[0].Content) != 0 {
		t.Fatalf("reasoning-only projection = item:%#v message:%#v, want zero visible payload", projection.items[0], projection.messages[0])
	}
}

func TestBudgetItemsPreservePartLevelIssueForLaterRewrite(t *testing.T) {
	t.Parallel()

	message := sdkModelMessage(t, sdk.Message{
		Role: sdk.MessageRoleTool,
		Content: []sdk.MessagePart{
			sdk.TextPart{Text: "visible payload"},
			sdk.ToolResultPart{ToolCallID: "orphan", ToolName: "lookup", Result: "bad"},
		},
	})
	projection := budgetItemsForHistoryRecords([]historyfrag.HistoryRecord{{ModelMessage: message}})

	wantIssues := []contextbudget.ToolPartIssue{{CarrierIndex: 0, PartIndex: 1, Reason: contextbudget.DropToolOrphanResult}}
	if !reflect.DeepEqual(projection.toolAnalysis.PartIssues, wantIssues) {
		t.Fatalf("part issues = %#v, want %#v", projection.toolAnalysis.PartIssues, wantIssues)
	}
	if projection.items[0].Retention == contextbudget.RetentionDrop {
		t.Fatalf("mixed-validity carrier was dropped wholesale: %#v", projection.items[0])
	}
	if len(projection.messages[0].Content) != 2 {
		t.Fatalf("adapter rewrote source before the rewrite stage: %#v", projection.messages[0])
	}
}

func sdkModelMessage(t *testing.T, message sdk.Message) conversation.ModelMessage {
	t.Helper()
	converted := messageconv.SDKMessagesToModelMessages([]sdk.Message{message})
	if len(converted) != 1 {
		t.Fatalf("SDK conversion returned %d messages", len(converted))
	}
	return converted[0]
}
