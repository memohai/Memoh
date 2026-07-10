package flow

import (
	"encoding/json"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/messageconv"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/userinput"
)

func intPtr(v int) *int { return &v }

func trimRecord(msg conversation.ModelMessage, mutate func(*historyfrag.HistoryRecord)) historyfrag.HistoryRecord {
	return historyRecord("trim-row", msg, mutate)
}

func TestTrimMessagesByTokens_DropsLeadingOrphanTool(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("1111"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
			},
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(50)
		}),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("2222"),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("done"),
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(60)
		}),
	}

	// The newest assistant and tool result fit, adding the older assistant
	// tool call exceeds the budget. The cutoff initially lands on the tool result,
	// which must be skipped to avoid an orphan tool message.
	budget := estimateMessageTokens(messages[2].ModelMessage) + estimateMessageTokens(messages[3].ModelMessage)
	trimmed, _ := trimMessagesByTokens(nil, messages, budget)
	if len(trimmed) != 2 {
		t.Fatalf("expected truncation notice and latest assistant, got %d messages: %+v", len(trimmed), trimmed)
	}
	if trimmed[0].Role != "system" || trimmed[1].Role != "assistant" {
		t.Fatalf("expected [system, assistant], got %+v", trimmed)
	}
	for _, msg := range trimmed {
		if msg.Role == "tool" {
			t.Fatalf("expected orphan tool to be skipped, got %+v", trimmed)
		}
	}
}

func TestTrimMessagesByTokens_KeepsToolWhenPaired(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: conversation.ToolCallFunction{
						Name:      "calc",
						Arguments: `{"x":1}`,
					},
				},
			},
		}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10)
		}),
		trimRecord(conversation.ModelMessage{
			Role:       "tool",
			ToolCallID: "call-1",
			Content:    conversation.NewTextContent("2"),
		}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 100)
	if len(trimmed) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(trimmed))
	}
	if trimmed[0].Role != "assistant" || trimmed[1].Role != "tool" {
		t.Fatalf("unexpected role order: %q -> %q", trimmed[0].Role, trimmed[1].Role)
	}
}

func TestTrimMessagesByTokens_NoUsage_KeepsAll(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("hi")}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 10)
	if len(trimmed) != 2 {
		t.Fatalf("messages without outputTokens should all be kept, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_ZeroMeansNoLimit(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10000)
		}),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("world")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10000)
		}),
	}

	// maxTokens = 0 means "no limit configured", should keep all messages.
	trimmed, _ := trimMessagesByTokens(nil, messages, 0)
	if len(trimmed) != 2 {
		t.Fatalf("maxTokens=0 should keep all messages, got %d", len(trimmed))
	}
}

func TestTrimMessagesByTokens_SmallBudgetTrims(t *testing.T) {
	t.Parallel()

	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old message")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(100)
		}),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old reply")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(200)
		}),
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("new message")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(50)
		}),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("new reply")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(60)
		}),
	}

	// Budget of 1: should trim aggressively, NOT return all messages.
	trimmed, _ := trimMessagesByTokens(nil, messages, 1)
	if len(trimmed) >= len(messages) {
		t.Fatalf("maxTokens=1 should trim history, but got %d messages (same as input)", len(trimmed))
	}
}

func TestTrimMessagesByTokens_EstimatesFallback(t *testing.T) {
	t.Parallel()

	// Long user message without usage data — should be estimated.
	longText := make([]byte, 400)
	for i := range longText {
		longText[i] = 'x'
	}
	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(string(longText))}, nil),
		trimRecord(conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("ok")}, func(record *historyfrag.HistoryRecord) {
			record.UsageOutputTokens = intPtr(10)
		}),
	}

	// Budget of 50: user message is ~100 estimated tokens (400/4), should be trimmed.
	trimmed, _ := trimMessagesByTokens(nil, messages, 50)
	// When trimming occurs, a system truncation notice is prepended.
	// So we expect: 1 system notice + 1 assistant message (kept) = 2 total.
	// The key check is that the long user message was removed.
	if len(trimmed) != 2 || trimmed[0].Role != "system" || trimmed[1].Role != "assistant" {
		t.Fatalf("expected [system notice, assistant message], got %d messages: %+v", len(trimmed), trimmed)
	}
}

func TestEstimateMessageTokensRoundsUpShortNonEmptyMessages(t *testing.T) {
	t.Parallel()

	message := conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("abc")}
	if got := estimateMessageTokens(message); got != 1 {
		t.Fatalf("estimateMessageTokens() = %d, want 1", got)
	}
}

func TestEstimateMessageTokensExcludesReasoningOnlyContent(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{{"type": "reasoning", "text": "private chain of thought"}})
	if err != nil {
		t.Fatalf("marshal reasoning content: %v", err)
	}
	if got := estimateMessageTokens(conversation.ModelMessage{Role: "assistant", Content: content}); got != 0 {
		t.Fatalf("reasoning-only estimate = %d, want 0", got)
	}
}

