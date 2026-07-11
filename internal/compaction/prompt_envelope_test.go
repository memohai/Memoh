package compaction

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/models"
)

type sqlcRow = sqlc.ListUncompactedMessagesBySessionRow

func envelopeRow(t *testing.T, role, text string, createdAt time.Time) sqlcRow {
	t.Helper()
	row := mkRow(t, role, jsonStr(text), 0)
	row.CreatedAt = pgtype.Timestamptz{Time: createdAt, Valid: true}
	return row
}

func pgTimestamp(milliseconds int64) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.UnixMilli(milliseconds), Valid: true}
}

func TestMaterializeCompactionEnvelopeRejectsUnknownContextBudget(t *testing.T) {
	t.Parallel()

	items, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("old context ", 100), time.UnixMilli(1000)),
	})
	for _, budget := range []int{0, -1} {
		_, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
			ContextTokenBudget: budget,
			Candidates:         items,
		})
		if !errors.Is(err, errCompactionBudgetUnknown) {
			t.Fatalf("budget %d error = %v, want errCompactionBudgetUnknown", budget, err)
		}
	}
}

func TestMaterializeCompactionEnvelopeRejectsProviderWithoutOutputLimit(t *testing.T) {
	t.Parallel()

	items, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("old context ", 100), time.UnixMilli(1000)),
	})
	model := models.NewSDKChatModel(models.SDKModelConfig{
		ClientType: string(models.ClientTypeOpenAICodex),
		ModelID:    "codex-summary-model",
	})

	_, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
		Model:              model,
		ContextTokenBudget: 10_000,
		Candidates:         items,
	})
	if !errors.Is(err, errCompactionOutputLimitUnsupported) {
		t.Fatalf("materializeCompactionEnvelope() error = %v, want errCompactionOutputLimitUnsupported", err)
	}
}

func TestMaterializeCompactionEnvelopeAccountsForDecoratedInputAndReservesOutput(t *testing.T) {
	t.Parallel()

	const contextBudget = 10_000
	items, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("detailed prior exchange ", 200), time.UnixMilli(2000)),
	})
	model := models.NewSDKChatModel(models.SDKModelConfig{
		ClientType: string(models.ClientTypeAnthropicMessages),
		ModelID:    "summary-model",
	})

	envelope, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
		Model:              model,
		PromptCacheTTL:     models.PromptCacheTTL5m,
		ContextTokenBudget: contextBudget,
		Candidates:         items,
	})
	if err != nil {
		t.Fatalf("materializeCompactionEnvelope() error = %v", err)
	}
	if envelope.System != "" || len(envelope.Messages) < 2 || envelope.Messages[0].Role != sdk.MessageRoleSystem {
		t.Fatalf("prompt-cache decorated envelope = system:%q messages:%#v", envelope.System, envelope.Messages)
	}
	encoded, err := json.Marshal(struct {
		System   string        `json:"system,omitempty"`
		Messages []sdk.Message `json:"messages"`
	}{System: envelope.System, Messages: envelope.Messages})
	if err != nil {
		t.Fatalf("marshal final envelope: %v", err)
	}
	if envelope.InputTokenUpperBound != len(encoded) {
		t.Fatalf("input upper bound = %d, encoded bytes %d", envelope.InputTokenUpperBound, len(encoded))
	}
	if envelope.MaxOutputTokens <= 0 || envelope.MaxOutputTokens > maxCompactionSummaryTokens {
		t.Fatalf("max output tokens = %d, want 1..%d", envelope.MaxOutputTokens, maxCompactionSummaryTokens)
	}
	if envelope.InputTokenUpperBound+envelope.MaxOutputTokens+envelope.HeadroomTokens > contextBudget {
		t.Fatalf(
			"final envelope = input:%d output:%d headroom:%d budget:%d",
			envelope.InputTokenUpperBound,
			envelope.MaxOutputTokens,
			envelope.HeadroomTokens,
			contextBudget,
		)
	}
}

func TestMaterializeCompactionEnvelopeUsesOnlyStrictlyEarlierArtifacts(t *testing.T) {
	t.Parallel()

	items, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("candidate context ", 100), time.UnixMilli(5000)),
	})
	envelope, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
		ContextTokenBudget: 10_000,
		Candidates:         items,
		Artifacts: []Artifact{
			{ID: "later", Summary: "LATER_ARTIFACT", AnchorStartMs: 5500, AnchorEndMs: 6000},
			{ID: "early", Summary: "EARLY_ARTIFACT", AnchorStartMs: 3000, AnchorEndMs: 4000},
			{ID: "equal", Summary: "EQUAL_ARTIFACT", AnchorStartMs: 4500, AnchorEndMs: 5000},
			{ID: "unknown", Summary: "UNKNOWN_ARTIFACT"},
		},
	})
	if err != nil {
		t.Fatalf("materializeCompactionEnvelope() error = %v", err)
	}
	prompt := envelopePromptText(envelope)
	if !strings.Contains(prompt, "EARLY_ARTIFACT") {
		t.Fatalf("strictly earlier artifact missing from prompt: %q", prompt)
	}
	for _, forbidden := range []string{"LATER_ARTIFACT", "EQUAL_ARTIFACT", "UNKNOWN_ARTIFACT"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("non-prior artifact %q leaked into prompt: %q", forbidden, prompt)
		}
	}
}

