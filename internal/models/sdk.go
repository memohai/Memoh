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

	memohcopilot "github.com/memohai/memoh/internal/copilot"
)

// SDKModelConfig holds provider and model information resolved from DB,
// used to construct a Twilight AI SDK Model instance.
type SDKModelConfig struct {
	ModelID        string
	ClientType     string
	APIKey         string //nolint:gosec // carries provider credential material at runtime
	CodexAccountID string
	BaseURL        string
	// ChatCompletionsCompat selects narrow compatibility behavior for
	// OpenAI-compatible /chat/completions backends.
	ChatCompletionsCompat string
	HTTPClient            *http.Client
	ReasoningConfig       *ReasoningConfig
}

// ReasoningConfig is the resolved extended-thinking decision for one call. The
// resolver makes a single decision (based on the model's thinking mode and the
// user's settings); the SDK layer mechanically translates it per provider. We
// never send token budgets — only effort strings and (for Anthropic 4.6+) the
// adaptive thinking flag.
type ReasoningConfig struct {
	// Active means thinking is on for this call.
	Active bool
	// Disabled means thinking was explicitly turned off (toggle=off). For
	// OpenAI-style providers without a real off switch, OffEffort approximates it.
	Disabled bool
	// Adaptive means the provider's thinking is adaptive (Anthropic 4.6+), wired
	// as thinking{type:"adaptive"} with no budget.
	Adaptive bool
	// Effort is the effort tier to send when active ("" lets the SDK default).
	Effort string
	// OffEffort is the effort an OpenAI-style provider should send when disabled
	// ("none" when supported, otherwise "minimal").
	OffEffort string
}

// NewSDKChatModel builds a Twilight AI SDK Model from the resolved model config.
func NewSDKChatModel(cfg SDKModelConfig) *sdk.Model {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = NewProviderHTTPClient(0)
	}
	chatCompletionsCompat := ResolveChatCompletionsCompat(cfg.BaseURL, cfg.ChatCompletionsCompat)

	switch ClientType(cfg.ClientType) {
	case ClientTypeOpenAICompletions:
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(cfg.APIKey),
			openaicompletions.WithHTTPClient(cfg.HTTPClient),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openaicompletions.WithBaseURL(cfg.BaseURL))
		}
		if isDeepSeekChatCompletionsCompat(chatCompletionsCompat) {
			opts = append(opts, openaicompletions.WithDeepSeekChatCompletionsCompat())
		}
		if isMiniMaxChatCompletionsCompat(chatCompletionsCompat) {
			opts = append(opts, openaicompletions.WithMiniMaxChatCompletionsCompat())
		}
		p := openaicompletions.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeOpenAIResponses:
		opts := []openairesponses.Option{
			openairesponses.WithAPIKey(cfg.APIKey),
		}
		opts = append(opts, openairesponses.WithHTTPClient(cfg.HTTPClient))
		if cfg.BaseURL != "" {
			opts = append(opts, openairesponses.WithBaseURL(cfg.BaseURL))
		}
		p := openairesponses.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeOpenAICodex:
		opts := []openaicodex.Option{
			openaicodex.WithAccessToken(cfg.APIKey),
		}
		opts = append(opts, openaicodex.WithHTTPClient(cfg.HTTPClient))
		if cfg.CodexAccountID != "" {
			opts = append(opts, openaicodex.WithAccountID(cfg.CodexAccountID))
		}
		return openaicodex.New(opts...).ChatModel(cfg.ModelID)

	case ClientTypeGitHubCopilot:
		return memohcopilot.NewModel(cfg.APIKey, cfg.ModelID, cfg.HTTPClient)

	case ClientTypeAnthropicMessages:
		opts := []anthropicmessages.Option{
			anthropicmessages.WithAPIKey(cfg.APIKey),
		}
		opts = append(opts, anthropicmessages.WithHTTPClient(cfg.HTTPClient))
		if cfg.BaseURL != "" {
			opts = append(opts, anthropicmessages.WithBaseURL(cfg.BaseURL))
		}
		// Anthropic thinking is only wired as adaptive (4.6+). We never send
		// budget_tokens (deprecated on 4.6, rejected on 4.7+). Effort is carried
		// per-request via output_config.effort (see BuildReasoningOptions).
		// Non-adaptive Anthropic (toggle, e.g. 4.5) sends effort only, no thinking.
		if cfg.ReasoningConfig != nil && cfg.ReasoningConfig.Active && cfg.ReasoningConfig.Adaptive {
			opts = append(opts, anthropicmessages.WithThinking(anthropicmessages.ThinkingConfig{
				Type: "adaptive",
			}))
		}
		p := anthropicmessages.New(opts...)
		return p.ChatModel(cfg.ModelID)

	case ClientTypeGoogleGenerativeAI:
		opts := []googlegenerative.Option{
			googlegenerative.WithAPIKey(cfg.APIKey),
		}
		opts = append(opts, googlegenerative.WithHTTPClient(cfg.HTTPClient))
		if cfg.BaseURL != "" {
			opts = append(opts, googlegenerative.WithBaseURL(cfg.BaseURL))
		}
		p := googlegenerative.New(opts...)
		return p.ChatModel(cfg.ModelID)

	default:
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(cfg.APIKey),
			openaicompletions.WithHTTPClient(cfg.HTTPClient),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openaicompletions.WithBaseURL(cfg.BaseURL))
		}
		if isDeepSeekChatCompletionsCompat(chatCompletionsCompat) {
			opts = append(opts, openaicompletions.WithDeepSeekChatCompletionsCompat())
		}
		if isMiniMaxChatCompletionsCompat(chatCompletionsCompat) {
			opts = append(opts, openaicompletions.WithMiniMaxChatCompletionsCompat())
		}
		p := openaicompletions.New(opts...)
		return p.ChatModel(cfg.ModelID)
	}
}

