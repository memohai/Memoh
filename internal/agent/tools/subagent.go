package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db/sqlc"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
)

// SpawnAgent is the interface the spawn tool uses to run subagent tasks.
// It is satisfied by *agent.Agent and avoids an import cycle.
type SpawnAgent interface {
	Generate(ctx context.Context, cfg SpawnRunConfig) (*SpawnResult, error)
}

// SpawnRunConfig mirrors agent.RunConfig fields needed by spawn.
type SpawnRunConfig struct {
	Model           *sdk.Model
	System          string
	Query           string
	SessionType     string
	Identity        SpawnIdentity
	LoopDetection   SpawnLoopConfig
	Messages        []sdk.Message
	ReasoningEffort string
}

// SpawnIdentity mirrors agent.SessionContext fields needed by spawn.
type SpawnIdentity struct {
	BotID             string
	ChatID            string
	SessionID         string
	ChannelIdentityID string
	CurrentPlatform   string
	SessionToken      string //nolint:gosec // #nosec G117 -- session identifier, not a secret
	IsSubagent        bool
}

// SpawnLoopConfig mirrors agent.LoopDetectionConfig.
type SpawnLoopConfig struct {
	Enabled bool
}

// SpawnResult mirrors agent.GenerateResult.
type SpawnResult struct {
	Messages []sdk.Message
	Text     string
	Usage    *sdk.Usage
}

// SpawnProvider exposes a "spawn" tool that runs one or more subagent tasks
// concurrently and returns results to the parent agent.
type SpawnProvider struct {
	agent          SpawnAgent
	settings       *settings.Service
	models         *models.Service
	queries        *sqlc.Queries
	sessionService *sessionpkg.Service
	messageService messagepkg.Writer
	systemPromptFn func(sessionType string) string
	modelCreator   ModelCreator
	logger         *slog.Logger
}

// NewSpawnProvider creates a SpawnProvider. The agent must be injected later
// via SetAgent to avoid a dependency cycle.
func NewSpawnProvider(
	log *slog.Logger,
	settingsSvc *settings.Service,
	modelsSvc *models.Service,
	queries *sqlc.Queries,
	sessionService *sessionpkg.Service,
) *SpawnProvider {
	if log == nil {
		log = slog.Default()
	}
	return &SpawnProvider{
		settings:       settingsSvc,
		models:         modelsSvc,
		queries:        queries,
		sessionService: sessionService,
		logger:         log.With(slog.String("tool", "spawn")),
	}
}

// SetAgent injects the agent after construction (breaking the DI cycle).
func (p *SpawnProvider) SetAgent(a SpawnAgent) {
	p.agent = a
}

// SetMessageService injects an optional message writer for persisting
// subagent conversation history.
func (p *SpawnProvider) SetMessageService(w messagepkg.Writer) {
	p.messageService = w
}

// SetSystemPromptFunc injects the function used to generate the system prompt
// (typically agent.GenerateSystemPrompt).
func (p *SpawnProvider) SetSystemPromptFunc(fn func(sessionType string) string) {
	p.systemPromptFn = fn
}

