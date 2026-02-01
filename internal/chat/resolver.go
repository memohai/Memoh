package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/models"
)

const defaultMaxContextMinutes = 24 * 60

type Resolver struct {
	modelsService   *models.Service
	queries         *sqlc.Queries
	memoryService   *memory.Service
	gatewayBaseURL  string
	timeout         time.Duration
	logger          *slog.Logger
	httpClient      *http.Client
	streamingClient *http.Client
}

func NewResolver(log *slog.Logger, modelsService *models.Service, queries *sqlc.Queries, memoryService *memory.Service, gatewayBaseURL string, timeout time.Duration) *Resolver {
	if strings.TrimSpace(gatewayBaseURL) == "" {
		gatewayBaseURL = "http://127.0.0.1:8081"
	}
	gatewayBaseURL = strings.TrimRight(gatewayBaseURL, "/")
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Resolver{
		modelsService:  modelsService,
		queries:        queries,
		memoryService:  memoryService,
		gatewayBaseURL: gatewayBaseURL,
		timeout:        timeout,
		logger:         log.With(slog.String("service", "chat")),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		streamingClient: &http.Client{},
	}
}

func (r *Resolver) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return ChatResponse{}, fmt.Errorf("query is required")
	}
	if strings.TrimSpace(req.UserID) == "" {
		return ChatResponse{}, fmt.Errorf("user id is required")
	}
	skipHistory := req.MaxContextLoadTime < 0

	chatModel, provider, err := r.selectChatModel(ctx, req)
	if err != nil {
		return ChatResponse{}, err
	}
	clientType, err := normalizeClientType(provider.ClientType)
	if err != nil {
		return ChatResponse{}, err
	}

	maxContextLoadTime, language, err := r.loadUserSettings(ctx, req.UserID)
	if err != nil {
		return ChatResponse{}, err
	}
	if req.MaxContextLoadTime > 0 {
		maxContextLoadTime = req.MaxContextLoadTime
	}
	if strings.TrimSpace(req.Language) != "" {
		language = req.Language
	}

	var messages []GatewayMessage
	if !skipHistory {
		messages, err = r.loadHistoryMessages(ctx, req.UserID, maxContextLoadTime)
		if err != nil {
			return ChatResponse{}, err
		}
	}
	if len(req.Messages) > 0 {
		messages = append(messages, req.Messages...)
	}
	messages = sanitizeGatewayMessages(messages)

	payload := agentGatewayRequest{
		APIKey:             provider.ApiKey,
		BaseURL:            provider.BaseUrl,
		Model:              chatModel.ModelID,
		ClientType:         clientType,
		Locale:             req.Locale,
		Language:           req.Language,
		MaxSteps:           req.MaxSteps,
		MaxContextLoadTime: normalizeMaxContextLoad(maxContextLoadTime),
		Platforms:          req.Platforms,
		CurrentPlatform:    req.CurrentPlatform,
		Messages:           messages,
		Query:              req.Query,
	}
	payload.Language = language

	resp, err := r.postChat(ctx, payload, req.Token)
	if err != nil {
		return ChatResponse{}, err
	}

	if err := r.storeHistory(ctx, req.UserID, req.Query, resp.Messages); err != nil {
		return ChatResponse{}, err
	}
	if err := r.storeMemory(ctx, req.UserID, req.Query, resp.Messages); err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{
		Messages: resp.Messages,
		Model:    chatModel.ModelID,
		Provider: provider.ClientType,
	}, nil
}

