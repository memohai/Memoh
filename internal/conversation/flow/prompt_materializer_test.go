package flow

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextassembly"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/messageconv"
)

func TestInitialPromptMaterializerReservesFinalSystemAndActualTools(t *testing.T) {
	t.Parallel()

	oldest := sdk.UserMessage(strings.Repeat("o", 16))
	newest := sdk.AssistantMessage(strings.Repeat("n", 16))
	fixed := sdk.UserMessage("fixed")
	cfg := agent.RunConfig{
		System: strings.Repeat("s", 16),
	}
	tools := []sdk.Tool{{
		Name:        "lookup",
		Description: "provider-visible tool",
		Parameters:  map[string]any{"type": "object"},
	}}
	fixedTokens, err := providerFixedPromptTokens(cfg, tools)
	if err != nil {
		t.Fatalf("estimate fixed prompt: %v", err)
	}
	messageLimit := messageconv.EstimateSDKMessageTokens(newest) + messageconv.EstimateSDKMessageTokens(fixed)
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{
			{ID: "oldest", Message: oldest, CompactableTokens: 4},
			{ID: "newest", Message: newest, CompactableTokens: 4},
			{ID: "memory", Message: fixed, Retention: contextbudget.RetentionRequired},
		},
		ContextBudget: fixedTokens + messageLimit,
	})
	cfg.Messages = promptBaseline(t, plan)

	result, err := plan.Materialize(context.Background(), cfg, tools)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if result.FixedTokens != fixedTokens || result.MessageTokens != messageLimit || result.TotalTokens != plan.contextBudget {
		t.Fatalf("accounting = fixed:%d message:%d total:%d budget:%d", result.FixedTokens, result.MessageTokens, result.TotalTokens, plan.contextBudget)
	}
	if got := messageTexts(result.Config.Messages); len(got) != 2 || got[0] != strings.Repeat("n", 16) || got[1] != "fixed" {
		t.Fatalf("materialized messages = %#v, want newest and required fixed", got)
	}
	if len(result.Allocation.Dropped) != 1 || result.Allocation.Dropped[0].ID != "oldest" || result.Allocation.CompactableTokens != 8 {
		t.Fatalf("allocation = %#v", result.Allocation)
	}
}

func TestInitialPromptMaterializerKeepsEveryPreparedMessageRequired(t *testing.T) {
	t.Parallel()

	history := sdk.UserMessage(strings.Repeat("h", 32))
	memory := sdk.UserMessage("memory")
	skill := sdk.UserMessage("skill")
	query := sdk.UserMessage("current", sdk.ImagePart{Image: "data:image/png;base64,abc", MediaType: "image/png"})
	hook := sdk.UserMessage("hook")
	requiredTokens := messageconv.EstimateSDKMessageTokens(memory) +
		messageconv.EstimateSDKMessageTokens(skill) +
		messageconv.EstimateSDKMessageTokens(query) +
		messageconv.EstimateSDKMessageTokens(hook)
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{
			{ID: "history", Message: history, CompactableTokens: 8},
			{ID: "memory", Message: memory, Retention: contextbudget.RetentionRequired},
			{ID: "skill", Message: skill, Retention: contextbudget.RetentionRequired},
		},
		ContextBudget: requiredTokens,
	})
	cfg := agent.RunConfig{Messages: append(promptBaseline(t, plan), query, hook)}

	result, err := plan.Materialize(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if got := messageTexts(result.Config.Messages); len(got) != 4 || got[0] != "memory" || got[1] != "skill" || got[2] != "current" || got[3] != "hook" {
		t.Fatalf("required messages = %#v", got)
	}
	if !messagesContainImage(result.Config.Messages) {
		t.Fatalf("required query image was dropped: %#v", result.Config.Messages)
	}
	if len(result.Allocation.Dropped) != 1 || result.Allocation.Dropped[0].ID != "history" || result.TotalTokens != requiredTokens {
		t.Fatalf("allocation = %#v total=%d required=%d", result.Allocation, result.TotalTokens, requiredTokens)
	}
}

