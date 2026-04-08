package nowledgemem

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

const (
	defaultBaseURL = "http://127.0.0.1:14242"
	clientTimeout  = 30 * time.Second
)

type nmemClient struct {
	baseURL    string
	httpClient *http.Client
}

func newNmemClient(config map[string]any) (*nmemClient, error) {
	baseURL := adapters.StringFromConfig(config, "base_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &nmemClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
	}, nil
}

// --- Request/Response types ---

type nmemMemoryCreateRequest struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

type nmemMemoryUpdateRequest struct {
	Content string `json:"content"`
}

type nmemSearchRequest struct {
	Query           string `json:"query"`
	Limit           int    `json:"limit,omitempty"`
	IncludeEntities bool   `json:"include_entities,omitempty"`
}

type nmemMemory struct {
	ID         string         `json:"id"`
	Title      string         `json:"title,omitempty"`
	Content    string         `json:"content"`
	Source     string         `json:"source,omitempty"`
	Time       string         `json:"time,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type nmemSearchResult struct {
	Memory          nmemMemory `json:"memory"`
	SimilarityScore float64    `json:"similarity_score"`
	RelevanceReason string     `json:"relevance_reason,omitempty"`
}

type nmemSearchResponse struct {
	Results []nmemSearchResult `json:"results"`
}

type nmemDeleteResponse struct {
	Message              string `json:"message,omitempty"`
	DeletedRelationships int    `json:"deleted_relationships,omitempty"`
	DeletedEntities      int    `json:"deleted_entities,omitempty"`
}

// --- Client methods ---

func (c *nmemClient) addMemory(ctx context.Context, content, source string) (*nmemMemory, error) {
	var result nmemMemory
	if err := c.doJSON(ctx, http.MethodPost, "/memories", nmemMemoryCreateRequest{
		Content: content,
		Source:  source,
	}, &result); err != nil {
		return nil, fmt.Errorf("nowledgemem add: %w", err)
	}
	return &result, nil
}

func (c *nmemClient) searchMemories(ctx context.Context, query string, limit int) ([]nmemSearchResult, error) {
	var result nmemSearchResponse
	if err := c.doJSON(ctx, http.MethodPost, "/memories/search", nmemSearchRequest{
		Query:           query,
		Limit:           limit,
		IncludeEntities: true,
	}, &result); err != nil {
		return nil, fmt.Errorf("nowledgemem search: %w", err)
	}
	return result.Results, nil
}

func (c *nmemClient) updateMemory(ctx context.Context, id, content string) (*nmemMemory, error) {
	var result nmemMemory
	if err := c.doJSON(ctx, http.MethodPatch, "/memories/"+id, nmemMemoryUpdateRequest{
		Content: content,
	}, &result); err != nil {
		return nil, fmt.Errorf("nowledgemem update: %w", err)
	}
	return &result, nil
}

func (c *nmemClient) deleteMemory(ctx context.Context, id string) error {
	var result nmemDeleteResponse
	if err := c.doJSON(ctx, http.MethodDelete, "/memories/"+id, nil, &result); err != nil {
		return fmt.Errorf("nowledgemem delete: %w", err)
	}
	return nil
}

// --- HTTP helper ---

func (c *nmemClient) doJSON(ctx context.Context, method, urlPath string, body any, result any) error {
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
	req.Header.Set("Accept", "application/json")

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
		return fmt.Errorf("nowledgemem API error %d: %s", resp.StatusCode, truncateBody(respBody))
	}
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

func truncateBody(b []byte) string {
	const maxLen = 300
	s := strings.TrimSpace(string(b))
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