func TestEstimateMessageTokensMatchesPipelineForStructuredContent(t *testing.T) {
	t.Parallel()

	for _, message := range []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent(" abc ")},
		{Role: "user", Content: conversation.NewTextContent("   ")},
		{
			Role: "assistant",
			ToolCalls: []conversation.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name:      "lookup",
					Arguments: `{"query":"weather"}`,
				},
			}},
		},
		{
			Role:       "tool",
			ToolCallID: "call-1",
			Name:       "lookup",
			Content:    conversation.NewTextContent("sunny"),
		},
	} {
		canonical := messageconv.CanonicalModelMessageContent(message)
		projected, err := json.Marshal(modelMessageToSDKMessage(message))
		if err != nil {
			t.Fatalf("marshal SDK projection: %v", err)
		}
		var envelope struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(projected, &envelope); err != nil {
			t.Fatalf("decode SDK projection: %v", err)
		}
		providerTokens := messageconv.EstimateCanonicalContentTokens(envelope.Content)
		canonicalTokens := messageconv.EstimateCanonicalContentTokens(canonical)
		if providerTokens != canonicalTokens {
			t.Fatalf("provider and canonical estimates differ: provider=%d canonical=%d", providerTokens, canonicalTokens)
		}
		composed := pipelinepkg.ComposeContext(nil, []pipelinepkg.TurnResponseEntry{{
			RequestedAtMs: 1,
			Role:          message.Role,
			RawContent:    canonical,
		}})
		if composed == nil {
			t.Fatal("pipeline composition returned nil")
		}
		got := estimateMessageTokens(message)
		if got <= 0 {
			t.Fatalf("structured estimate = %d, want positive cost for %s", got, canonical)
		}
		if want := composed.EstimatedTokens; got != want {
			t.Fatalf("structured estimate mismatch: flow=%d pipeline=%d content=%s", got, want, canonical)
		}
	}
}

func TestModelMessageProjectionPreservesNativeParts(t *testing.T) {
	t.Parallel()

	cache := &sdk.CacheControl{Type: "ephemeral", TTL: "1h"}
	modelMessages := messageconv.SDKMessagesToModelMessages([]sdk.Message{
		{
			Role: sdk.MessageRoleUser,
			Content: []sdk.MessagePart{
				sdk.TextPart{
					Text:             " padded ",
					CacheControl:     cache,
					ProviderMetadata: map[string]any{"provider": map[string]any{"key": "value"}},
				},
				sdk.ImagePart{Image: "base64-image", MediaType: "image/png", CacheControl: cache},
				sdk.FilePart{Data: "base64-file", MediaType: "text/plain", Filename: "a.txt", CacheControl: cache},
			},
		},
		sdk.ToolMessage(sdk.ToolResultPart{
			ToolCallID: "call-1",
			ToolName:   "lookup",
			Result:     map[string]any{"status": "failed"},
			IsError:    true,
		}),
	})

	user := modelMessageToSDKMessage(modelMessages[0])
	if len(user.Content) != 3 {
		t.Fatalf("projected user parts = %d, want 3", len(user.Content))
	}
	text, ok := user.Content[0].(sdk.TextPart)
	if !ok || text.Text != " padded " || text.CacheControl == nil || text.CacheControl.TTL != "1h" || len(text.ProviderMetadata) == 0 {
		t.Fatalf("text part lost semantic fields: %#v", user.Content[0])
	}
	image, ok := user.Content[1].(sdk.ImagePart)
	if !ok || image.Image != "base64-image" || image.MediaType != "image/png" || image.CacheControl == nil {
		t.Fatalf("image part was not preserved: %#v", user.Content[1])
	}
	file, ok := user.Content[2].(sdk.FilePart)
	if !ok || file.Data != "base64-file" || file.Filename != "a.txt" || file.CacheControl == nil {
		t.Fatalf("file part was not preserved: %#v", user.Content[2])
	}

	tool := modelMessageToSDKMessage(modelMessages[1])
	result, ok := tool.Content[0].(sdk.ToolResultPart)
	if !ok || !result.IsError || result.ToolCallID != "call-1" {
		t.Fatalf("tool result lost error state: %#v", tool.Content[0])
	}
}

func TestModelMessageProjectionNormalizesMixedLegacyToolResult(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{{
		"type":       "tool-result",
		"toolCallId": "call-1",
		"toolName":   "lookup",
		"result":     nil,
		"output":     map[string]any{"status": "ok"},
	}})
	if err != nil {
		t.Fatalf("marshal mixed tool result: %v", err)
	}
	message := modelMessageToSDKMessage(conversation.ModelMessage{Role: "tool", Content: content})
	result, ok := message.Content[0].(sdk.ToolResultPart)
	if !ok {
		t.Fatalf("projected part = %#v, want tool result", message.Content[0])
	}
	payload, ok := result.Result.(map[string]any)
	if !ok || payload["status"] != "ok" {
		t.Fatalf("projected result = %#v, want legacy output", result.Result)
	}
}

