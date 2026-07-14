package contextassembly

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/messageconv"
)

type opaquePart struct{}

func (opaquePart) PartType() sdk.MessagePartType { return "opaque" }

func TestAssembleRepairsCanonicalizesAndMetersFinalEntries(t *testing.T) {
	t.Parallel()

	usage := &sdk.Usage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}
	sources := []Source{
		{
			ID: "assistant-call",
			Message: sdk.Message{
				Role: sdk.MessageRoleAssistant,
				Content: []sdk.MessagePart{
					sdk.ReasoningPart{Text: "private"},
					sdk.TextPart{Text: "visible"},
					sdk.ToolCallPart{ToolCallID: " call-1 ", ToolName: " lookup ", Input: map[string]any{"q": "memoh"}},
					sdk.ToolCallPart{ToolCallID: "call-2", ToolName: "search", Input: map[string]any{"q": "context"}},
				},
				Usage: usage,
			},
			CompactableTokens: 11,
		},
		{
			ID: "reasoning-boundary",
			Message: sdk.Message{
				Role:    sdk.MessageRoleAssistant,
				Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private boundary"}},
			},
			CompactableTokens: 2,
		},
		{ID: "boundary", Message: sdk.UserMessage("next"), CompactableTokens: 3},
		{
			ID: "invalid-only",
			Message: sdk.ToolMessage(sdk.ToolResultPart{
				ToolCallID: "orphan",
				ToolName:   "lookup",
				Result:     "stale",
			}),
			CompactableTokens: 4,
		},
	}
	original := snapshotSources(t, sources)

	result, err := Assemble(Request{Sources: sources, SyntheticToolResult: "interrupted"})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(result.Entries) != 4 {
		t.Fatalf("entries = %#v, want assistant, two synthetic results, user", result.Entries)
	}
	if result.Entries[0].SourceIndex != 0 || result.Entries[0].Synthetic || result.Entries[0].Message.Usage != usage {
		t.Fatalf("assistant provenance = %#v", result.Entries[0])
	}
	if len(result.Entries[0].Message.Content) != 3 {
		t.Fatalf("assistant content = %#v, want reasoning removed", result.Entries[0].Message.Content)
	}
	call, ok := result.Entries[0].Message.Content[1].(sdk.ToolCallPart)
	if !ok || call.ToolCallID != "call-1" || call.ToolName != "lookup" {
		t.Fatalf("normalized call = %#v", result.Entries[0].Message.Content[1])
	}
	for _, entry := range result.Entries[1:3] {
		if entry.SourceIndex != -1 || !entry.Synthetic {
			t.Fatalf("synthetic provenance = %#v, want no metadata-bearing source", entry)
		}
	}
	if result.Allocation.CompactableTokens != 20 {
		t.Fatalf("compactable tokens = %d, want raw pressure 20", result.Allocation.CompactableTokens)
	}
	if got, want := decisionIDs(result.Allocation.Dropped), []string{"reasoning-boundary", "invalid-only"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("policy-repaired sources = %#v, want %#v", got, want)
	}
	if result.EmittedTokens != estimateEntries(result.Entries) || result.EmittedTokens != result.Allocation.SelectedTokens {
		t.Fatalf("token accounting = emitted:%d selected:%d recomputed:%d", result.EmittedTokens, result.Allocation.SelectedTokens, estimateEntries(result.Entries))
	}
	if analysis := messageconv.AnalyzeSDKToolOccurrences(entryMessages(result.Entries)); len(analysis.PartIssues) != 0 || len(analysis.DanglingCalls) != 0 {
		t.Fatalf("final tool analysis = %#v", analysis)
	}
	if got := snapshotSources(t, sources); !reflect.DeepEqual(got, original) {
		t.Fatalf("Assemble mutated input:\ngot  %s\nwant %s", got, original)
	}
}

