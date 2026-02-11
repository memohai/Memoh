package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/settings"
)

const (
	defaultMaxContextMinutes   = 24 * 60
	memoryContextLimitPerScope = 4
	memoryContextMaxItems      = 8
	memoryContextItemMaxChars  = 220
)

// SkillEntry represents a skill loaded from the container.
type SkillEntry struct {
	Name        string
	Description string
	Content     string
	Metadata    map[string]any
}

// SkillLoader loads skills for a given bot from its container.
type SkillLoader interface {
	LoadSkills(ctx context.Context, botID string) ([]SkillEntry, error)
}

// Resolver orchestrates chat with the agent gateway.
type Resolver struct {
	modelsService   *models.Service
	queries         *sqlc.Queries
	memoryService   *memory.Service
	chatService     *Service
	settingsService *settings.Service
	mcpService      *mcp.ConnectionService
	skillLoader     SkillLoader
	gatewayBaseURL  string
	timeout         time.Duration
	logger          *slog.Logger
	httpClient      *http.Client
	streamingClient *http.Client
}

// NewResolver creates a Resolver that communicates with the agent gateway.
func NewResolver(
	log *slog.Logger,
	modelsService *models.Service,
	queries *sqlc.Queries,
	memoryService *memory.Service,
	chatService *Service,
	settingsService *settings.Service,
	mcpService *mcp.ConnectionService,
	gatewayBaseURL string,
	timeout time.Duration,
) *Resolver {
	if strings.TrimSpace(gatewayBaseURL) == "" {
		gatewayBaseURL = "http://127.0.0.1:8081"
	}
	gatewayBaseURL = strings.TrimRight(gatewayBaseURL, "/")
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Resolver{
		modelsService:   modelsService,
		queries:         queries,
		memoryService:   memoryService,
		chatService:     chatService,
		settingsService: settingsService,
		mcpService:      mcpService,
		gatewayBaseURL:  gatewayBaseURL,
		timeout:         timeout,
		logger:          log.With(slog.String("service", "chat_resolver")),
		httpClient:      &http.Client{Timeout: timeout},
		streamingClient: &http.Client{},
	}
}

// SetSkillLoader sets the skill loader used to populate usable skills in gateway requests.
func (r *Resolver) SetSkillLoader(sl SkillLoader) {
	r.skillLoader = sl
}

// --- gateway payload ---

type gatewayModelConfig struct {
	ModelID    string   `json:"modelId"`
	ClientType string   `json:"clientType"`
	Input      []string `json:"input"`
	APIKey     string   `json:"apiKey"`
	BaseURL    string   `json:"baseUrl"`
}

type gatewayIdentity struct {
	BotID             string `json:"botId"`
	SessionID         string `json:"sessionId"`
	ContainerID       string `json:"containerId"`
	ChannelIdentityID string `json:"channelIdentityId"`
	DisplayName       string `json:"displayName"`
	CurrentPlatform   string `json:"currentPlatform,omitempty"`
	ReplyTarget       string `json:"replyTarget,omitempty"`
	SessionToken      string `json:"sessionToken,omitempty"`
}

type gatewaySkill struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type gatewayRequest struct {
	Model             gatewayModelConfig `json:"model"`
	ActiveContextTime int                `json:"activeContextTime"`
	Channels          []string           `json:"channels"`
	CurrentChannel    string             `json:"currentChannel"`
	AllowedActions    []string           `json:"allowedActions,omitempty"`
	MCPConnections    []map[string]any   `json:"mcpConnections"`
	Messages          []ModelMessage     `json:"messages"`
	Skills            []string           `json:"skills"`
	UsableSkills      []gatewaySkill     `json:"usableSkills"`
	Query             string             `json:"query"`
	Identity          gatewayIdentity    `json:"identity"`
	Attachments       []any              `json:"attachments"`
}

type gatewayResponse struct {
	Messages []ModelMessage `json:"messages"`
	Skills   []string       `json:"skills"`
}

