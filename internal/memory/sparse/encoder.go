// Package sparse provides a Go client for the sparse encoding Python service.
// The Python service loads the OpenSearch neural sparse model from HuggingFace
// and exposes HTTP endpoints for text → sparse vector encoding.
package sparse

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
)

// SparseVector holds the non-zero components of a sparse text encoding.
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// Client calls the Python sparse encoding service.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a sparse encoding client pointing to the Python service.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// EncodeDocument encodes a document text into a sparse vector using the neural model.
func (c *Client) EncodeDocument(ctx context.Context, text string) (*SparseVector, error) {
	return c.encode(ctx, "/encode/document", text)
}

// EncodeQuery encodes a query text into a sparse vector (IDF-weighted tokenizer lookup).
func (c *Client) EncodeQuery(ctx context.Context, text string) (*SparseVector, error) {
	return c.encode(ctx, "/encode/query", text)
}

// EncodeDocuments encodes multiple document texts in a single batch call.
func (c *Client) EncodeDocuments(ctx context.Context, texts []string) ([]SparseVector, error) {
	body, err := json.Marshal(map[string]any{"texts": texts})
	if err != nil {
		return nil, err
	}
	endpoint, err := joinEndpointURL(c.baseURL, "/encode/documents")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req) //nolint:gosec // G704: URL is validated and derived from operator-configured sparse encoder base URL
	if err != nil {
		return nil, fmt.Errorf("sparse encode failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sparse encode error (status %d): %s", resp.StatusCode, string(respBody))
	}
	var vectors []SparseVector
	if err := json.NewDecoder(resp.Body).Decode(&vectors); err != nil {
		return nil, err
	}
	return vectors, nil
}

func (c *Client) Health(ctx context.Context) error {
	endpoint, err := joinEndpointURL(c.baseURL, "/health")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req) //nolint:gosec // G704: URL is validated and derived from operator-configured sparse encoder base URL
	if err != nil {
		return fmt.Errorf("sparse health check failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sparse health error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Client) encode(ctx context.Context, path, text string) (*SparseVector, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}
	endpoint, err := joinEndpointURL(c.baseURL, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req) //nolint:gosec // G704: URL is validated and derived from operator-configured sparse encoder base URL
	if err != nil {
		return nil, fmt.Errorf("sparse encode failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sparse encode error (status %d): %s", resp.StatusCode, string(respBody))
	}
	var vec SparseVector
	if err := json.NewDecoder(resp.Body).Decode(&vec); err != nil {
		return nil, err
	}
	return &vec, nil
}

func joinEndpointURL(baseURL, path string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("sparse encode base URL is required")
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid sparse encode base URL: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("invalid sparse encode base URL scheme: %q", base.Scheme)
	}
	if base.Host == "" {
		return "", errors.New("invalid sparse encode base URL: host is required")
	}

	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid sparse encode path: %w", err)
	}
	return base.ResolveReference(ref).String(), nil
}
