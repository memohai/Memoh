package flow

import (
	"strconv"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/messageconv"
)

type budgetSourceInput struct {
	id          string
	message     conversation.ModelMessage
	required    bool
	compactable bool
}

type budgetSourceProjection struct {
	items        []contextbudget.Item
	messages     []sdk.Message
	toolAnalysis contextbudget.ToolOccurrenceAnalysis
}

func budgetItemsForHistoryRecords(records []historyfrag.HistoryRecord) budgetSourceProjection {
	sources := make([]budgetSourceInput, len(records))
	for index, record := range records {
		activeArtifact := isActiveContextArtifact(record)
		sources[index] = budgetSourceInput{
			id:          historyBudgetItemID(record, index),
			message:     record.ModelMessage,
			required:    record.Required || activeArtifact,
			compactable: !activeArtifact,
		}
	}
	return projectBudgetSources(sources)
}

func budgetItemsForPipelineEntries(entries []composedPipelineMessage) budgetSourceProjection {
	sources := make([]budgetSourceInput, len(entries))
	for index, entry := range entries {
		required := entry.hasSummary || entry.forceKeep
		id := "pipeline:" + strconv.Itoa(index)
		if entry.hasSummary {
			id = historyBudgetItemID(entry.summaryRecord, index)
		}
		sources[index] = budgetSourceInput{
			id:          id,
			message:     entry.message,
			required:    required,
			compactable: !entry.hasSummary,
		}
	}
	return projectBudgetSources(sources)
}

func projectBudgetSources(sources []budgetSourceInput) budgetSourceProjection {
	projection := budgetSourceProjection{
		items:    make([]contextbudget.Item, len(sources)),
		messages: make([]sdk.Message, len(sources)),
	}
	carriers := make([]contextbudget.ToolCarrier, len(sources))
	for index, source := range sources {
		message := canonicalBudgetMessage(source.message)
		projection.messages[index] = message
		carriers[index] = budgetToolCarrier(message)
		retention := contextbudget.RetentionCandidate
		if source.required {
			retention = contextbudget.RetentionRequired
		}
		projection.items[index] = contextbudget.Item{
			ID:          source.id,
			Tokens:      messageconv.EstimateModelMessageTokens(source.message),
			Retention:   retention,
			Compactable: source.compactable,
		}
	}
	projection.toolAnalysis = contextbudget.AnalyzeToolOccurrences(carriers)
	for index, binding := range projection.toolAnalysis.Bindings {
		projection.items[index].Group = binding.Group
	}
	return projection
}

func canonicalBudgetMessage(message conversation.ModelMessage) sdk.Message {
	return messageconv.ModelMessageToSDKMessage(conversation.ModelMessage{
		Role:    message.Role,
		Content: messageconv.CanonicalModelMessageContent(message),
	})
}

func budgetToolCarrier(message sdk.Message) contextbudget.ToolCarrier {
	carrier := contextbudget.ToolCarrier{
		BoundaryBefore: !strings.EqualFold(strings.TrimSpace(string(message.Role)), string(sdk.MessageRoleTool)),
	}
	for partIndex, part := range message.Content {
		switch typed := part.(type) {
		case sdk.ToolCallPart:
			carrier.Parts = append(carrier.Parts, contextbudget.ToolPart{
				PartIndex: partIndex,
				Kind:      contextbudget.ToolPartCall,
				CallID:    typed.ToolCallID,
			})
		case sdk.ToolResultPart:
			carrier.Parts = append(carrier.Parts, contextbudget.ToolPart{
				PartIndex: partIndex,
				Kind:      contextbudget.ToolPartResult,
				CallID:    typed.ToolCallID,
			})
		}
	}
	return carrier
}

func isActiveContextArtifact(record historyfrag.HistoryRecord) bool {
	return record.Kind == contextfrag.KindConversationSummary || record.Lifecycle == historyfrag.LifecycleActiveSummary
}

func historyBudgetItemID(record historyfrag.HistoryRecord, index int) string {
	if strings.TrimSpace(record.Ref.Namespace) != "" && strings.TrimSpace(record.Ref.ID) != "" {
		return record.Ref.StableKey()
	}
	if id := strings.TrimSpace(record.DBMessageID); id != "" {
		return "history-message:" + id
	}
	if id := strings.TrimSpace(record.CompactID); id != "" {
		return "compaction:" + id
	}
	return "history:" + strconv.Itoa(index)
}
