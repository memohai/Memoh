package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestBuildReasoningOptionsDeepSeekChatCompletionsCompat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *ReasoningConfig
		wantOpts int
	}{
		{
			name:     "disabled sends explicit none effort",
			config:   &ReasoningConfig{Disabled: true},
			wantOpts: 1,
		},
		{
			name:     "active with effort forwards effort",
			config:   &ReasoningConfig{Active: true, Effort: "high"},
			wantOpts: 1,
		},
		{
			name:     "active without effort leaves effort unset for model default",
			config:   &ReasoningConfig{Active: true},
			wantOpts: 0,
		},
		{
			name:     "nil config produces no options",
			config:   nil,
			wantOpts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := BuildReasoningOptions(SDKModelConfig{
				ClientType:            string(ClientTypeOpenAICompletions),
				ChatCompletionsCompat: ChatCompletionsCompatDeepSeek,
				ReasoningConfig:       tt.config,
			})
			if len(opts) != tt.wantOpts {
				t.Fatalf("expected %d options, got %d", tt.wantOpts, len(opts))
			}
		})
	}
}

func TestBuildReasoningOptionsMiniMaxChatCompletionsCompat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *ReasoningConfig
		wantOpts int
	}{
		{
			name:     "disabled sends explicit none effort",
			config:   &ReasoningConfig{Disabled: true},
			wantOpts: 1,
		},
		{
			name:     "active without effort forwards nothing (provider default applies)",
			config:   &ReasoningConfig{Active: true},
			wantOpts: 0,
		},
		{
			name:     "active with effort forwards effort",
			config:   &ReasoningConfig{Active: true, Effort: "high"},
			wantOpts: 1,
		},
		{
			name:     "nil config produces no options",
			config:   nil,
			wantOpts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := BuildReasoningOptions(SDKModelConfig{
				ClientType:            string(ClientTypeOpenAICompletions),
				ChatCompletionsCompat: ChatCompletionsCompatMiniMax,
				ReasoningConfig:       tt.config,
			})
			if len(opts) != tt.wantOpts {
				t.Fatalf("expected %d options, got %d", tt.wantOpts, len(opts))
			}
		})
	}
}

func TestNewSDKChatModelDeepSeekChatCompletionsCompatDisablesThinking(t *testing.T) {
	t.Parallel()

	var body struct {
		ReasoningEffort *string `json:"reasoning_effort"`
		Thinking        *struct {
			Type string `json:"type"`
		} `json:"thinking"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-deepseek",
			"model": "deepseek-v4-flash",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	model := NewSDKChatModel(SDKModelConfig{
		ModelID:               "deepseek-v4-flash",
		ClientType:            string(ClientTypeOpenAICompletions),
		BaseURL:               srv.URL,
		ChatCompletionsCompat: ChatCompletionsCompatDeepSeek,
		APIKey:                "test-key",
	})
	if model == nil {
		t.Fatal("expected a model, got nil")
	}
	if model.Provider == nil || model.Provider.Name() != string(ClientTypeOpenAICompletions) {
		t.Fatalf("expected openai completions provider, got %+v", model.Provider)
	}

	_, err := sdk.GenerateTextResult(
		context.Background(),
		sdk.WithModel(model),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage("hi")}),
		sdk.WithReasoningEffort(ReasoningEffortNone),
	)
	if err != nil {
		t.Fatalf("generate text: %v", err)
	}

	if body.ReasoningEffort != nil {
		t.Fatalf("reasoning_effort should be omitted, got %q", *body.ReasoningEffort)
	}
	if body.Thinking == nil || body.Thinking.Type != "disabled" {
		t.Fatalf("thinking: got %#v, want disabled", body.Thinking)
	}
}

func TestNewSDKChatModelMiniMaxChatCompletionsCompatDisablesThinking(t *testing.T) {
	t.Parallel()

	var body struct {
		ReasoningEffort *string `json:"reasoning_effort"`
		ReasoningSplit  bool    `json:"reasoning_split"`
		Thinking        *struct {
			Type string `json:"type"`
		} `json:"thinking"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-minimax",
			"model": "MiniMax-M3",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	compat := ResolveChatCompletionsCompat(srv.URL, ChatCompletionsCompatMiniMax)
	model := NewSDKChatModel(SDKModelConfig{
		ModelID:               "MiniMax-M3",
		ClientType:            string(ClientTypeOpenAICompletions),
		BaseURL:               srv.URL,
		ChatCompletionsCompat: compat,
		APIKey:                "test-key",
	})
	if model == nil {
		t.Fatal("expected a model, got nil")
	}

	opts := []sdk.GenerateOption{
		sdk.WithModel(model),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage("hi")}),
	}
	opts = append(opts, BuildReasoningOptions(SDKModelConfig{
		ClientType:            string(ClientTypeOpenAICompletions),
		ChatCompletionsCompat: compat,
		ReasoningConfig:       &ReasoningConfig{Disabled: true},
	})...)
	_, err := sdk.GenerateTextResult(context.Background(), opts...)
	if err != nil {
		t.Fatalf("generate text: %v", err)
	}

	if !body.ReasoningSplit {
		t.Fatal("expected reasoning_split=true")
	}
	if body.ReasoningEffort != nil {
		t.Fatalf("reasoning_effort should be omitted, got %q", *body.ReasoningEffort)
	}
	if body.Thinking == nil || body.Thinking.Type != "disabled" {
		t.Fatalf("thinking: got %#v, want disabled", body.Thinking)
	}
}