func (r *Resolver) TriggerSchedule(ctx context.Context, userID string, schedule SchedulePayload, token string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}
	if strings.TrimSpace(schedule.Command) == "" {
		return fmt.Errorf("schedule command is required")
	}

	req := ChatRequest{
		UserID:   userID,
		Query:    schedule.Command,
		Locale:   "",
		Language: "",
	}
	chatModel, provider, err := r.selectChatModel(ctx, req)
	if err != nil {
		return err
	}
	clientType, err := normalizeClientType(provider.ClientType)
	if err != nil {
		return err
	}

	maxContextLoadTime, language, err := r.loadUserSettings(ctx, userID)
	if err != nil {
		return err
	}

	messages, err := r.loadHistoryMessages(ctx, userID, maxContextLoadTime)
	if err != nil {
		return err
	}

	payload := agentGatewayScheduleRequest{
		APIKey:             provider.ApiKey,
		BaseURL:            provider.BaseUrl,
		Model:              chatModel.ModelID,
		ClientType:         clientType,
		Locale:             "",
		Language:           language,
		MaxSteps:           0,
		MaxContextLoadTime: normalizeMaxContextLoad(maxContextLoadTime),
		Platforms:          nil,
		CurrentPlatform:    "",
		Messages:           messages,
		Query:              schedule.Command,
		Schedule:           schedule,
	}

	resp, err := r.postSchedule(ctx, payload, token)
	if err != nil {
		return err
	}
	if err := r.storeHistory(ctx, userID, schedule.Command, resp.Messages); err != nil {
		return err
	}
	if err := r.storeMemory(ctx, userID, schedule.Command, resp.Messages); err != nil {
		return err
	}
	return nil
}

func (r *Resolver) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, <-chan error) {
	chunkChan := make(chan StreamChunk)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		if strings.TrimSpace(req.Query) == "" {
			errChan <- fmt.Errorf("query is required")
			return
		}
		if strings.TrimSpace(req.UserID) == "" {
			errChan <- fmt.Errorf("user id is required")
			return
		}
		skipHistory := req.MaxContextLoadTime < 0

		chatModel, provider, err := r.selectChatModel(ctx, req)
		if err != nil {
			errChan <- err
			return
		}
		clientType, err := normalizeClientType(provider.ClientType)
		if err != nil {
			errChan <- err
			return
		}

		maxContextLoadTime, language, err := r.loadUserSettings(ctx, req.UserID)
		if err != nil {
			errChan <- err
			return
		}
		if req.MaxContextLoadTime > 0 {
			maxContextLoadTime = req.MaxContextLoadTime
		}
		if strings.TrimSpace(req.Language) != "" {
			language = req.Language
		}

		var messages []GatewayMessage
		if !skipHistory {
			messages, err = r.loadHistoryMessages(ctx, req.UserID, maxContextLoadTime)
			if err != nil {
				errChan <- err
				return
			}
		}
		if len(req.Messages) > 0 {
			messages = append(messages, req.Messages...)
		}
		messages = sanitizeGatewayMessages(messages)

		payload := agentGatewayRequest{
			APIKey:             provider.ApiKey,
			BaseURL:            provider.BaseUrl,
			Model:              chatModel.ModelID,
			ClientType:         clientType,
			Locale:             req.Locale,
			Language:           req.Language,
			MaxSteps:           req.MaxSteps,
			MaxContextLoadTime: normalizeMaxContextLoad(maxContextLoadTime),
			Platforms:          req.Platforms,
			CurrentPlatform:    req.CurrentPlatform,
			Messages:           messages,
			Query:              req.Query,
		}
		payload.Language = language

		if err := r.streamChat(ctx, payload, req.UserID, req.Query, req.Token, chunkChan); err != nil {
			errChan <- err
			return
		}
	}()

	return chunkChan, errChan
}

type agentGatewayRequest struct {
	APIKey             string           `json:"apiKey"`
	BaseURL            string           `json:"baseUrl"`
	Model              string           `json:"model"`
	ClientType         string           `json:"clientType"`
	Locale             string           `json:"locale,omitempty"`
	Language           string           `json:"language,omitempty"`
	MaxSteps           int              `json:"maxSteps,omitempty"`
	MaxContextLoadTime int              `json:"maxContextLoadTime"`
	Platforms          []string         `json:"platforms,omitempty"`
	CurrentPlatform    string           `json:"currentPlatform,omitempty"`
	Messages           []GatewayMessage `json:"messages"`
	Query              string           `json:"query"`
}