func TestAssembleReservesNoticeWithSecondAllocation(t *testing.T) {
	t.Parallel()

	limit := 8
	result, err := Assemble(Request{
		EnvelopeLimit: &limit,
		Notice:        "12345678",
		Sources: []Source{
			{ID: "oldest", Message: sdk.UserMessage("1111111111111111")},
			{ID: "middle", Message: sdk.AssistantMessage("2222222222222222")},
			{ID: "newest", Message: sdk.UserMessage("3333333333333333")},
		},
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if got, want := decisionIDs(result.Allocation.Kept), []string{"newest"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want %#v", got, want)
	}
	if got, want := decisionIDs(result.Allocation.Dropped), []string{"oldest", "middle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dropped = %#v, want notice reserve to drop two sources %#v", got, want)
	}
	if len(result.Entries) != 2 || result.Entries[0].SourceIndex != -1 || result.Entries[0].Message.Role != sdk.MessageRoleSystem {
		t.Fatalf("entries = %#v, want notice then newest", result.Entries)
	}
	if !result.Allocation.BudgetTrimmed || result.EmittedTokens != 6 || result.EmittedTokens > limit {
		t.Fatalf("notice allocation = %#v, emitted=%d limit=%d", result.Allocation, result.EmittedTokens, limit)
	}
}

func TestAssemblePolicyDropDoesNotEmitNotice(t *testing.T) {
	t.Parallel()

	limit := 10
	result, err := Assemble(Request{
		EnvelopeLimit: &limit,
		Notice:        "trimmed",
		Sources: []Source{
			{ID: "policy", Message: sdk.UserMessage("drop"), Retention: contextbudget.RetentionDrop, CompactableTokens: 7},
			{ID: "keep", Message: sdk.AssistantMessage("keep"), CompactableTokens: 5},
		},
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].SourceIndex != 1 {
		t.Fatalf("entries = %#v, want only kept source and no notice", result.Entries)
	}
	if result.Allocation.BudgetTrimmed || !result.Allocation.Changed || result.Allocation.CompactableTokens != 12 {
		t.Fatalf("policy allocation = %#v", result.Allocation)
	}
}

func TestAssembleKeepsRepeatedToolOccurrencesAtomicAndIndependent(t *testing.T) {
	t.Parallel()

	call := func(name string) sdk.Message {
		return sdk.Message{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "reused-id",
				ToolName:   name,
				Input:      map[string]any{},
			}},
		}
	}
	result := func(name string) sdk.Message {
		return sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "reused-id", ToolName: name, Result: "ok"})
	}
	secondCall := call("second")
	secondResult := result("second")
	limit := messageconv.EstimateSDKMessageTokens(secondCall) + messageconv.EstimateSDKMessageTokens(secondResult)

	assembled, err := Assemble(Request{
		EnvelopeLimit: &limit,
		Sources: []Source{
			{ID: "first-call", Message: call("first"), CompactableTokens: 1},
			{ID: "first-result", Message: result("first"), CompactableTokens: 1},
			{ID: "second-call", Message: secondCall, CompactableTokens: 1},
			{ID: "second-result", Message: secondResult, CompactableTokens: 1},
		},
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if got, want := decisionIDs(assembled.Allocation.Kept), []string{"second-call", "second-result"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept = %#v, want newer occurrence %#v", got, want)
	}
	if got, want := decisionIDs(assembled.Allocation.Dropped), []string{"first-call", "first-result"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dropped = %#v, want atomic older occurrence %#v", got, want)
	}
	if len(assembled.Entries) != 2 || assembled.Entries[0].SourceIndex != 2 || assembled.Entries[1].SourceIndex != 3 {
		t.Fatalf("entries = %#v, want only second occurrence", assembled.Entries)
	}
	if assembled.Allocation.CompactableTokens != 4 || assembled.EmittedTokens != limit {
		t.Fatalf("accounting = allocation:%#v emitted:%d limit:%d", assembled.Allocation, assembled.EmittedTokens, limit)
	}
}

func TestAssembleReturnsRequiredOverflowWithoutNotice(t *testing.T) {
	t.Parallel()

	limit := 1
	result, err := Assemble(Request{
		EnvelopeLimit: &limit,
		Notice:        "trimmed",
		Sources: []Source{
			{
				ID: "reasoning-only",
				Message: sdk.Message{
					Role:    sdk.MessageRoleAssistant,
					Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private"}},
				},
				CompactableTokens: 7,
			},
			{
				ID:        "artifact",
				Message:   sdk.SystemMessage("12345678"),
				Retention: contextbudget.RetentionRequired,
			},
		},
	})
	var overflow *OverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("error = %v, want *OverflowError", err)
	}
	if overflow.Error() == "" {
		t.Fatal("OverflowError.Error() is empty")
	}
	if overflow.Limit != limit || overflow.EmittedTokens != result.EmittedTokens || !reflect.DeepEqual(overflow.Result, result) {
		t.Fatalf("overflow = %#v, result = %#v", overflow, result)
	}
	if len(result.Entries) != 1 || result.Entries[0].SourceIndex != 1 || result.Entries[0].Message.Role != sdk.MessageRoleSystem {
		t.Fatalf("required overflow entries = %#v, want complete result without notice", result.Entries)
	}
	if result.Allocation.BudgetTrimmed || result.Allocation.SourcesFit || result.Allocation.SourceOverflowTokens != 1 {
		t.Fatalf("required overflow allocation = %#v", result.Allocation)
	}
}

