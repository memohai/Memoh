package tui

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

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type LoginResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // CLI needs to persist and reuse the JWT access token
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Timezone    string `json:"timezone,omitempty"`
}

type ChatRequest struct {
	BotID           string
	SessionID       string
	Text            string
	ModelID         string
	ReasoningEffort string
}

type ChatEvent struct {
	Type    string
	Message string
	Data    conversation.UIMessage
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: NormalizeServerURL(baseURL),
		Token:   strings.TrimSpace(token),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Login(ctx context.Context, username, password string) (LoginResponse, error) {
	var resp LoginResponse
	err := c.doJSON(ctx, http.MethodPost, "/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, &resp)
	return resp, err
}

func (c *Client) ListBots(ctx context.Context) ([]bots.Bot, error) {
	var resp bots.ListBotsResponse
	err := c.doJSON(ctx, http.MethodGet, "/bots", nil, &resp)
	return resp.Items, err
}

func (c *Client) CreateBot(ctx context.Context, req bots.CreateBotRequest) (bots.Bot, error) {
	var resp bots.Bot
	err := c.doJSON(ctx, http.MethodPost, "/bots", req, &resp)
	return resp, err
}

func (c *Client) DeleteBot(ctx context.Context, botID string) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/bots/%s", botID), nil, nil)
}

func (c *Client) ListSessions(ctx context.Context, botID string) ([]session.Session, error) {
	var resp struct {
		Items []session.Session `json:"items"`
	}
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/bots/%s/sessions", botID), nil, &resp)
	return resp.Items, err
}

func (c *Client) CreateSession(ctx context.Context, botID, title string) (session.Session, error) {
	var resp session.Session
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/bots/%s/sessions", botID), map[string]string{
		"title": title,
	}, &resp)
	return resp, err
}

func (c *Client) ListMessages(ctx context.Context, botID, sessionID string) ([]conversation.UITurn, error) {
	path := fmt.Sprintf("/bots/%s/messages?format=ui", botID)
	if strings.TrimSpace(sessionID) != "" {
		path += "&session_id=" + url.QueryEscape(sessionID)
	}
	var resp struct {
		Items []conversation.UITurn `json:"items"`
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &resp)
	return resp.Items, err
}

func (c *Client) ListRawMessages(ctx context.Context, botID, sessionID string) ([]messagepkg.Message, error) {
	path := fmt.Sprintf("/bots/%s/messages", botID)
	if strings.TrimSpace(sessionID) != "" {
		path += "?session_id=" + url.QueryEscape(sessionID)
	}
	var resp struct {
		Items []messagepkg.Message `json:"items"`
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &resp)
	return resp.Items, err
}

func (c *Client) StreamChat(ctx context.Context, req ChatRequest, onEvent func(ChatEvent) error) error {
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("missing access token")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = fmt.Sprintf("/bots/%s/web/ws", req.BotID)
	q := u.Query()
	q.Set("token", c.Token)
	u.RawQuery = q.Encode()

	conn, resp, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("dial websocket: %w", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	payload := map[string]string{
		"type":       "message",
		"text":       req.Text,
		"session_id": req.SessionID,
	}
	if strings.TrimSpace(req.ModelID) != "" {
		payload["model_id"] = req.ModelID
	}
	if strings.TrimSpace(req.ReasoningEffort) != "" {
		payload["reasoning_effort"] = req.ReasoningEffort
	}
	if err := wsjson.Write(ctx, conn, payload); err != nil {
		return fmt.Errorf("write websocket request: %w", err)
	}

	for {
		var envelope struct {
			Type    string          `json:"type"`
			Message string          `json:"message,omitempty"`
			Data    json.RawMessage `json:"data,omitempty"`
		}
		if err := wsjson.Read(ctx, conn, &envelope); err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return nil
			}
			return fmt.Errorf("read websocket event: %w", err)
		}

		switch envelope.Type {
		case "start", "end":
			if err := onEvent(ChatEvent{Type: envelope.Type}); err != nil {
				return err
			}
			if envelope.Type == "end" {
				return nil
			}
		case "error":
			if err := onEvent(ChatEvent{Type: "error", Message: envelope.Message}); err != nil {
				return err
			}
			return errors.New(strings.TrimSpace(envelope.Message))
		case "message":
			var uiMessage conversation.UIMessage
			if err := json.Unmarshal(envelope.Data, &uiMessage); err != nil {
				return fmt.Errorf("decode chat message: %w", err)
			}
			if err := onEvent(ChatEvent{Type: "message", Data: uiMessage}); err != nil {
				return err
			}
		}
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req) //nolint:gosec // BaseURL is user-controlled CLI config by design
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("%s", message)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
