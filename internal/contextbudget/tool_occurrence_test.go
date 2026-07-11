package contextbudget

import (
	"reflect"
	"testing"
)

func callPart(index int, id string) ToolPart {
	return ToolPart{PartIndex: index, Kind: ToolPartCall, CallID: id}
}

func resultPart(index int, id string) ToolPart {
	return ToolPart{PartIndex: index, Kind: ToolPartResult, CallID: id}
}

func TestAnalyzeToolOccurrencesGroupsNonAdjacentBatchedResults(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(0, "call-a"), callPart(1, "call-b")}},
		{},
		{Parts: []ToolPart{resultPart(0, "call-a")}},
		{Parts: []ToolPart{callPart(0, "call-c")}},
		{Parts: []ToolPart{resultPart(0, "call-b"), resultPart(1, "call-c")}},
	})

	group := got.Bindings[0].Group
	if group == "" {
		t.Fatal("tool occurrence group is empty")
	}
	for _, index := range []int{0, 2, 3, 4} {
		if got.Bindings[index].Group != group {
			t.Fatalf("binding[%d] = %#v, want group %q", index, got.Bindings[index], group)
		}
	}
	if got.Bindings[1] != (ToolCarrierBinding{}) || len(got.PartIssues) != 0 || len(got.DanglingCalls) != 0 {
		t.Fatalf("unexpected analysis metadata: %#v", got)
	}
	wantMatches := []ToolPartMatch{
		{CallCarrierIndex: 0, CallPartIndex: 0, ResultCarrierIndex: 2, ResultPartIndex: 0, CallID: "call-a"},
		{CallCarrierIndex: 0, CallPartIndex: 1, ResultCarrierIndex: 4, ResultPartIndex: 0, CallID: "call-b"},
		{CallCarrierIndex: 3, CallPartIndex: 0, ResultCarrierIndex: 4, ResultPartIndex: 1, CallID: "call-c"},
	}
	if !reflect.DeepEqual(got.Matches, wantMatches) {
		t.Fatalf("matches = %#v, want %#v", got.Matches, wantMatches)
	}
}

func TestAnalyzeToolOccurrencesIsolatesRepeatedCallIDs(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(0, "call-0")}},
		{Parts: []ToolPart{resultPart(0, "call-0")}},
		{},
		{Parts: []ToolPart{callPart(0, "call-0")}},
		{Parts: []ToolPart{resultPart(0, "call-0")}},
	})

	if got.Bindings[0].Group == "" || got.Bindings[0].Group != got.Bindings[1].Group {
		t.Fatalf("first occurrence = %#v, want one group", got.Bindings[:2])
	}
	if got.Bindings[3].Group == "" || got.Bindings[3].Group != got.Bindings[4].Group {
		t.Fatalf("second occurrence = %#v, want one group", got.Bindings[3:])
	}
	if got.Bindings[0].Group == got.Bindings[3].Group {
		t.Fatalf("repeated raw ID collapsed occurrences: %#v", got.Bindings)
	}
}

func TestAnalyzeToolOccurrencesClassifiesResultAfterReusedDanglingCallAsOrphan(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(0, "call-0")}},
		{Parts: []ToolPart{resultPart(0, "call-0")}},
		{BoundaryBefore: true, Parts: []ToolPart{callPart(0, "call-0")}},
		{BoundaryBefore: true},
		{Parts: []ToolPart{resultPart(0, "call-0")}},
	})

	want := []ToolPartIssue{{CarrierIndex: 4, PartIndex: 0, Reason: DropToolOrphanResult}}
	if !reflect.DeepEqual(got.PartIssues, want) {
		t.Fatalf("part issues = %#v, want %#v", got.PartIssues, want)
	}
}

func TestAnalyzeToolOccurrencesReportsOverlappedCallAsDangling(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(2, "call-0")}},
		{Parts: []ToolPart{callPart(3, "call-0")}},
		{Parts: []ToolPart{resultPart(4, "call-0")}},
	})

	wantDangling := []DanglingToolCall{{CarrierIndex: 0, PartIndex: 2, CallID: "call-0", CloseBefore: 1}}
	if !reflect.DeepEqual(got.DanglingCalls, wantDangling) {
		t.Fatalf("dangling calls = %#v, want %#v", got.DanglingCalls, wantDangling)
	}
	if got.Bindings[0].Group == got.Bindings[1].Group || got.Bindings[1].Group != got.Bindings[2].Group {
		t.Fatalf("latest occurrence was not isolated: %#v", got.Bindings)
	}
}

func TestAnalyzeToolOccurrencesRespectsCarrierBoundaries(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(0, "call-a")}},
		{BoundaryBefore: true},
		{Parts: []ToolPart{resultPart(0, "call-a")}},
	})

	wantDangling := []DanglingToolCall{{CarrierIndex: 0, PartIndex: 0, CallID: "call-a", CloseBefore: 1}}
	if !reflect.DeepEqual(got.DanglingCalls, wantDangling) {
		t.Fatalf("dangling calls = %#v, want %#v", got.DanglingCalls, wantDangling)
	}
	wantIssues := []ToolPartIssue{{CarrierIndex: 2, PartIndex: 0, Reason: DropToolOrphanResult}}
	if !reflect.DeepEqual(got.PartIssues, wantIssues) {
		t.Fatalf("part issues = %#v, want %#v", got.PartIssues, wantIssues)
	}
	if got.Bindings[0].Group == "" || got.Bindings[2].Group != "" {
		t.Fatalf("boundary linked an orphan result: %#v", got.Bindings)
	}
}

