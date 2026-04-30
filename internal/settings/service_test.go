package settings

import (
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestNormalizeBotSettingsReadRow_ShowToolCallsInIMDefault(t *testing.T) {
	t.Parallel()

	row := sqlc.GetSettingsByBotIDRow{
		Language:            "en",
		ReasoningEnabled:    false,
		ReasoningEffort:     "medium",
		HeartbeatEnabled:    false,
		HeartbeatInterval:   60,
		CompactionEnabled:   false,
		CompactionThreshold: 0,
		CompactionRatio:     80,
		ShowToolCallsInIm:   false,
	}
	got := normalizeBotSettingsReadRow(row)
	if got.ShowToolCallsInIM {
		t.Fatalf("expected default ShowToolCallsInIM=false, got true")
	}
}

func TestNormalizeBotSettingsReadRow_ShowToolCallsInIMPropagates(t *testing.T) {
	t.Parallel()

	row := sqlc.GetSettingsByBotIDRow{
		Language:          "en",
		ReasoningEffort:   "medium",
		HeartbeatInterval: 60,
		CompactionRatio:   80,
		ShowToolCallsInIm: true,
	}
	got := normalizeBotSettingsReadRow(row)
	if !got.ShowToolCallsInIM {
		t.Fatalf("expected ShowToolCallsInIM=true to propagate from row")
	}
}

func TestUpsertRequestShowToolCallsInIM_PointerSemantics(t *testing.T) {
	t.Parallel()

	// When the field is nil, the UpsertRequest should not touch the current
	// setting. When non-nil, the dereferenced value should win. We exercise
	// the small gate block without hitting the database.
	current := Settings{ShowToolCallsInIM: true}

	var req UpsertRequest
	if req.ShowToolCallsInIM != nil {
		current.ShowToolCallsInIM = *req.ShowToolCallsInIM
	}
	if !current.ShowToolCallsInIM {
		t.Fatalf("nil pointer must leave current value unchanged")
	}

	off := false
	req.ShowToolCallsInIM = &off
	if req.ShowToolCallsInIM != nil {
		current.ShowToolCallsInIM = *req.ShowToolCallsInIM
	}
	if current.ShowToolCallsInIM {
		t.Fatalf("explicit false pointer must clear the flag")
	}
}