func TestMaterializeCompactionEnvelopeDropsOversizedPriorBeforeCandidates(t *testing.T) {
	t.Parallel()

	items, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("candidate survives ", 100), time.UnixMilli(5000)),
	})
	envelope, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
		ContextTokenBudget: 4_000,
		Candidates:         items,
		Artifacts: []Artifact{{
			ID:            "huge-prior",
			Summary:       "OVERSIZED_PRIOR_" + strings.Repeat("x", 20_000),
			AnchorStartMs: 1000,
			AnchorEndMs:   2000,
		}},
	})
	if err != nil {
		t.Fatalf("materializeCompactionEnvelope() error = %v", err)
	}
	prompt := envelopePromptText(envelope)
	if strings.Contains(prompt, "OVERSIZED_PRIOR_") || !strings.Contains(prompt, "candidate survives") {
		t.Fatalf("optional prior displaced required candidate: %q", prompt)
	}
}

func TestMaterializeCompactionEnvelopeTrimsCompleteToolGroupsAtomically(t *testing.T) {
	t.Parallel()

	old := envelopeRow(t, "user", strings.Repeat("oversized old prefix ", 20_000), time.UnixMilli(1000))
	call := toolCallRow(t, 0)
	call.CreatedAt = pgTimestamp(2000)
	result := toolResultRow(t, 0)
	result.CreatedAt = pgTimestamp(3000)
	items, _ := itemsFromRows([]sqlcRow{old, call, result})

	envelope, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
		ContextTokenBudget: 4_000,
		Candidates:         items,
	})
	if err != nil {
		t.Fatalf("materializeCompactionEnvelope() error = %v", err)
	}
	if len(envelope.MessageIDs) != 2 || envelope.MessageIDs[0] != call.ID || envelope.MessageIDs[1] != result.ID {
		t.Fatalf("selected ids = %#v, want complete call/result suffix", envelope.MessageIDs)
	}
}

func TestMaterializeCompactionEnvelopeRejectsOversizedCompleteGroup(t *testing.T) {
	t.Parallel()

	call := toolCallRow(t, 0)
	call.Content = []byte(`[{"type":"text","text":"` + strings.Repeat("x", 20_000) + `"},{"type":"tool-call","toolCallId":"c1","toolName":"search","input":{"query":"large"}}]`)
	result := toolResultRow(t, 0)
	result.Content = []byte(`[{"type":"tool-result","toolCallId":"c1","toolName":"search","result":"` + strings.Repeat("y", 20_000) + `"}]`)
	items, _ := itemsFromRows([]sqlcRow{call, result})

	_, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
		ContextTokenBudget: 1_000,
		Candidates:         items,
	})
	if !errors.Is(err, errCompactionEnvelopeTooSmall) {
		t.Fatalf("materializeCompactionEnvelope() error = %v, want errCompactionEnvelopeTooSmall", err)
	}
}

func TestMaterializeCompactionEnvelopeUsesSafeInputUpperBound(t *testing.T) {
	t.Parallel()

	highEntropy, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("!@#$%^&*", 500), time.UnixMilli(1000)),
	})
	cjk, _ := itemsFromRows([]sqlcRow{
		envelopeRow(t, "user", strings.Repeat("界", 1400), time.UnixMilli(1000)),
	})
	toolResult := mkRow(t, "tool", jsonStr(strings.Repeat("z", 10_000)), 0)
	toolPayload, _ := itemsFromRows([]sqlcRow{toolResult})

	for _, test := range []struct {
		name       string
		budget     int
		candidates []CompactionCandidate
	}{
		{name: "high entropy ascii", budget: 5_000, candidates: highEntropy},
		{name: "cjk", budget: 5_000, candidates: cjk},
		{name: "tool payload", budget: 2_500, candidates: toolPayload},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			envelope, err := materializeCompactionEnvelope(compactionEnvelopeRequest{
				ContextTokenBudget: test.budget,
				Candidates:         test.candidates,
			})
			if !errors.Is(err, errCompactionEnvelopeTooSmall) {
				t.Fatalf("materializeCompactionEnvelope() = upper:%d error:%v, want safe upper-bound rejection", envelope.InputTokenUpperBound, err)
			}
		})
	}
}

func TestMaterializeCompactionEnvelopeReplayMeterIgnoresHistoricalUsage(t *testing.T) {
	t.Parallel()

	rowA := envelopeRow(t, "assistant", strings.Repeat("same replay payload ", 100), time.UnixMilli(1000))
	rowB := rowA
	rowA.Usage = []byte(`{"outputTokens":1}`)
	rowB.Usage = []byte(`{"outputTokens":999999}`)
	itemsA, _ := itemsFromRows([]sqlcRow{rowA})
	itemsB, _ := itemsFromRows([]sqlcRow{rowB})

	first, err := materializeCompactionEnvelope(compactionEnvelopeRequest{ContextTokenBudget: 10_000, Candidates: itemsA})
	if err != nil {
		t.Fatalf("first envelope: %v", err)
	}
	second, err := materializeCompactionEnvelope(compactionEnvelopeRequest{ContextTokenBudget: 10_000, Candidates: itemsB})
	if err != nil {
		t.Fatalf("second envelope: %v", err)
	}
	if first.RawReplayTokens != second.RawReplayTokens || first.RawReplayTokens <= 0 {
		t.Fatalf("raw replay tokens = %d/%d, want equal positive canonical costs", first.RawReplayTokens, second.RawReplayTokens)
	}
}

func envelopePromptText(envelope compactionEnvelope) string {
	var text strings.Builder
	text.WriteString(envelope.System)
	for _, message := range envelope.Messages {
		for _, part := range message.Content {
			if value, ok := part.(sdk.TextPart); ok {
				text.WriteString(value.Text)
			}
		}
	}
	return text.String()
}