func TestInitialPromptMaterializerAcceptsEmptyBaselineWithPreparedQuery(t *testing.T) {
	t.Parallel()

	query := sdk.UserMessage("first turn")
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{})
	result, err := plan.Materialize(
		context.Background(),
		agent.RunConfig{Messages: []sdk.Message{query}},
		nil,
	)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if got := messageTexts(result.Config.Messages); len(got) != 1 || got[0] != "first turn" {
		t.Fatalf("prepared query = %#v", got)
	}
	if len(result.Allocation.Kept) != 1 || result.Allocation.Kept[0].ID != "prepared:0" {
		t.Fatalf("allocation = %#v", result.Allocation)
	}
}

func TestInitialPromptMaterializerReturnsTypedRequiredOverflow(t *testing.T) {
	t.Parallel()

	required := sdk.UserMessage(strings.Repeat("r", 32))
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{{
			ID:        "current-message",
			Message:   required,
			Retention: contextbudget.RetentionRequired,
		}},
		ContextBudget: 1,
		Notice:        "history truncated",
	})
	result, err := plan.Materialize(context.Background(), agent.RunConfig{Messages: promptBaseline(t, plan)}, nil)
	var envelopeErr *PromptEnvelopeOverflowError
	if !errors.As(err, &envelopeErr) {
		t.Fatalf("error = %v, want *PromptEnvelopeOverflowError", err)
	}
	var assemblyErr *contextassembly.OverflowError
	if !errors.As(err, &assemblyErr) {
		t.Fatalf("error = %v, want wrapped *contextassembly.OverflowError", err)
	}
	if envelopeErr.ContextBudget != 1 || envelopeErr.TotalTokens != result.TotalTokens || envelopeErr.MessageLimit != 1 {
		t.Fatalf("envelope error = %#v result=%#v", envelopeErr, result)
	}
	if len(envelopeErr.RequiredIDs) != 1 || envelopeErr.RequiredIDs[0] != "current-message" {
		t.Fatalf("required IDs = %#v", envelopeErr.RequiredIDs)
	}
	if result.Allocation.BudgetTrimmed || len(result.Config.Messages) != 1 || result.Config.Messages[0].Role == sdk.MessageRoleSystem {
		t.Fatalf("required overflow forged truncation notice: %#v", result)
	}
}

func TestPipelineCurrentImageStaysOnIdentifiedCurrentSource(t *testing.T) {
	t.Parallel()

	current := sdk.UserMessage("pipeline current")
	memory := sdk.UserMessage("memory context")
	image := sdk.ImagePart{
		Image:        "data:image/png;base64,current",
		MediaType:    "image/png",
		CacheControl: &sdk.CacheControl{Type: "ephemeral", TTL: "1h"},
	}
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{
			{ID: "pipeline-current:message-1", Message: current, Retention: contextbudget.RetentionRequired, CompactableTokens: 4},
			{ID: "memory", Message: memory, Retention: contextbudget.RetentionRequired},
		},
		CurrentSourceID: "pipeline-current:message-1",
	})
	if plan.sources[0].Retention != contextbudget.RetentionRequired || plan.sources[0].CompactableTokens != 4 {
		t.Fatalf("current source policy = %#v, want required with caller-owned raw pressure", plan.sources[0])
	}
	cfg := agent.RunConfig{
		Messages:     promptBaseline(t, plan),
		InlineImages: []sdk.ImagePart{image},
	}

	result, err := plan.Materialize(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(result.Config.Messages) != 2 || !messageHasImage(result.Config.Messages[0]) || messageHasImage(result.Config.Messages[1]) {
		t.Fatalf("pipeline image attached to wrong source: %#v", result.Config.Messages)
	}
	if !result.Config.ContextQueryMaterialized {
		t.Fatal("pipeline inline images were not marked materialized")
	}
	if result.Entries[0].SourceIndex != 0 || result.Entries[1].SourceIndex != 1 {
		t.Fatalf("source provenance = %#v", result.Entries)
	}
	if messageHasImage(plan.sources[0].Message) || messageHasImage(current) {
		t.Fatalf("materialization mutated source plan: %#v", plan.sources)
	}
	cfg.InlineImages[0].CacheControl.TTL = "mutated"
	entryImage := result.Entries[0].Message.Content[1].(sdk.ImagePart)
	if entryImage.CacheControl.TTL != "1h" {
		t.Fatalf("input image mutated audit entry: %#v", result.Entries[0])
	}
	result.Config.Messages[0].Content[0] = sdk.TextPart{Text: "mutated result config"}
	if result.Entries[0].Message.Content[0].(sdk.TextPart).Text != "pipeline current" {
		t.Fatalf("result config shares content with audit entries: %#v", result.Entries)
	}
}