// gatewaySchedule matches the agent gateway ScheduleModel for /chat/trigger-schedule.
type gatewaySchedule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	MaxCalls    *int   `json:"maxCalls,omitempty"`
	Command     string `json:"command"`
}

// triggerScheduleRequest is the payload for POST /chat/trigger-schedule.
type triggerScheduleRequest struct {
	Model             gatewayModelConfig `json:"model"`
	ActiveContextTime int                `json:"activeContextTime"`
	Channels          []string           `json:"channels"`
	CurrentChannel    string             `json:"currentChannel"`
	AllowedActions    []string           `json:"allowedActions,omitempty"`
	MCPConnections    []map[string]any   `json:"mcpConnections"`
	Messages          []ModelMessage     `json:"messages"`
	Skills            []string           `json:"skills"`
	UsableSkills      []gatewaySkill     `json:"usableSkills"`
	Identity          gatewayIdentity    `json:"identity"`
	Attachments       []any              `json:"attachments"`
	Schedule          gatewaySchedule    `json:"schedule"`
}

// --- resolved context (shared by Chat / StreamChat / TriggerSchedule) ---

type resolvedContext struct {
	payload  gatewayRequest
	model    models.GetResponse
	provider sqlc.LlmProvider
}

func (r *Resolver) resolve(ctx context.Context, req ChatRequest) (resolvedContext, error) {
	if strings.TrimSpace(req.Query) == "" {
		return resolvedContext{}, fmt.Errorf("query is required")
	}
	if strings.TrimSpace(req.BotID) == "" {
		return resolvedContext{}, fmt.Errorf("bot id is required")
	}
	if strings.TrimSpace(req.ChatID) == "" {
		return resolvedContext{}, fmt.Errorf("chat id is required")
	}

	skipHistory := req.MaxContextLoadTime < 0

	botSettings, err := r.loadBotSettings(ctx, req.BotID)
	if err != nil {
		return resolvedContext{}, err
	}

	// Check chat-level model override.
	var chatSettings Settings
	if r.chatService != nil {
		chatSettings, err = r.chatService.GetSettings(ctx, req.ChatID)
		if err != nil {
			return resolvedContext{}, err
		}
		r.enforceGroupMemoryPolicy(ctx, req.ChatID, &chatSettings)
	}

	userSettings, err := r.loadUserSettings(ctx, req.UserID)
	if err != nil {
		return resolvedContext{}, err
	}
	chatModel, provider, err := r.selectChatModel(ctx, req, botSettings, userSettings, chatSettings)
	if err != nil {
		return resolvedContext{}, err
	}
	clientType, err := normalizeClientType(provider.ClientType)
	if err != nil {
		return resolvedContext{}, err
	}
	maxCtx := coalescePositiveInt(req.MaxContextLoadTime, botSettings.MaxContextLoadTime, defaultMaxContextMinutes)

	var messages []ModelMessage
	if !skipHistory && r.chatService != nil {
		messages, err = r.loadChatMessages(ctx, req.ChatID, maxCtx)
		if err != nil {
			return resolvedContext{}, err
		}
	}
	if memoryMsg := r.loadMemoryContextMessage(ctx, req, chatSettings); memoryMsg != nil {
		messages = append(messages, *memoryMsg)
	}
	messages = append(messages, req.Messages...)
	messages = sanitizeMessages(messages)
	skills := dedup(req.Skills)
	containerID := r.resolveContainerID(ctx, req.BotID, req.ContainerID)

	var usableSkills []gatewaySkill
	if r.skillLoader != nil {
		entries, err := r.skillLoader.LoadSkills(ctx, req.BotID)
		if err != nil {
			r.logger.Warn("failed to load usable skills", slog.String("bot_id", req.BotID), slog.Any("error", err))
		} else {
			usableSkills = make([]gatewaySkill, 0, len(entries))
			for _, e := range entries {
				usableSkills = append(usableSkills, gatewaySkill{
					Name:        e.Name,
					Description: e.Description,
					Content:     e.Content,
					Metadata:    e.Metadata,
				})
			}
		}
	}
	if usableSkills == nil {
		usableSkills = []gatewaySkill{}
	}

	mcpConnections := []map[string]any{}
	if r.mcpService != nil {
		items, err := r.mcpService.ListActiveByBot(ctx, req.BotID)
		if err != nil {
			r.logger.Warn("failed to load mcp connections", slog.String("bot_id", req.BotID), slog.Any("error", err))
		} else {
			for _, item := range items {
				payload := map[string]any{}
				for k, v := range item.Config {
					payload[k] = v
				}
				payload["name"] = item.Name
				payload["type"] = item.Type
				mcpConnections = append(mcpConnections, payload)
			}
		}
	}

	payload := gatewayRequest{
		Model: gatewayModelConfig{
			ModelID:    chatModel.ModelID,
			ClientType: clientType,
			Input:      chatModel.Input,
			APIKey:     provider.ApiKey,
			BaseURL:    provider.BaseUrl,
		},
		ActiveContextTime: maxCtx,
		Channels:          nonNilStrings(req.Channels),
		CurrentChannel:    req.CurrentChannel,
		AllowedActions:    req.AllowedActions,
		MCPConnections:    mcpConnections,
		Messages:          nonNilModelMessages(messages),
		Skills:            nonNilStrings(skills),
		UsableSkills:      usableSkills,
		Query:             req.Query,
		Identity: gatewayIdentity{
			BotID:             req.BotID,
			SessionID:         req.ChatID,
			ContainerID:       containerID,
			ChannelIdentityID: firstNonEmpty(req.SourceChannelIdentityID, req.UserID),
			DisplayName:       firstNonEmpty(req.DisplayName, "User"),
			CurrentPlatform:   req.CurrentChannel,
			ReplyTarget:       "",
			SessionToken:      req.ChatToken,
		},
		Attachments: []any{},
	}

	return resolvedContext{payload: payload, model: chatModel, provider: provider}, nil
}