func TestNewSDKChatModelMiniMaxChatCompletionsCompatEnablesThinking(t *testing.T) {
	t.Parallel()

	var body struct {
		ReasoningEffort *string `json:"reasoning_effort"`
		ReasoningSplit  bool    `json:"reasoning_split"`
		Thinking        *struct {
			Type string `json:"type"`
		} `json:"thinking"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-minimax",
			"model": "MiniMax-M3",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	compat := ResolveChatCompletionsCompat(srv.URL, ChatCompletionsCompatMiniMax)
	model := NewSDKChatModel(SDKModelConfig{
		ModelID:               "MiniMax-M3",
		ClientType:            string(ClientTypeOpenAICompletions),
		BaseURL:               srv.URL,
		ChatCompletionsCompat: compat,
		APIKey:                "test-key",
	})
	if model == nil {
		t.Fatal("expected a model, got nil")
	}

	opts := []sdk.GenerateOption{
		sdk.WithModel(model),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage("hi")}),
	}
	opts = append(opts, BuildReasoningOptions(SDKModelConfig{
		ClientType:            string(ClientTypeOpenAICompletions),
		ChatCompletionsCompat: compat,
		ReasoningConfig:       &ReasoningConfig{Active: true, Effort: "high"},
	})...)
	_, err := sdk.GenerateTextResult(context.Background(), opts...)
	if err != nil {
		t.Fatalf("generate text: %v", err)
	}

	if !body.ReasoningSplit {
		t.Fatal("expected reasoning_split=true")
	}
	if body.ReasoningEffort != nil {
		t.Fatalf("reasoning_effort should be omitted, got %q", *body.ReasoningEffort)
	}
	if body.Thinking == nil || body.Thinking.Type != "adaptive" {
		t.Fatalf("thinking: got %#v, want adaptive", body.Thinking)
	}
}

func TestNewSDKChatModelOpenAIWireMapsMaxEffortToXHigh(t *testing.T) {
	t.Parallel()

	var body struct {
		ReasoningEffort *string `json:"reasoning_effort"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-openai",
			"model": "openrouter/anthropic/claude-opus-4.8",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	model := NewSDKChatModel(SDKModelConfig{
		ModelID:    "openrouter/anthropic/claude-opus-4.8",
		ClientType: string(ClientTypeOpenAICompletions),
		BaseURL:    srv.URL,
		APIKey:     "test-key",
	})

	opts := BuildReasoningOptions(SDKModelConfig{
		ClientType:      string(ClientTypeOpenAICompletions),
		ReasoningConfig: &ReasoningConfig{Active: true, Effort: ReasoningEffortMax},
	})
	_, err := sdk.GenerateTextResult(
		context.Background(),
		append([]sdk.GenerateOption{
			sdk.WithModel(model),
			sdk.WithMessages([]sdk.Message{sdk.UserMessage("hi")}),
		}, opts...)...,
	)
	if err != nil {
		t.Fatalf("generate text: %v", err)
	}

	if body.ReasoningEffort == nil || *body.ReasoningEffort != ReasoningEffortXHigh {
		t.Fatalf("reasoning_effort: got %v, want xhigh", body.ReasoningEffort)
	}
}

func TestResolveChatCompletionsCompatInfersDeepSeekBaseURL(t *testing.T) {
	t.Parallel()

	got := ResolveChatCompletionsCompat("https://api.deepseek.com/v1", "")
	if got != ChatCompletionsCompatDeepSeek {
		t.Fatalf("expected deepseek compat, got %q", got)
	}
}

func TestResolveChatCompletionsCompatInfersMiniMaxBaseURL(t *testing.T) {
	t.Parallel()

	tests := []string{
		"https://api.minimax.io/v1",
		"https://api.minimaxi.com/v1",
	}
	for _, baseURL := range tests {
		t.Run(baseURL, func(t *testing.T) {
			t.Parallel()
			got := ResolveChatCompletionsCompat(baseURL, "")
			if got != ChatCompletionsCompatMiniMax {
				t.Fatalf("expected minimax compat, got %q", got)
			}
		})
	}
}
