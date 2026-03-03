package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type remoteClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type remoteSessionCreateRequest struct {
	BotID          string `json:"bot_id"`
	SessionID      string `json:"session_id"`
	ContextDir     string `json:"context_dir"`
	IdleTTLSeconds int32  `json:"idle_ttl_seconds"`
}

type remoteSessionCreateResponse struct {
	RemoteSessionID string `json:"remote_session_id"`
	WorkerID        string `json:"worker_id"`
}

type remoteActionRequest struct {
	Name   string         `json:"name"`
	URL    string         `json:"url,omitempty"`
	Target string         `json:"target,omitempty"`
	Value  string         `json:"value,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

func newRemoteClient(baseURL, apiKey string, timeoutSec int32) *remoteClient {
	return &remoteClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

func (c *remoteClient) CreateSession(ctx context.Context, req remoteSessionCreateRequest) (remoteSessionCreateResponse, error) {
	var out remoteSessionCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, "/sessions", req, &out); err != nil {
		return out, err
	}
	if strings.TrimSpace(out.RemoteSessionID) == "" {
		return out, fmt.Errorf("browser server returned empty remote_session_id")
	}
	return out, nil
}

func (c *remoteClient) ExecuteAction(ctx context.Context, remoteSessionID string, req remoteActionRequest) (map[string]any, error) {
	var out map[string]any
	path := "/sessions/" + strings.TrimSpace(remoteSessionID) + "/actions"
	if err := c.doJSON(ctx, http.MethodPost, path, req, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (c *remoteClient) CloseSession(ctx context.Context, remoteSessionID string) error {
	path := "/sessions/" + strings.TrimSpace(remoteSessionID)
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

func (c *remoteClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("browser server %s %s failed: %s", method, path, msg)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
	}
	return nil
}