// --- Chat ---

// Chat sends a synchronous chat request to the agent gateway and stores the result.
func (r *Resolver) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return ChatResponse{}, err
	}
	resp, err := r.postChat(ctx, rc.payload, req.Token)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := r.storeRound(ctx, req, resp.Messages); err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		Messages: resp.Messages,
		Skills:   resp.Skills,
		Model:    rc.model.ModelID,
		Provider: rc.provider.ClientType,
	}, nil
}

// --- TriggerSchedule ---

// TriggerSchedule executes a scheduled command through the agent gateway trigger-schedule endpoint.
func (r *Resolver) TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) error {
	if strings.TrimSpace(botID) == "" {
		return fmt.Errorf("bot id is required")
	}
	if strings.TrimSpace(payload.Command) == "" {
		return fmt.Errorf("schedule command is required")
	}

	chatID := payload.ChatID
	if strings.TrimSpace(chatID) == "" {
		chatID = "schedule-" + payload.ID
	}
	req := ChatRequest{
		BotID:  botID,
		ChatID: chatID,
		Query:  payload.Command,
		UserID: payload.OwnerUserID,
		Token:  token,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return err
	}

	triggerReq := triggerScheduleRequest{
		Model:             rc.payload.Model,
		ActiveContextTime: rc.payload.ActiveContextTime,
		Channels:          rc.payload.Channels,
		CurrentChannel:    rc.payload.CurrentChannel,
		AllowedActions:    rc.payload.AllowedActions,
		MCPConnections:    rc.payload.MCPConnections,
		Messages:          rc.payload.Messages,
		Skills:            rc.payload.Skills,
		UsableSkills:      rc.payload.UsableSkills,
		Identity: gatewayIdentity{
			BotID:             rc.payload.Identity.BotID,
			SessionID:         rc.payload.Identity.SessionID,
			ContainerID:       rc.payload.Identity.ContainerID,
			ChannelIdentityID: strings.TrimSpace(payload.OwnerUserID),
			DisplayName:       "Scheduler",
		},
		Attachments: rc.payload.Attachments,
		Schedule: gatewaySchedule{
			ID:          payload.ID,
			Name:        payload.Name,
			Description: payload.Description,
			Pattern:     payload.Pattern,
			MaxCalls:    payload.MaxCalls,
			Command:     payload.Command,
		},
	}

	resp, err := r.postTriggerSchedule(ctx, triggerReq, token)
	if err != nil {
		return err
	}
	return r.storeRound(ctx, req, resp.Messages)
}

