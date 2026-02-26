package web

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
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
	case string(searchproviders.ProviderSogou):
		return p.callSogouSearch(ctx, configJSON, query, count)
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
		return buildSearchHTTPError(resp.StatusCode, body), nil
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
		return buildSearchHTTPError(resp.StatusCode, body), nil
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
		return buildSearchHTTPError(resp.StatusCode, body), nil
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

func (p *Executor) callSogouSearch(ctx context.Context, configJSON []byte, query string, count int) (map[string]any, error) {
	cfg := parseConfig(configJSON)
	host := firstNonEmpty(stringValue(cfg["base_url"]), "wsa.tencentcloudapi.com")
	secretID := stringValue(cfg["secret_id"])
	secretKey := stringValue(cfg["secret_key"])
	if secretID == "" || secretKey == "" {
		return mcpgw.BuildToolErrorResult("Sogou search requires Tencent Cloud SecretId and SecretKey"), nil
	}

	action := "SearchPro"
	version := "2025-05-08"
	service := "wsa"
	payload, _ := json.Marshal(map[string]any{
		"Query": query,
		"Mode":  0,
	})

	now := time.Now().UTC()
	timestamp := fmt.Sprintf("%d", now.Unix())
	date := now.Format("2006-01-02")

	hashedPayload := sha256Hex(payload)
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\n",
		"application/json", host)
	signedHeaders := "content-type;host"
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		"POST", "/", "", canonicalHeaders, signedHeaders, hashedPayload)

	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%s\n%s\n%s",
		timestamp, credentialScope, sha256Hex([]byte(canonicalRequest)))

	secretDate := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	secretService := hmacSHA256(secretDate, []byte(service))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSHA256(secretSigning, []byte(stringToSign)))

	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, credentialScope, signedHeaders, signature)

	timeout := parseTimeout(configJSON, 15*time.Second)
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/", bytes.NewReader(payload))
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", timestamp)
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
		return buildSearchHTTPError(resp.StatusCode, body), nil
	}
	var rawResp struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error,omitempty"`
			Pages []json.RawMessage `json:"Pages"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(body, &rawResp); err != nil {
		return mcpgw.BuildToolErrorResult("invalid search response"), nil
	}
	if rawResp.Response.Error != nil {
		return mcpgw.BuildToolErrorResult("Sogou search failed: " + rawResp.Response.Error.Message), nil
	}

	type sogouPage struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Passage string  `json:"passage"`
		Score   float64 `json:"scour"`
	}
	var pages []sogouPage
	for _, raw := range rawResp.Response.Pages {
		var rawStr string
		if err := json.Unmarshal(raw, &rawStr); err == nil {
			var page sogouPage
			if err := json.Unmarshal([]byte(rawStr), &page); err == nil {
				pages = append(pages, page)
			}
		} else {
			var page sogouPage
			if err := json.Unmarshal(raw, &page); err == nil {
				pages = append(pages, page)
			}
		}
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Score > pages[j].Score
	})
	results := make([]map[string]any, 0, len(pages))
	for i, page := range pages {
		if i >= count {
			break
		}
		results = append(results, map[string]any{
			"title":       page.Title,
			"url":         page.URL,
			"description": page.Passage,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"results": results,
	}), nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// buildSearchHTTPError builds an error result for non-2xx search API responses.
// It includes the HTTP status code and attempts to extract a brief error detail
// from the response body (capped at 200 characters to avoid context blowout).
func buildSearchHTTPError(statusCode int, body []byte) map[string]any {
	detail := extractJSONErrorMessage(body)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if len(detail) > 200 {
		detail = detail[:200] + "..."
	}
	if detail != "" {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("search request failed (HTTP %d): %s", statusCode, detail))
	}
	return mcpgw.BuildToolErrorResult(fmt.Sprintf("search request failed (HTTP %d)", statusCode))
}

// extractJSONErrorMessage probes common JSON error response patterns and returns
// the first human-readable message found, or "" if none.
func extractJSONErrorMessage(body []byte) string {
	var obj map[string]any
	if json.Unmarshal(body, &obj) != nil {
		return ""
	}
	for _, key := range []string{"error", "message", "detail", "error_message"} {
		v, ok := obj[key]
		if !ok {
			continue
		}
		switch val := v.(type) {
		case string:
			return val
		case map[string]any:
			if msg, ok := val["message"].(string); ok {
				return msg
			}
		}
	}
	return ""
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