type agentGatewayScheduleRequest struct {
	APIKey             string           `json:"apiKey"`
	BaseURL            string           `json:"baseUrl"`
	Model              string           `json:"model"`
	ClientType         string           `json:"clientType"`
	Locale             string           `json:"locale,omitempty"`
	Language           string           `json:"language,omitempty"`
	MaxSteps           int              `json:"maxSteps,omitempty"`
	MaxContextLoadTime int              `json:"maxContextLoadTime"`
	Platforms          []string         `json:"platforms,omitempty"`
	CurrentPlatform    string           `json:"currentPlatform,omitempty"`
	Messages           []GatewayMessage `json:"messages"`
	Query              string           `json:"query"`
	Schedule           SchedulePayload  `json:"schedule"`
}

type agentGatewayResponse struct {
	Messages []GatewayMessage `json:"messages"`
}

func (r *Resolver) postChat(ctx context.Context, payload agentGatewayRequest, token string) (agentGatewayResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return agentGatewayResponse{}, err
	}
	url := r.gatewayBaseURL + "/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return agentGatewayResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return agentGatewayResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return agentGatewayResponse{}, fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(payload)))
	}

	var parsed agentGatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return agentGatewayResponse{}, err
	}
	return parsed, nil
}

func (r *Resolver) postSchedule(ctx context.Context, payload agentGatewayScheduleRequest, token string) (agentGatewayResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return agentGatewayResponse{}, err
	}
	url := r.gatewayBaseURL + "/chat/schedule"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return agentGatewayResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return agentGatewayResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return agentGatewayResponse{}, fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(payload)))
	}

	var parsed agentGatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return agentGatewayResponse{}, err
	}
	return parsed, nil
}
func (r *Resolver) streamChat(ctx context.Context, payload agentGatewayRequest, userID, query, token string, chunkChan chan<- StreamChunk) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := r.gatewayBaseURL + "/chat/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := r.streamingClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(payload)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	currentEventType := ""
	stored := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		chunkChan <- StreamChunk([]byte(data))

		if stored {
			continue
		}

		if handled, err := r.tryStoreFromStreamPayload(ctx, userID, query, currentEventType, data); err != nil {
			return err
		} else if handled {
			stored = true
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Resolver) loadHistoryMessages(ctx context.Context, userID string, maxContextLoadTime int) ([]GatewayMessage, error) {
	if r.queries == nil {
		return nil, fmt.Errorf("history queries not configured")
	}
	pgUserID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	from := time.Now().UTC().Add(-time.Duration(normalizeMaxContextLoad(maxContextLoadTime)) * time.Minute)
	rows, err := r.queries.ListHistoryByUserSince(ctx, sqlc.ListHistoryByUserSinceParams{
		User: pgUserID,
		Timestamp: pgtype.Timestamptz{
			Time:  from,
			Valid: true,
		},
	})
	if err != nil {
		return nil, err
	}
	messages := make([]GatewayMessage, 0, len(rows))
	for _, row := range rows {
		var batch []GatewayMessage
		if len(row.Messages) == 0 {
			continue
		}
		if err := json.Unmarshal(row.Messages, &batch); err != nil {
			return nil, err
		}
		messages = append(messages, batch...)
	}
	return messages, nil
}

func (r *Resolver) storeHistory(ctx context.Context, userID, query string, responseMessages []GatewayMessage) error {
	if r.queries == nil {
		return fmt.Errorf("history queries not configured")
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}
	if strings.TrimSpace(query) == "" && len(responseMessages) == 0 {
		return nil
	}
	payload, err := json.Marshal(responseMessages)
	if err != nil {
		return err
	}
	pgUserID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	if err := r.ensureUserExists(ctx, pgUserID); err != nil {
		return err
	}
	_, err = r.queries.CreateHistory(ctx, sqlc.CreateHistoryParams{
		Messages: payload,
		Timestamp: pgtype.Timestamptz{
			Time:  time.Now().UTC(),
			Valid: true,
		},
		User: pgUserID,
	})
	if err != nil {
		return err
	}
	return err
}