func TestInitialPromptMaterializerDoesNotAliasPreparedSuffix(t *testing.T) {
	t.Parallel()

	input := map[string]any{"q": "original"}
	usage := &sdk.Usage{InputTokens: 3, TotalTokens: 3}
	prepared := sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-1",
			ToolName:   "lookup",
			Input:      input,
		}},
		Usage: usage,
	}
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{})
	cfg := agent.RunConfig{Messages: []sdk.Message{prepared}, ContextQueryMaterialized: true}
	result, err := plan.Materialize(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	input["q"] = "mutated"
	usage.InputTokens = 99
	entryCall := result.Entries[0].Message.Content[0].(sdk.ToolCallPart)
	if entryCall.Input.(map[string]any)["q"] != "original" || result.Entries[0].Message.Usage.InputTokens != 3 {
		t.Fatalf("prepared input mutated audit entries: %#v", result.Entries)
	}
}

func TestInitialPromptPlanRequiresStableCurrentSourceIdentity(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		sources []contextassembly.Source
	}{
		{
			name:    "missing",
			sources: []contextassembly.Source{{ID: "other", Message: sdk.UserMessage("other")}},
		},
		{
			name: "duplicate",
			sources: []contextassembly.Source{
				{ID: "current", Message: sdk.UserMessage("one")},
				{ID: "current", Message: sdk.UserMessage("two")},
			},
		},
		{
			name:    "non-user",
			sources: []contextassembly.Source{{ID: "current", Message: sdk.AssistantMessage("wrong role")}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := newInitialPromptPlan(initialPromptPlanInput{
				Sources:         test.sources,
				CurrentSourceID: "current",
			})
			if err == nil {
				t.Fatal("newInitialPromptPlan() error = nil")
			}
		})
	}

	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{{ID: "current", Message: sdk.UserMessage("current")}},
	})
	_, err := plan.Materialize(context.Background(), agent.RunConfig{
		Messages:     promptBaseline(t, plan),
		InlineImages: []sdk.ImagePart{{Image: "data:image/png;base64,abc"}},
	}, nil)
	if err == nil {
		t.Fatal("Materialize() accepted inline images without current source identity")
	}
}

func TestInitialPromptMaterializerDoesNotMisclassifyUnrenderableRequired(t *testing.T) {
	t.Parallel()

	plan := mustInitialPromptPlan(t, initialPromptPlanInput{})
	reasoningOnly := sdk.Message{
		Role:    sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private"}},
	}
	_, err := plan.Materialize(context.Background(), agent.RunConfig{Messages: []sdk.Message{reasoningOnly}}, nil)
	var unrenderable *contextassembly.UnrenderableRequiredError
	if !errors.As(err, &unrenderable) {
		t.Fatalf("error = %v, want *contextassembly.UnrenderableRequiredError", err)
	}
	var overflow *PromptEnvelopeOverflowError
	if errors.As(err, &overflow) {
		t.Fatalf("unrenderable required source was misclassified as overflow: %#v", overflow)
	}
}