// --- StreamChat ---

// StreamChat sends a streaming chat request to the agent gateway.
func (r *Resolver) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, <-chan error) {
	chunkCh := make(chan StreamChunk)
	errCh := make(chan error, 1)
	r.logger.Info("gateway stream start",
		slog.String("bot_id", req.BotID),
		slog.String("chat_id", req.ChatID),
	)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		rc, err := r.resolve(ctx, req)
		if err != nil {
			r.logger.Error("gateway stream resolve failed",
				slog.String("bot_id", req.BotID),
				slog.String("chat_id", req.ChatID),
				slog.Any("error", err),
			)
			errCh <- err
			return
		}
		if err := r.streamChat(ctx, rc.payload, req, chunkCh); err != nil {
			r.logger.Error("gateway stream request failed",
				slog.String("bot_id", req.BotID),
				slog.String("chat_id", req.ChatID),
				slog.Any("error", err),
			)
			errCh <- err
		}
	}()
	return chunkCh, errCh
}

// --- HTTP helpers ---

func (r *Resolver) postChat(ctx context.Context, payload gatewayRequest, token string) (gatewayResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return gatewayResponse{}, err
	}
	url := r.gatewayBaseURL + "/chat/"
	r.logger.Info("gateway request", slog.String("url", url), slog.String("body_prefix", truncate(string(body), 200)))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return gatewayResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		httpReq.Header.Set("Authorization", token)
	}

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return gatewayResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return gatewayResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.logger.Error("gateway error", slog.String("url", url), slog.Int("status", resp.StatusCode), slog.String("body_prefix", truncate(string(respBody), 300)))
		return gatewayResponse{}, fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(respBody)))
	}

	var parsed gatewayResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		r.logger.Error("gateway response parse failed", slog.String("body_prefix", truncate(string(respBody), 300)), slog.Any("error", err))
		return gatewayResponse{}, fmt.Errorf("failed to parse gateway response: %w", err)
	}
	return parsed, nil
}

// postTriggerSchedule sends a trigger-schedule request to the agent gateway.
func (r *Resolver) postTriggerSchedule(ctx context.Context, payload triggerScheduleRequest, token string) (gatewayResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return gatewayResponse{}, err
	}
	url := r.gatewayBaseURL + "/chat/trigger-schedule"
	r.logger.Info("gateway trigger-schedule request", slog.String("url", url), slog.String("schedule_id", payload.Schedule.ID))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return gatewayResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		httpReq.Header.Set("Authorization", token)
	}

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return gatewayResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return gatewayResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.logger.Error("gateway trigger-schedule error", slog.String("url", url), slog.Int("status", resp.StatusCode), slog.String("body_prefix", truncate(string(respBody), 300)))
		return gatewayResponse{}, fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(respBody)))
	}

	var parsed gatewayResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		r.logger.Error("gateway trigger-schedule response parse failed", slog.String("body_prefix", truncate(string(respBody), 300)), slog.Any("error", err))
		return gatewayResponse{}, fmt.Errorf("failed to parse gateway response: %w", err)
	}
	return parsed, nil
}