func (r *Resolver) ensureUserExists(ctx context.Context, userID pgtype.UUID) error {
	if !userID.Valid {
		return fmt.Errorf("invalid user id")
	}
	if _, err := r.queries.GetUserByID(ctx, userID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	username := "user-" + uuid.UUID(userID.Bytes).String()
	password := uuid.NewString()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = r.queries.CreateUserWithID(ctx, sqlc.CreateUserWithIDParams{
		ID:           userID,
		Username:     username,
		Email:        pgtype.Text{Valid: false},
		PasswordHash: string(hashed),
		Role:         "member",
		DisplayName:  pgtype.Text{String: username, Valid: true},
		AvatarUrl:    pgtype.Text{Valid: false},
		IsActive:     true,
		DataRoot:     pgtype.Text{Valid: false},
	})
	return err
}

func (r *Resolver) storeMemory(ctx context.Context, userID, query string, responseMessages []GatewayMessage) error {
	if r.memoryService == nil {
		return nil
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}
	if strings.TrimSpace(query) == "" && len(responseMessages) == 0 {
		return nil
	}

	userMessage := GatewayMessage{
		"role":    "user",
		"content": query,
	}
	messages := append([]GatewayMessage{userMessage}, responseMessages...)
	memoryMessages := make([]memory.Message, 0, len(messages))
	for _, msg := range messages {
		role, content := gatewayMessageToMemory(msg)
		if strings.TrimSpace(content) == "" {
			continue
		}
		memoryMessages = append(memoryMessages, memory.Message{
			Role:    role,
			Content: content,
		})
	}
	if len(memoryMessages) == 0 {
		return nil
	}

	_, err := r.memoryService.Add(ctx, memory.AddRequest{
		Messages: memoryMessages,
		UserID:   userID,
	})
	return err
}

func (r *Resolver) tryStoreFromStreamPayload(ctx context.Context, userID, query, eventType, data string) (bool, error) {
	// Case 1: event: done + data: {messages: [...]}
	if eventType == "done" {
		if parsed, ok := parseGatewayResponse([]byte(data)); ok {
			return r.storeRound(ctx, userID, query, parsed.Messages)
		}
	}

	// Case 2: data: {"type":"done","data":{messages:[...]}}
	var envelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err == nil {
		if envelope.Type == "done" && len(envelope.Data) > 0 {
			if parsed, ok := parseGatewayResponse(envelope.Data); ok {
				return r.storeRound(ctx, userID, query, parsed.Messages)
			}
		}
	}

	// Case 3: data: {messages:[...]} without event
	if parsed, ok := parseGatewayResponse([]byte(data)); ok {
		return r.storeRound(ctx, userID, query, parsed.Messages)
	}
	return false, nil
}

func parseGatewayResponse(payload []byte) (agentGatewayResponse, bool) {
	var parsed agentGatewayResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return agentGatewayResponse{}, false
	}
	if len(parsed.Messages) == 0 {
		return agentGatewayResponse{}, false
	}
	return parsed, true
}

func (r *Resolver) storeRound(ctx context.Context, userID, query string, messages []GatewayMessage) (bool, error) {
	if err := r.storeHistory(ctx, userID, query, messages); err != nil {
		return true, err
	}
	if err := r.storeMemory(ctx, userID, query, messages); err != nil {
		return true, err
	}
	return true, nil
}

func gatewayMessageToMemory(msg GatewayMessage) (string, string) {
	role := "assistant"
	if raw, ok := msg["role"].(string); ok && strings.TrimSpace(raw) != "" {
		role = raw
	}
	if raw, ok := msg["content"]; ok {
		switch v := raw.(type) {
		case string:
			return role, v
		default:
			if encoded, err := json.Marshal(v); err == nil {
				return role, string(encoded)
			}
		}
	}
	if encoded, err := json.Marshal(msg); err == nil {
		return role, string(encoded)
	}
	return role, ""
}

func sanitizeGatewayMessages(messages []GatewayMessage) []GatewayMessage {
	if len(messages) == 0 {
		return messages
	}
	cleaned := make([]GatewayMessage, 0, len(messages))
	for _, msg := range messages {
		if !isMeaningfulGatewayMessage(msg) {
			continue
		}
		cleaned = append(cleaned, msg)
	}
	return cleaned
}

func isMeaningfulGatewayMessage(msg GatewayMessage) bool {
	if len(msg) == 0 {
		return false
	}
	if raw, ok := msg["role"].(string); ok && strings.TrimSpace(raw) != "" {
		return true
	}
	if raw, ok := msg["content"]; ok {
		switch v := raw.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return true
			}
		default:
			if !isEmptyValue(v) {
				return true
			}
		}
	}
	for _, value := range msg {
		if !isEmptyValue(value) {
			return true
		}
	}
	return false
}

