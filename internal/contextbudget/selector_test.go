package contextbudget

import (
	"reflect"
	"testing"
)

func TestAllocateDropsLowerTierBeforeProtectedSources(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(9),
		Items: []Item{
			{ID: "old", Tokens: 4, DropTier: 0, CompactableTokens: 4},
			{ID: "summary", Tokens: 6, Retention: RetentionRequired},
			{ID: "new", Tokens: 3, DropTier: 1, CompactableTokens: 3},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"summary", "new"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want %#v", got, want)
	}
	if got, want := decisionIDs(result.Dropped), []string{"old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dropped = %#v, want %#v", got, want)
	}
	if result.Dropped[0].Reason != DropBudget {
		t.Fatalf("drop reason = %q, want %q", result.Dropped[0].Reason, DropBudget)
	}
	if result.SourceTokens != 13 || result.SelectedTokens != 9 || result.CompactableTokens != 7 {
		t.Fatalf("token accounting = source:%d selected:%d compactable:%d", result.SourceTokens, result.SelectedTokens, result.CompactableTokens)
	}
	if !result.SourcesFit || !result.BudgetTrimmed || !result.Changed || result.SourceOverflowTokens != 0 {
		t.Fatalf("result flags = fits:%v budget_trimmed:%v changed:%v overflow:%d", result.SourcesFit, result.BudgetTrimmed, result.Changed, result.SourceOverflowTokens)
	}
}

func TestAllocateKeepsGroupsAtomic(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(5),
		Items: []Item{
			{ID: "tool-call", Group: "tool:1", Tokens: 3, DropTier: 2},
			{ID: "filler", Tokens: 3, DropTier: 1},
			{ID: "tool-result", Group: "tool:1", Tokens: 2, DropTier: 2},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"tool-call", "tool-result"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want atomic tool group %#v", got, want)
	}
	if got, want := decisionIDs(result.Dropped), []string{"filler"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dropped = %#v, want %#v", got, want)
	}
}

func TestAllocateKeepsOccurrenceScopedGroupsIndependent(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(5),
		Items: []Item{
			{ID: "call_0.call.first", Group: "exchange:0", Tokens: 2, DropTier: 0},
			{ID: "call_0.result.first", Group: "exchange:0", Tokens: 2, DropTier: 0},
			{ID: "call_0.call.second", Group: "exchange:1", Tokens: 2, DropTier: 1},
			{ID: "call_0.result.second", Group: "exchange:1", Tokens: 2, DropTier: 1},
			{ID: "call_0.orphan", Tokens: 1, Retention: RetentionDrop, PolicyReason: "tool:orphan_result"},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"call_0.call.second", "call_0.result.second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want only the newer occurrence %#v", got, want)
	}
	if got, want := decisionIDs(result.Dropped), []string{"call_0.call.first", "call_0.result.first", "call_0.orphan"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dropped = %#v, want %#v", got, want)
	}
	if result.Dropped[2].Reason != "tool:orphan_result" {
		t.Fatalf("orphan reason = %q, want tool:orphan_result", result.Dropped[2].Reason)
	}
}

func TestAllocateAgesAtomicGroupByItsLastMember(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(4),
		Items: []Item{
			{ID: "call", Group: "exchange:0", Tokens: 2, DropTier: 1},
			{ID: "filler", Tokens: 4, DropTier: 1},
			{ID: "result", Group: "exchange:0", Tokens: 2, DropTier: 1},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"call", "result"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want recently completed group %#v", got, want)
	}
}

