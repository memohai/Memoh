package historyfrag

import (
	"strings"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

const NamespaceCompactionLog = "compaction_log"

func SummaryRecord(compactID string, summary string, coveredRefs []contextfrag.ContextRef, scope contextfrag.Scope) HistoryRecord {
	rec := summaryRecordBase(compactID, summary, scope)
	rec.Kind = contextfrag.KindConversationSummary
	rec.Lifecycle = LifecycleActiveSummary
	coverage := contextfrag.NewSummaryCoverage(rec.Ref, coveredRefs)
	rec.Coverage = &coverage
	return rec
}

func summaryRecordBase(compactID string, summary string, scope contextfrag.Scope) HistoryRecord {
	compactID = strings.TrimSpace(compactID)
	return HistoryRecord{
		Ref: contextfrag.ContextRef{
			Namespace:  NamespaceCompactionLog,
			ID:         compactID,
			Version:    1,
			Schema:     contextfrag.SchemaContextRef,
			Durability: contextfrag.RefDurable,
		},
		SourceKind: SourceCompactionLog,
		ModelMessage: conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("<summary>\n" + summary + "\n</summary>"),
		},
		Scope: scope,
		Provenance: contextfrag.Provenance{
			Source:    string(SourceCompactionLog),
			SourceID:  compactID,
			Collector: CollectorHistoryRecords,
		},
		CompactID: compactID,
		Budget:    contextfrag.BudgetPolicy{Overflow: contextfrag.OverflowKeep},
	}
}
