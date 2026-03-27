package models

import (
	"net/http"
	"strings"

	anthropicmessages "github.com/memohai/twilight-ai/provider/anthropic/messages"
	googlegenerative "github.com/memohai/twilight-ai/provider/google/generativeai"
	openaicodex "github.com/memohai/twilight-ai/provider/openai/codex"
	openaicompletions "github.com/memohai/twilight-ai/provider/openai/completions"
	openairesponses "github.com/memohai/twilight-ai/provider/openai/responses"
	sdk "github.com/memohai/twilight-ai/sdk"
)

// SDKModelConfig holds provider and model information resolved from DB,
// used to construct a Twilight AI SDK Model instance.
type SDKModelConfig struct {
	ModelID         string
	ClientType      string
	APIKey          string //nolint:gosec // carries provider credential material at runtime
	CodexAccountID  string
	BaseURL         string
	HTTPClient      *http.Client
	ReasoningConfig *ReasoningConfig
}

// ReasoningConfig controls extended thinking/reasoning behavior.
type ReasoningConfig struct {
	Enabled bool
	Effort  string
}

var (
	anthropicBudget = map[string]int{"low": 5000, "medium": 16000, "high": 50000}
	googleBudget    = map[string]int{"low": 5000, "medium": 16000, "high": 50000}
)

// NewSDKChatModel builds a Twilight AI SDK Model from the resolved model config.
func NewSDKChatModel(cfg SDKModelConfig) *sdk.Model {
	switch ClientType(cfg.ClientType) {
	case ClientTypeOpenAICompletions:
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(cfg.APIKey),
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, openaicompletions.WithHTTPClient(cfg.HTTPClient))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openaicompletions.WithBaseURL(cfg.BaseURL))
		}
		p := openaicompletions.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeOpenAIResponses:
		opts := []openairesponses.Option{
			openairesponses.WithAPIKey(cfg.APIKey),
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, openairesponses.WithHTTPClient(cfg.HTTPClient))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openairesponses.WithBaseURL(cfg.BaseURL))
		}
		p := openairesponses.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeOpenAICodex:
		opts := []openaicodex.Option{
			openaicodex.WithAccessToken(cfg.APIKey),
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, openaicodex.WithHTTPClient(cfg.HTTPClient))
		}
		if cfg.CodexAccountID != "" {
			opts = append(opts, openaicodex.WithAccountID(cfg.CodexAccountID))
		}
		return openaicodex.New(opts...).ChatModel(cfg.ModelID)

	case ClientTypeAnthropicMessages:
		opts := []anthropicmessages.Option{
			anthropicmessages.WithAPIKey(cfg.APIKey),
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, anthropicmessages.WithHTTPClient(cfg.HTTPClient))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, anthropicmessages.WithBaseURL(cfg.BaseURL))
		}
		if cfg.ReasoningConfig != nil && cfg.ReasoningConfig.Enabled {
			budget := ReasoningBudgetTokens(cfg.ClientType, cfg.ReasoningConfig.Effort)
			opts = append(opts, anthropicmessages.WithThinking(anthropicmessages.ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: budget,
			}))
		}
		p := anthropicmessages.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeGoogleGenerativeAI:
		opts := []googlegenerative.Option{
			googlegenerative.WithAPIKey(cfg.APIKey),
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, googlegenerative.WithHTTPClient(cfg.HTTPClient))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, googlegenerative.WithBaseURL(cfg.BaseURL))
		}
		p := googlegenerative.New(opts...)
		return p.ChatModel(cfg.ModelID)

	default:
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(cfg.APIKey),
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, openaicompletions.WithHTTPClient(cfg.HTTPClient))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openaicompletions.WithBaseURL(cfg.BaseURL))
		}
		p := openaicompletions.New(opts...)
		return p.ChatModel(cfg.ModelID)
	}
}

// BuildReasoningOptions returns SDK generation options for reasoning/thinking.
func BuildReasoningOptions(cfg SDKModelConfig) []sdk.GenerateOption {
	if cfg.ReasoningConfig == nil || !cfg.ReasoningConfig.Enabled {
		return nil
	}
	effort := cfg.ReasoningConfig.Effort
	if effort == "" {
		effort = "medium"
	}

	switch ClientType(cfg.ClientType) {
	case ClientTypeAnthropicMessages:
		return nil
	case ClientTypeOpenAIResponses, ClientTypeOpenAICompletions, ClientTypeOpenAICodex:
		return []sdk.GenerateOption{sdk.WithReasoningEffort(effort)}
	case ClientTypeGoogleGenerativeAI:
		return nil
	default:
		return []sdk.GenerateOption{sdk.WithReasoningEffort(effort)}
	}
}

// ReasoningBudgetTokens returns the token budget for extended thinking based on client type and effort.
func ReasoningBudgetTokens(clientType, effort string) int {
	if effort == "" {
		effort = "medium"
	}
	switch ClientType(clientType) {
	case ClientTypeAnthropicMessages:
		if b, ok := anthropicBudget[effort]; ok {
			return b
		}
		return anthropicBudget["medium"]
	case ClientTypeGoogleGenerativeAI:
		if b, ok := googleBudget[effort]; ok {
			return b
		}
		return googleBudget["medium"]
	default:
		return 0
	}
}

// ResolveClientType infers the client type string from an SDK Model's provider name.
func ResolveClientType(model *sdk.Model) string {
	if model == nil || model.Provider == nil {
		return string(ClientTypeOpenAICompletions)
	}
	name := model.Provider.Name()
	switch {
	case strings.Contains(name, "anthropic"):
		return string(ClientTypeAnthropicMessages)
	case strings.Contains(name, "google"):
		return string(ClientTypeGoogleGenerativeAI)
	case strings.Contains(name, "codex"):
		return string(ClientTypeOpenAICodex)
	case strings.Contains(name, "responses"):
		return string(ClientTypeOpenAIResponses)
	default:
		return string(ClientTypeOpenAICompletions)
	}
}
