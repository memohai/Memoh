package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/db/sqlc"
	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
	subagentsvc "github.com/memohai/memoh/internal/subagent"
)

const (
	toolListSubagents  = "list_subagents"
	toolDeleteSubagent = "delete_subagent"
	toolQuerySubagent  = "query_subagent"

	gatewayTimeout = 120 * time.Second
)

type Executor struct {
	logger         *slog.Logger
	service        *subagentsvc.Service
	settings       *settings.Service
	models         *models.Service
	queries        *sqlc.Queries
	gatewayBaseURL string
	httpClient     *http.Client
}

func NewExecutor(
	log *slog.Logger,
	service *subagentsvc.Service,
	settingsSvc *settings.Service,
	modelsSvc *models.Service,
	queries *sqlc.Queries,
	gatewayBaseURL string,
) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger:         log.With(slog.String("provider", "subagent_tool")),
		service:        service,
		settings:       settingsSvc,
		models:         modelsSvc,
		queries:        queries,
		gatewayBaseURL: strings.TrimRight(gatewayBaseURL, "/"),
		httpClient:     &http.Client{Timeout: gatewayTimeout},
	}
}

func (e *Executor) ListTools(_ context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if e.service == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	if session.IsSubagent {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolListSubagents,
			Description: "List subagents for current bot",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        toolDeleteSubagent,
			Description: "Delete a subagent by id",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Subagent ID"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        toolQuerySubagent,
			Description: "Query a subagent. If the subagent does not exist it will be created automatically.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "description": "The name of the subagent"},
					"description": map[string]any{"type": "string", "description": "A short description of the subagent purpose (used when creating)"},
					"query":       map[string]any{"type": "string", "description": "The prompt to ask the subagent to do."},
				},
				"required": []string{"name", "description", "query"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if e.service == nil {
		return mcpgw.BuildToolErrorResult("subagent service not available"), nil
	}
	if session.IsSubagent {
		return mcpgw.BuildToolErrorResult("subagent tools are not available in subagent context"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	switch toolName {
	case toolListSubagents:
		return e.callList(ctx, botID)
	case toolDeleteSubagent:
		return e.callDelete(ctx, arguments)
	case toolQuerySubagent:
		return e.callQuery(ctx, session, botID, arguments)
	default:
		return nil, mcpgw.ErrToolNotFound
	}
}

func (e *Executor) callList(ctx context.Context, botID string) (map[string]any, error) {
	items, err := e.service.List(ctx, botID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"id":          item.ID,
			"name":        item.Name,
			"description": item.Description,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{"items": result}), nil
}

func (e *Executor) callDelete(ctx context.Context, arguments map[string]any) (map[string]any, error) {
	id := mcpgw.StringArg(arguments, "id")
	if id == "" {
		return mcpgw.BuildToolErrorResult("id is required"), nil
	}
	if err := e.service.Delete(ctx, id); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{"success": true}), nil
}

func (e *Executor) callQuery(ctx context.Context, session mcpgw.ToolSessionContext, botID string, arguments map[string]any) (map[string]any, error) {
	name := mcpgw.StringArg(arguments, "name")
	description := mcpgw.StringArg(arguments, "description")
	query := mcpgw.StringArg(arguments, "query")
	if name == "" || description == "" || query == "" {
		return mcpgw.BuildToolErrorResult("name, description, and query are required"), nil
	}

	target, err := e.service.GetOrCreate(ctx, botID, subagentsvc.CreateRequest{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("failed to get or create subagent: %v", err)), nil
	}

	modelCfg, provider, err := e.resolveModel(ctx, botID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("failed to resolve model: %v", err)), nil
	}

	gwResp, err := e.postSubagent(ctx, session, subagentGatewayRequest{
		Model: subagentModelConfig{
			ModelID:    modelCfg.ModelID,
			ClientType: string(modelCfg.ClientType),
			Input:      modelCfg.InputModalities,
			APIKey:     provider.ApiKey,
			BaseURL:    provider.BaseUrl,
		},
		Identity: subagentIdentity{
			BotID:             botID,
			ChannelIdentityID: session.ChannelIdentityID,
			CurrentPlatform:   session.CurrentPlatform,
			SessionToken:      session.SessionToken,
		},
		Messages: target.Messages,
		Query:    query,
		Name:     name,
		Desc:     description,
	})
	if err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("subagent query failed: %v", err)), nil
	}

	updatedMessages := append(target.Messages, gwResp.Messages...)
	usage := mergeUsage(target.Usage, gwResp.Usage)
	if _, err := e.service.UpdateContext(ctx, target.ID, subagentsvc.UpdateContextRequest{
		Messages: updatedMessages,
		Usage:    usage,
	}); err != nil {
		e.logger.Warn("failed to persist subagent context", slog.String("subagent_id", target.ID), slog.Any("error", err))
	}

	resultContent := gwResp.Text
	if resultContent == "" && len(gwResp.Messages) > 0 {
		last := gwResp.Messages[len(gwResp.Messages)-1]
		if content, ok := last["content"]; ok {
			resultContent = fmt.Sprintf("%v", content)
		}
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"success": true,
		"result":  resultContent,
	}), nil
}

