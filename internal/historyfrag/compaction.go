package historyfrag

import (
	"strings"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/messageconv"
)

const NamespaceCompactionLog = "compaction_log"

func LegacySummaryRecord(compactID string, summary string, scope contextfrag.Scope) HistoryRecord {
	compactID = strings.TrimSpace(compactID)
	modelMessage := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("<summary>\n" + summary + "\n</summary>"),
	}
	return HistoryRecord{
		Ref: contextfrag.ContextRef{
			Namespace:  NamespaceCompactionLog,
			ID:         compactID,
			Version:    1,
			Schema:     contextfrag.SchemaContextRef,
			Durability: contextfrag.RefDurable,
		},
		Kind:         contextfrag.KindConversationEvent,
		SourceKind:   SourceCompactionLog,
		Lifecycle:    LifecycleLegacySummary,
		ModelMessage: modelMessage,
		SDKMessage:   messageconv.ModelMessageToSDKMessage(modelMessage),
		Scope:        scope,
		Provenance: contextfrag.Provenance{
			Source:    string(SourceCompactionLog),
			SourceID:  compactID,
			Collector: CollectorHistoryRecords,
		},
		CompactID: compactID,
	}
}