func TestAnalyzeToolOccurrencesClassifiesOrphanAndDuplicateResultParts(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{resultPart(1, "never-opened")}},
		{Parts: []ToolPart{callPart(2, "call-a")}},
		{Parts: []ToolPart{resultPart(3, "call-a"), resultPart(4, "call-a")}},
	})

	want := []ToolPartIssue{
		{CarrierIndex: 0, PartIndex: 1, Reason: DropToolOrphanResult},
		{CarrierIndex: 2, PartIndex: 4, Reason: DropToolDuplicateResult},
	}
	if !reflect.DeepEqual(got.PartIssues, want) {
		t.Fatalf("part issues = %#v, want %#v", got.PartIssues, want)
	}
	if got.Bindings[0].Group != "" || got.Bindings[1].Group == "" || got.Bindings[1].Group != got.Bindings[2].Group {
		t.Fatalf("invalid part changed valid carrier grouping: %#v", got.Bindings)
	}
}

func TestAnalyzeToolOccurrencesReportsPartiallyClosedBatch(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(0, "call-a"), callPart(1, "call-b")}},
		{Parts: []ToolPart{resultPart(0, "call-a")}},
	})

	if got.Bindings[0].Group == "" || got.Bindings[0].Group != got.Bindings[1].Group {
		t.Fatalf("partial batch was not grouped: %#v", got.Bindings)
	}
	want := []DanglingToolCall{{CarrierIndex: 0, PartIndex: 1, CallID: "call-b", CloseBefore: 2}}
	if !reflect.DeepEqual(got.DanglingCalls, want) {
		t.Fatalf("dangling calls = %#v, want %#v", got.DanglingCalls, want)
	}
}

func TestAnalyzeToolOccurrencesRejectsOnlyDuplicateCallPart(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(1, "call-a"), callPart(2, "call-a")}},
		{Parts: []ToolPart{resultPart(0, "call-a")}},
	})

	wantIssues := []ToolPartIssue{{CarrierIndex: 0, PartIndex: 2, Reason: DropToolDuplicateCall}}
	if !reflect.DeepEqual(got.PartIssues, wantIssues) {
		t.Fatalf("part issues = %#v, want %#v", got.PartIssues, wantIssues)
	}
	if len(got.DanglingCalls) != 0 || got.Bindings[0].Group != got.Bindings[1].Group {
		t.Fatalf("valid call/result did not survive duplicate rewrite: %#v", got)
	}
}

func TestAnalyzeToolOccurrencesRejectsBlankIDsWithoutGrouping(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{{Parts: []ToolPart{
		callPart(1, " "),
		resultPart(2, ""),
	}}})

	want := []ToolPartIssue{
		{CarrierIndex: 0, PartIndex: 1, Reason: DropToolOrphanCall},
		{CarrierIndex: 0, PartIndex: 2, Reason: DropToolOrphanResult},
	}
	if !reflect.DeepEqual(got.PartIssues, want) {
		t.Fatalf("part issues = %#v, want %#v", got.PartIssues, want)
	}
	if got.Bindings[0].Group != "" || len(got.DanglingCalls) != 0 {
		t.Fatalf("invalid parts formed an occurrence: %#v", got)
	}
}

func TestAnalyzeToolOccurrencesDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	carriers := []ToolCarrier{{Parts: []ToolPart{callPart(0, " call-a ")}}, {Parts: []ToolPart{resultPart(0, " call-a ")}}}
	original := []ToolCarrier{{Parts: append([]ToolPart(nil), carriers[0].Parts...)}, {Parts: append([]ToolPart(nil), carriers[1].Parts...)}}
	AnalyzeToolOccurrences(carriers)
	if !reflect.DeepEqual(carriers, original) {
		t.Fatalf("input mutated: got %#v want %#v", carriers, original)
	}
}

func TestAnalyzeToolOccurrencesOrdersMultipleDanglingCallsDeterministically(t *testing.T) {
	t.Parallel()

	got := AnalyzeToolOccurrences([]ToolCarrier{
		{Parts: []ToolPart{callPart(2, "call-z"), callPart(1, "call-b0"), callPart(1, "call-a")}},
		{Parts: []ToolPart{callPart(0, "call-b")}},
		{BoundaryBefore: true},
		{Parts: []ToolPart{callPart(0, "call-x"), callPart(1, "call-y")}},
		{Parts: []ToolPart{resultPart(0, "call-x"), resultPart(1, "call-y")}},
		{Parts: []ToolPart{callPart(0, "call-last")}},
	})

	want := []DanglingToolCall{
		{CarrierIndex: 0, PartIndex: 1, CallID: "call-a", CloseBefore: 2},
		{CarrierIndex: 0, PartIndex: 1, CallID: "call-b0", CloseBefore: 2},
		{CarrierIndex: 0, PartIndex: 2, CallID: "call-z", CloseBefore: 2},
		{CarrierIndex: 1, PartIndex: 0, CallID: "call-b", CloseBefore: 2},
		{CarrierIndex: 5, PartIndex: 0, CallID: "call-last", CloseBefore: 6},
	}
	if !reflect.DeepEqual(got.DanglingCalls, want) {
		t.Fatalf("dangling calls = %#v, want %#v", got.DanglingCalls, want)
	}
	if got.Bindings[3].Group == "" || got.Bindings[3].Group != got.Bindings[4].Group {
		t.Fatalf("batched result did not preserve one group: %#v", got.Bindings[3:5])
	}
}
