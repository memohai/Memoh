package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
	subagentsvc "github.com/memohai/memoh/internal/subagent"
	sdk "github.com/memohai/twilight-ai/sdk"
)

const subagentGatewayTimeout = 120 * time.Second

type SubagentProvider struct {
	logger         *slog.Logger
	service        *subagentsvc.Service
	settings       *settings.Service
	models         *models.Service
	queries        *sqlc.Queries
	gatewayBaseURL string
	httpClient     *http.Client
}

func NewSubagentProvider(
	log *slog.Logger,
	service *subagentsvc.Service,
	settingsSvc *settings.Service,
	modelsSvc *models.Service,
	queries *sqlc.Queries,
	gatewayBaseURL string,
) *SubagentProvider {
	if log == nil {
		log = slog.Default()
	}
	return &SubagentProvider{
		logger:         log.With(slog.String("tool", "subagent")),
		service:        service,
		settings:       settingsSvc,
		models:         modelsSvc,
		queries:        queries,
		gatewayBaseURL: strings.TrimRight(gatewayBaseURL, "/"),
		httpClient:     &http.Client{Timeout: subagentGatewayTimeout},
	}
}

func (p *SubagentProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if p.service == nil || session.IsSubagent {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name: "list_subagents", Description: "List subagents for current bot",
			Parameters: emptyObjectSchema(),
			Execute: func(ctx *sdk.ToolExecContext, _ any) (any, error) {
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, fmt.Errorf("bot_id is required")
				}
				items, err := p.service.List(ctx, botID)
				if err != nil {
					return nil, err
				}
				result := make([]map[string]any, 0, len(items))
				for _, item := range items {
					result = append(result, map[string]any{"id": item.ID, "name": item.Name, "description": item.Description})
				}
				return map[string]any{"items": result}, nil
			},
		},
		{
			Name: "delete_subagent", Description: "Delete a subagent by id",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Subagent ID"},
				},
				"required": []string{"id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				id := StringArg(args, "id")
				if id == "" {
					return nil, fmt.Errorf("id is required")
				}
				if err := p.service.Delete(ctx, id); err != nil {
					return nil, err
				}
				return map[string]any{"success": true}, nil
			},
		},
		{
			Name: "query_subagent", Description: "Query a subagent. If the subagent does not exist it will be created automatically.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "description": "The name of the subagent"},
					"description": map[string]any{"type": "string", "description": "A short description of the subagent purpose (used when creating)"},
					"query":       map[string]any{"type": "string", "description": "The prompt to ask the subagent to do."},
				},
				"required": []string{"name", "description", "query"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execQuery(ctx, sess, inputAsMap(input))
			},
		},
	}, nil
}

func (p *SubagentProvider) execQuery(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, fmt.Errorf("bot_id is required")
	}
	name := StringArg(args, "name")
	description := StringArg(args, "description")
	query := StringArg(args, "query")
	if name == "" || description == "" || query == "" {
		return nil, fmt.Errorf("name, description, and query are required")
	}
	target, err := p.service.GetOrCreate(ctx, botID, subagentsvc.CreateRequest{Name: name, Description: description})
	if err != nil {
		return nil, fmt.Errorf("failed to get or create subagent: %v", err)
	}
	modelCfg, provider, err := p.resolveModel(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model: %v", err)
	}
	gwResp, err := p.postSubagent(ctx, session, subagentGWRequest{
		Model: subagentModelCfg{
			ModelID: modelCfg.ModelID, ClientType: string(modelCfg.ClientType),
			Input: modelCfg.InputModalities, APIKey: provider.ApiKey, BaseURL: provider.BaseUrl,
		},
		Identity: subagentIdentityCfg{
			BotID: botID, ChannelIdentityID: session.ChannelIdentityID,
			CurrentPlatform: session.CurrentPlatform, SessionToken: session.SessionToken,
		},
		Messages: target.Messages, Query: query, Name: name, Desc: description,
	})
	if err != nil {
		return nil, fmt.Errorf("subagent query failed: %v", err)
	}
	updatedMessages := slices.Clone(target.Messages)
	updatedMessages = append(updatedMessages, gwResp.Messages...)
	usage := mergeSubagentUsage(target.Usage, gwResp.Usage)
	if _, err := p.service.UpdateContext(ctx, target.ID, subagentsvc.UpdateContextRequest{
		Messages: updatedMessages, Usage: usage,
	}); err != nil {
		p.logger.Warn("failed to persist subagent context", slog.String("subagent_id", target.ID), slog.Any("error", err))
	}
	resultContent := gwResp.Text
	if resultContent == "" && len(gwResp.Messages) > 0 {
		last := gwResp.Messages[len(gwResp.Messages)-1]
		if content, ok := last["content"]; ok {
			resultContent = fmt.Sprintf("%v", content)
		}
	}
	return map[string]any{"success": true, "result": resultContent}, nil
}

func (p *SubagentProvider) resolveModel(ctx context.Context, botID string) (models.GetResponse, sqlc.LlmProvider, error) {
	if p.settings == nil || p.models == nil || p.queries == nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, errors.New("model resolution services not configured")
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	chatModelID := strings.TrimSpace(botSettings.ChatModelID)
	if chatModelID == "" {
		return models.GetResponse{}, sqlc.LlmProvider{}, errors.New("no chat model configured for bot")
	}
	model, err := p.models.GetByID(ctx, chatModelID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	provider, err := models.FetchProviderByID(ctx, p.queries, model.LlmProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	return model, provider, nil
}

type subagentModelCfg struct {
	ModelID    string   `json:"modelId"`
	ClientType string   `json:"clientType"`
	Input      []string `json:"input"`
	APIKey     string   `json:"apiKey"`     //nolint:gosec // forwarded to agent gateway
	BaseURL    string   `json:"baseUrl"`
}

type subagentIdentityCfg struct {
	BotID             string `json:"botId"`
	ChannelIdentityID string `json:"channelIdentityId"`
	CurrentPlatform   string `json:"currentPlatform,omitempty"`
	SessionToken      string `json:"sessionToken,omitempty"` //nolint:gosec // session token forwarded
}

type subagentGWRequest struct {
	Model    subagentModelCfg    `json:"model"`
	Identity subagentIdentityCfg `json:"identity"`
	Messages []map[string]any    `json:"messages"`
	Query    string              `json:"query"`
	Name     string              `json:"name"`
	Desc     string              `json:"description"`
}

type subagentGWResponse struct {
	Messages []map[string]any `json:"messages"`
	Text     string           `json:"text,omitempty"`
	Usage    json.RawMessage  `json:"usage,omitempty"`
}

func (p *SubagentProvider) postSubagent(ctx context.Context, session SessionContext, payload subagentGWRequest) (subagentGWResponse, error) {
	url := p.gatewayBaseURL + "/chat/subagent"
	body, err := json.Marshal(payload)
	if err != nil {
		return subagentGWResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return subagentGWResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(session.SessionToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.httpClient.Do(req) //nolint:gosec // URL is from operator-configured agent gateway
	if err != nil {
		return subagentGWResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return subagentGWResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := string(respBody)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return subagentGWResponse{}, fmt.Errorf("agent gateway error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(detail))
	}
	var parsed subagentGWResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return subagentGWResponse{}, fmt.Errorf("failed to parse gateway response: %w", err)
	}
	return parsed, nil
}

func mergeSubagentUsage(existing map[string]any, delta json.RawMessage) map[string]any {
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