func TestAllocateReportsRequiredOverflowWithoutDiscardingSources(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(10),
		Items: []Item{
			{ID: "summary-a", Tokens: 7, Retention: RetentionRequired},
			{ID: "summary-b", Tokens: 5, Retention: RetentionRequired},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"summary-a", "summary-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want %#v", got, want)
	}
	if len(result.Dropped) != 0 || result.Changed || result.BudgetTrimmed {
		t.Fatalf("required sources were treated as dropped: %#v", result)
	}
	if result.SourcesFit || result.SourceOverflowTokens != 2 || result.SelectedTokens != 12 {
		t.Fatalf("overflow = fits:%v tokens:%d selected:%d", result.SourcesFit, result.SourceOverflowTokens, result.SelectedTokens)
	}
}

func TestAllocateAppliesPolicyDropsWithoutGlobalLimit(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		Items: []Item{
			{ID: "keep", Tokens: 3},
			{ID: "drop", Tokens: 5, Retention: RetentionDrop, CompactableTokens: 5},
			{ID: "required-call", Group: "required-tool", Tokens: 2, Retention: RetentionRequired},
			{ID: "required-result", Group: "required-tool", Tokens: 4, Retention: RetentionDrop},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"keep", "required-call", "required-result"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want %#v", got, want)
	}
	if got, want := decisionIDs(result.Dropped), []string{"drop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dropped = %#v, want %#v", got, want)
	}
	if result.Dropped[0].Reason != DropPolicy || !result.SourcesFit || !result.Changed || result.BudgetTrimmed {
		t.Fatalf("policy result = %#v", result)
	}
	if result.CompactableTokens != 5 {
		t.Fatalf("compactable tokens = %d, want policy-dropped raw pressure 5", result.CompactableTokens)
	}
}

func TestAllocateDoesNotInferBudgetTrimFromPolicyReason(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{Items: []Item{{
		ID:           "policy-drop",
		Tokens:       3,
		Retention:    RetentionDrop,
		PolicyReason: DropBudget,
	}}})

	if !result.Changed || result.BudgetTrimmed {
		t.Fatalf("drop provenance = changed:%v budget_trimmed:%v", result.Changed, result.BudgetTrimmed)
	}
	if len(result.Dropped) != 1 || result.Dropped[0].Reason != DropBudget {
		t.Fatalf("display reason = %#v, want custom reason %q", result.Dropped, DropBudget)
	}
}

func TestAllocateDropsOldestSourceWhenTiersTie(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(4),
		Items: []Item{
			{ID: "older", Tokens: 4, DropTier: 1},
			{ID: "newer", Tokens: 4, DropTier: 1},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"newer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want %#v", got, want)
	}
}

func TestAllocateKeepsMonotonicSupersetsAsBudgetGrows(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "old-passive", Tokens: 3, DropTier: 0},
		{ID: "old-directed-call", Group: "tool:0", Tokens: 2, DropTier: 2},
		{ID: "new-passive", Tokens: 2, DropTier: 0},
		{ID: "old-directed-result", Group: "tool:0", Tokens: 2, DropTier: 2},
		{ID: "new-neutral", Tokens: 5, DropTier: 1},
		{ID: "current", Tokens: 6, Retention: RetentionRequired},
	}
	var previous map[string]struct{}
	for limit := 0; limit <= 20; limit++ {
		result := Allocate(Request{SourceLimit: tokenLimit(limit), Items: items})
		kept := make(map[string]struct{}, len(result.Kept))
		for _, decision := range result.Kept {
			kept[decision.ID] = struct{}{}
		}
		for id := range previous {
			if _, ok := kept[id]; !ok {
				t.Fatalf("limit %d dropped %q retained at the smaller budget", limit, id)
			}
		}
		previous = kept
	}
}

func TestAllocateExactFitAndUnlimitedDoNotMutateInput(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "first", Tokens: 2, DropTier: 0},
		{ID: "second", Tokens: 3, DropTier: 1},
	}
	original := append([]Item(nil), items...)
	exact := Allocate(Request{SourceLimit: tokenLimit(5), Items: items})
	unlimited := Allocate(Request{Items: items})

	if exact.Changed || exact.BudgetTrimmed || !exact.SourcesFit || exact.SelectedTokens != 5 {
		t.Fatalf("exact-fit result = %#v", exact)
	}
	if unlimited.Changed || unlimited.BudgetTrimmed || !unlimited.SourcesFit || unlimited.SelectedTokens != 5 {
		t.Fatalf("unlimited result = %#v", unlimited)
	}
	if !reflect.DeepEqual(items, original) {
		t.Fatalf("Allocate mutated input: got %#v want %#v", items, original)
	}
}

