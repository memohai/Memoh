package historyfrag

import (
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messageconv"
)

// ToFrag renders the history record for context-frag manifests. Consumers that
// need provider-continuity details should classify from HistoryRecord.ModelMessage.
func ToFrag(record HistoryRecord) contextfrag.ContextFrag {
	msg := messageconv.ModelMessageToSDKMessage(record.ModelMessage)
	kind := record.Kind
	if kind == "" {
		kind = contextfrag.KindConversationEvent
	}
	provenance := record.Provenance
	if strings.TrimSpace(provenance.Source) == "" {
		provenance.Source = string(record.SourceKind)
	}
	if strings.TrimSpace(provenance.SourceID) == "" {
		provenance.SourceID = strings.TrimSpace(record.Ref.ID)
	}
	if strings.TrimSpace(provenance.Collector) == "" {
		provenance.Collector = CollectorHistoryRecords
	}

	frag := contextfrag.MessageFrag(contextfrag.MessageFragInput{
		ID:         fragmentID(record),
		Message:    msg,
		Kind:       kind,
		Slot:       contextfrag.SlotHistory,
		Priority:   priorityForMessage(msg),
		CacheClass: contextfrag.CacheNever,
		Trust:      trustForHistoryRecord(record),
		Scope:      record.Scope,
		Source:     provenance.Source,
		SourceID:   provenance.SourceID,
		Collector:  provenance.Collector,
		Index:      provenance.Index,
	})
	return contextfrag.WithContextRef(frag, record.Ref)
}

func ToModelMessages(records []HistoryRecord) []conversation.ModelMessage {
	out := make([]conversation.ModelMessage, 0, len(records))
	for _, record := range records {
		out = append(out, record.ModelMessage)
	}
	return out
}

func ToSDKMessages(records []HistoryRecord) []sdk.Message {
	out := make([]sdk.Message, 0, len(records))
	for _, record := range records {
		out = append(out, messageconv.ModelMessageToSDKMessage(record.ModelMessage))
	}
	return out
}

func fragmentID(record HistoryRecord) string {
	source := strings.TrimSpace(string(record.SourceKind))
	if source == "" {
		source = "history"
	}
	id := strings.TrimSpace(record.Ref.ID)
	if id == "" {
		id = strings.TrimSpace(record.DBMessageID)
	}
	if id == "" {
		return "history." + source
	}
	return "history." + source + "." + id
}

func priorityForMessage(msg sdk.Message) int {
	switch msg.Role {
	case sdk.MessageRoleSystem:
		return 30
	case sdk.MessageRoleTool:
		return 55
	default:
		return 70
	}
}

func trustForHistoryRecord(HistoryRecord) contextfrag.TrustLevel {
	return contextfrag.TrustExternal
}
