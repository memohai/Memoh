package agent

import (
	sdk "github.com/memohai/twilight-ai/sdk"

	anthropicmessages "github.com/memohai/twilight-ai/provider/anthropic/messages"
	googlegenerative "github.com/memohai/twilight-ai/provider/google/generativeai"
	openaicompletions "github.com/memohai/twilight-ai/provider/openai/completions"
	openairesponses "github.com/memohai/twilight-ai/provider/openai/responses"
)

// ClientType constants matching the database model configuration.
const (
	ClientTypeOpenAICompletions = "openai-completions"
	ClientTypeOpenAIResponses   = "openai-responses"
	ClientTypeAnthropicMessages = "anthropic-messages"
	ClientTypeGoogleGenerativeAI = "google-generative-ai"
)

// Reasoning budget maps per client type.
var (
	anthropicBudget = map[string]int{"low": 5000, "medium": 16000, "high": 50000}
	googleBudget    = map[string]int{"low": 5000, "medium": 16000, "high": 50000}
)

// CreateModel builds a Twilight AI SDK Model from the resolved model config.
func CreateModel(cfg ModelConfig) *sdk.Model {
	switch cfg.ClientType {
	case ClientTypeOpenAICompletions:
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(cfg.APIKey),
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
		if cfg.BaseURL != "" {
			opts = append(opts, openairesponses.WithBaseURL(cfg.BaseURL))
		}
		p := openairesponses.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeAnthropicMessages:
		opts := []anthropicmessages.Option{
			anthropicmessages.WithAPIKey(cfg.APIKey),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, anthropicmessages.WithBaseURL(cfg.BaseURL))
		}
		p := anthropicmessages.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeGoogleGenerativeAI:
		opts := []googlegenerative.Option{
			googlegenerative.WithAPIKey(cfg.APIKey),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, googlegenerative.WithBaseURL(cfg.BaseURL))
		}
		p := googlegenerative.New(opts...)
		return p.ChatModel(cfg.ModelID)

	default:
		// OpenAI-compatible fallback
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(cfg.APIKey),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openaicompletions.WithBaseURL(cfg.BaseURL))
		}
		p := openaicompletions.New(opts...)
		return p.ChatModel(cfg.ModelID)
	}
}

// BuildReasoningOptions returns SDK generation options for reasoning/thinking.
func BuildReasoningOptions(cfg ModelConfig) []sdk.GenerateOption {
	if cfg.ReasoningConfig == nil || !cfg.ReasoningConfig.Enabled {
		return nil
	}
	effort := cfg.ReasoningConfig.Effort
	if effort == "" {
		effort = "medium"
	}

	switch cfg.ClientType {
	case ClientTypeAnthropicMessages:
		// Anthropic uses thinking budget — no SDK option, handled by provider
		return nil
	case ClientTypeOpenAIResponses, ClientTypeOpenAICompletions:
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
	switch clientType {
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
