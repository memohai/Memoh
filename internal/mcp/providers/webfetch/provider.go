package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	readability "github.com/go-shiori/go-readability"

	mcpgw "github.com/memohai/memoh/internal/mcp"
)

const (
	toolWebFetch   = "web_fetch"
	maxTextContent = 10000
	fetchTimeout   = 30 * time.Second
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
)

type Executor struct {
	logger *slog.Logger
	client *http.Client
}

func NewExecutor(log *slog.Logger) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger: log.With(slog.String("provider", "webfetch_tool")),
		client: &http.Client{Timeout: fetchTimeout},
	}
}

func (e *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolWebFetch,
			Description: "Fetch a URL and convert the response to readable content. Supports HTML (converts to Markdown), JSON, XML, and plain text formats.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to fetch",
					},
					"format": map[string]any{
						"type":        "string",
						"enum":        []string{"auto", "markdown", "json", "xml", "text"},
						"description": "Output format (default: auto - detects from content type)",
					},
				},
				"required": []string{"url"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, _ mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolWebFetch {
		return nil, mcpgw.ErrToolNotFound
	}

	rawURL := strings.TrimSpace(mcpgw.StringArg(arguments, "url"))
	if rawURL == "" {
		return mcpgw.BuildToolErrorResult("url is required"), nil
	}
	format := strings.TrimSpace(mcpgw.StringArg(arguments, "format"))
	if format == "" {
		format = "auto"
	}

	return e.callWebFetch(ctx, rawURL, format)
}

func (e *Executor) callWebFetch(ctx context.Context, rawURL, format string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid url: %v", err)), nil
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := e.client.Do(req) //nolint:gosec // intentionally fetches user-specified URLs
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("HTTP error: %d %s", resp.StatusCode, resp.Status)), nil
	}

	contentType := resp.Header.Get("Content-Type")
	detected := format
	if format == "auto" {
		detected = detectFormat(contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	switch detected {
	case "json":
		return e.processJSON(rawURL, contentType, body)
	case "xml":
		return e.processXML(rawURL, contentType, body)
	case "markdown":
		return e.processHTML(rawURL, contentType, body)
	default:
		return e.processText(rawURL, contentType, body)
	}
}

func detectFormat(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "application/json"):
		return "json"
	case strings.Contains(ct, "application/xml"), strings.Contains(ct, "text/xml"):
		return "xml"
	case strings.Contains(ct, "text/html"):
		return "markdown"
	default:
		return "text"
	}
}

func (e *Executor) processJSON(fetchedURL, contentType string, body []byte) (map[string]any, error) {
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return mcpgw.BuildToolErrorResult("Failed to parse JSON"), nil
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"success":     true,
		"url":         fetchedURL,
		"format":      "json",
		"contentType": contentType,
		"data":        data,
	}), nil
}

func (e *Executor) processXML(fetchedURL, contentType string, body []byte) (map[string]any, error) {
	content := string(body)
	if len(content) > maxTextContent {
		content = content[:maxTextContent]
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"success":     true,
		"url":         fetchedURL,
		"format":      "xml",
		"contentType": contentType,
		"content":     content,
	}), nil
}

func (e *Executor) processHTML(fetchedURL, contentType string, body []byte) (map[string]any, error) {
	parsed, err := url.Parse(fetchedURL)
	if err != nil {
		parsed = &url.URL{}
	}

	article, err := readability.FromReader(strings.NewReader(string(body)), parsed)
	if err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("Failed to extract readable content from HTML: %v", err)), nil
	}

	if strings.TrimSpace(article.Content) == "" {
		return mcpgw.BuildToolErrorResult("Failed to extract readable content from HTML"), nil
	}

	markdown, err := htmltomarkdown.ConvertString(article.Content)
	if err != nil {
		e.logger.Warn("html-to-markdown conversion failed, falling back to text", slog.Any("error", err))
		markdown = article.TextContent
	}

	textPreview := article.TextContent
	if len(textPreview) > 500 {
		textPreview = textPreview[:500]
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"success":     true,
		"url":         fetchedURL,
		"format":      "markdown",
		"contentType": contentType,
		"title":       article.Title,
		"byline":      article.Byline,
		"excerpt":     article.Excerpt,
		"content":     markdown,
		"textContent": textPreview,
		"length":      article.Length,
	}), nil
}

func (e *Executor) processText(fetchedURL, contentType string, body []byte) (map[string]any, error) {
	content := string(body)
	length := len(content)
	if length > maxTextContent {
		content = content[:maxTextContent]
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"success":     true,
		"url":         fetchedURL,
		"format":      "text",
		"contentType": contentType,
		"content":     content,
		"length":      length,
	}), nil
}
