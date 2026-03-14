package mem0

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
)

const (
	mem0DefaultBaseURL     = "https://api.mem0.ai"
	mem0OutputFormatV11    = "v1.1"
	mem0VersionV2          = "v2"
	mem0BatchDeleteMaxSize = 1000
	mem0ListPageSize       = 1000
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
		baseURL = mem0DefaultBaseURL
	}
	apiKey := adapters.StringFromConfig(config, "api_key")
	if apiKey == "" {
		return nil, errors.New("mem0: api_key is required for SaaS")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &mem0Client{
		baseURL:   baseURL,
		apiKey:    apiKey,
		orgID:     adapters.StringFromConfig(config, "org_id"),
		projectID: adapters.StringFromConfig(config, "project_id"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

type mem0AddRequest struct {
	Messages     []adapters.Message `json:"messages,omitempty"`
	UserID       string             `json:"user_id,omitempty"`
	AgentID      string             `json:"agent_id,omitempty"`
	RunID        string             `json:"run_id,omitempty"`
	Metadata     map[string]any     `json:"metadata,omitempty"`
	Infer        *bool              `json:"infer,omitempty"`
	AsyncMode    *bool              `json:"async_mode,omitempty"`
	OutputFormat string             `json:"output_format,omitempty"`
	Version      string             `json:"version,omitempty"`
	OrgID        string             `json:"org_id,omitempty"`
	ProjectID    string             `json:"project_id,omitempty"`
}

type mem0AddEvent struct {
	ID    string `json:"id"`
	Event string `json:"event,omitempty"`
	Data  struct {
		Memory string `json:"memory,omitempty"`
		Text   string `json:"text,omitempty"`
	} `json:"data"`
}

type mem0Memory struct {
	ID        string         `json:"id"`
	Memory    string         `json:"memory"`
	Text      string         `json:"text,omitempty"`
	Hash      string         `json:"hash,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Score     float64        `json:"score,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type mem0SearchRequest struct {
	Query     string         `json:"query"`
	Version   string         `json:"version,omitempty"`
	Filters   map[string]any `json:"filters,omitempty"`
	TopK      int            `json:"top_k,omitempty"`
	OrgID     string         `json:"org_id,omitempty"`
	ProjectID string         `json:"project_id,omitempty"`
}

type mem0GetAllRequest struct {
	Filters      map[string]any `json:"filters"`
	Page         int            `json:"page,omitempty"`
	PageSize     int            `json:"page_size,omitempty"`
	OutputFormat string         `json:"output_format,omitempty"`
	OrgID        string         `json:"org_id,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
}

type mem0UpdateRequest struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type mem0MemoriesResponse struct {
	Results   []mem0Memory `json:"results"`
	Memories  []mem0Memory `json:"memories"`
	Total     int          `json:"total,omitempty"`
	Relations []any        `json:"relations,omitempty"`
}

type mem0AddEventsResponse struct {
	Results []mem0AddEvent `json:"results"`
}

func (c *mem0Client) Add(ctx context.Context, req mem0AddRequest) ([]mem0Memory, error) {
	if req.OutputFormat == "" {
		req.OutputFormat = mem0OutputFormatV11
	}
	if req.Version == "" {
		req.Version = mem0VersionV2
	}
	req.OrgID = c.orgID
	req.ProjectID = c.projectID
	body, err := c.doJSONRaw(ctx, http.MethodPost, "/v1/memories/", req)
	if err != nil {
		return nil, fmt.Errorf("mem0 add: %w", err)
	}
	memories, err := parseMem0AddMemories(body)
	if err != nil {
		return nil, fmt.Errorf("mem0 add: %w", err)
	}
	return memories, nil
}

func (c *mem0Client) Search(ctx context.Context, req mem0SearchRequest) ([]mem0Memory, error) {
	if req.Version == "" {
		req.Version = mem0VersionV2
	}
	req.OrgID = c.orgID
	req.ProjectID = c.projectID
	body, err := c.doJSONRaw(ctx, http.MethodPost, "/v2/memories/search/", req)
	if err != nil {
		return nil, fmt.Errorf("mem0 search: %w", err)
	}
	results, err := parseMem0Memories(body)
	if err != nil {
		return nil, fmt.Errorf("mem0 search: %w", err)
	}
	return results, nil
}

func (c *mem0Client) GetAll(ctx context.Context, req mem0GetAllRequest) ([]mem0Memory, error) {
	req.OrgID = c.orgID
	req.ProjectID = c.projectID
	if req.OutputFormat == "" {
		req.OutputFormat = mem0OutputFormatV11
	}
	body, err := c.doJSONRaw(ctx, http.MethodPost, "/v2/memories/", req)
	if err != nil {
		return nil, fmt.Errorf("mem0 get all: %w", err)
	}
	results, err := parseMem0Memories(body)
	if err != nil {
		return nil, fmt.Errorf("mem0 get all: %w", err)
	}
	return results, nil
}

func (c *mem0Client) ListAllByAgent(ctx context.Context, agentID string) ([]mem0Memory, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent_id is required")
	}
	all := make([]mem0Memory, 0)
	seen := map[string]struct{}{}
	for page := 1; ; page++ {
		results, err := c.GetAll(ctx, mem0GetAllRequest{
			Filters: map[string]any{
				"agent_id": agentID,
			},
			Page:     page,
			PageSize: mem0ListPageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range results {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			all = append(all, item)
		}
		if len(results) < mem0ListPageSize {
			break
		}
	}
	return all, nil
}

func (c *mem0Client) Update(ctx context.Context, memoryID string, text string, metadata map[string]any) (*mem0Memory, error) {
	var result mem0Memory
	if err := c.doJSON(ctx, http.MethodPut, "/v1/memories/"+memoryID+"/", mem0UpdateRequest{
		Text:     text,
		Metadata: metadata,
	}, &result); err != nil {
		return nil, fmt.Errorf("mem0 update: %w", err)
	}
	result = normalizeMem0Memory(result)
	return &result, nil
}

func (c *mem0Client) Delete(ctx context.Context, memoryID string) error {
	if err := c.doJSON(ctx, http.MethodDelete, "/v1/memories/"+memoryID+"/", nil, nil); err != nil {
		return fmt.Errorf("mem0 delete: %w", err)
	}
	return nil
}

func (c *mem0Client) DeleteAll(ctx context.Context, agentID string) error {
	q := url.Values{}
	q.Set("agent_id", agentID)
	if c.orgID != "" {
		q.Set("org_id", c.orgID)
	}
	if c.projectID != "" {
		q.Set("project_id", c.projectID)
	}
	u := "/v1/memories/?" + q.Encode()
	if err := c.doJSON(ctx, http.MethodDelete, u, nil, nil); err != nil {
		return fmt.Errorf("mem0 delete all: %w", err)
	}
	return nil
}

func (c *mem0Client) BatchDelete(ctx context.Context, memoryIDs []string) error {
	if len(memoryIDs) == 0 {
		return nil
	}
	if len(memoryIDs) > mem0BatchDeleteMaxSize {
		return fmt.Errorf("mem0 batch delete: maximum %d memories allowed", mem0BatchDeleteMaxSize)
	}
	memories := make([]map[string]string, 0, len(memoryIDs))
	for _, id := range memoryIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		memories = append(memories, map[string]string{"memory_id": id})
	}
	if len(memories) == 0 {
		return nil
	}
	ids := make([]string, 0, len(memories))
	for _, memory := range memories {
		ids = append(ids, memory["memory_id"])
	}
	if err := c.doJSON(ctx, http.MethodDelete, "/v1/batch/", map[string]any{
		"memory_ids": ids,
		"memories":   memories,
	}, nil); err != nil {
		return fmt.Errorf("mem0 batch delete: %w", err)
	}
	return nil
}

func (c *mem0Client) doJSON(ctx context.Context, method, urlPath string, body any, result any) error {
	respBody, err := c.doJSONRaw(ctx, method, urlPath, body)
	if err != nil {
		return err
	}
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

func (c *mem0Client) doJSONRaw(ctx context.Context, method, urlPath string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+urlPath, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+c.apiKey)
	if c.orgID != "" {
		req.Header.Set("X-Org-Id", c.orgID)
	}
	if c.projectID != "" {
		req.Header.Set("X-Project-Id", c.projectID)
	}
	resp, err := c.httpClient.Do(req) //nolint:gosec // URL is from admin-configured base_url
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mem0 API error %d: %s", resp.StatusCode, truncateBody(respBody))
	}
	return respBody, nil
}

func parseMem0AddMemories(body []byte) ([]mem0Memory, error) {
	memories, err := parseMem0Memories(body)
	if err == nil && hasConcreteMem0Memories(memories) {
		return memories, nil
	}

	var envelope mem0AddEventsResponse
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Results) > 0 {
		return mem0EventsToMemories(envelope.Results), nil
	}

	var events []mem0AddEvent
	if err := json.Unmarshal(body, &events); err == nil && len(events) > 0 {
		return mem0EventsToMemories(events), nil
	}

	if err == nil {
		return memories, nil
	}
	return nil, err
}

func parseMem0Memories(body []byte) ([]mem0Memory, error) {
	var list []mem0Memory
	if err := json.Unmarshal(body, &list); err == nil {
		return normalizeMem0Memories(list), nil
	}

	var envelope mem0MemoriesResponse
	if err := json.Unmarshal(body, &envelope); err == nil {
		switch {
		case len(envelope.Results) > 0:
			return normalizeMem0Memories(envelope.Results), nil
		case len(envelope.Memories) > 0:
			return normalizeMem0Memories(envelope.Memories), nil
		default:
			return []mem0Memory{}, nil
		}
	}

	return nil, errors.New("unsupported mem0 response shape")
}

func mem0EventsToMemories(events []mem0AddEvent) []mem0Memory {
	memories := make([]mem0Memory, 0, len(events))
	for _, event := range events {
		memory := strings.TrimSpace(event.Data.Memory)
		if memory == "" {
			memory = strings.TrimSpace(event.Data.Text)
		}
		memories = append(memories, mem0Memory{
			ID:     strings.TrimSpace(event.ID),
			Memory: memory,
		})
	}
	return memories
}

func normalizeMem0Memories(memories []mem0Memory) []mem0Memory {
	for i := range memories {
		memories[i] = normalizeMem0Memory(memories[i])
	}
	return memories
}

func normalizeMem0Memory(memory mem0Memory) mem0Memory {
	if strings.TrimSpace(memory.Memory) == "" {
		memory.Memory = strings.TrimSpace(memory.Text)
	}
	return memory
}

func hasConcreteMem0Memories(memories []mem0Memory) bool {
	for _, memory := range memories {
		if strings.TrimSpace(memory.Memory) != "" {
			return true
		}
	}
	return false
}

func truncateBody(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "..."
	}
	return string(b)
}
