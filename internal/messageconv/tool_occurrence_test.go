package messageconv

import (
	"reflect"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
)

func TestRepairSDKToolOccurrencesRewritesOnlyInvalidParts(t *testing.T) {
	t.Parallel()

	messages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.TextPart{Text: "visible"},
				sdk.ToolCallPart{ToolCallID: "call-a", ToolName: "lookup", Input: map[string]any{}},
				sdk.ToolCallPart{ToolCallID: "call-a", ToolName: "duplicate", Input: map[string]any{}},
			},
		},
		{
			Role: sdk.MessageRoleTool,
			Content: []sdk.MessagePart{
				sdk.ToolResultPart{ToolCallID: "call-a", ToolName: "lookup", Result: "ok"},
				sdk.ToolResultPart{ToolCallID: "call-a", ToolName: "lookup", Result: "duplicate"},
				sdk.ToolResultPart{ToolCallID: "orphan", ToolName: "lookup", Result: "orphan"},
			},
		},
	}
	original := cloneSDKMessages(messages)

	got := RepairSDKToolOccurrences(messages, "interrupted")
	if !got.Changed || len(got.Entries) != 2 || len(got.RemovedParts) != 3 || len(got.SynthesizedCalls) != 0 {
		t.Fatalf("repair result = %#v", got)
	}
	if got.Entries[0].SourceIndex != 0 || got.Entries[0].Synthetic || got.Entries[1].SourceIndex != 1 || got.Entries[1].Synthetic {
		t.Fatalf("source mappings = %#v", got.Entries)
	}
	if len(got.Entries[0].Message.Content) != 2 {
		t.Fatalf("assistant content = %#v, want text plus one valid call", got.Entries[0].Message.Content)
	}
	if text, ok := got.Entries[0].Message.Content[0].(sdk.TextPart); !ok || text.Text != "visible" {
		t.Fatalf("visible text was not preserved: %#v", got.Entries[0].Message.Content)
	}
	if len(got.Entries[1].Message.Content) != 1 {
		t.Fatalf("tool content = %#v, want one valid result", got.Entries[1].Message.Content)
	}
	if !reflect.DeepEqual(messages, original) {
		t.Fatalf("input mutated:\ngot  %#v\nwant %#v", messages, original)
	}
	assertCleanSDKToolOccurrences(t, repairEntryMessages(got.Entries))
}

func TestRepairSDKToolOccurrencesSynthesizesErrorsBeforeBarrier(t *testing.T) {
	t.Parallel()

	messages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.ToolCallPart{ToolCallID: "call-a", ToolName: "first", Input: map[string]any{}},
				sdk.ToolCallPart{ToolCallID: "call-b", ToolName: "second", Input: map[string]any{}},
			},
		},
		sdk.UserMessage("next question"),
	}

	got := RepairSDKToolOccurrences(messages, " interrupted ")
	if len(got.Entries) != 4 || len(got.SynthesizedCalls) != 2 {
		t.Fatalf("repair result = %#v, want original call, two closures, user barrier", got)
	}
	for index := 1; index <= 2; index++ {
		entry := got.Entries[index]
		if !entry.Synthetic || entry.SourceIndex != 0 || entry.Message.Role != sdk.MessageRoleTool || len(entry.Message.Content) != 1 {
			t.Fatalf("synthetic entry[%d] = %#v", index, entry)
		}
		result, ok := entry.Message.Content[0].(sdk.ToolResultPart)
		if !ok || !result.IsError || result.Result != "interrupted" {
			t.Fatalf("synthetic result[%d] = %#v", index, entry.Message.Content[0])
		}
	}
	if got.Entries[3].SourceIndex != 1 || got.Entries[3].Message.Role != sdk.MessageRoleUser {
		t.Fatalf("barrier entry = %#v", got.Entries[3])
	}
	assertCleanSDKToolOccurrences(t, repairEntryMessages(got.Entries))
}

func TestRepairSDKToolOccurrencesDropsInvalidOnlyCarrier(t *testing.T) {
	t.Parallel()

	got := RepairSDKToolOccurrences([]sdk.Message{
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "orphan", ToolName: "lookup", Result: "bad"}),
		sdk.UserMessage("keep"),
	}, "interrupted")
	if len(got.Entries) != 1 || got.Entries[0].SourceIndex != 1 || got.Entries[0].Message.Role != sdk.MessageRoleUser {
		t.Fatalf("entries = %#v, want only source 1", got.Entries)
	}
	if len(got.RemovedParts) != 1 || got.RemovedParts[0].Reason != contextbudget.DropToolOrphanResult {
		t.Fatalf("removed parts = %#v", got.RemovedParts)
	}
}