func TestInitialPromptMaterializerPreservesUnrenderableCauseWhenAlsoOverflowing(t *testing.T) {
	t.Parallel()

	visible := sdk.UserMessage(strings.Repeat("v", 32))
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{{
			ID:        "visible-required",
			Message:   visible,
			Retention: contextbudget.RetentionRequired,
		}},
		ContextBudget: 1,
	})
	reasoningOnly := sdk.Message{
		Role:    sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "private"}},
	}
	cfg := agent.RunConfig{Messages: append(promptBaseline(t, plan), reasoningOnly)}
	_, err := plan.Materialize(context.Background(), cfg, nil)
	var overflow *PromptEnvelopeOverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("error = %v, want *PromptEnvelopeOverflowError", err)
	}
	var unrenderable *contextassembly.UnrenderableRequiredError
	if !errors.As(err, &unrenderable) {
		t.Fatalf("error = %v, want wrapped *contextassembly.UnrenderableRequiredError", err)
	}
}

func TestInitialPromptMaterializerTreatsNegativeBudgetAsUnlimited(t *testing.T) {
	t.Parallel()

	message := sdk.UserMessage(strings.Repeat("u", 32))
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources:       []contextassembly.Source{{ID: "history", Message: message}},
		ContextBudget: -1,
	})
	result, err := plan.Materialize(context.Background(), agent.RunConfig{Messages: promptBaseline(t, plan)}, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(result.Config.Messages) != 1 || !result.Allocation.SourcesFit {
		t.Fatalf("unlimited result = %#v", result)
	}
}

func TestInitialPromptMaterializerPreservesHistoryProvenanceAcrossSyntheticRepair(t *testing.T) {
	t.Parallel()

	usage := &sdk.Usage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}
	call := sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-1",
			ToolName:   "lookup",
			Input:      map[string]any{"q": "memoh"},
		}},
		Usage: usage,
	}
	boundary := sdk.UserMessage("next")
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{
			{ID: "history-call", Message: call, CompactableTokens: 7},
			{ID: "history-boundary", Message: boundary, CompactableTokens: 2},
		},
	})
	mutatedBaseline := promptBaseline(t, plan)
	mutatedCall := mutatedBaseline[0].Content[0].(sdk.ToolCallPart)
	mutatedCall.Input.(map[string]any)["q"] = "mutated baseline"
	mutatedBaseline[0].Usage.InputTokens = 99
	baseline := promptBaseline(t, plan)
	if len(baseline) != 3 {
		t.Fatalf("derived baseline = %#v, want call, synthetic closure, boundary", baseline)
	}
	baselineCall := baseline[0].Content[0].(sdk.ToolCallPart)
	if baselineCall.Input.(map[string]any)["q"] != "memoh" || baseline[0].Usage.InputTokens != 3 {
		t.Fatalf("baseline snapshot was mutated through clone: %#v", baseline)
	}

	result, err := plan.Materialize(context.Background(), agent.RunConfig{Messages: baseline}, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(result.Entries) != 3 || result.Entries[0].SourceIndex != 0 || result.Entries[1].SourceIndex != -1 || !result.Entries[1].Synthetic || result.Entries[2].SourceIndex != 1 {
		t.Fatalf("entry provenance = %#v", result.Entries)
	}
	if result.Config.Messages[0].Usage == usage || result.Config.Messages[0].Usage.InputTokens != usage.InputTokens || result.Config.Messages[1].Usage != nil {
		t.Fatalf("synthetic inherited source usage: %#v", result.Config.Messages)
	}
	if result.Allocation.CompactableTokens != 9 || result.MessageTokens != estimatePromptMessages(result.Config.Messages) {
		t.Fatalf("accounting = %#v emitted=%d", result.Allocation, result.MessageTokens)
	}
	configCall := result.Config.Messages[0].Content[0].(sdk.ToolCallPart)
	configCall.Input.(map[string]any)["q"] = "mutated result"
	result.Config.Messages[0].Usage.InputTokens = 77
	entryCall := result.Entries[0].Message.Content[0].(sdk.ToolCallPart)
	if entryCall.Input.(map[string]any)["q"] != "memoh" || result.Entries[0].Message.Usage.InputTokens != 3 {
		t.Fatalf("result config mutated audit entries: %#v", result.Entries)
	}
}