func TestModelMessageProjectionDoesNotDuplicateLegacyToolCall(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{{
		"type":       "tool-call",
		"toolCallId": "call-1",
		"toolName":   "lookup",
		"input":      map[string]any{"query": "weather"},
	}})
	if err != nil {
		t.Fatalf("marshal native tool call: %v", err)
	}
	message := modelMessageToSDKMessage(conversation.ModelMessage{
		Role:    "assistant",
		Content: content,
		ToolCalls: []conversation.ToolCall{{
			ID: "call-1",
			Function: conversation.ToolCallFunction{
				Name:      "lookup",
				Arguments: `{"query":"weather"}`,
			},
		}},
	})
	if len(message.Content) != 1 {
		t.Fatalf("projected parts = %d, want one occurrence", len(message.Content))
	}
}

func TestTrimMessagesByTokens_PreservesRequiredMessage(t *testing.T) {
	t.Parallel()

	longText := make([]byte, 400)
	for i := range longText {
		longText[i] = 'x'
	}
	messages := []historyfrag.HistoryRecord{
		trimRecord(conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("retry this exact prompt"),
		}, func(record *historyfrag.HistoryRecord) {
			record.Required = true
		}),
		trimRecord(conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent(string(longText)),
		}, nil),
		trimRecord(conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("recent reply"),
		}, nil),
	}

	trimmed, _ := trimMessagesByTokens(nil, messages, 5)
	if len(trimmed) < 2 {
		t.Fatalf("expected system notice and required prompt, got %d", len(trimmed))
	}
	if trimmed[0].Role != "system" {
		t.Fatalf("first message role = %q, want system", trimmed[0].Role)
	}
	if trimmed[1].Role != "user" || trimmed[1].TextContent() != "retry this exact prompt" {
		t.Fatalf("required message was not preserved in order: %+v", trimmed)
	}
}

func TestStripToolMessages_RemovesAssistantToolCallContentParts(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{
		{"type": "reasoning", "text": "thinking"},
		{"type": "tool-call", "toolName": "read", "toolCallId": "call-1", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	filtered := stripToolMessages([]conversation.ModelMessage{
		{
			Role:    "assistant",
			Content: content,
		},
		{
			Role:    "assistant",
			Content: conversation.NewTextContent("保留这条消息"),
		},
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 message after filtering, got %d", len(filtered))
	}
	if filtered[0].TextContent() != "保留这条消息" {
		t.Fatalf("unexpected remaining message: %+v", filtered[0])
	}
}

func TestStripToolMessages_PreservesAskUserInteraction(t *testing.T) {
	t.Parallel()

	callContent, err := json.Marshal([]map[string]any{
		{"type": "text", "text": "请回答这一题："},
		{
			"type":       "tool-call",
			"toolName":   userinput.ToolNameAskUser,
			"toolCallId": "ask-1",
			"input": map[string]any{
				"questions": []any{
					map[string]any{
						"text": "选哪一个？",
						"kind": "single_select",
						"options": []any{
							map[string]any{"label": "A"},
							map[string]any{"label": "B"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal call content: %v", err)
	}
	resultContent, err := json.Marshal([]map[string]any{
		{
			"type":       "tool-result",
			"toolName":   userinput.ToolNameAskUser,
			"toolCallId": "ask-1",
			"result": map[string]any{
				"status": "submitted",
				"answers": []any{
					map[string]any{
						"question": "选哪一个？",
						"selected": []any{map[string]any{"label": "B"}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	readContent, err := json.Marshal([]map[string]any{
		{"type": "tool-call", "toolName": "read", "toolCallId": "read-1", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal read content: %v", err)
	}

	filtered := stripToolMessages([]conversation.ModelMessage{
		{Role: "assistant", Content: callContent},
		{Role: "tool", Content: resultContent},
		{Role: "assistant", Content: readContent},
		{Role: "tool", Content: conversation.NewTextContent("large output")},
	})

	if len(filtered) != 2 {
		t.Fatalf("expected ask_user call and result to remain, got %d messages: %+v", len(filtered), filtered)
	}
	if filtered[0].Role != "assistant" || filtered[1].Role != "tool" {
		t.Fatalf("unexpected roles after filtering: %+v", filtered)
	}

	var callParts []map[string]any
	if err := json.Unmarshal(filtered[0].Content, &callParts); err != nil {
		t.Fatalf("unmarshal preserved call content: %v", err)
	}
	if len(callParts) != 2 || callParts[1]["toolName"] != userinput.ToolNameAskUser {
		t.Fatalf("ask_user tool call was not preserved: %#v", callParts)
	}

	var resultParts []map[string]any
	if err := json.Unmarshal(filtered[1].Content, &resultParts); err != nil {
		t.Fatalf("unmarshal preserved result content: %v", err)
	}
	if len(resultParts) != 1 || resultParts[0]["toolName"] != userinput.ToolNameAskUser {
		t.Fatalf("ask_user tool result was not preserved: %#v", resultParts)
	}
}
