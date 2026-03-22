package agent

import (
	"context"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/tools"
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
		Model:           cfg.Model,
		System:          cfg.System,
		Query:           cfg.Query,
		SessionType:     cfg.SessionType,
		Messages:        messages,
		ReasoningEffort: cfg.ReasoningEffort,
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

// SpawnSystemPrompt returns the system prompt for a given session type.
func SpawnSystemPrompt(sessionType string) string {
	return GenerateSystemPrompt(SystemPromptParams{
		SessionType: sessionType,
	})
}

// SpawnModelCreatorFunc returns a tools.ModelCreator that delegates to agent.CreateModel.
func SpawnModelCreatorFunc() tools.ModelCreator {
	return func(modelID, clientType, apiKey, baseURL string) *sdk.Model {
		return CreateModel(ModelConfig{
			ModelID:    modelID,
			ClientType: clientType,
			APIKey:     apiKey,
			BaseURL:    baseURL,
		})
	}
}