func TestInitialPromptPlanAppliesToolPolicyWithoutChangingRawPressure(t *testing.T) {
	t.Parallel()

	callUsage := &sdk.Usage{InputTokens: 3, TotalTokens: 3}
	boundaryUsage := &sdk.Usage{InputTokens: 5, TotalTokens: 5}
	call := sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.TextPart{Text: "visible"},
			sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", Input: map[string]any{}},
		},
		Usage: callUsage,
	}
	toolResult := sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "lookup", Result: "large"})
	boundary := sdk.UserMessage("next")
	boundary.Usage = boundaryUsage
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{
			{ID: "call", Message: call, CompactableTokens: 7},
			{ID: "result", Message: toolResult, CompactableTokens: 11},
			{ID: "boundary", Message: boundary, CompactableTokens: 2},
		},
		StripTools: true,
	})
	result, err := plan.Materialize(context.Background(), agent.RunConfig{Messages: promptBaseline(t, plan)}, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if got := messageTexts(result.Config.Messages); len(got) != 2 || got[0] != "visible" || got[1] != "next" {
		t.Fatalf("tool policy messages = %#v", result.Config.Messages)
	}
	if result.Allocation.CompactableTokens != 20 || len(result.Allocation.Dropped) != 1 || result.Allocation.Dropped[0].ID != "result" || result.Allocation.BudgetTrimmed {
		t.Fatalf("tool policy allocation = %#v", result.Allocation)
	}
	if result.Config.Messages[0].Usage == nil || result.Config.Messages[0].Usage.InputTokens != 3 || result.Config.Messages[1].Usage == nil || result.Config.Messages[1].Usage.InputTokens != 5 {
		t.Fatalf("tool policy lost source usage: %#v", result.Config.Messages)
	}
}

func TestInitialPromptPlanToolPolicyPreservesRequiredOccurrence(t *testing.T) {
	t.Parallel()

	call := sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.TextPart{Text: "visible"},
			sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", Input: map[string]any{}},
		},
	}
	toolResult := sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "lookup", Result: "required"})
	plan := mustInitialPromptPlan(t, initialPromptPlanInput{
		Sources: []contextassembly.Source{
			{ID: "call", Message: call, CompactableTokens: 7},
			{ID: "required-result", Message: toolResult, Retention: contextbudget.RetentionRequired, CompactableTokens: 11},
		},
		StripTools: true,
	})
	result, err := plan.Materialize(context.Background(), agent.RunConfig{Messages: promptBaseline(t, plan)}, nil)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(result.Config.Messages) != 2 || len(result.Allocation.Dropped) != 0 || result.Allocation.CompactableTokens != 18 {
		t.Fatalf("required occurrence = %#v", result)
	}
	analysis := messageconv.AnalyzeSDKToolOccurrences(result.Config.Messages)
	if len(analysis.Matches) != 1 || len(analysis.DanglingCalls) != 0 || len(analysis.PartIssues) != 0 {
		t.Fatalf("required occurrence closure = %#v", analysis)
	}
}

func messageTexts(messages []sdk.Message) []string {
	texts := make([]string, 0, len(messages))
	for _, message := range messages {
		for _, part := range message.Content {
			if text, ok := part.(sdk.TextPart); ok {
				texts = append(texts, text.Text)
			}
		}
	}
	return texts
}

func messageHasImage(message sdk.Message) bool {
	for _, part := range message.Content {
		if _, ok := part.(sdk.ImagePart); ok {
			return true
		}
	}
	return false
}

func estimatePromptMessages(messages []sdk.Message) int {
	total := 0
	for _, message := range messages {
		total += messageconv.EstimateSDKMessageTokens(message)
	}
	return total
}

func mustInitialPromptPlan(t *testing.T, input initialPromptPlanInput) initialPromptPlan {
	t.Helper()
	plan, err := newInitialPromptPlan(input)
	if err != nil {
		t.Fatalf("newInitialPromptPlan() error = %v", err)
	}
	return plan
}

func promptBaseline(t *testing.T, plan initialPromptPlan) []sdk.Message {
	t.Helper()
	messages, err := plan.BaselineMessages()
	if err != nil {
		t.Fatalf("BaselineMessages() error = %v", err)
	}
	return messages
}
