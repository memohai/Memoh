package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/models"
)

// SpawnAdapter wraps *Agent to satisfy tools.SpawnAgent without creating
// an import cycle (tools -> agent).
type SpawnAdapter struct {
	agent *Agent
}

// NewSpawnAdapter creates a SpawnAdapter from the given Agent.
func NewSpawnAdapter(a *Agent) *SpawnAdapter {
	return &SpawnAdapter{agent: a}
}

func (s *SpawnAdapter) Generate(ctx context.Context, cfg tools.SpawnRunConfig) (*tools.SpawnResult, error) {
	messages := cfg.Messages
	if cfg.Query != "" {
		messages = append(messages, sdk.Message{
			Role:    sdk.MessageRoleUser,
			Content: []sdk.MessagePart{sdk.TextPart{Text: cfg.Query}},
		})
	}

	rc := RunConfig{
		Model:            cfg.Model,
		System:           cfg.System,
		Query:            cfg.Query,
		SessionType:      cfg.SessionType,
		Messages:         messages,
		ReasoningEffort:  cfg.ReasoningEffort,
		PromptCacheTTL:   cfg.PromptCacheTTL,
		SupportsToolCall: true,
		Identity: SessionContext{
			BotID:             cfg.Identity.BotID,
			ChatID:            cfg.Identity.ChatID,
			SessionID:         cfg.Identity.SessionID,
			ChannelIdentityID: cfg.Identity.ChannelIdentityID,
			CurrentPlatform:   cfg.Identity.CurrentPlatform,
			SessionToken:      cfg.Identity.SessionToken,
			IsSubagent:        cfg.Identity.IsSubagent,
		},
		LoopDetection: LoopDetectionConfig{
			Enabled: cfg.LoopDetection.Enabled,
		},
	}

	result, err := s.agent.Generate(ctx, rc)
	if err != nil {
		return nil, err
	}

	return &tools.SpawnResult{
		Messages: result.Messages,
		Text:     result.Text,
		Usage:    result.Usage,
	}, nil
}

// GenerateWithWatchdog runs the agent in streaming mode, touching the
// provided touchFn on every stream event (token, tool progress, etc.).
// It collects the full result and returns it in the same shape as Generate.
// This enables activity-based watchdog monitoring for subagent execution.
func (s *SpawnAdapter) GenerateWithWatchdog(ctx context.Context, cfg tools.SpawnRunConfig, touchFn func()) (*tools.SpawnResult, error) {
	messages := cfg.Messages
	if cfg.Query != "" {
		messages = append(messages, sdk.Message{
			Role:    sdk.MessageRoleUser,
			Content: []sdk.MessagePart{sdk.TextPart{Text: cfg.Query}},
		})
	}

	rc := RunConfig{
		Model:            cfg.Model,
		System:           cfg.System,
		Query:            cfg.Query,
		SessionType:      cfg.SessionType,
		Messages:         messages,
		ReasoningEffort:  cfg.ReasoningEffort,
		PromptCacheTTL:   cfg.PromptCacheTTL,
		SupportsToolCall: true,
		Identity: SessionContext{
			BotID:             cfg.Identity.BotID,
			ChatID:            cfg.Identity.ChatID,
			SessionID:         cfg.Identity.SessionID,
			ChannelIdentityID: cfg.Identity.ChannelIdentityID,
			CurrentPlatform:   cfg.Identity.CurrentPlatform,
			SessionToken:      cfg.Identity.SessionToken,
			IsSubagent:        cfg.Identity.IsSubagent,
		},
		LoopDetection: LoopDetectionConfig{
			Enabled: cfg.LoopDetection.Enabled,
		},
	}

	// Use Stream instead of Generate to get per-token/per-tool activity signals.
	eventCh := s.agent.Stream(ctx, rc)

	var allText strings.Builder
	var finalMessages []sdk.Message
	var totalUsage sdk.Usage

	for evt := range eventCh {
		// Touch the watchdog on every event — this is the activity signal.
		touchFn()

		switch evt.Type {
		case EventTextDelta:
			allText.WriteString(evt.Delta)
		case EventAgentEnd, EventAgentAbort:
			if evt.Messages != nil {
				_ = json.Unmarshal(evt.Messages, &finalMessages)
			}
			if evt.Usage != nil {
				_ = json.Unmarshal(evt.Usage, &totalUsage)
			}
		}
	}

	// Check if context was cancelled (watchdog fired or parent cancelled).
	if ctx.Err() != nil {
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		return nil, ctx.Err()
	}

	return &tools.SpawnResult{
		Messages: finalMessages,
		Text:     allText.String(),
		Usage:    &totalUsage,
	}, nil
}

// SpawnSystemPrompt returns the system prompt for a given session type.
func SpawnSystemPrompt(sessionType string) string {
	return GenerateSystemPrompt(SystemPromptParams{
		SessionType: sessionType,
	})
}

// SpawnModelCreatorFunc returns a tools.ModelCreator backed by the shared SDK model factory.
// This keeps subagent model creation aligned with the shared SDK model factory.
func SpawnModelCreatorFunc() tools.ModelCreator {
	return func(modelID, clientType, apiKey, codexAccountID, baseURL string, httpClient *http.Client) *sdk.Model {
		return models.NewSDKChatModel(models.SDKModelConfig{
			ModelID:        modelID,
			ClientType:     clientType,
			APIKey:         apiKey,
			CodexAccountID: codexAccountID,
			BaseURL:        baseURL,
			HTTPClient:     httpClient,
		})
	}
}
