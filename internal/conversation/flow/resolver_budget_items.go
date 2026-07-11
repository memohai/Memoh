package flow

import (
	"strconv"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextassembly"
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
	sources          []contextassembly.Source
	originalMessages []conversation.ModelMessage
	currentSourceID  string
}

type budgetSourceAssembly struct {
	messages      []conversation.ModelMessage
	sourceIndexes []int
	allocation    contextbudget.Allocation
	emittedTokens int
}

func budgetSourcesForHistoryRecords(records []historyfrag.HistoryRecord) budgetSourceProjection {
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

func budgetSourcesForPipelineEntries(entries []composedPipelineMessage) budgetSourceProjection {
	sources := make([]budgetSourceInput, len(entries))
	currentSourceID := ""
	for index, entry := range entries {
		required := entry.hasSummary || entry.forceKeep
		id := "pipeline:" + strconv.Itoa(index)
		if entry.hasSummary {
			id = historyBudgetItemID(entry.summaryRecord, index)
		}
		if entry.forceKeep {
			if strings.TrimSpace(entry.currentSourceID) == "" {
				entry.currentSourceID = "pipeline-current:request"
			}
			id = entry.currentSourceID
			currentSourceID = id
		}
		sources[index] = budgetSourceInput{
			id:          id,
			message:     entry.message,
			required:    required,
			compactable: !entry.hasSummary,
		}
	}
	projection := projectBudgetSources(sources)
	projection.currentSourceID = currentSourceID
	return projection
}

func projectBudgetSources(sources []budgetSourceInput) budgetSourceProjection {
	projection := budgetSourceProjection{
		sources:          make([]contextassembly.Source, len(sources)),
		originalMessages: make([]conversation.ModelMessage, len(sources)),
	}
	for index, source := range sources {
		tokens := messageconv.EstimateModelMessageTokens(source.message)
		if sanitized := sanitizeMessages([]conversation.ModelMessage{source.message}); len(sanitized) > 0 {
			source.message = sanitized[0]
		} else {
			source.message = conversation.ModelMessage{}
		}
		projection.originalMessages[index] = source.message
		retention := contextbudget.RetentionCandidate
		if source.required {
			retention = contextbudget.RetentionRequired
		}
		projection.sources[index] = contextassembly.Source{
			ID:        source.id,
			Message:   messageconv.ModelMessageToSDKMessage(source.message),
			Retention: retention,
		}
		if source.compactable {
			projection.sources[index].CompactableTokens = tokens
		}
	}
	return projection
}

func assembleBudgetSources(projection budgetSourceProjection, envelopeLimit *int, notice string) (budgetSourceAssembly, error) {
	result, err := contextassembly.Assemble(contextassembly.Request{
		Sources:             projection.sources,
		EnvelopeLimit:       envelopeLimit,
		Notice:              notice,
		SyntheticToolResult: syntheticToolClosureError,
	})
	assembled := budgetSourceAssembly{
		messages:      make([]conversation.ModelMessage, 0, len(result.Entries)),
		sourceIndexes: make([]int, 0, len(result.Entries)),
		allocation:    result.Allocation,
		emittedTokens: result.EmittedTokens,
	}
	for _, entry := range result.Entries {
		converted := messageconv.SDKMessagesToModelMessages([]sdk.Message{entry.Message})
		if len(converted) == 0 {
			continue
		}
		message := converted[0]
		if entry.SourceIndex >= 0 && entry.SourceIndex < len(projection.originalMessages) {
			message.Usage = projection.originalMessages[entry.SourceIndex].Usage
		}
		assembled.messages = append(assembled.messages, message)
		assembled.sourceIndexes = append(assembled.sourceIndexes, entry.SourceIndex)
	}
	return assembled, err
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