func (e *Executor) resolveModel(ctx context.Context, botID string) (models.GetResponse, sqlc.LlmProvider, error) {
	if e.settings == nil || e.models == nil || e.queries == nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, errors.New("model resolution services not configured")
	}
	botSettings, err := e.settings.GetBot(ctx, botID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	chatModelID := strings.TrimSpace(botSettings.ChatModelID)
	if chatModelID == "" {
		return models.GetResponse{}, sqlc.LlmProvider{}, errors.New("no chat model configured for bot")
	}
	model, err := e.models.GetByID(ctx, chatModelID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	provider, err := models.FetchProviderByID(ctx, e.queries, model.LlmProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	return model, provider, nil
}

// --- gateway types ---

type subagentModelConfig struct {
	ModelID    string   `json:"modelId"`
	ClientType string   `json:"clientType"`
	Input      []string `json:"input"`
	APIKey     string   `json:"apiKey"` //nolint:gosec // forwarded to agent gateway
	BaseURL    string   `json:"baseUrl"`
}

type subagentIdentity struct {
	BotID             string `json:"botId"`
	ChannelIdentityID string `json:"channelIdentityId"`
	CurrentPlatform   string `json:"currentPlatform,omitempty"`
	SessionToken      string `json:"sessionToken,omitempty"` //nolint:gosec // session token forwarded to agent gateway
}

type subagentGatewayRequest struct {
	Model    subagentModelConfig `json:"model"`
	Identity subagentIdentity    `json:"identity"`
	Messages []map[string]any    `json:"messages"`
	Query    string              `json:"query"`
	Name     string              `json:"name"`
	Desc     string              `json:"description"`
}

type subagentGatewayResponse struct {
	Messages []map[string]any `json:"messages"`
	Text     string           `json:"text,omitempty"`
	Usage    json.RawMessage  `json:"usage,omitempty"`
}

func (e *Executor) postSubagent(ctx context.Context, session mcpgw.ToolSessionContext, payload subagentGatewayRequest) (subagentGatewayResponse, error) {
	url := e.gatewayBaseURL + "/chat/subagent"
	body, err := json.Marshal(payload)
	if err != nil {
		return subagentGatewayResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return subagentGatewayResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(session.SessionToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.httpClient.Do(req) //nolint:gosec // URL is from operator-configured agent gateway
	if err != nil {
		return subagentGatewayResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return subagentGatewayResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := string(respBody)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return subagentGatewayResponse{}, fmt.Errorf("agent gateway error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(detail))
	}

	var parsed subagentGatewayResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return subagentGatewayResponse{}, fmt.Errorf("failed to parse gateway response: %w", err)
	}
	return parsed, nil
}

func mergeUsage(existing map[string]any, delta json.RawMessage) map[string]any {
	if existing == nil {
		existing = map[string]any{}
	}
	if len(delta) == 0 {
		return existing
	}
	var deltaMap map[string]any
	if err := json.Unmarshal(delta, &deltaMap); err != nil {
		return existing
	}
	for key, val := range deltaMap {
		if num, ok := toFloat64(val); ok {
			if prev, ok := toFloat64(existing[key]); ok {
				existing[key] = prev + num
			} else {
				existing[key] = num
			}
		}
	}
	return existing
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
