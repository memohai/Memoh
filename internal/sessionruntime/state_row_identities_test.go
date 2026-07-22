package sessionruntime

import (
	"bytes"
	"encoding/json"
	"testing"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

func TestEmptyAdmissionRowLedgerSerializesAsArray(t *testing.T) {
	ledger := runtimeLedgerForAdmission(RunAdmissionView{})
	if ledger == nil {
		t.Fatal("empty admission row ledger is nil")
	}
	payload, err := json.Marshal(CurrentRunView{RowLedger: ledger})
	if err != nil {
		t.Fatalf("marshal current run: %v", err)
	}
	if !json.Valid(payload) || !bytes.Contains(payload, []byte(`"row_ledger":[]`)) {
		t.Fatalf("current run payload = %s", payload)
	}
}

func TestRuntimeTextAppendCarriesRowIdentityLedger(t *testing.T) {
	row := conversation.UIRowIdentity{
		StableID: "assistant-row", Role: "assistant", TurnID: "turn-1", TurnPosition: 4, TurnMessageSeq: 2,
	}
	delta, ok := runtimeDeltaForAgentEvent(
		agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "hello"},
		[]conversation.UIMessage{{
			ID: 3, StableID: row.StableID, TurnPosition: row.TurnPosition, TurnMessageSeq: row.TurnMessageSeq,
			RowIdentities: []conversation.UIRowIdentity{row}, Type: conversation.UIMessageText, Content: "hello",
		}},
	)
	if !ok || len(delta.MessageAppends) != 1 {
		t.Fatalf("runtime delta = %#v, %v", delta, ok)
	}
	appendDelta := delta.MessageAppends[0]
	if appendDelta.StableID != row.StableID || appendDelta.TurnPosition != 4 || appendDelta.TurnMessageSeq != 2 || len(appendDelta.RowIdentities) != 1 || appendDelta.RowIdentities[0].TurnID != "turn-1" {
		t.Fatalf("runtime message append identity = %#v", appendDelta)
	}
}
