package messageconv

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
)

func TestModelMessageToSDKMessageText(t *testing.T) {
	t.Parallel()

	got := ModelMessageToSDKMessage(conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent("hello"),
	})

	assertSameJSON(t, got, sdk.UserMessage("hello"))
}

func TestModelMessageToSDKMessageStructuredParts(t *testing.T) {
	t.Parallel()

	got := ModelMessageToSDKMessage(conversation.ModelMessage{
		Role: "assistant",
		Content: mustJSON(t, []map[string]any{
			{"type": "text", "text": "checking"},
			{"type": "tool-call", "toolCallId": "call-1", "toolName": "lookup", "input": map[string]any{"q": "memoh"}},
		}),
	})

	assertSameJSON(t, got, sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.TextPart{Text: "checking"},
			sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", Input: map[string]any{"q": "memoh"}},
		},
	})
}

func TestSDKMessagesToModelMessagesPreservesUsage(t *testing.T) {
	t.Parallel()

	got := SDKMessagesToModelMessages([]sdk.Message{{
		Role:    sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.TextPart{Text: "hi"}},
		Usage:   &sdk.Usage{InputTokens: 3, OutputTokens: 4, TotalTokens: 7},
	}})

	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "assistant" {
		t.Fatalf("role = %q, want assistant", got[0].Role)
	}
	assertSameJSON(t, got[0].Content, json.RawMessage(`"hi"`))
	var usage sdk.Usage
	if err := json.Unmarshal(got[0].Usage, &usage); err != nil {
		t.Fatalf("unmarshal usage: %v", err)
	}
	if usage.InputTokens != 3 || usage.OutputTokens != 4 || usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want input/output/total 3/4/7", usage)
	}
}

func TestModelMessageToSDKMessageInvalidLegacyContentKeepsRole(t *testing.T) {
	t.Parallel()

	got := ModelMessageToSDKMessage(conversation.ModelMessage{
		Role:    "tool",
		Content: json.RawMessage(`{"not":"a valid sdk content shape"}`),
	})

	if got.Role != sdk.MessageRoleTool {
		t.Fatalf("role = %q, want tool", got.Role)
	}
	if len(got.Content) != 0 {
		t.Fatalf("content = %#v, want empty invalid fallback", got.Content)
	}
}

func TestCanonicalSDKMessageRemovesReasoningWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	usage := &sdk.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}
	input := sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{
			sdk.ReasoningPart{Text: "private"},
			sdk.TextPart{Text: "visible", ProviderMetadata: map[string]any{"signature": "kept"}},
		},
		Usage: usage,
	}
	got := CanonicalSDKMessage(input)

	if len(got.Content) != 1 || got.Usage != usage {
		t.Fatalf("canonical message = %#v", got)
	}
	if text, ok := got.Content[0].(sdk.TextPart); !ok || text.Text != "visible" {
		t.Fatalf("canonical content = %#v", got.Content)
	}
	if len(input.Content) != 2 {
		t.Fatalf("CanonicalSDKMessage mutated input: %#v", input)
	}
	if gotTokens, wantTokens := EstimateSDKMessageTokens(input), EstimateSDKMessageTokens(got); gotTokens != wantTokens || gotTokens == 0 {
		t.Fatalf("SDK estimate = %d, want %d", gotTokens, wantTokens)
	}
}

func TestEstimateSDKToolDefinitionTokensMetersProviderVisibleSchema(t *testing.T) {
	t.Parallel()

	base := sdk.Tool{
		Name:        "lookup",
		Description: "Find a record",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}
	baseTokens, err := EstimateSDKToolDefinitionTokens([]sdk.Tool{base})
	if err != nil {
		t.Fatalf("estimate base tool: %v", err)
	}
	if baseTokens <= 0 {
		t.Fatalf("base tokens = %d, want positive", baseTokens)
	}

	nonProviderFields := base
	nonProviderFields.RequireApproval = true
	nonProviderFields.Execute = func(*sdk.ToolExecContext, any) (any, error) { return nil, nil }
	got, err := EstimateSDKToolDefinitionTokens([]sdk.Tool{nonProviderFields})
	if err != nil {
		t.Fatalf("estimate non-provider fields: %v", err)
	}
	if got != baseTokens {
		t.Fatalf("non-provider fields changed estimate: got %d want %d", got, baseTokens)
	}

	larger := base
	larger.Description += " with a substantially longer provider-visible description"
	largerTokens, err := EstimateSDKToolDefinitionTokens([]sdk.Tool{larger})
	if err != nil {
		t.Fatalf("estimate larger tool: %v", err)
	}
	if largerTokens <= baseTokens {
		t.Fatalf("larger schema tokens = %d, want > %d", largerTokens, baseTokens)
	}

	inferredSchema, err := jsonschema.For[inferredToolParameters](nil)
	if err != nil {
		t.Fatalf("infer expected schema: %v", err)
	}
	explicit := base
	explicit.Parameters = inferredSchema
	explicitTokens, err := EstimateSDKToolDefinitionTokens([]sdk.Tool{explicit})
	if err != nil {
		t.Fatalf("estimate explicit inferred schema: %v", err)
	}
	inferred := base
	inferred.Parameters = inferredToolParameters{}
	inferredTokens, err := EstimateSDKToolDefinitionTokens([]sdk.Tool{inferred})
	if err != nil {
		t.Fatalf("estimate inferred struct schema: %v", err)
	}
	if inferredTokens != explicitTokens {
		t.Fatalf("struct schema tokens = %d, want provider-resolved schema tokens %d", inferredTokens, explicitTokens)
	}

	sentinel := errors.New("cannot encode schema")
	_, err = EstimateSDKToolDefinitionTokens([]sdk.Tool{{
		Name: "broken",
		Parameters: map[string]any{
			"broken": json.Marshaler(errorMarshaler{err: sentinel}),
		},
	}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("unmarshalable schema error = %v, want %v", err, sentinel)
	}
}

type inferredToolParameters struct {
	Query string `json:"query" jsonschema:"the lookup query"`
}

type errorMarshaler struct {
	err error
}

func (m errorMarshaler) MarshalJSON() ([]byte, error) {
	return nil, m.err
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	return raw
}

func assertSameJSON(t *testing.T, got any, want any) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(gotRaw) != string(wantRaw) {
		t.Fatalf("json mismatch:\ngot  %s\nwant %s", gotRaw, wantRaw)
	}
}