func (p *SpawnProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.agent == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        "spawn",
			Description: "Spawn one or more subagents to work on tasks in parallel. Each task runs in its own context with file, exec, and web tools. All results are returned together.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tasks": map[string]any{
						"type":        "array",
						"description": "List of task instructions. Each string is a self-contained prompt for one subagent.",
						"items":       map[string]any{"type": "string"},
					},
				},
				"required": []string{"tasks"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSpawn(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

type spawnResult struct {
	Task      string `json:"task"`
	SessionID string `json:"session_id,omitempty"`
	Text      string `json:"text"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func (p *SpawnProvider) execSpawn(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}

	tasksRaw, ok := args["tasks"]
	if !ok {
		return nil, errors.New("tasks is required")
	}
	tasks, err := toStringSlice(tasksRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil, errors.New("at least one task is required")
	}

	sdkModel, modelID, err := p.resolveModel(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}

	systemPrompt := ""
	if p.systemPromptFn != nil {
		systemPrompt = p.systemPromptFn(sessionpkg.TypeSubagent)
	}

	results := make([]spawnResult, len(tasks))
	var wg sync.WaitGroup
	wg.Add(len(tasks))

	for i, task := range tasks {
		go func(idx int, query string) {
			defer wg.Done()
			results[idx] = p.runSubagentTask(ctx, session, sdkModel, modelID, systemPrompt, query)
		}(i, task)
	}
	wg.Wait()

	return map[string]any{"results": results}, nil
}

func (p *SpawnProvider) runSubagentTask(
	ctx context.Context,
	parentSession SessionContext,
	model *sdk.Model,
	modelID string,
	systemPrompt string,
	query string,
) spawnResult {
	res := spawnResult{Task: query}

	var sessionID string
	if p.sessionService != nil {
		sess, err := p.sessionService.Create(ctx, sessionpkg.CreateInput{
			BotID:           parentSession.BotID,
			Type:            sessionpkg.TypeSubagent,
			Title:           truncateTitle(query, 100),
			ParentSessionID: parentSession.SessionID,
		})
		if err != nil {
			p.logger.Warn("failed to create subagent session", slog.Any("error", err))
		} else {
			sessionID = sess.ID
			res.SessionID = sessionID
		}
	}

	cfg := SpawnRunConfig{
		Model:       model,
		System:      systemPrompt,
		Query:       query,
		SessionType: sessionpkg.TypeSubagent,
		Identity: SpawnIdentity{
			BotID:             parentSession.BotID,
			ChatID:            parentSession.ChatID,
			SessionID:         sessionID,
			ChannelIdentityID: parentSession.ChannelIdentityID,
			CurrentPlatform:   parentSession.CurrentPlatform,
			SessionToken:      parentSession.SessionToken,
			IsSubagent:        true,
		},
		LoopDetection: SpawnLoopConfig{Enabled: true},
	}

	genResult, err := p.agent.Generate(ctx, cfg)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	res.Text = genResult.Text
	res.Success = true

	if p.messageService != nil && sessionID != "" {
		p.persistMessages(ctx, parentSession.BotID, sessionID, modelID, query, genResult)
	}

	return res
}

func (p *SpawnProvider) persistMessages(
	ctx context.Context,
	botID, sessionID, modelID, query string,
	result *SpawnResult,
) {
	userContent, _ := json.Marshal(map[string]any{
		"role":    "user",
		"content": query,
	})
	if _, err := p.messageService.Persist(ctx, messagepkg.PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   userContent,
	}); err != nil {
		p.logger.Warn("persist subagent user message failed", slog.Any("error", err))
	}

	for _, msg := range result.Messages {
		if msg.Role == sdk.MessageRoleUser {
			continue
		}
		content, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var usage json.RawMessage
		if msg.Usage != nil {
			usage, _ = json.Marshal(msg.Usage)
		}
		if _, err := p.messageService.Persist(ctx, messagepkg.PersistInput{
			BotID:     botID,
			SessionID: sessionID,
			Role:      string(msg.Role),
			Content:   content,
			Usage:     usage,
			ModelID:   modelID,
		}); err != nil {
			p.logger.Warn("persist subagent message failed", slog.Any("error", err))
		}
	}
}

// ModelCreator creates an sdk.Model from provider config. Set via SetModelCreator.
type ModelCreator func(modelID, clientType, apiKey, codexAccountID, baseURL string, httpClient *http.Client) *sdk.Model

// SetModelCreator injects the function used to create SDK models
// (typically agent.CreateModel wrapped to match the signature).
func (p *SpawnProvider) SetModelCreator(fn ModelCreator) {
	p.modelCreator = fn
}

func (p *SpawnProvider) resolveModel(ctx context.Context, botID string) (*sdk.Model, string, error) {
	if p.settings == nil || p.models == nil || p.queries == nil {
		return nil, "", errors.New("model resolution services not configured")
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, "", err
	}
	chatModelID := strings.TrimSpace(botSettings.ChatModelID)
	if chatModelID == "" {
		return nil, "", errors.New("no chat model configured for bot")
	}
	modelInfo, err := p.models.GetByID(ctx, chatModelID)
	if err != nil {
		return nil, "", err
	}
	provider, err := models.FetchProviderByID(ctx, p.queries, modelInfo.LlmProviderID)
	if err != nil {
		return nil, "", err
	}
	if p.modelCreator == nil {
		return nil, "", errors.New("model creator not configured")
	}
	authResolver := providers.NewService(nil, p.queries, "")
	creds, err := authResolver.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return nil, "", err
	}
	sdkModel := p.modelCreator(
		modelInfo.ModelID,
		provider.ClientType,
		creds.APIKey,
		creds.CodexAccountID,
		provider.BaseUrl,
		nil,
	)
	return sdkModel, modelInfo.ID, nil
}

func toStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			result = append(result, s)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", v)
	}
}

func truncateTitle(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-3]) + "..."
}
