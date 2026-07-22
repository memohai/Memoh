package sessionruntime

import (
	"sort"
	"strings"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func runtimeLedgerForAdmission(admission RunAdmissionView) []conversation.UIRowIdentity {
	ledger := make([]conversation.UIRowIdentity, len(admission.RowLedger))
	copy(ledger, admission.RowLedger)
	if admission.TurnReservation == nil {
		return mergeRowLedger(ledger)
	}
	request := admission.TurnReservation.Request
	if strings.TrimSpace(request.MessageID) == "" {
		return mergeRowLedger(ledger)
	}
	return mergeRowLedger(ledger, rowIdentityFromReservation(request))
}

func rowIdentityFromReservation(row messagepkg.RuntimeRowReservation) conversation.UIRowIdentity {
	return conversation.UIRowIdentity{
		StableID:       row.MessageID,
		Role:           row.Role,
		TurnID:         row.TurnID,
		TurnPosition:   row.TurnPosition,
		TurnMessageSeq: row.TurnMessageSeq,
	}
}

func uiRowIdentitiesFromAgent(rows []agentpkg.RowIdentity) []conversation.UIRowIdentity {
	if len(rows) == 0 {
		return nil
	}
	result := make([]conversation.UIRowIdentity, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.StableID) == "" {
			continue
		}
		result = append(result, conversation.UIRowIdentity{
			StableID:       row.StableID,
			Role:           row.Role,
			TurnID:         row.TurnID,
			TurnPosition:   row.TurnPosition,
			TurnMessageSeq: row.TurnMessageSeq,
		})
	}
	return result
}

func mergeRowLedger(ledger []conversation.UIRowIdentity, rows ...conversation.UIRowIdentity) []conversation.UIRowIdentity {
	for _, incoming := range rows {
		if strings.TrimSpace(incoming.StableID) == "" {
			continue
		}
		index := -1
		for i := range ledger {
			if ledger[i].StableID == incoming.StableID {
				index = i
				break
			}
		}
		if index >= 0 {
			ledger[index] = incoming
		} else {
			ledger = append(ledger, incoming)
		}
	}
	sort.SliceStable(ledger, func(i, j int) bool {
		if ledger[i].TurnPosition != ledger[j].TurnPosition {
			return ledger[i].TurnPosition < ledger[j].TurnPosition
		}
		if ledger[i].TurnMessageSeq != ledger[j].TurnMessageSeq {
			return ledger[i].TurnMessageSeq < ledger[j].TurnMessageSeq
		}
		return ledger[i].StableID < ledger[j].StableID
	})
	return ledger
}