func (r *Resolver) streamChat(ctx context.Context, payload gatewayRequest, req ChatRequest, chunkCh chan<- StreamChunk) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := r.gatewayBaseURL + "/chat/stream"
	r.logger.Info("gateway stream request", slog.String("url", url), slog.String("body_prefix", truncate(string(body), 200)))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(req.Token) != "" {
		httpReq.Header.Set("Authorization", req.Token)
	}

	resp, err := r.streamingClient.Do(httpReq)
	if err != nil {
		r.logger.Error("gateway stream connect failed", slog.String("url", url), slog.Any("error", err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		r.logger.Error("gateway stream error", slog.String("url", url), slog.Int("status", resp.StatusCode), slog.String("body_prefix", truncate(string(errBody), 300)))
		return fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(errBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	currentEvent := ""
	stored := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		chunkCh <- StreamChunk([]byte(data))

		if stored {
			continue
		}
		if handled, storeErr := r.tryStoreStream(ctx, req, currentEvent, data); storeErr != nil {
			return storeErr
		} else if handled {
			stored = true
		}
	}
	return scanner.Err()
}

// tryStoreStream attempts to extract final messages from a stream event and persist them.
func (r *Resolver) tryStoreStream(ctx context.Context, req ChatRequest, eventType, data string) (bool, error) {
	// event: done + data: {messages: [...]}
	if eventType == "done" {
		var resp gatewayResponse
		if err := json.Unmarshal([]byte(data), &resp); err == nil && len(resp.Messages) > 0 {
			return true, r.storeRound(ctx, req, resp.Messages)
		}
	}

	// data: {"type":"text_delta"|"agent_end"|"done", ...}
	var envelope struct {
		Type     string          `json:"type"`
		Data     json.RawMessage `json:"data"`
		Messages []ModelMessage  `json:"messages"`
		Skills   []string        `json:"skills"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err == nil {
		if (envelope.Type == "agent_end" || envelope.Type == "done") && len(envelope.Messages) > 0 {
			return true, r.storeRound(ctx, req, envelope.Messages)
		}
		if envelope.Type == "done" && len(envelope.Data) > 0 {
			var resp gatewayResponse
			if err := json.Unmarshal(envelope.Data, &resp); err == nil && len(resp.Messages) > 0 {
				return true, r.storeRound(ctx, req, resp.Messages)
			}
		}
	}

	// fallback: data: {messages: [...]}
	var resp gatewayResponse
	if err := json.Unmarshal([]byte(data), &resp); err == nil && len(resp.Messages) > 0 {
		return true, r.storeRound(ctx, req, resp.Messages)
	}
	return false, nil
}

// --- container resolution ---

func (r *Resolver) resolveContainerID(ctx context.Context, botID, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if r.queries != nil {
		pgBotID, err := parseUUID(botID)
		if err == nil {
			row, err := r.queries.GetContainerByBotID(ctx, pgBotID)
			if err == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID
			}
		}
	}
	return "mcp-" + botID
}

// --- message loading ---

func (r *Resolver) loadChatMessages(ctx context.Context, chatID string, maxContextMinutes int) ([]ModelMessage, error) {
	since := time.Now().UTC().Add(-time.Duration(maxContextMinutes) * time.Minute)
	msgs, err := r.chatService.ListMessagesSince(ctx, chatID, since)
	if err != nil {
		return nil, err
	}
	var result []ModelMessage
	for _, m := range msgs {
		var mm ModelMessage
		if err := json.Unmarshal(m.Content, &mm); err != nil {
			// Fallback: treat content as text string.
			mm = ModelMessage{Role: m.Role, Content: m.Content}
		} else {
			mm.Role = m.Role
		}
		result = append(result, mm)
	}
	return result, nil
}

type memoryContextItem struct {
	Namespace string
	Item      memory.MemoryItem
}

func (r *Resolver) loadMemoryContextMessage(ctx context.Context, req ChatRequest, settings Settings) *ModelMessage {
	if r.memoryService == nil {
		return nil
	}
	if strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.ChatID) == "" {
		return nil
	}
	type memoryScope struct {
		Namespace string
		ScopeID   string
	}
	var scopes []memoryScope
	if settings.EnableChatMemory {
		scopes = append(scopes, memoryScope{Namespace: "chat", ScopeID: req.ChatID})
	}
	if settings.EnablePrivateMemory && strings.TrimSpace(req.UserID) != "" {
		scopes = append(scopes, memoryScope{Namespace: "private", ScopeID: req.UserID})
	}
	if settings.EnablePublicMemory {
		scopes = append(scopes, memoryScope{Namespace: "public", ScopeID: req.BotID})
	}
	if len(scopes) == 0 {
		return nil
	}

	results := make([]memoryContextItem, 0, len(scopes)*memoryContextLimitPerScope)
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		resp, err := r.memoryService.Search(ctx, memory.SearchRequest{
			Query: req.Query,
			BotID: req.BotID,
			Limit: memoryContextLimitPerScope,
			Filters: map[string]any{
				"namespace": scope.Namespace,
				"scopeId":   scope.ScopeID,
				"botId":     req.BotID,
			},
		})
		if err != nil {
			r.logger.Warn("memory search for context failed",
				slog.String("namespace", scope.Namespace),
				slog.Any("error", err),
			)
			continue
		}
		for _, item := range resp.Results {
			key := strings.TrimSpace(item.ID)
			if key == "" {
				key = scope.Namespace + ":" + strings.TrimSpace(item.Memory)
			}
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			results = append(results, memoryContextItem{Namespace: scope.Namespace, Item: item})
		}
	}
	if len(results) == 0 {
		return nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Item.Score > results[j].Item.Score
	})
	if len(results) > memoryContextMaxItems {
		results = results[:memoryContextMaxItems]
	}

	var sb strings.Builder
	sb.WriteString("Relevant memory context (use when helpful):\n")
	for _, entry := range results {
		text := strings.TrimSpace(entry.Item.Memory)
		if text == "" {
			continue
		}
		sb.WriteString("- [")
		sb.WriteString(entry.Namespace)
		sb.WriteString("] ")
		sb.WriteString(truncateMemorySnippet(text, memoryContextItemMaxChars))
		sb.WriteString("\n")
	}
	payload := strings.TrimSpace(sb.String())
	if payload == "" {
		return nil
	}
	msg := ModelMessage{
		Role:    "system",
		Content: NewTextContent(payload),
	}
	return &msg
}

// --- store helpers ---

func (r *Resolver) storeRound(ctx context.Context, req ChatRequest, messages []ModelMessage) error {
	// Add user query as the first message if not already present in the round.
	// This ensures the user's prompt is persisted alongside the assistant's response.
	fullRound := make([]ModelMessage, 0, len(messages)+1)
	hasUserQuery := false
	for _, m := range messages {
		if m.Role == "user" && m.TextContent() == req.Query {
			hasUserQuery = true
			break
		}
	}
	if !hasUserQuery && strings.TrimSpace(req.Query) != "" {
		fullRound = append(fullRound, ModelMessage{
			Role:    "user",
			Content: NewTextContent(req.Query),
		})
	}
	fullRound = append(fullRound, messages...)

	r.storeMessages(ctx, req, fullRound)
	r.storeMemory(ctx, req.BotID, req.ChatID, req.UserID, req.Query, fullRound)
	return nil
}

func (r *Resolver) storeMessages(ctx context.Context, req ChatRequest, messages []ModelMessage) {
	if r.chatService == nil {
		return
	}
	if strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.ChatID) == "" {
		return
	}
	// Build route-level metadata for traceability.
	var meta map[string]any
	if strings.TrimSpace(req.RouteID) != "" || strings.TrimSpace(req.CurrentChannel) != "" {
		meta = map[string]any{}
		if strings.TrimSpace(req.RouteID) != "" {
			meta["route_id"] = req.RouteID
		}
		if strings.TrimSpace(req.CurrentChannel) != "" {
			meta["platform"] = req.CurrentChannel
		}
	}
	for _, msg := range messages {
		content, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		senderChannelIdentityID := ""
		senderUserID := ""
		externalMessageID := ""
		if msg.Role == "user" {
			senderChannelIdentityID = req.SourceChannelIdentityID
			senderUserID = req.UserID
			externalMessageID = req.ExternalMessageID
		}
		if _, err := r.chatService.PersistMessage(
			ctx,
			req.ChatID,
			req.BotID,
			req.RouteID,
			senderChannelIdentityID,
			senderUserID,
			req.CurrentChannel,
			externalMessageID,
			msg.Role,
			content,
			meta,
		); err != nil {
			r.logger.Warn("persist message failed", slog.Any("error", err))
		}
	}
}

func (r *Resolver) storeMemory(ctx context.Context, botID, chatID, userID, query string, messages []ModelMessage) {
	if r.memoryService == nil {
		return
	}
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(chatID) == "" {
		return
	}
	memMsgs := make([]memory.Message, 0, len(messages))
	for _, msg := range messages {
		text := strings.TrimSpace(msg.TextContent())
		if text == "" {
			continue
		}
		role := msg.Role
		if strings.TrimSpace(role) == "" {
			role = "assistant"
		}
		memMsgs = append(memMsgs, memory.Message{Role: role, Content: text})
	}
	if len(memMsgs) == 0 {
		return
	}

	// Load chat settings to determine which namespaces to write to.
	var cs Settings
	if r.chatService != nil {
		settings, err := r.chatService.GetSettings(ctx, chatID)
		if err != nil {
			r.logger.Warn("load chat settings for memory write failed", slog.Any("error", err))
		} else {
			cs = settings
			r.enforceGroupMemoryPolicy(ctx, chatID, &cs)
		}
	}

	// Always write to chat namespace if enabled (default true).
	if cs.EnableChatMemory {
		r.addMemory(ctx, botID, memMsgs, "chat", chatID)
	}

	// Write to private namespace if enabled and user id is known.
	if cs.EnablePrivateMemory && strings.TrimSpace(userID) != "" {
		r.addMemory(ctx, botID, memMsgs, "private", userID)
	}

	// Write to public namespace if enabled.
	if cs.EnablePublicMemory {
		r.addMemory(ctx, botID, memMsgs, "public", botID)
	}
}

func (r *Resolver) addMemory(ctx context.Context, botID string, msgs []memory.Message, namespace, scopeID string) {
	filters := map[string]any{
		"namespace": namespace,
		"scopeId":   scopeID,
		"botId":     botID,
	}
	if _, err := r.memoryService.Add(ctx, memory.AddRequest{
		Messages: msgs,
		BotID:    botID,
		Filters:  filters,
	}); err != nil {
		r.logger.Warn("store memory failed",
			slog.String("namespace", namespace),
			slog.String("scope_id", scopeID),
			slog.Any("error", err),
		)
	}
}

// --- model selection ---

func (r *Resolver) selectChatModel(ctx context.Context, req ChatRequest, botSettings settings.Settings, us resolvedUserSettings, cs Settings) (models.GetResponse, sqlc.LlmProvider, error) {
	if r.modelsService == nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("models service not configured")
	}
	modelID := strings.TrimSpace(req.Model)
	providerFilter := strings.TrimSpace(req.Provider)

	// Priority: request model > chat settings > bot settings > user settings.
	if modelID == "" && providerFilter == "" {
		if value := strings.TrimSpace(cs.ModelID); value != "" {
			modelID = value
		} else if value := strings.TrimSpace(botSettings.ChatModelID); value != "" {
			modelID = value
		} else if value := strings.TrimSpace(us.ChatModelID); value != "" {
			modelID = value
		}
	}

	if modelID == "" {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("chat model not configured: specify model in request or bot settings")
	}

	if providerFilter == "" {
		return r.fetchChatModel(ctx, modelID)
	}

	candidates, err := r.listCandidates(ctx, providerFilter)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	for _, m := range candidates {
		if m.ModelID == modelID {
			prov, err := models.FetchProviderByID(ctx, r.queries, m.LlmProviderID)
			if err != nil {
				return models.GetResponse{}, sqlc.LlmProvider{}, err
			}
			return m, prov, nil
		}
	}
	return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("chat model %q not found for provider %q", modelID, providerFilter)
}

func (r *Resolver) fetchChatModel(ctx context.Context, modelID string) (models.GetResponse, sqlc.LlmProvider, error) {
	model, err := r.modelsService.GetByModelID(ctx, modelID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	if model.Type != models.ModelTypeChat {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("model is not a chat model")
	}
	prov, err := models.FetchProviderByID(ctx, r.queries, model.LlmProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	return model, prov, nil
}

func (r *Resolver) listCandidates(ctx context.Context, providerFilter string) ([]models.GetResponse, error) {
	var all []models.GetResponse
	var err error
	if providerFilter != "" {
		all, err = r.modelsService.ListByClientType(ctx, models.ClientType(providerFilter))
	} else {
		all, err = r.modelsService.ListByType(ctx, models.ModelTypeChat)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]models.GetResponse, 0, len(all))
	for _, m := range all {
		if m.Type == models.ModelTypeChat {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// --- settings ---

type resolvedUserSettings struct {
	ChatModelID string
}

func (r *Resolver) loadUserSettings(ctx context.Context, userID string) (resolvedUserSettings, error) {
	if r.settingsService == nil || strings.TrimSpace(userID) == "" {
		return resolvedUserSettings{}, nil
	}
	s, err := r.settingsService.Get(ctx, userID)
	if err != nil {
		return resolvedUserSettings{}, err
	}
	return resolvedUserSettings{
		ChatModelID: strings.TrimSpace(s.ChatModelID),
	}, nil
}

func (r *Resolver) loadBotSettings(ctx context.Context, botID string) (settings.Settings, error) {
	if r.settingsService == nil {
		return settings.Settings{
			MaxContextLoadTime: settings.DefaultMaxContextLoadTime,
			Language:           settings.DefaultLanguage,
		}, nil
	}
	return r.settingsService.GetBot(ctx, botID)
}

// --- utility ---

func normalizeClientType(clientType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(clientType)) {
	case "openai", "openai-compat":
		return "openai", nil
	case "anthropic":
		return "anthropic", nil
	case "google":
		return "google", nil
	default:
		return "", fmt.Errorf("unsupported agent gateway client type: %s", clientType)
	}
}

func sanitizeMessages(messages []ModelMessage) []ModelMessage {
	cleaned := make([]ModelMessage, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) == "" {
			continue
		}
		if !msg.HasContent() && strings.TrimSpace(msg.ToolCallID) == "" {
			continue
		}
		cleaned = append(cleaned, msg)
	}
	return cleaned
}

func dedup(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, s := range items {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func coalescePositiveInt(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return defaultMaxContextMinutes
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilModelMessages(m []ModelMessage) []ModelMessage {
	if m == nil {
		return []ModelMessage{}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func truncateMemorySnippet(s string, n int) string {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) <= n {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:n]) + "..."
}

func parseUUID(id string) (pgtype.UUID, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return pgtype.UUID{}, fmt.Errorf("empty id")
	}
	var pgID pgtype.UUID
	if err := pgID.Scan(trimmed); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	return pgID, nil
}

func (r *Resolver) enforceGroupMemoryPolicy(ctx context.Context, chatID string, settings *Settings) {
	if r == nil || r.chatService == nil || settings == nil {
		return
	}
	chatObj, err := r.chatService.Get(ctx, chatID)
	if err != nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(chatObj.Kind), KindGroup) {
		settings.EnablePrivateMemory = false
	}
}
