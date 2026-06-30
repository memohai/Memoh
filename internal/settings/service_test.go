package settings

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acpfeedback"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

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

func TestNormalizeBotSettingsReadRow_ChatRuntimeFields(t *testing.T) {
	t.Parallel()

	got := normalizeBotSettingsReadRow(sqlc.GetSettingsByBotIDRow{
		Language:           "en",
		ReasoningEffort:    "medium",
		HeartbeatInterval:  60,
		CompactionRatio:    80,
		ChatRuntime:        ChatRuntimeACPAgent,
		ChatAcpAgentID:     pgtype.Text{String: "Codex", Valid: true},
		ChatAcpProjectPath: "/data/app",
		ChatAcpProjectMode: "project",
	})
	if got.ChatRuntime != ChatRuntimeACPAgent {
		t.Fatalf("ChatRuntime = %q, want %q", got.ChatRuntime, ChatRuntimeACPAgent)
	}
	if got.ChatACPAgentID != "codex" {
		t.Fatalf("ChatACPAgentID = %q, want codex", got.ChatACPAgentID)
	}
	if got.ChatACPProjectPath != "/data/app" {
		t.Fatalf("ChatACPProjectPath = %q, want /data/app", got.ChatACPProjectPath)
	}

	def := normalizeBotSettingsReadRow(sqlc.GetSettingsByBotIDRow{
		Language:          "en",
		ReasoningEffort:   "medium",
		HeartbeatInterval: 60,
		CompactionRatio:   80,
	})
	if def.ChatRuntime != ChatRuntimeModel || def.ChatACPProjectPath != DefaultACPProjectPath || def.ChatACPProjectMode != DefaultACPProjectMode {
		t.Fatalf("default chat runtime fields = %#v", def)
	}
}

func TestValidateChatRuntimeSettings(t *testing.T) {
	t.Parallel()

	metadata := []byte(`{"acp":{"agents":{"codex":{"enabled":true,"setup_mode":"api_key","managed":{"api_key":"sk-test"}}}}}`)
	valid := Settings{
		ChatModelID:        "11111111-1111-1111-1111-111111111111",
		ChatRuntime:        ChatRuntimeACPAgent,
		ChatACPAgentID:     "codex",
		ChatACPProjectPath: DefaultACPProjectPath,
		ChatACPProjectMode: DefaultACPProjectMode,
	}
	if err := validateChatRuntimeSettings(metadata, valid); err != nil {
		t.Fatalf("validateChatRuntimeSettings(valid) error = %v", err)
	}

	noModel := valid
	noModel.ChatModelID = ""
	if err := validateChatRuntimeSettings(metadata, noModel); err == nil {
		t.Fatal("validateChatRuntimeSettings without chat model error = nil, want error")
	}

	disabled := valid
	if err := validateChatRuntimeSettings([]byte(`{"acp":{"agents":{"codex":{"enabled":false}}}}`), disabled); feedbackCode(err) != acpfeedback.CodeAgentNotEnabled {
		t.Fatalf("validateChatRuntimeSettings disabled agent code = %q, want %q", feedbackCode(err), acpfeedback.CodeAgentNotEnabled)
	}

	missingKey := valid
	if err := validateChatRuntimeSettings([]byte(`{"acp":{"agents":{"codex":{"enabled":true,"setup_mode":"api_key","managed":{}}}}}`), missingKey); feedbackCode(err) != acpfeedback.CodeAgentNotConfigured {
		t.Fatalf("validateChatRuntimeSettings missing api key code = %q, want %q", feedbackCode(err), acpfeedback.CodeAgentNotConfigured)
	}
}

func feedbackCode(err error) string {
	var feedback *acpfeedback.Error
	if errors.As(err, &feedback) {
		return feedback.Code
	}
	return ""
}

func TestNormalizeBotSettingDefaultHeartbeatInterval(t *testing.T) {
	t.Parallel()

	got := normalizeBotSetting("en", "auto", "allow", false, "medium", false, 0, false, 0, 80)
	if got.HeartbeatInterval != DefaultHeartbeatInterval {
		t.Fatalf("heartbeat interval = %d, want %d", got.HeartbeatInterval, DefaultHeartbeatInterval)
	}
}

func TestReasoningEffortPreservesNonEmptyValue(t *testing.T) {
	t.Parallel()

	const effort = "provider-specific-tier"
	if !isValidReasoningEffort(effort) {
		t.Fatalf("isValidReasoningEffort(%q) = false, want true", effort)
	}
	got := normalizeBotSetting("en", "auto", "allow", true, effort, false, 60, false, 0, 80)
	if got.ReasoningEffort != effort {
		t.Fatalf("normalizeBotSetting effort = %q, want %q", got.ReasoningEffort, effort)
	}
}