// BuildReasoningOptions returns per-request SDK generation options for
// reasoning/thinking. It only ever sets an effort string (output_config.effort
// for Anthropic, reasoning.effort for OpenAI); the adaptive thinking flag is set
// at provider construction time in NewSDKChatModel. No token budgets are sent.
func BuildReasoningOptions(cfg SDKModelConfig) []sdk.GenerateOption {
	rc := cfg.ReasoningConfig
	if rc == nil {
		return nil
	}
	ct := ClientType(cfg.ClientType)

	// DeepSeek and MiniMax keep the generic Chat Completions transport but gate
	// thinking via a toggle rather than reasoning_effort. Their SDK compat layer
	// maps reasoning_effort "none" to thinking-off and any other effort to
	// thinking-on, so we forward "none" to disable and an explicit effort to
	// enable. Enabled-without-effort (adaptive) forwards nothing and lets the
	// provider's default thinking behavior apply.
	if ct == ClientTypeOpenAICompletions &&
		(isDeepSeekChatCompletionsCompat(cfg.ChatCompletionsCompat) || isMiniMaxChatCompletionsCompat(cfg.ChatCompletionsCompat)) {
		switch {
		case rc.Disabled:
			return []sdk.GenerateOption{sdk.WithReasoningEffort(ReasoningEffortNone)}
		case rc.Active && rc.Effort != "":
			return []sdk.GenerateOption{sdk.WithReasoningEffort(openAIWireEffort(rc.Effort))}
		default:
			return nil
		}
	}

	switch ct {
	case ClientTypeAnthropicMessages:
		// Effort only; thinking (adaptive) is set on the provider. When disabled,
		// send nothing (absence of thinking == off for Anthropic).
		if rc.Active && rc.Effort != "" {
			return []sdk.GenerateOption{sdk.WithReasoningEffort(rc.Effort)}
		}
		return nil

	case ClientTypeGoogleGenerativeAI:
		// Google thinking is out of scope for the effort wire; leave untouched.
		return nil

	case ClientTypeOpenAIResponses, ClientTypeOpenAICodex, ClientTypeOpenAICompletions:
		return openAIEffortOptions(rc)

	default:
		return openAIEffortOptions(rc)
	}
}

// openAIEffortOptions maps a reasoning decision to OpenAI-style reasoning.effort.
// OpenAI models have no real on/off switch, so "off" is approximated by the
// lowest effort the model supports (none when available, otherwise minimal).
func openAIEffortOptions(rc *ReasoningConfig) []sdk.GenerateOption {
	switch {
	case rc.Active:
		effort := openAIWireEffort(rc.Effort)
		if effort == "" {
			effort = ReasoningEffortMedium
		}
		return []sdk.GenerateOption{sdk.WithReasoningEffort(effort)}
	case rc.Disabled:
		off := openAIWireEffort(rc.OffEffort)
		if off == "" {
			off = ReasoningEffortMinimal
		}
		return []sdk.GenerateOption{sdk.WithReasoningEffort(off)}
	default:
		return nil
	}
}

// openAIWireEffort is a last-resort guard that rewrites effort values the
// OpenAI wire format rejects. The primary filter lives in the resolver's
// effectiveReasoningEfforts (which removes "max" from the selectable set for
// OpenAI-format clients), and the Twilight SDK's openai provider package
// also normalizes max→xhigh independently. This function is defence-in-depth.
func openAIWireEffort(effort string) string {
	if effort == ReasoningEffortMax {
		return ReasoningEffortXHigh
	}
	return effort
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
	case strings.Contains(name, "github-copilot"), strings.Contains(name, "copilot"):
		return string(ClientTypeGitHubCopilot)
	case strings.Contains(name, "codex"):
		return string(ClientTypeOpenAICodex)
	case strings.Contains(name, "responses"):
		return string(ClientTypeOpenAIResponses)
	default:
		return string(ClientTypeOpenAICompletions)
	}
}
