package capabilities

import (
	"context"
	"reflect"
	"testing"

	"github.com/memohai/memoh/internal/models"
)

func ptrBool(b bool) *bool { return &b }
func ptrInt(i int) *int    { return &i }

func TestDerive_AdaptiveOpus(t *testing.T) {
	// claude-opus-4-8: adaptive + xhigh + max.
	caps := derive(litellmEntry{
		SupportsReasoning:            ptrBool(true),
		SupportsAdaptiveThinking:     ptrBool(true),
		SupportsXHighReasoningEffort: ptrBool(true),
		SupportsMaxReasoningEffort:   ptrBool(true),
		SupportsVision:               ptrBool(true),
		SupportsFunctionCalling:      ptrBool(true),
		MaxInputTokens:               ptrInt(1000000),
	})
	if caps.ThinkingMode != models.ThinkingModeAdaptive {
		t.Fatalf("thinking mode = %q, want adaptive", caps.ThinkingMode)
	}
	want := []string{"low", "medium", "high", "xhigh", "max"}
	if !reflect.DeepEqual(caps.EffortLevels, want) {
		t.Fatalf("effort levels = %v, want %v", caps.EffortLevels, want)
	}
}

func TestDerive_ToggleGPT5Minimal(t *testing.T) {
	// gpt-5: reasoning + minimal, none/xhigh explicitly false, not adaptive.
	caps := derive(litellmEntry{
		SupportsReasoning:              ptrBool(true),
		SupportsNoneReasoningEffort:    ptrBool(false),
		SupportsMinimalReasoningEffort: ptrBool(true),
		SupportsXHighReasoningEffort:   ptrBool(false),
	})
	if caps.ThinkingMode != models.ThinkingModeToggle {
		t.Fatalf("thinking mode = %q, want toggle", caps.ThinkingMode)
	}
	want := []string{"minimal", "low", "medium", "high"}
	if !reflect.DeepEqual(caps.EffortLevels, want) {
		t.Fatalf("effort levels = %v, want %v", caps.EffortLevels, want)
	}
}

func TestDerive_PlainReasoning(t *testing.T) {
	// o3: reasoning only → toggle with base tiers.
	caps := derive(litellmEntry{SupportsReasoning: ptrBool(true)})
	if caps.ThinkingMode != models.ThinkingModeToggle {
		t.Fatalf("thinking mode = %q", caps.ThinkingMode)
	}
	want := []string{"low", "medium", "high"}
	if !reflect.DeepEqual(caps.EffortLevels, want) {
		t.Fatalf("effort levels = %v, want %v", caps.EffortLevels, want)
	}
}

func TestDerive_ExplicitNoReasoning(t *testing.T) {
	caps := derive(litellmEntry{SupportsReasoning: ptrBool(false)})
	if caps.ThinkingMode != models.ThinkingModeNone {
		t.Fatalf("thinking mode = %q, want none", caps.ThinkingMode)
	}
	if caps.EffortLevels != nil {
		t.Fatalf("effort levels should be nil, got %v", caps.EffortLevels)
	}
}

func TestDerive_SilentRegistryIsUnknown(t *testing.T) {
	caps := derive(litellmEntry{SupportsVision: ptrBool(true)})
	if caps.ThinkingMode != "" {
		t.Fatalf("thinking mode should be unknown (empty), got %q", caps.ThinkingMode)
	}
	if caps.Vision == nil || !*caps.Vision {
		t.Fatalf("vision should be filled")
	}
}

func TestRegistry_LookupViaInjectedFetch(t *testing.T) {
	reg := NewRegistry(withFetchFn(func(context.Context) (map[string]litellmEntry, error) {
		return map[string]litellmEntry{
			"claude-opus-4-8": {
				SupportsReasoning:            ptrBool(true),
				SupportsAdaptiveThinking:     ptrBool(true),
				SupportsXHighReasoningEffort: ptrBool(true),
				SupportsMaxReasoningEffort:   ptrBool(true),
			},
		}, nil
	}), withoutBundledSnapshot())

	caps, ok := reg.Lookup(context.Background(), "openrouter/anthropic/claude-opus-4.8")
	if !ok {
		t.Fatalf("expected lookup hit")
	}
	if caps.ThinkingMode != models.ThinkingModeAdaptive {
		t.Fatalf("thinking mode = %q", caps.ThinkingMode)
	}
}

func TestRegistry_FastVariantBorrowsBaseShapeNotContext(t *testing.T) {
	reg := NewRegistry(withFetchFn(func(context.Context) (map[string]litellmEntry, error) {
		return map[string]litellmEntry{
			// Base model only; the "-fast" variant is not catalogued.
			"claude-opus-4-8": {
				SupportsReasoning:            ptrBool(true),
				SupportsAdaptiveThinking:     ptrBool(true),
				SupportsXHighReasoningEffort: ptrBool(true),
				MaxInputTokens:               ptrInt(1000000),
			},
			// A sibling non-latency variant whose context window differs.
			"gpt-5-mini": {
				SupportsReasoning: ptrBool(true),
				MaxInputTokens:    ptrInt(272000),
			},
		}, nil
	}), withoutBundledSnapshot())

	// fast variant: borrows the base reasoning shape but NOT the 1M context.
	caps, ok := reg.Lookup(context.Background(), "anthropic/claude-opus-4.8-fast")
	if !ok {
		t.Fatalf("expected fast variant to fall back to base shape")
	}
	if caps.ThinkingMode != models.ThinkingModeAdaptive {
		t.Fatalf("thinking mode = %q, want adaptive (borrowed from base)", caps.ThinkingMode)
	}
	if caps.ContextWindow != nil {
		t.Fatalf("context window must NOT be inherited from base, got %v", *caps.ContextWindow)
	}

	// Non-latency variant with no key (e.g. "-pro") must NOT borrow the base.
	if _, ok := reg.Lookup(context.Background(), "anthropic/claude-opus-4.8-pro"); ok {
		t.Fatalf("pro variant must not fall back to base")
	}
}

func TestRegistry_FailOpenReturnsUnknown(t *testing.T) {
	reg := NewRegistry(withFetchFn(func(context.Context) (map[string]litellmEntry, error) {
		return nil, context.DeadlineExceeded
	}), withoutBundledSnapshot())
	if _, ok := reg.Lookup(context.Background(), "claude-opus-4-8"); ok {
		t.Fatalf("lookup should miss when registry never loaded")
	}
}

func TestRegistry_BundledSnapshotProvidesBaseline(t *testing.T) {
	reg := NewRegistry()
	caps, ok := reg.Lookup(context.Background(), "openrouter/anthropic/claude-opus-4.8")
	if !ok {
		t.Fatalf("expected bundled snapshot lookup hit")
	}
	if caps.ThinkingMode != models.ThinkingModeAdaptive {
		t.Fatalf("thinking mode = %q, want adaptive", caps.ThinkingMode)
	}
}

func TestRegistry_BundledSnapshotSurvivesRefreshFailure(t *testing.T) {
	reg := NewRegistry(
		WithTTL(0),
		withFetchFn(func(context.Context) (map[string]litellmEntry, error) {
			return nil, context.DeadlineExceeded
		}),
	)
	caps, ok := reg.Lookup(context.Background(), "openrouter/anthropic/claude-opus-4.8")
	if !ok {
		t.Fatalf("expected bundled snapshot lookup hit after failed refresh")
	}
	if caps.ThinkingMode != models.ThinkingModeAdaptive {
		t.Fatalf("thinking mode = %q, want adaptive", caps.ThinkingMode)
	}
}
