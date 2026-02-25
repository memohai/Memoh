package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
)

const (
	toolWebSearch = "web_search"
)

type Executor struct {
	logger          *slog.Logger
	settings        *settings.Service
	searchProviders *searchproviders.Service
}

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

func (p *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
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

	switch strings.TrimSpace(providerName) {
	case string(searchproviders.ProviderBrave):
		return p.callBraveSearch(ctx, configJSON, query, count)
	case string(searchproviders.ProviderBing):
		return p.callBingSearch(ctx, configJSON, query, count)
	case string(searchproviders.ProviderGoogle):
		return p.callGoogleSearch(ctx, configJSON, query, count)
	case string(searchproviders.ProviderDuckDuckGo):
		return p.callDuckDuckGoSearch(ctx, configJSON, query, count)
	default:
		return mcpgw.BuildToolErrorResult("unsupported search provider"), nil
	}
}

func (p *Executor) callBraveSearch(ctx context.Context, configJSON []byte, query string, count int) (map[string]any, error) {
	cfg := parseConfig(configJSON)
	endpoint := strings.TrimRight(firstNonEmpty(stringValue(cfg["base_url"]), "https://api.search.brave.com/res/v1/web/search"), "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return mcpgw.BuildToolErrorResult("invalid search provider base_url"), nil
	}
	params := reqURL.Query()
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", count))
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
	defer resp.Body.Close()
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

func (p *Executor) callBingSearch(ctx context.Context, configJSON []byte, query string, count int) (map[string]any, error) {
	cfg := parseConfig(configJSON)
	endpoint := strings.TrimRight(firstNonEmpty(stringValue(cfg["base_url"]), "https://api.bing.microsoft.com/v7.0/search"), "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return mcpgw.BuildToolErrorResult("invalid search provider base_url"), nil
	}
	params := reqURL.Query()
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", count))
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
		req.Header.Set("Ocp-Apim-Subscription-Key", strings.TrimSpace(apiKey))
	}
	resp, err := client.Do(req)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpgw.BuildToolErrorResult("search request failed"), nil
	}
	var raw struct {
		WebPages struct {
			Value []struct {
				Name    string `json:"name"`
				URL     string `json:"url"`
				Snippet string `json:"snippet"`
			} `json:"value"`
		} `json:"webPages"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return mcpgw.BuildToolErrorResult("invalid search response"), nil
	}
	results := make([]map[string]any, 0, len(raw.WebPages.Value))
	for _, item := range raw.WebPages.Value {
		results = append(results, map[string]any{
			"title":       item.Name,
			"url":         item.URL,
			"description": item.Snippet,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"results": results,
	}), nil
}

func (p *Executor) callGoogleSearch(ctx context.Context, configJSON []byte, query string, count int) (map[string]any, error) {
	cfg := parseConfig(configJSON)
	endpoint := strings.TrimRight(firstNonEmpty(stringValue(cfg["base_url"]), "https://customsearch.googleapis.com/customsearch/v1"), "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return mcpgw.BuildToolErrorResult("invalid search provider base_url"), nil
	}
	cx := stringValue(cfg["cx"])
	if cx == "" {
		return mcpgw.BuildToolErrorResult("Google Custom Search requires cx (Search Engine ID)"), nil
	}
	if count > 10 {
		count = 10
	}
	params := reqURL.Query()
	params.Set("q", query)
	params.Set("cx", cx)
	params.Set("num", fmt.Sprintf("%d", count))
	apiKey := stringValue(cfg["api_key"])
	if strings.TrimSpace(apiKey) != "" {
		params.Set("key", strings.TrimSpace(apiKey))
	}
	reqURL.RawQuery = params.Encode()

	timeout := parseTimeout(configJSON, 15*time.Second)
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpgw.BuildToolErrorResult("search request failed"), nil
	}
	var raw struct {
		Items []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return mcpgw.BuildToolErrorResult("invalid search response"), nil
	}
	results := make([]map[string]any, 0, len(raw.Items))
	for _, item := range raw.Items {
		results = append(results, map[string]any{
			"title":       item.Title,
			"url":         item.Link,
			"description": item.Snippet,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"results": results,
	}), nil
}

func (p *Executor) callDuckDuckGoSearch(ctx context.Context, configJSON []byte, query string, count int) (map[string]any, error) {
	cfg := parseConfig(configJSON)
	endpoint := firstNonEmpty(stringValue(cfg["base_url"]), "https://html.duckduckgo.com/html/")

	timeout := parseTimeout(configJSON, 15*time.Second)
	client := &http.Client{Timeout: timeout}
	form := url.Values{}
	form.Set("q", query)
	form.Set("b", "")
	form.Set("kl", "")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpgw.BuildToolErrorResult("search request failed"), nil
	}

	htmlStr := string(body)
	links := ddgResultLinkRe.FindAllStringSubmatch(htmlStr, -1)
	titles := ddgResultTitleRe.FindAllStringSubmatch(htmlStr, -1)
	snippets := ddgResultSnippetRe.FindAllStringSubmatch(htmlStr, -1)

	n := len(links)
	if len(titles) < n {
		n = len(titles)
	}
	if count < n {
		n = count
	}

	results := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		rawURL := html.UnescapeString(links[i][1])
		realURL := extractDDGURL(rawURL)
		title := html.UnescapeString(strings.TrimSpace(titles[i][1]))
		snippet := ""
		if i < len(snippets) {
			snippet = html.UnescapeString(strings.TrimSpace(ddgHTMLTagRe.ReplaceAllString(snippets[i][1], "")))
		}
		if realURL == "" {
			continue
		}
		results = append(results, map[string]any{
			"title":       title,
			"url":         realURL,
			"description": snippet,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"results": results,
	}), nil
}

var (
	ddgResultLinkRe    = regexp.MustCompile(`class="result__a"[^>]*href="([^"]+)"`)
	ddgResultTitleRe   = regexp.MustCompile(`class="result__a"[^>]*>([^<]+)<`)
	ddgResultSnippetRe = regexp.MustCompile(`class="result__snippet"[^>]*>([\s\S]*?)</a>`)
	ddgHTMLTagRe       = regexp.MustCompile(`<[^>]*>`)
)

func extractDDGURL(rawURL string) string {
	if strings.Contains(rawURL, "uddg=") {
		parsed, err := url.Parse(rawURL)
		if err == nil {
			if uddg := parsed.Query().Get("uddg"); uddg != "" {
				return uddg
			}
		}
	}
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	return rawURL
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