func TestAssembleIgnoresBlankNotice(t *testing.T) {
	t.Parallel()

	limit := 1
	result, err := Assemble(Request{EnvelopeLimit: &limit, Notice: "  "})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(result.Entries) != 0 || result.EmittedTokens != 0 {
		t.Fatalf("invalid notice was emitted: %#v", result)
	}
}

func TestAssembleReturnsUnrenderableRequiredError(t *testing.T) {
	t.Parallel()

	result, err := Assemble(Request{Sources: []Source{
		{
			ID: "required-reasoning",
			Message: sdk.Message{
				Role:    sdk.MessageRoleAssistant,
				Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private"}},
			},
			Retention:         contextbudget.RetentionRequired,
			CompactableTokens: 9,
		},
		{
			ID:                "required-empty",
			Message:           sdk.UserMessage(""),
			Retention:         contextbudget.RetentionRequired,
			CompactableTokens: 4,
		},
	}})
	var unrenderable *UnrenderableRequiredError
	if !errors.As(err, &unrenderable) {
		t.Fatalf("error = %v, want *UnrenderableRequiredError", err)
	}
	if unrenderable.Error() == "" {
		t.Fatal("UnrenderableRequiredError.Error() is empty")
	}
	if got, want := decisionIDs(unrenderable.Sources), []string{"required-reasoning", "required-empty"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrenderable sources = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(unrenderable.Result, result) || len(result.Entries) != 0 || result.Allocation.CompactableTokens != 13 {
		t.Fatalf("unrenderable result = %#v error=%#v", result, unrenderable)
	}
}

func TestAssembleReturnsNoticeOverflowWithCompleteResult(t *testing.T) {
	t.Parallel()

	limit := 1
	result, err := Assemble(Request{
		EnvelopeLimit: &limit,
		Notice:        "12345678",
		Sources: []Source{
			{ID: "old", Message: sdk.UserMessage("1111")},
			{ID: "new", Message: sdk.AssistantMessage("2222")},
		},
	})
	var overflow *OverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("error = %v, want notice *OverflowError", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].SourceIndex != -1 || result.EmittedTokens != 2 {
		t.Fatalf("notice overflow result = %#v", result)
	}
	if !result.Allocation.BudgetTrimmed || !result.Allocation.SourcesFit || overflow.Limit != 1 || overflow.EmittedTokens != 2 {
		t.Fatalf("notice overflow = %#v allocation=%#v", overflow, result.Allocation)
	}
}

func TestHasProviderVisiblePayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		part sdk.MessagePart
		want bool
	}{
		{name: "blank text", part: sdk.TextPart{Text: "  "}},
		{name: "text", part: sdk.TextPart{Text: "visible"}, want: true},
		{name: "empty image", part: sdk.ImagePart{}},
		{name: "image", part: sdk.ImagePart{Image: "data:image/png;base64,a"}, want: true},
		{name: "empty file", part: sdk.FilePart{}},
		{name: "file", part: sdk.FilePart{Data: "ZGF0YQ=="}, want: true},
		{name: "tool call", part: sdk.ToolCallPart{}, want: true},
		{name: "tool result", part: sdk.ToolResultPart{}, want: true},
		{name: "reasoning", part: sdk.ReasoningPart{Text: "private"}},
		{name: "future part", part: opaquePart{}, want: true},
	}
	if hasProviderVisiblePayload(sdk.Message{}) {
		t.Fatal("empty message reported provider-visible payload")
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := hasProviderVisiblePayload(sdk.Message{Content: []sdk.MessagePart{test.part}})
			if got != test.want {
				t.Fatalf("hasProviderVisiblePayload() = %v, want %v", got, test.want)
			}
		})
	}
}

func snapshotSources(t *testing.T, sources []Source) []byte {
	t.Helper()
	snapshot, err := json.Marshal(sources)
	if err != nil {
		t.Fatalf("marshal source snapshot: %v", err)
	}
	return snapshot
}

func estimateEntries(entries []Entry) int {
	total := 0
	for _, entry := range entries {
		total += messageconv.EstimateSDKMessageTokens(entry.Message)
	}
	return total
}

func entryMessages(entries []Entry) []sdk.Message {
	messages := make([]sdk.Message, len(entries))
	for i, entry := range entries {
		messages[i] = entry.Message
	}
	return messages
}

func decisionIDs(decisions []contextbudget.Decision) []string {
	ids := make([]string, len(decisions))
	for i, decision := range decisions {
		ids[i] = decision.ID
	}
	return ids
}