func TestAllocateDistinguishesExplicitZeroLimitFromUnlimited(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "candidate", Tokens: 3},
		{ID: "required", Tokens: 2, Retention: RetentionRequired},
	}
	unlimited := Allocate(Request{Items: items})

	for _, limit := range []int{0, -4} {
		explicitZero := Allocate(Request{SourceLimit: tokenLimit(limit), Items: items})
		if got, want := decisionIDs(explicitZero.Kept), []string{"required"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("limit %d kept = %#v, want %#v", limit, got, want)
		}
		if got, want := decisionIDs(explicitZero.Dropped), []string{"candidate"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("limit %d dropped = %#v, want %#v", limit, got, want)
		}
		if explicitZero.SourcesFit || explicitZero.SourceOverflowTokens != 2 || !explicitZero.BudgetTrimmed {
			t.Fatalf("limit %d result = %#v, want required overflow 2 after budget trim", limit, explicitZero)
		}
	}
	if unlimited.Changed || !unlimited.SourcesFit || unlimited.SelectedTokens != 5 {
		t.Fatalf("unlimited result = %#v", unlimited)
	}
}

func TestAllocateReportsRequiredOnlyOverflowAtExplicitZero(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(0),
		Items:       []Item{{ID: "required", Tokens: 2, Retention: RetentionRequired}},
	})

	if len(result.Kept) != 1 || len(result.Dropped) != 0 || result.SourcesFit || result.SourceOverflowTokens != 2 {
		t.Fatalf("required zero-limit overflow = %#v", result)
	}
	if result.Changed || result.BudgetTrimmed {
		t.Fatalf("required overflow was reported as a source drop: %#v", result)
	}
}

func TestAllocateDoesNotDropZeroCostCandidatesToAddressOverflow(t *testing.T) {
	t.Parallel()

	result := Allocate(Request{
		SourceLimit: tokenLimit(0),
		Items: []Item{
			{ID: "provider-invisible", Tokens: 0},
			{ID: "required", Tokens: 2, Retention: RetentionRequired},
		},
	})

	if got, want := decisionIDs(result.Kept), []string{"provider-invisible", "required"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want zero-cost source retained %#v", got, want)
	}
	if len(result.Dropped) != 0 || result.Changed || result.BudgetTrimmed {
		t.Fatalf("zero-cost source was falsely reported as a budget trim: %#v", result)
	}
	if result.SourcesFit || result.SourceOverflowTokens != 2 {
		t.Fatalf("required overflow = %#v", result)
	}
}

func TestAllocateMetersRawCompactablePressureIndependently(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "rewritten", Tokens: 2, CompactableTokens: 9},
		{ID: "policy-drop", Tokens: 4, CompactableTokens: 7, Retention: RetentionDrop},
		{ID: "artifact", Tokens: 6, Retention: RetentionRequired},
		{ID: "invalid-pressure", Tokens: 1, CompactableTokens: -3},
	}
	for limit := 0; limit <= 20; limit++ {
		result := Allocate(Request{SourceLimit: tokenLimit(limit), Items: items})
		if result.CompactableTokens != 16 {
			t.Fatalf("limit %d compactable tokens = %d, want invariant raw pressure 16", limit, result.CompactableTokens)
		}
	}
}

func decisionIDs(decisions []Decision) []string {
	ids := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		ids = append(ids, decision.ID)
	}
	return ids
}

func tokenLimit(tokens int) *int {
	return &tokens
}
