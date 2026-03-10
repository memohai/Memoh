package mem0

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

type mem0Client struct {
	baseURL    string
	apiKey     string
	orgID      string
	projectID  string
	httpClient *http.Client
}

func newMem0Client(config map[string]any) (*mem0Client, error) {
	baseURL := adapters.StringFromConfig(config, "base_url")
	if baseURL == "" {
		return nil, fmt.Errorf("mem0: base_url is required")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &mem0Client{
		baseURL:   baseURL,
		apiKey:    adapters.StringFromConfig(config, "api_key"),
		orgID:     adapters.StringFromConfig(config, "org_id"),
		projectID: adapters.StringFromConfig(config, "project_id"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

type mem0AddRequest struct {
	Messages []adapters.Message `json:"messages,omitempty"`
	UserID   string             `json:"user_id,omitempty"`
	AgentID  string             `json:"agent_id,omitempty"`
	RunID    string             `json:"run_id,omitempty"`
	Metadata map[string]any     `json:"metadata,omitempty"`
}

type mem0AddResponse struct {
	Results []mem0Memory `json:"results"`
}

type mem0Memory struct {
	ID        string         `json:"id"`
	Memory    string         `json:"memory"`
	Hash      string         `json:"hash,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type mem0SearchRequest struct {
	Query   string `json:"query"`
	UserID  string `json:"user_id,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type mem0UpdateRequest struct {
	Text string `json:"text"`
}

func (c *mem0Client) Add(ctx context.Context, req mem0AddRequest) ([]mem0Memory, error) {
	var resp mem0AddResponse
	if err := c.doJSON(ctx, http.MethodPost, "/memories", req, &resp); err != nil {
		return nil, fmt.Errorf("mem0 add: %w", err)
	}
	return resp.Results, nil
}

func (c *mem0Client) Search(ctx context.Context, req mem0SearchRequest) ([]mem0Memory, error) {
	var results []mem0Memory
	if err := c.doJSON(ctx, http.MethodPost, "/memories/search", req, &results); err != nil {
		return nil, fmt.Errorf("mem0 search: %w", err)
	}
	return results, nil
}

func (c *mem0Client) GetAll(ctx context.Context, agentID string, limit int) ([]mem0Memory, error) {
	url := "/memories?agent_id=" + agentID
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}
	var results []mem0Memory
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &results); err != nil {
		return nil, fmt.Errorf("mem0 get all: %w", err)
	}
	return results, nil
}

func (c *mem0Client) Update(ctx context.Context, memoryID string, text string) (*mem0Memory, error) {
	var result mem0Memory
	if err := c.doJSON(ctx, http.MethodPut, "/memories/"+memoryID, mem0UpdateRequest{Text: text}, &result); err != nil {
		return nil, fmt.Errorf("mem0 update: %w", err)
	}
	return &result, nil
}

func (c *mem0Client) Delete(ctx context.Context, memoryID string) error {
	if err := c.doJSON(ctx, http.MethodDelete, "/memories/"+memoryID, nil, nil); err != nil {
		return fmt.Errorf("mem0 delete: %w", err)
	}
	return nil
}

func (c *mem0Client) DeleteAll(ctx context.Context, agentID string) error {
	url := "/memories?agent_id=" + agentID
	if err := c.doJSON(ctx, http.MethodDelete, url, nil, nil); err != nil {
		return fmt.Errorf("mem0 delete all: %w", err)
	}
	return nil
}

func (c *mem0Client) doJSON(ctx context.Context, method, urlPath string, body any, result any) error {
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
		req.Header.Set("Authorization", "Token "+c.apiKey)
	}
	if c.orgID != "" {
		req.Header.Set("X-Org-Id", c.orgID)
	}
	if c.projectID != "" {
		req.Header.Set("X-Project-Id", c.projectID)
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
		return fmt.Errorf("mem0 API error %d: %s", resp.StatusCode, truncateBody(respBody))
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
