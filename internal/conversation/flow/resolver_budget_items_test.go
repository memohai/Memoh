package flow

import (
	"errors"
	"reflect"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextassembly"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/messageconv"
)

func TestBudgetSourceAdaptersAssembleIdenticalToolOccurrences(t *testing.T) {
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

	historyProjection := budgetSourcesForHistoryRecords(records)
	pipelineProjection := budgetSourcesForPipelineEntries(entries)
	if len(historyProjection.sources) != len(pipelineProjection.sources) {
		t.Fatalf("source counts differ: history=%d pipeline=%d", len(historyProjection.sources), len(pipelineProjection.sources))
	}
	for i := range historyProjection.sources {
		historySource := historyProjection.sources[i]
		pipelineSource := pipelineProjection.sources[i]
		if !reflect.DeepEqual(historySource.Message, pipelineSource.Message) || historySource.CompactableTokens != pipelineSource.CompactableTokens || historySource.Retention != pipelineSource.Retention {
			t.Fatalf("source[%d] differs: history=%#v pipeline=%#v", i, historySource, pipelineSource)
		}
	}
	historyAssembly, err := assembleBudgetSources(historyProjection, nil, "")
	if err != nil {
		t.Fatalf("assemble history: %v", err)
	}
	pipelineAssembly, err := assembleBudgetSources(pipelineProjection, nil, "")
	if err != nil {
		t.Fatalf("assemble pipeline: %v", err)
	}
	if !reflect.DeepEqual(historyAssembly.messages, pipelineAssembly.messages) || !reflect.DeepEqual(historyAssembly.sourceIndexes, pipelineAssembly.sourceIndexes) {
		t.Fatalf("assembled streams differ:\nhistory  %#v\npipeline %#v", historyAssembly, pipelineAssembly)
	}
	if !reflect.DeepEqual(allocationWithoutIDs(historyAssembly.allocation), allocationWithoutIDs(pipelineAssembly.allocation)) {
		t.Fatalf("allocations differ:\nhistory  %#v\npipeline %#v", historyAssembly.allocation, pipelineAssembly.allocation)
	}
	if historyAssembly.emittedTokens != pipelineAssembly.emittedTokens {
		t.Fatalf("emitted tokens differ: history=%d pipeline=%d", historyAssembly.emittedTokens, pipelineAssembly.emittedTokens)
	}
}

func TestBudgetSourcesForHistoryRecordsMakeRequiredClosureAtomic(t *testing.T) {
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
	projection := budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{
		{Ref: contextfrag.ContextRef{Namespace: "history", ID: "call"}, ModelMessage: messages[0], Required: true},
		{Ref: contextfrag.ContextRef{Namespace: "history", ID: "result"}, ModelMessage: messages[1]},
	})

	if projection.sources[0].Retention != contextbudget.RetentionRequired || projection.sources[0].CompactableTokens <= 0 {
		t.Fatalf("required raw source = %#v", projection.sources[0])
	}
	limit := 1
	assembled, err := assembleBudgetSources(projection, &limit, "")
	var overflow *contextassembly.OverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("error = %v, want required closure overflow", err)
	}
	if len(assembled.allocation.Kept) != 2 || len(assembled.messages) != 2 || assembled.allocation.SourcesFit || assembled.allocation.SourceOverflowTokens <= 0 {
		t.Fatalf("required closure assembly = %#v", assembled)
	}
}

func TestBudgetSourcesForHistoryRecordsRequireEveryActiveArtifactByIdentity(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	first := historyfrag.SummaryRecord("artifact-a", "same text", nil, scope)
	second := historyfrag.SummaryRecord("artifact-b", "same text", nil, scope)
	projection := budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{first, second})

	if projection.sources[0].ID == projection.sources[1].ID {
		t.Fatalf("distinct artifacts share identity %q", projection.sources[0].ID)
	}
	for i, source := range projection.sources {
		if source.Retention != contextbudget.RetentionRequired || source.CompactableTokens != 0 {
			t.Fatalf("artifact source[%d] = %#v, want required and non-compactable", i, source)
		}
	}
	limit := 1
	assembled, err := assembleBudgetSources(projection, &limit, "")
	var overflow *contextassembly.OverflowError
	if !errors.As(err, &overflow) || len(assembled.allocation.Kept) != 2 || assembled.allocation.SourcesFit || len(assembled.allocation.Dropped) != 0 {
		t.Fatalf("artifact overflow was hidden: assembly=%#v error=%v", assembled, err)
	}
}

