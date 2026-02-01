package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type LLMClient struct {
	baseURL string
	apiKey  string
	model   string
	logger  *slog.Logger
	http    *http.Client
}

func NewLLMClient(log *slog.Logger, baseURL, apiKey, model string, timeout time.Duration) *LLMClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if model == "" {
		model = "gpt-4.1-nano-2025-04-14"
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &LLMClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		logger:  log.With(slog.String("client", "llm")),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *LLMClient) Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
	if len(req.Messages) == 0 {
		return ExtractResponse{}, fmt.Errorf("messages is required")
	}
	parsedMessages := parseMessages(formatMessages(req.Messages))
	systemPrompt, userPrompt := getFactRetrievalMessages(parsedMessages)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return ExtractResponse{}, err
	}

	var parsed ExtractResponse
	if err := json.Unmarshal([]byte(removeCodeBlocks(content)), &parsed); err != nil {
		return ExtractResponse{}, err
	}
	return parsed, nil
}

func (c *LLMClient) Decide(ctx context.Context, req DecideRequest) (DecideResponse, error) {
	if len(req.Facts) == 0 {
		return DecideResponse{}, fmt.Errorf("facts is required")
	}
	retrieved := make([]map[string]string, 0, len(req.Candidates))
	for _, candidate := range req.Candidates {
		retrieved = append(retrieved, map[string]string{
			"id":   candidate.ID,
			"text": candidate.Memory,
		})
	}
	prompt := getUpdateMemoryMessages(retrieved, req.Facts)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return DecideResponse{}, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(removeCodeBlocks(content)), &raw); err != nil {
		return DecideResponse{}, err
	}

	memoryItems := normalizeMemoryItems(raw["memory"])
	actions := make([]DecisionAction, 0, len(memoryItems))
	for _, item := range memoryItems {
		event := strings.ToUpper(asString(item["event"]))
		if event == "" {
			event = "ADD"
		}
		if event == "NONE" {
			continue
		}

		text := asString(item["text"])
		if text == "" {
			text = asString(item["fact"])
		}
		if strings.TrimSpace(text) == "" {
			continue
		}

		actions = append(actions, DecisionAction{
			Event:     event,
			ID:        normalizeID(item["id"]),
			Text:      text,
			OldMemory: asString(item["old_memory"]),
		})
	}
	return DecideResponse{Actions: actions}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string            `json:"model"`
	Temperature    float32           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Messages       []chatMessage     `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (c *LLMClient) callChat(ctx context.Context, messages []chatMessage) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("llm api key is required")
	}
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Temperature: 0,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
		Messages: messages,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm error: %s", strings.TrimSpace(string(b)))
	}

	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm response missing content")
	}
	return parsed.Choices[0].Message.Content, nil
}

func formatMessages(messages []Message) []string {
	formatted := make([]string, 0, len(messages))
	for _, message := range messages {
		formatted = append(formatted, fmt.Sprintf("%s: %s", message.Role, message.Content))
	}
	return formatted
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%f", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	default:
		return ""
	}
}

func normalizeID(value interface{}) string {
	id := asString(value)
	if id == "" {
		return ""
	}
	return id
}

func normalizeMemoryItems(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
		return items
	case map[string]interface{}:
		// If this map looks like a single item, wrap it.
		if _, hasText := typed["text"]; hasText {
			return []map[string]interface{}{typed}
		}
		if _, hasFact := typed["fact"]; hasFact {
			return []map[string]interface{}{typed}
		}
		if _, hasEvent := typed["event"]; hasEvent {
			return []map[string]interface{}{typed}
		}
		// Otherwise treat as map of items.
		items := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
		return items
	default:
		return nil
	}
}