func TestRepairSDKToolOccurrencesRejectsProviderInvalidRoles(t *testing.T) {
	t.Parallel()

	got := RepairSDKToolOccurrences([]sdk.Message{
		{
			Role: sdk.MessageRoleUser,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "user-call",
				ToolName:   "lookup",
				Input:      map[string]any{},
			}},
		},
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "call-a",
				ToolName:   "lookup",
				Input:      map[string]any{},
			}},
		},
		{
			Role: sdk.MessageRole("Tool"),
			Content: []sdk.MessagePart{sdk.ToolResultPart{
				ToolCallID: "call-a",
				ToolName:   "lookup",
				Result:     "not provider-visible",
			}},
		},
	}, "interrupted")
	if len(got.RemovedParts) != 2 {
		t.Fatalf("removed parts = %#v, want invalid user call and noncanonical result", got.RemovedParts)
	}
	for _, issue := range got.RemovedParts {
		if issue.Reason != contextbudget.DropToolInvalidRole {
			t.Fatalf("invalid-role issue = %#v", issue)
		}
	}
	if len(got.Entries) != 2 || got.Entries[0].SourceIndex != 1 || !got.Entries[1].Synthetic {
		t.Fatalf("entries = %#v, want assistant call plus synthetic closure", got.Entries)
	}
	result, ok := got.Entries[1].Message.Content[0].(sdk.ToolResultPart)
	if !ok || result.ToolCallID != "call-a" || !result.IsError {
		t.Fatalf("synthetic result = %#v", got.Entries[1].Message.Content)
	}
	assertCleanSDKToolOccurrences(t, repairEntryMessages(got.Entries))
}

func TestRepairSDKToolOccurrencesNormalizesMatchedIDsAndNames(t *testing.T) {
	t.Parallel()

	messages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: " call-a ",
				ToolName:   " lookup ",
				Input:      map[string]any{},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{
			ToolCallID: "call-a",
			ToolName:   "wrong-name",
			Result:     "ok",
		}),
	}
	original := cloneSDKMessages(messages)
	got := RepairSDKToolOccurrences(messages, "interrupted")

	if !got.Changed || got.NormalizedParts != 2 || len(got.Entries) != 2 {
		t.Fatalf("normalization result = %#v", got)
	}
	call := got.Entries[0].Message.Content[0].(sdk.ToolCallPart)
	result := got.Entries[1].Message.Content[0].(sdk.ToolResultPart)
	if call.ToolCallID != "call-a" || call.ToolName != "lookup" {
		t.Fatalf("normalized call = %#v", call)
	}
	if result.ToolCallID != call.ToolCallID || result.ToolName != call.ToolName {
		t.Fatalf("normalized result = %#v, want call identity %#v", result, call)
	}
	if !reflect.DeepEqual(messages, original) {
		t.Fatalf("normalization mutated input: got %#v want %#v", messages, original)
	}
	assertCleanSDKToolOccurrences(t, repairEntryMessages(got.Entries))
}

func TestRepairSDKToolOccurrencesDropsCallWithoutName(t *testing.T) {
	t.Parallel()

	got := RepairSDKToolOccurrences([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.TextPart{Text: "visible"},
			sdk.ToolCallPart{ToolCallID: "call-a", ToolName: " ", Input: map[string]any{}},
		},
	}}, "interrupted")

	if len(got.Entries) != 1 || len(got.Entries[0].Message.Content) != 1 {
		t.Fatalf("entries = %#v, want visible text only", got.Entries)
	}
	if len(got.RemovedParts) != 1 || got.RemovedParts[0].Reason != contextbudget.DropToolInvalidName {
		t.Fatalf("removed parts = %#v, want invalid name", got.RemovedParts)
	}
	if len(got.SynthesizedCalls) != 0 {
		t.Fatalf("invalid call synthesized a result: %#v", got.SynthesizedCalls)
	}
}

func TestRepairSDKToolOccurrencesIsIdempotent(t *testing.T) {
	t.Parallel()

	first := RepairSDKToolOccurrences([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-a",
			ToolName:   "lookup",
			Input:      map[string]any{},
		}},
	}}, "interrupted")
	second := RepairSDKToolOccurrences(repairEntryMessages(first.Entries), "interrupted")
	if second.Changed {
		t.Fatalf("second repair changed clean stream: %#v", second)
	}
	if !reflect.DeepEqual(repairEntryMessages(second.Entries), repairEntryMessages(first.Entries)) {
		t.Fatalf("repair is not idempotent:\nfirst  %#v\nsecond %#v", first.Entries, second.Entries)
	}
}

func assertCleanSDKToolOccurrences(t *testing.T, messages []sdk.Message) {
	t.Helper()
	analysis := AnalyzeSDKToolOccurrences(messages)
	if len(analysis.PartIssues) != 0 || len(analysis.DanglingCalls) != 0 {
		t.Fatalf("tool stream remains invalid: %#v", analysis)
	}
}

func repairEntryMessages(entries []SDKToolRepairEntry) []sdk.Message {
	messages := make([]sdk.Message, len(entries))
	for i, entry := range entries {
		messages[i] = entry.Message
	}
	return messages
}

func cloneSDKMessages(messages []sdk.Message) []sdk.Message {
	cloned := make([]sdk.Message, len(messages))
	for i, message := range messages {
		cloned[i] = message
		cloned[i].Content = append([]sdk.MessagePart(nil), message.Content...)
	}
	return cloned
}