func TestBudgetSourcesForPipelineEntriesRequireSummaryAndCurrentSource(t *testing.T) {
	t.Parallel()

	summary := historyfrag.SummaryRecord("artifact-a", "summary", nil, contextfrag.Scope{})
	projection := budgetSourcesForPipelineEntries([]composedPipelineMessage{
		{message: summary.ModelMessage, summaryRecord: summary, hasSummary: true},
		{message: sdkModelMessage(t, sdk.UserMessage("raw history"))},
		{message: sdkModelMessage(t, sdk.UserMessage("current")), forceKeep: true},
	})

	if projection.sources[0].Retention != contextbudget.RetentionRequired || projection.sources[0].CompactableTokens != 0 {
		t.Fatalf("summary source = %#v, want required artifact", projection.sources[0])
	}
	if projection.sources[1].Retention != contextbudget.RetentionCandidate || projection.sources[1].CompactableTokens != messageconv.EstimateSDKMessageTokens(projection.sources[1].Message) {
		t.Fatalf("raw source = %#v, want compactable candidate", projection.sources[1])
	}
	if projection.sources[2].Retention != contextbudget.RetentionRequired || projection.sources[2].CompactableTokens != messageconv.EstimateSDKMessageTokens(projection.sources[2].Message) {
		t.Fatalf("current source = %#v, want required raw source", projection.sources[2])
	}
}

func TestBudgetSourceAssemblyOwnsCanonicalProjection(t *testing.T) {
	t.Parallel()

	message := sdkModelMessage(t, sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ReasoningPart{Text: "private reasoning", ProviderMetadata: map[string]any{"signature": "secret"}},
			sdk.TextPart{Text: "visible"},
		},
	})
	projection := budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{{ModelMessage: message}})
	if len(projection.sources[0].Message.Content) != 2 {
		t.Fatalf("adapter rewrote source before assembly: %#v", projection.sources[0].Message)
	}
	assembled, err := assembleBudgetSources(projection, nil, "")
	if err != nil {
		t.Fatalf("assemble visible source: %v", err)
	}
	if len(assembled.messages) != 1 || assembled.messages[0].TextContent() != "visible" || assembled.emittedTokens != 2 {
		t.Fatalf("canonical assembly = %#v", assembled)
	}

	reasoningOnly := sdkModelMessage(t, sdk.Message{
		Role:    sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private only"}},
	})
	assembled, err = assembleBudgetSources(budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{{ModelMessage: reasoningOnly}}), nil, "")
	if err != nil {
		t.Fatalf("assemble reasoning-only source: %v", err)
	}
	if len(assembled.messages) != 0 || len(assembled.allocation.Dropped) != 1 || assembled.allocation.BudgetTrimmed {
		t.Fatalf("reasoning-only assembly = %#v", assembled)
	}
}

func TestBudgetSourceAssemblyRepairsPartsWithoutDroppingVisibleCarrier(t *testing.T) {
	t.Parallel()

	message := sdkModelMessage(t, sdk.Message{
		Role: sdk.MessageRoleTool,
		Content: []sdk.MessagePart{
			sdk.TextPart{Text: "visible payload"},
			sdk.ToolResultPart{ToolCallID: "orphan", ToolName: "lookup", Result: "bad"},
		},
	})
	projection := budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{{ModelMessage: message}})
	if len(projection.sources[0].Message.Content) != 2 {
		t.Fatalf("adapter rewrote source before assembly: %#v", projection.sources[0].Message)
	}
	assembled, err := assembleBudgetSources(projection, nil, "")
	if err != nil {
		t.Fatalf("assemble mixed-validity source: %v", err)
	}
	if len(assembled.messages) != 1 || assembled.messages[0].TextContent() != "visible payload" || len(assembled.allocation.Dropped) != 0 {
		t.Fatalf("mixed-validity assembly = %#v", assembled)
	}
}

func TestBudgetSourceAssemblyPreservesUsageOnlyForSourceEntries(t *testing.T) {
	t.Parallel()

	call := sdkModelMessage(t, sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-a",
			ToolName:   "lookup",
			Input:      map[string]any{},
		}},
	})
	call.Usage = []byte(`{"inputTokens":3,"outputTokens":5,"totalTokens":8}`)
	projection := budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{
		{ModelMessage: call},
		{ModelMessage: sdkModelMessage(t, sdk.UserMessage("boundary"))},
	})
	assembled, err := assembleBudgetSources(projection, nil, "")
	if err != nil {
		t.Fatalf("assemble dangling call: %v", err)
	}
	if got, want := assembled.sourceIndexes, []int{0, -1, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source indexes = %#v, want %#v", got, want)
	}
	if len(assembled.messages[0].Usage) == 0 || len(assembled.messages[1].Usage) != 0 {
		t.Fatalf("usage leaked across generated entry: %#v", assembled.messages)
	}
}

func allocationWithoutIDs(allocation contextbudget.Allocation) contextbudget.Allocation {
	allocation.Kept = append([]contextbudget.Decision(nil), allocation.Kept...)
	allocation.Dropped = append([]contextbudget.Decision(nil), allocation.Dropped...)
	for i := range allocation.Kept {
		allocation.Kept[i].ID = ""
	}
	for i := range allocation.Dropped {
		allocation.Dropped[i].ID = ""
	}
	return allocation
}

func sdkModelMessage(t *testing.T, message sdk.Message) conversation.ModelMessage {
	t.Helper()
	converted := messageconv.SDKMessagesToModelMessages([]sdk.Message{message})
	if len(converted) != 1 {
		t.Fatalf("SDK conversion returned %d messages", len(converted))
	}
	return converted[0]
}
