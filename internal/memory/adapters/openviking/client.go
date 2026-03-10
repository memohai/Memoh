package openviking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
)

type openVikingClient struct {
	baseURL    string
	apiKey     string
	memoryRoot string
	httpClient *http.Client
}

func newOpenVikingClient(config map[string]any) (*openVikingClient, error) {
	baseURL := adapters.StringFromConfig(config, "base_url")
	if baseURL == "" {
		return nil, fmt.Errorf("openviking: base_url is required")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	memoryRoot := adapters.StringFromConfig(config, "memory_root")
	if memoryRoot == "" {
		memoryRoot = "viking://agent/"
	}
	return &openVikingClient{
		baseURL:    baseURL,
		apiKey:     adapters.StringFromConfig(config, "api_key"),
		memoryRoot: strings.TrimRight(memoryRoot, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

type ovMemory struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Score     float64        `json:"score,omitempty"`
}

type ovAddRequest struct {
	AgentID string `json:"agent_id"`
	Content string `json:"content"`
}

type ovSearchRequest struct {
	Query   string `json:"query"`
	AgentID string `json:"agent_id"`
	Limit   int    `json:"limit,omitempty"`
}

type ovUpdateRequest struct {
	Content string `json:"content"`
}

func (c *openVikingClient) Add(ctx context.Context, agentID, content string) (*ovMemory, error) {
	var result ovMemory
	if err := c.doJSON(ctx, http.MethodPost, "/memories", ovAddRequest{
		AgentID: agentID,
		Content: content,
	}, &result); err != nil {
		return nil, fmt.Errorf("openviking add: %w", err)
	}
	return &result, nil
}

func (c *openVikingClient) Search(ctx context.Context, agentID, query string, limit int) ([]ovMemory, error) {
	var results []ovMemory
	if err := c.doJSON(ctx, http.MethodPost, "/memories/search", ovSearchRequest{
		Query:   query,
		AgentID: agentID,
		Limit:   limit,
	}, &results); err != nil {
		return nil, fmt.Errorf("openviking search: %w", err)
	}
	return results, nil
}

func (c *openVikingClient) GetAll(ctx context.Context, agentID string, limit int) ([]ovMemory, error) {
	url := "/memories?agent_id=" + agentID
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}
	var results []ovMemory
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &results); err != nil {
		return nil, fmt.Errorf("openviking get all: %w", err)
	}
	return results, nil
}

func (c *openVikingClient) Update(ctx context.Context, memoryID, content string) (*ovMemory, error) {
	var result ovMemory
	if err := c.doJSON(ctx, http.MethodPut, "/memories/"+memoryID, ovUpdateRequest{Content: content}, &result); err != nil {
		return nil, fmt.Errorf("openviking update: %w", err)
	}
	return &result, nil
}

func (c *openVikingClient) Delete(ctx context.Context, memoryID string) error {
	if err := c.doJSON(ctx, http.MethodDelete, "/memories/"+memoryID, nil, nil); err != nil {
		return fmt.Errorf("openviking delete: %w", err)
	}
	return nil
}

func (c *openVikingClient) DeleteAll(ctx context.Context, agentID string) error {
	url := "/memories?agent_id=" + agentID
	if err := c.doJSON(ctx, http.MethodDelete, url, nil, nil); err != nil {
		return fmt.Errorf("openviking delete all: %w", err)
	}
	return nil
}

func (c *openVikingClient) doJSON(ctx context.Context, method, urlPath string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+urlPath, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openviking API error %d: %s", resp.StatusCode, truncateBody(respBody))
	}
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

func truncateBody(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "..."
	}
	return string(b)
}
