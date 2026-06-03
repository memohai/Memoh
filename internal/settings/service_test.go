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

func TestNormalizeBotSettingsReadRow_CommandUILanguage(t *testing.T) {
	t.Parallel()

	// Explicit value propagates from the read row.
	got := normalizeBotSettingsReadRow(sqlc.GetSettingsByBotIDRow{
		Language:          "en",
		CommandUiLanguage: "zh",
		ReasoningEffort:   "medium",
		HeartbeatInterval: 60,
		CompactionRatio:   80,
	})
	if got.CommandUILanguage != "zh" {
		t.Fatalf("CommandUILanguage = %q, want zh", got.CommandUILanguage)
	}

	// Empty value defaults to "auto" (mirrors the DB column default).
	def := normalizeBotSettingsReadRow(sqlc.GetSettingsByBotIDRow{
		Language:          "en",
		ReasoningEffort:   "medium",
		HeartbeatInterval: 60,
		CompactionRatio:   80,
	})
	if def.CommandUILanguage != DefaultCommandUILanguage {
		t.Fatalf("default CommandUILanguage = %q, want %q", def.CommandUILanguage, DefaultCommandUILanguage)
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

func TestNormalizeBotSettingDefaultHeartbeatInterval(t *testing.T) {
	t.Parallel()

	got := normalizeBotSetting("en", "auto", "allow", false, "medium", false, 0, false, 0, 80)
	if got.HeartbeatInterval != DefaultHeartbeatInterval {
		t.Fatalf("heartbeat interval = %d, want %d", got.HeartbeatInterval, DefaultHeartbeatInterval)
	}
	if got.HeartbeatInterval != 1440 {
		t.Fatalf("heartbeat interval = %d, want 1440", got.HeartbeatInterval)
	}
}

func TestReasoningEffortAllowsFullModelLadder(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"none", "low", "medium", "high", "xhigh"} {
		if !isValidReasoningEffort(effort) {
			t.Fatalf("isValidReasoningEffort(%q) = false, want true", effort)
		}
		got := normalizeBotSetting("en", "auto", "allow", true, effort, false, 60, false, 0, 80)
		if got.ReasoningEffort != effort {
			t.Fatalf("normalizeBotSetting effort = %q, want %q", got.ReasoningEffort, effort)
		}
	}
}
