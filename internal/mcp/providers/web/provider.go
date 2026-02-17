// Package web provides the MCP web provider (Brave Search and fetch tools).
package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
)

const (
	toolWebSearch = "web_search"
)

// Executor is the MCP tool executor for web_search (and optional fetch) using configured search provider.
type Executor struct {
	logger          *slog.Logger
	settings        *settings.Service
	searchProviders *searchproviders.Service
}

// NewExecutor creates a web tool executor.
func NewExecutor(log *slog.Logger, settingsSvc *settings.Service, searchSvc *searchproviders.Service) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger:          log.With(slog.String("provider", "web_tool")),
		settings:        settingsSvc,
		searchProviders: searchSvc,
	}
}

// ListTools returns web_search (and optionally fetch) tool descriptors.
func (p *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if p.settings == nil || p.searchProviders == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolWebSearch,
			Description: "Search web results via configured search provider.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query"},
					"count": map[string]any{"type": "integer", "description": "Number of results, default 5"},
				},
				"required": []string{"query"},
			},
		},
	}, nil
}

// CallTool runs web_search (or fetch) using the bot's configured search provider.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if p.settings == nil || p.searchProviders == nil {
		return mcpgw.BuildToolErrorResult("web tools are not available"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	searchProviderID := strings.TrimSpace(botSettings.SearchProviderID)
	if searchProviderID == "" {
		return mcpgw.BuildToolErrorResult("search provider not configured for this bot"), nil
	}
	provider, err := p.searchProviders.GetRawByID(ctx, searchProviderID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	switch toolName {
	case toolWebSearch:
		return p.callWebSearch(ctx, provider.Provider, provider.Config, arguments)
	default:
		return nil, mcpgw.ErrToolNotFound
	}
}

func (p *Executor) callWebSearch(ctx context.Context, providerName string, configJSON []byte, arguments map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(mcpgw.StringArg(arguments, "query"))
	if query == "" {
		return mcpgw.BuildToolErrorResult("query is required"), nil
	}
	count := 5
	if value, ok, err := mcpgw.IntArg(arguments, "count"); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	} else if ok && value > 0 {
		count = value
	}
	if count > 20 {
		count = 20
	}

	if strings.TrimSpace(providerName) != string(searchproviders.ProviderBrave) {
		return mcpgw.BuildToolErrorResult("unsupported search provider"), nil
	}
	cfg := parseConfig(configJSON)
	endpoint := strings.TrimRight(firstNonEmpty(stringValue(cfg["base_url"]), "https://api.search.brave.com/res/v1/web/search"), "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return mcpgw.BuildToolErrorResult("invalid search provider base_url"), nil
	}
	params := reqURL.Query()
	params.Set("q", query)
	params.Set("count", strconv.Itoa(count))
	reqURL.RawQuery = params.Encode()

	timeout := parseTimeout(configJSON, 15*time.Second)
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	req.Header.Set("Accept", "application/json")
	apiKey := stringValue(cfg["api_key"])
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("X-Subscription-Token", strings.TrimSpace(apiKey))
	}
	resp, err := client.Do(req)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			p.logger.Warn("web search: close response body failed", slog.Any("error", err))
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpgw.BuildToolErrorResult("search request failed"), nil
	}
	var raw struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return mcpgw.BuildToolErrorResult("invalid search response"), nil
	}
	results := make([]map[string]any, 0, len(raw.Web.Results))
	for _, item := range raw.Web.Results {
		results = append(results, map[string]any{
			"title":       item.Title,
			"url":         item.URL,
			"description": item.Description,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"results": results,
	}), nil
}

func parseTimeout(configJSON []byte, fallback time.Duration) time.Duration {
	cfg := parseConfig(configJSON)
	raw, ok := cfg["timeout_seconds"]
	if !ok {
		return fallback
	}
	switch value := raw.(type) {
	case float64:
		if value > 0 {
			return time.Duration(value * float64(time.Second))
		}
	case int:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	}
	return fallback
}

func parseConfig(configJSON []byte) map[string]any {
	if len(configJSON) == 0 {
		return map[string]any{}
	}
	var cfg map[string]any
	if err := json.Unmarshal(configJSON, &cfg); err != nil || cfg == nil {
		return map[string]any{}
	}
	return cfg
}

func stringValue(raw any) string {
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
