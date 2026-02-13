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

func NewLLMClient(log *slog.Logger, baseURL, apiKey, model string, timeout time.Duration) (*LLMClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("llm client: base url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("llm client: api key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("llm client: model is required")
	}
	if log == nil {
		log = slog.Default()
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &LLMClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		logger:  log.With(slog.String("client", "llm")),
		http: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *LLMClient) Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
	if len(req.Messages) == 0 {
		return ExtractResponse{}, fmt.Errorf("messages is required")
	}
	parsedMessages := strings.Join(formatMessages(req.Messages), "\n")
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

	cleaned := removeCodeBlocks(content)
	var memoryItems []map[string]any

	// Try parsing as object first
	var raw map[string]any
	if err := json.Unmarshal([]byte(cleaned), &raw); err == nil {
		memoryItems = normalizeMemoryItems(raw["memory"])
	} else {
		// If object parsing fails, try parsing as array directly
		var arr []any
		if err := json.Unmarshal([]byte(cleaned), &arr); err != nil {
			return DecideResponse{}, fmt.Errorf("failed to parse LLM response: %w", err)
		}
		memoryItems = normalizeMemoryItems(arr)
	}
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
			ID:        asString(item["id"]),
			Text:      text,
			OldMemory: asString(item["old_memory"]),
		})
	}
	return DecideResponse{Actions: actions}, nil
}

func (c *LLMClient) Compact(ctx context.Context, req CompactRequest) (CompactResponse, error) {
	if len(req.Memories) == 0 {
		return CompactResponse{}, fmt.Errorf("memories is required")
	}
	memories := make([]map[string]string, 0, len(req.Memories))
	for _, m := range req.Memories {
		entry := map[string]string{
			"id":   m.ID,
			"text": m.Memory,
		}
		if m.CreatedAt != "" {
			entry["created_at"] = m.CreatedAt
		}
		memories = append(memories, entry)
	}
	systemPrompt, userPrompt := getCompactMemoryMessages(memories, req.TargetCount, req.DecayDays)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return CompactResponse{}, err
	}
	var parsed CompactResponse
	if err := json.Unmarshal([]byte(removeCodeBlocks(content)), &parsed); err != nil {
		return CompactResponse{}, fmt.Errorf("failed to parse compact response: %w", err)
	}
	return parsed, nil
}

func (c *LLMClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("text is required")
	}
	systemPrompt, userPrompt := getLanguageDetectionMessages(text)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(removeCodeBlocks(content)), &parsed); err != nil {
		return "", err
	}
	lang := strings.ToLower(strings.TrimSpace(parsed.Language))
	if !isAllowedLanguageCode(lang) {
		return "", fmt.Errorf("unsupported language code: %s", lang)
	}
	return lang, nil
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

func asString(value any) string {
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

func normalizeMemoryItems(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
		return items
	case map[string]any:
		// If this map looks like a single item, wrap it.
		if _, hasText := typed["text"]; hasText {
			return []map[string]any{typed}
		}
		if _, hasFact := typed["fact"]; hasFact {
			return []map[string]any{typed}
		}
		if _, hasEvent := typed["event"]; hasEvent {
			return []map[string]any{typed}
		}
		// Otherwise treat as map of items.
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
		return items
	default:
		return nil
	}
}

func isAllowedLanguageCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "ar", "bg", "ca", "cjk", "ckb", "da", "de", "el", "en", "es", "eu",
		"fa", "fi", "fr", "ga", "gl", "hi", "hr", "hu", "hy", "id", "in",
		"it", "nl", "no", "pl", "pt", "ro", "ru", "sv", "tr":
		return true
	default:
		return false
	}
}