func isEmptyValue(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []interface{}:
		if len(v) == 0 {
			return true
		}
		for _, item := range v {
			if !isEmptyValue(item) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		if len(v) == 0 {
			return true
		}
		for _, item := range v {
			if !isEmptyValue(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (r *Resolver) selectChatModel(ctx context.Context, req ChatRequest) (models.GetResponse, sqlc.LlmProvider, error) {
	if r.modelsService == nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("models service not configured")
	}
	modelID := strings.TrimSpace(req.Model)
	providerFilter := strings.TrimSpace(req.Provider)

	if modelID != "" && providerFilter == "" {
		model, err := r.modelsService.GetByModelID(ctx, modelID)
		if err != nil {
			return models.GetResponse{}, sqlc.LlmProvider{}, err
		}
		if model.Type != models.ModelTypeChat {
			return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("model is not a chat model")
		}
		provider, err := models.FetchProviderByID(ctx, r.queries, model.LlmProviderID)
		if err != nil {
			return models.GetResponse{}, sqlc.LlmProvider{}, err
		}
		return model, provider, nil
	}

	if providerFilter == "" && modelID == "" {
		defaultModel, err := r.modelsService.GetByEnableAs(ctx, models.EnableAsChat)
		if err == nil {
			provider, err := models.FetchProviderByID(ctx, r.queries, defaultModel.LlmProviderID)
			if err != nil {
				return models.GetResponse{}, sqlc.LlmProvider{}, err
			}
			return defaultModel, provider, nil
		}
	}

	var candidates []models.GetResponse
	var err error
	if providerFilter != "" {
		candidates, err = r.modelsService.ListByClientType(ctx, models.ClientType(providerFilter))
	} else {
		candidates, err = r.modelsService.ListByType(ctx, models.ModelTypeChat)
	}
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}

	filtered := make([]models.GetResponse, 0, len(candidates))
	for _, model := range candidates {
		if model.Type != models.ModelTypeChat {
			continue
		}
		filtered = append(filtered, model)
	}
	if len(filtered) == 0 {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("no chat models available")
	}

	if modelID != "" {
		for _, model := range filtered {
			if model.ModelID == modelID {
				provider, err := models.FetchProviderByID(ctx, r.queries, model.LlmProviderID)
				if err != nil {
					return models.GetResponse{}, sqlc.LlmProvider{}, err
				}
				return model, provider, nil
			}
		}
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("chat model not found")
	}

	selected := filtered[0]
	provider, err := models.FetchProviderByID(ctx, r.queries, selected.LlmProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	return selected, provider, nil
}

func normalizeMaxContextLoad(value int) int {
	if value <= 0 {
		return defaultMaxContextMinutes
	}
	return value
}

func (r *Resolver) loadUserSettings(ctx context.Context, userID string) (int, string, error) {
	if r.queries == nil {
		return defaultMaxContextMinutes, "Same as user input", nil
	}
	pgUserID, err := parseUUID(userID)
	if err != nil {
		return 0, "", err
	}
	settingsRow, err := r.queries.GetSettingsByUserID(ctx, pgUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultMaxContextMinutes, "Same as user input", nil
		}
		return 0, "", err
	}
	maxLoad, language := normalizeUserSettingRow(settingsRow)
	return maxLoad, language, nil
}

func normalizeUserSettingRow(row sqlc.UserSetting) (int, string) {
	maxLoad := int(row.MaxContextLoadTime)
	if maxLoad <= 0 {
		maxLoad = defaultMaxContextMinutes
	}
	language := strings.TrimSpace(row.Language)
	if language == "" {
		language = "Same as user input"
	}
	return maxLoad, language
}

func normalizeClientType(clientType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(clientType)) {
	case "openai":
		return "openai", nil
	case "openai-compat":
		return "openai", nil
	case "anthropic":
		return "anthropic", nil
	case "google":
		return "google", nil
	default:
		return "", fmt.Errorf("unsupported agent gateway client type: %s", clientType)
	}
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}
