package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

func TestOffEffortFor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		levels []string
		want   string
	}{
		{"none wins", []string{models.ReasoningEffortNone, "low", "medium"}, models.ReasoningEffortNone},
		{"minimal when no none", []string{models.ReasoningEffortMinimal, "low", "medium"}, models.ReasoningEffortMinimal},
		{"lowest advertised when neither", []string{"medium", "high", "xhigh"}, "medium"},
		{"legacy base falls back to low", []string{"low", "medium", "high"}, "low"},
		{"empty falls back to minimal", nil, models.ReasoningEffortMinimal},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := offEffortFor(tt.levels); got != tt.want {
				t.Fatalf("offEffortFor(%v) = %q, want %q", tt.levels, got, tt.want)
			}
		})
	}
}

func TestMatchesModelReference_ModelID(t *testing.T) {
	t.Parallel()

	model := models.GetResponse{
		ID:      "a55f0d2d-1547-49a0-b085-ec4ab778f4b8",
		ModelID: "gpt-4o",
	}

	if !matchesModelReference(model, "gpt-4o") {
		t.Fatal("expected model slug to match")
	}
}

func TestMatchesModelReference_UUID(t *testing.T) {
	t.Parallel()

	model := models.GetResponse{
		ID:      "a55f0d2d-1547-49a0-b085-ec4ab778f4b8",
		ModelID: "gpt-4o",
	}

	if !matchesModelReference(model, "a55f0d2d-1547-49a0-b085-ec4ab778f4b8") {
		t.Fatal("expected model UUID to match")
	}
}

func TestMatchesModelReference_NoMatch(t *testing.T) {
	t.Parallel()

	model := models.GetResponse{
		ID:      "a55f0d2d-1547-49a0-b085-ec4ab778f4b8",
		ModelID: "gpt-4o",
	}

	if matchesModelReference(model, "gpt-4.1") {
		t.Fatal("expected non-matching model reference to fail")
	}
}

func TestMatchesModelReference_TrimmedInput(t *testing.T) {
	t.Parallel()

	model := models.GetResponse{
		ID:      "a55f0d2d-1547-49a0-b085-ec4ab778f4b8",
		ModelID: "gpt-4o",
	}

	if !matchesModelReference(model, "  gpt-4o  ") {
		t.Fatal("expected trimmed model slug to match")
	}
}

func TestBuildModelSelectionRequest_PreservesOverrides(t *testing.T) {
	t.Parallel()

	req := buildModelSelectionRequest(baseRunConfigParams{
		BotID:           "bot-1",
		SessionID:       "session-1",
		CurrentPlatform: "web",
		Model:           "model-override",
		Provider:        "openai-responses",
	}, "chat-1")

	if req.BotID != "bot-1" {
		t.Fatalf("unexpected bot id: %q", req.BotID)
	}
	if req.ChatID != "chat-1" {
		t.Fatalf("unexpected chat id: %q", req.ChatID)
	}
	if req.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %q", req.SessionID)
	}
	if req.CurrentChannel != "web" {
		t.Fatalf("unexpected current channel: %q", req.CurrentChannel)
	}
	if req.Model != "model-override" {
		t.Fatalf("unexpected model override: %q", req.Model)
	}
	if req.Provider != "openai-responses" {
		t.Fatalf("unexpected provider override: %q", req.Provider)
	}
}

func TestResolveReasoningConfig(t *testing.T) {
	t.Parallel()

	// Legacy data: reasoning compat without an explicit thinking_mode resolves to
	// toggle via the SupportsReasoning/ResolveThinkingMode bridge.
	toggleModel := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				Compatibilities: []string{models.CompatReasoning},
			},
		},
	}
	// Adaptive-capable model (Claude 4.6+ family): user can turn thinking off,
	// but when enabled it uses adaptive thinking.
	adaptiveModel := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				ThinkingMode:     models.ThinkingModeAdaptive,
				ReasoningEfforts: []string{"low", "medium", "high", "xhigh", "max"},
			},
		},
	}
	noneEffortModel := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				ThinkingMode:     models.ThinkingModeToggle,
				ReasoningEfforts: []string{"none", "minimal", "low", "medium", "high"},
			},
		},
	}
	// Legacy Anthropic (<=4.5): toggle mode advertising only the implicit
	// low/medium/high base. On the Anthropic wire this must stay non-adaptive so
	// the SDK sends thinking{type:"enabled", budget_tokens:N}.
	legacyAnthropicModel := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				ThinkingMode:     models.ThinkingModeToggle,
				ReasoningEfforts: []string{"low", "medium", "high"},
			},
		},
	}
	// Cloud-variant Claude 4.6+: the registry left it toggle (no
	// supports_adaptive_thinking) but it advertises 4.6+ effort tiers, so the
	// Anthropic wire promotes it to adaptive to stay off the legacy budget path.
	cloudEffortModel := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				ThinkingMode:     models.ThinkingModeToggle,
				ReasoningEfforts: []string{"low", "medium", "high", "xhigh", "max"},
			},
		},
	}
	plainModel := models.GetResponse{}

	tests := []struct {
		name          string
		model         models.GetResponse
		botSettings   settings.Settings
		requestEffort string
		clientType    string
		want          *models.ReasoningConfig
	}{
		{
			name:          "disable overrides bot default",
			model:         toggleModel,
			botSettings:   settings.Settings{ReasoningEnabled: true, ReasoningEffort: models.ReasoningEffortHigh},
			requestEffort: reasoningEffortDisable,
			want:          &models.ReasoningConfig{Disabled: true},
		},
		{
			name:          "legacy adaptive request enables toggle with default effort",
			model:         toggleModel,
			requestEffort: reasoningEffortAdaptive,
			want:          &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortMedium},
		},
		{
			name:          "unsupported none effort falls back to bot default",
			model:         toggleModel,
			botSettings:   settings.Settings{ReasoningEnabled: true, ReasoningEffort: models.ReasoningEffortHigh},
			requestEffort: models.ReasoningEffortNone,
			want:          &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortHigh},
		},
		{
			name:          "explicit none effort is preserved when model supports it",
			model:         noneEffortModel,
			botSettings:   settings.Settings{ReasoningEnabled: true, ReasoningEffort: models.ReasoningEffortHigh},
			requestEffort: models.ReasoningEffortNone,
			want:          &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortNone},
		},
		{
			name:          "explicit effort is trimmed",
			model:         toggleModel,
			requestEffort: " low ",
			want:          &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortLow},
		},
		{
			name:        "bot default is used when no request override",
			model:       toggleModel,
			botSettings: settings.Settings{ReasoningEnabled: true, ReasoningEffort: " high "},
			want:        &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortHigh},
		},
		{
			name:        "bot default falls back to medium",
			model:       toggleModel,
			botSettings: settings.Settings{ReasoningEnabled: true},
			want:        &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortMedium},
		},
		{
			name:        "disabled bot explicitly disables reasoning",
			model:       toggleModel,
			botSettings: settings.Settings{ReasoningEnabled: false, ReasoningEffort: models.ReasoningEffortHigh},
			want:        &models.ReasoningConfig{Disabled: true},
		},
		{
			name:          "adaptive model can still be disabled",
			model:         adaptiveModel,
			requestEffort: reasoningEffortDisable,
			want:          &models.ReasoningConfig{Disabled: true},
		},
		{
			name:          "adaptive model honors explicit effort",
			model:         adaptiveModel,
			requestEffort: models.ReasoningEffortXHigh,
			want:          &models.ReasoningConfig{Active: true, Adaptive: true, Effort: models.ReasoningEffortXHigh},
		},
		{
			name:          "openai wire drops max and falls back to medium",
			model:         adaptiveModel,
			requestEffort: models.ReasoningEffortMax,
			clientType:    string(models.ClientTypeOpenAICompletions),
			want:          &models.ReasoningConfig{Active: true, Adaptive: true, Effort: models.ReasoningEffortMedium},
		},
		{
			name:          "anthropic wire preserves max",
			model:         adaptiveModel,
			requestEffort: models.ReasoningEffortMax,
			clientType:    string(models.ClientTypeAnthropicMessages),
			want:          &models.ReasoningConfig{Active: true, Adaptive: true, Effort: models.ReasoningEffortMax},
		},
		{
			name:        "legacy anthropic stays non-adaptive for budget path",
			model:       legacyAnthropicModel,
			botSettings: settings.Settings{ReasoningEnabled: true, ReasoningEffort: models.ReasoningEffortHigh},
			clientType:  string(models.ClientTypeAnthropicMessages),
			want:        &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortHigh},
		},
		{
			name:        "anthropic cloud variant with effort tiers is promoted to adaptive",
			model:       cloudEffortModel,
			botSettings: settings.Settings{ReasoningEnabled: true, ReasoningEffort: models.ReasoningEffortHigh},
			clientType:  string(models.ClientTypeAnthropicMessages),
			want:        &models.ReasoningConfig{Active: true, Adaptive: true, Effort: models.ReasoningEffortHigh},
		},
		{
			name:        "non-anthropic effort tiers are not promoted to adaptive",
			model:       cloudEffortModel,
			botSettings: settings.Settings{ReasoningEnabled: true, ReasoningEffort: models.ReasoningEffortHigh},
			clientType:  string(models.ClientTypeOpenAICompletions),
			want:        &models.ReasoningConfig{Active: true, Effort: models.ReasoningEffortHigh},
		},
		{
			name:          "model without reasoning ignores request",
			model:         plainModel,
			requestEffort: models.ReasoningEffortHigh,
			want:          nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveReasoningConfig(tt.model, tt.botSettings, tt.requestEffort, tt.clientType)
			if got == nil || tt.want == nil {
				if got != tt.want {
					t.Fatalf("expected %#v, got %#v", tt.want, got)
				}
				return
			}
			if got.Active != tt.want.Active || got.Disabled != tt.want.Disabled ||
				got.Adaptive != tt.want.Adaptive || got.Effort != tt.want.Effort {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}
