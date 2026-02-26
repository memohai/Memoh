package flow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/inbox"
	"github.com/memohai/memoh/internal/memory"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/settings"
)

const (
	defaultMaxContextMinutes   = 24 * 60
	memoryContextLimitPerScope = 4
	memoryContextMaxItems      = 8
	memoryContextItemMaxChars  = 220
	sharedMemoryNamespace      = "bot"
	// Keep gateway payload bounded when inlining binary attachments as data URLs.
	gatewayInlineAttachmentMaxBytes int64 = 20 * 1024 * 1024
	// SSE payloads (especially attachment/tool results) can be very large.
	// bufio.Scanner hard-fails with "token too long" if a single line exceeds its max token size.
	// Use a reader-based parser and enforce an explicit per-line cap here. The agent gateway
	// stream is expected to chunk large JSON payloads across multiple SSE "data:" lines, so
	// this limit should stay relatively small.
	gatewaySSEMaxLineBytes = 256 * 1024
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

// ConversationSettingsReader defines settings lookup behavior needed by flow resolution.
type ConversationSettingsReader interface {
	GetSettings(ctx context.Context, conversationID string) (conversation.Settings, error)
}

// gatewayAssetLoader resolves content_hash references to binary payloads for gateway dispatch.
type gatewayAssetLoader interface {
	OpenForGateway(ctx context.Context, botID, contentHash string) (reader io.ReadCloser, mime string, err error)
}

// Resolver orchestrates chat with the agent gateway.
type Resolver struct {
	modelsService   *models.Service
	queries         *sqlc.Queries
	memoryService   *memory.Service
	conversationSvc ConversationSettingsReader
	messageService  messagepkg.Service
	settingsService *settings.Service
	inboxService    *inbox.Service
	skillLoader     SkillLoader
	assetLoader     gatewayAssetLoader
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
	conversationSvc ConversationSettingsReader,
	messageService messagepkg.Service,
	settingsService *settings.Service,
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
		conversationSvc: conversationSvc,
		messageService:  messageService,
		settingsService: settingsService,
		gatewayBaseURL:  gatewayBaseURL,
		timeout:         timeout,
		logger:          log.With(slog.String("service", "conversation_resolver")),
		httpClient:      &http.Client{Timeout: timeout},
		streamingClient: &http.Client{},
	}
}

// SetSkillLoader sets the skill loader used to populate usable skills in gateway requests.
func (r *Resolver) SetSkillLoader(sl SkillLoader) {
	r.skillLoader = sl
}

// SetGatewayAssetLoader configures optional asset loading used to inline
// attachments before calling the agent gateway.
func (r *Resolver) SetGatewayAssetLoader(loader gatewayAssetLoader) {
	r.assetLoader = loader
}

// SetInboxService configures inbox support for injecting unread items into the
// system prompt and marking them as read after a response.
func (r *Resolver) SetInboxService(service *inbox.Service) {
	r.inboxService = service
}

// --- gateway payload ---

type gatewayReasoningConfig struct {
	Enabled bool   `json:"enabled"`
	Effort  string `json:"effort"`
}

type gatewayModelConfig struct {
	ModelID    string                  `json:"modelId"`
	ClientType string                  `json:"clientType"`
	Input      []string                `json:"input"`
	APIKey     string                  `json:"apiKey"`
	BaseURL    string                  `json:"baseUrl"`
	Reasoning  *gatewayReasoningConfig `json:"reasoning,omitempty"`
}

type gatewayIdentity struct {
	BotID             string `json:"botId"`
	ContainerID       string `json:"containerId"`
	ChannelIdentityID string `json:"channelIdentityId"`
	DisplayName       string `json:"displayName"`
	CurrentPlatform   string `json:"currentPlatform,omitempty"`
	ConversationType  string `json:"conversationType,omitempty"`
	SessionToken      string `json:"sessionToken,omitempty"`
}

type gatewaySkill struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type gatewayInboxItem struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Header    map[string]any `json:"header"`
	Content   string         `json:"content"`
	CreatedAt string         `json:"createdAt"`
}

type gatewayRequest struct {
	Model             gatewayModelConfig          `json:"model"`
	ActiveContextTime int                         `json:"activeContextTime"`
	Channels          []string                    `json:"channels"`
	CurrentChannel    string                      `json:"currentChannel"`
	AllowedActions    []string                    `json:"allowedActions,omitempty"`
	Messages          []conversation.ModelMessage `json:"messages"`
	Skills            []string                    `json:"skills"`
	UsableSkills      []gatewaySkill              `json:"usableSkills"`
	Query             string                      `json:"query"`
	Identity          gatewayIdentity             `json:"identity"`
	Attachments       []any                       `json:"attachments"`
	Inbox             []gatewayInboxItem          `json:"inbox,omitempty"`
}

type gatewayResponse struct {
	Messages []conversation.ModelMessage `json:"messages"`
	Skills   []string                    `json:"skills"`
	Text     string                      `json:"text,omitempty"`
	Usage    json.RawMessage             `json:"usage,omitempty"`
	Usages   []json.RawMessage           `json:"usages,omitempty"`
}

type gatewayUsage struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
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
// It omits "query" from JSON so the trigger-schedule endpoint does not receive it.
type triggerScheduleRequest struct {
	gatewayRequest
	Schedule gatewaySchedule `json:"schedule"`
}

// MarshalJSON marshals the request without the "query" field for trigger-schedule.
func (t triggerScheduleRequest) MarshalJSON() ([]byte, error) {
	type alias struct {
		gatewayRequest
		Schedule gatewaySchedule `json:"schedule"`
	}
	raw, err := json.Marshal(alias(t))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "query")
	return json.Marshal(m)
}

// gatewayHeartbeat matches the agent gateway HeartbeatModel for /chat/trigger-heartbeat.
type gatewayHeartbeat struct {
	Interval int `json:"interval"`
}

// triggerHeartbeatRequest is the payload for POST /chat/trigger-heartbeat.
type triggerHeartbeatRequest struct {
	gatewayRequest
	Heartbeat gatewayHeartbeat `json:"heartbeat"`
}

// MarshalJSON marshals the request without the "query" field for trigger-heartbeat.
func (t triggerHeartbeatRequest) MarshalJSON() ([]byte, error) {
	type alias struct {
		gatewayRequest
		Heartbeat gatewayHeartbeat `json:"heartbeat"`
	}
	raw, err := json.Marshal(alias(t))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "query")
	return json.Marshal(m)
}

// --- resolved context (shared by Chat / StreamChat / TriggerSchedule) ---

type resolvedContext struct {
	payload      gatewayRequest
	model        models.GetResponse
	provider     sqlc.LlmProvider
	inboxItemIDs []string
}

func (r *Resolver) resolve(ctx context.Context, req conversation.ChatRequest) (resolvedContext, error) {
	if strings.TrimSpace(req.Query) == "" && len(req.Attachments) == 0 {
		return resolvedContext{}, fmt.Errorf("query or attachments is required")
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
	var chatSettings conversation.Settings
	if r.conversationSvc != nil {
		chatSettings, err = r.conversationSvc.GetSettings(ctx, req.ChatID)
		if err != nil {
			return resolvedContext{}, err
		}
	}

	chatModel, provider, err := r.selectChatModel(ctx, req, botSettings, chatSettings)
	if err != nil {
		return resolvedContext{}, err
	}
	clientType := string(chatModel.ClientType)

	maxCtx := coalescePositiveInt(req.MaxContextLoadTime, botSettings.MaxContextLoadTime, defaultMaxContextMinutes)
	maxTokens := botSettings.MaxContextTokens

	// Build non-history parts first so we can reserve their token cost before
	// trimming history messages.
	memoryMsg := r.loadMemoryContextMessage(ctx, req)
	reqMessages := pruneMessagesForGateway(nonNilModelMessages(req.Messages))
	if memoryMsg != nil {
		pruned, _ := pruneMessageForGateway(*memoryMsg)
		memoryMsg = &pruned
	}
	var overhead int
	if memoryMsg != nil {
		overhead += estimateMessageTokens(*memoryMsg)
	}
	for _, m := range reqMessages {
		overhead += estimateMessageTokens(m)
	}
	// Reserve space for the system prompt built by the agent gateway
	// (IDENTITY.md, SOUL.md, TOOLS.md, skills, boilerplate, user prompt, etc.).
	const systemPromptReserve = 4096
	overhead += systemPromptReserve

	historyBudget := maxTokens - overhead
	if historyBudget < 0 {
		historyBudget = 0
	}

	r.logger.Debug("context token budget",
		slog.Int("max_tokens", maxTokens),
		slog.Int("overhead", overhead),
		slog.Int("system_prompt_reserve", systemPromptReserve),
		slog.Int("history_budget", historyBudget),
	)

	var messages []conversation.ModelMessage
	if !skipHistory && r.conversationSvc != nil {
		loaded, loadErr := r.loadMessages(ctx, req.ChatID, maxCtx)
		if loadErr != nil {
			return resolvedContext{}, loadErr
		}
		loaded = pruneHistoryForGateway(loaded)
		messages = trimMessagesByTokens(loaded, historyBudget)
		r.logger.Debug("context trim result",
			slog.Int("loaded_messages", len(loaded)),
			slog.Int("kept_messages", len(messages)),
			slog.Int("trimmed_messages", len(loaded)-len(messages)),
			slog.Int("history_budget", historyBudget),
		)
	}
	if memoryMsg != nil {
		messages = append(messages, *memoryMsg)
	}
	messages = append(messages, reqMessages...)
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
				skill, ok := normalizeGatewaySkill(e)
				if !ok {
					continue
				}
				usableSkills = append(usableSkills, skill)
			}
		}
	}
	if usableSkills == nil {
		usableSkills = []gatewaySkill{}
	}

	var inboxGatewayItems []gatewayInboxItem
	var inboxItemIDs []string
	if r.inboxService != nil {
		maxInbox := botSettings.MaxInboxItems
		if maxInbox <= 0 {
			maxInbox = settings.DefaultMaxInboxItems
		}
		items, err := r.inboxService.ListUnread(ctx, req.BotID, maxInbox)
		if err != nil {
			r.logger.Warn("failed to load inbox items", slog.String("bot_id", req.BotID), slog.Any("error", err))
		} else if len(items) > 0 {
			inboxGatewayItems = make([]gatewayInboxItem, 0, len(items))
			inboxItemIDs = make([]string, 0, len(items))
			for _, item := range items {
				inboxGatewayItems = append(inboxGatewayItems, gatewayInboxItem{
					ID:        item.ID,
					Source:    item.Source,
					Header:    item.Header,
					Content:   item.Content,
					CreatedAt: item.CreatedAt.Format(time.RFC3339),
				})
				inboxItemIDs = append(inboxItemIDs, item.ID)
			}
		}
	}

	attachments := r.routeAndMergeAttachments(ctx, chatModel, req)
	displayName := r.resolveDisplayName(ctx, req)

	headerifiedQuery := FormatUserHeader(
		strings.TrimSpace(req.ExternalMessageID),
		strings.TrimSpace(req.SourceChannelIdentityID),
		displayName,
		req.CurrentChannel,
		strings.TrimSpace(req.ConversationType),
		strings.TrimSpace(req.ConversationName),
		extractFileRefPaths(attachments),
		req.Query,
	)

	var reasoning *gatewayReasoningConfig
	if chatModel.SupportsReasoning && botSettings.ReasoningEnabled {
		reasoning = &gatewayReasoningConfig{
			Enabled: true,
			Effort:  botSettings.ReasoningEffort,
		}
	}

	payload := gatewayRequest{
		Model: gatewayModelConfig{
			ModelID:    chatModel.ModelID,
			ClientType: clientType,
			Input:      chatModel.InputModalities,
			APIKey:     provider.ApiKey,
			BaseURL:    provider.BaseUrl,
			Reasoning:  reasoning,
		},
		ActiveContextTime: maxCtx,
		Channels:          nonNilStrings(req.Channels),
		CurrentChannel:    req.CurrentChannel,
		AllowedActions:    req.AllowedActions,
		Messages:          nonNilModelMessages(messages),
		Skills:            nonNilStrings(skills),
		UsableSkills:      usableSkills,
		Query:             headerifiedQuery,
		Identity: gatewayIdentity{
			BotID:             req.BotID,
			ContainerID:       containerID,
			ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
			DisplayName:       displayName,
			CurrentPlatform:   req.CurrentChannel,
			ConversationType:  strings.TrimSpace(req.ConversationType),
			SessionToken:      req.ChatToken,
		},
		Attachments: attachments,
		Inbox:       inboxGatewayItems,
	}

	return resolvedContext{payload: payload, model: chatModel, provider: provider, inboxItemIDs: inboxItemIDs}, nil
}

// --- Chat ---

// Chat sends a synchronous chat request to the agent gateway and stores the result.
func (r *Resolver) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	req.Query = rc.payload.Query
	resp, err := r.postChat(ctx, rc.payload, req.Token)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	if err := r.storeRound(ctx, req, resp.Messages, resp.Usage, resp.Usages); err != nil {
		return conversation.ChatResponse{}, err
	}
	r.markInboxRead(ctx, req.BotID, rc.inboxItemIDs)
	return conversation.ChatResponse{
		Messages: resp.Messages,
		Skills:   resp.Skills,
		Model:    rc.model.ModelID,
		Provider: string(rc.model.ClientType),
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

	req := conversation.ChatRequest{
		BotID:  botID,
		ChatID: botID,
		Query:  payload.Command,
		UserID: payload.OwnerUserID,
		Token:  token,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return err
	}

	schedulePayload := rc.payload
	schedulePayload.Identity.ChannelIdentityID = strings.TrimSpace(payload.OwnerUserID)
	schedulePayload.Identity.DisplayName = "Scheduler"

	triggerReq := triggerScheduleRequest{
		gatewayRequest: schedulePayload,
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
	return r.storeRound(ctx, req, resp.Messages, resp.Usage, resp.Usages)
}

// --- TriggerHeartbeat ---

// TriggerHeartbeat executes a heartbeat check through the agent gateway trigger-heartbeat endpoint.
func (r *Resolver) TriggerHeartbeat(ctx context.Context, botID string, payload heartbeat.TriggerPayload, token string) (heartbeat.TriggerResult, error) {
	if strings.TrimSpace(botID) == "" {
		return heartbeat.TriggerResult{}, fmt.Errorf("bot id is required")
	}

	// If a dedicated heartbeat model is configured, use it instead of the
	// default chat model.  We load the bot settings first so that we can
	// set req.Model, which takes highest priority in selectChatModel.
	var heartbeatModel string
	if botSettings, err := r.loadBotSettings(ctx, botID); err == nil {
		heartbeatModel = strings.TrimSpace(botSettings.HeartbeatModelID)
	}

	req := conversation.ChatRequest{
		BotID:  botID,
		ChatID: botID,
		Query:  "heartbeat",
		UserID: payload.OwnerUserID,
		Token:  token,
		Model:  heartbeatModel,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return heartbeat.TriggerResult{}, err
	}

	hbPayload := rc.payload
	hbPayload.Identity.ChannelIdentityID = strings.TrimSpace(payload.OwnerUserID)
	hbPayload.Identity.DisplayName = "Heartbeat"

	triggerReq := triggerHeartbeatRequest{
		gatewayRequest: hbPayload,
		Heartbeat: gatewayHeartbeat{
			Interval: payload.Interval,
		},
	}

	resp, err := r.postTriggerHeartbeat(ctx, triggerReq, token)
	if err != nil {
		return heartbeat.TriggerResult{}, err
	}

	status := "alert"
	text := strings.TrimSpace(resp.Text)
	if isHeartbeatOK(text) {
		status = "ok"
	}

	var usageBytes []byte
	if resp.Usage != nil {
		usageBytes, _ = json.Marshal(resp.Usage)
	}

	return heartbeat.TriggerResult{
		Status:     status,
		Text:       text,
		Usage:      resp.Usage,
		UsageBytes: usageBytes,
	}, nil
}

func isHeartbeatOK(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "HEARTBEAT_OK") || strings.HasSuffix(t, "HEARTBEAT_OK") || t == "HEARTBEAT_OK"
}

// --- StreamChat ---

// StreamChat sends a streaming chat request to the agent gateway.
func (r *Resolver) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	chunkCh := make(chan conversation.StreamChunk)
	errCh := make(chan error, 1)
	r.logger.Info("gateway stream start",
		slog.String("bot_id", req.BotID),
		slog.String("chat_id", req.ChatID),
	)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		streamReq := req
		rc, err := r.resolve(ctx, streamReq)
		if err != nil {
			r.logger.Error("gateway stream resolve failed",
				slog.String("bot_id", streamReq.BotID),
				slog.String("chat_id", streamReq.ChatID),
				slog.Any("error", err),
			)
			errCh <- err
			return
		}
		streamReq.Query = rc.payload.Query
		if !streamReq.UserMessagePersisted {
			if err := r.persistUserMessage(ctx, streamReq); err != nil {
				r.logger.Error("gateway stream persist user message failed",
					slog.String("bot_id", streamReq.BotID),
					slog.String("chat_id", streamReq.ChatID),
					slog.Any("error", err),
				)
				errCh <- err
				return
			}
			streamReq.UserMessagePersisted = true
		}
		if err := r.streamChat(ctx, rc.payload, streamReq, chunkCh); err != nil {
			r.logger.Error("gateway stream request failed",
				slog.String("bot_id", streamReq.BotID),
				slog.String("chat_id", streamReq.ChatID),
				slog.Any("error", err),
			)
			errCh <- err
			return
		}
		r.markInboxRead(ctx, streamReq.BotID, rc.inboxItemIDs)
	}()
	return chunkCh, errCh
}

// --- HTTP helpers ---

func (r *Resolver) postChat(ctx context.Context, payload gatewayRequest, token string) (gatewayResponse, error) {
	url := r.gatewayBaseURL + "/chat/"
	r.logger.Info(
		"gateway request",
		slog.String("url", url),
		slog.Int("messages", len(payload.Messages)),
		slog.Int("attachments", len(payload.Attachments)),
	)

	httpReq, err := newJSONRequestWithContext(ctx, http.MethodPost, url, payload)
	if err != nil {
		return gatewayResponse{}, err
	}
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
	url := r.gatewayBaseURL + "/chat/trigger-schedule"
	r.logger.Info("gateway trigger-schedule request", slog.String("url", url), slog.String("schedule_id", payload.Schedule.ID))

	httpReq, err := newJSONRequestWithContext(ctx, http.MethodPost, url, payload)
	if err != nil {
		return gatewayResponse{}, err
	}
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

// postTriggerHeartbeat sends a trigger-heartbeat request to the agent gateway.
func (r *Resolver) postTriggerHeartbeat(ctx context.Context, payload triggerHeartbeatRequest, token string) (gatewayResponse, error) {
	url := r.gatewayBaseURL + "/chat/trigger-heartbeat"
	r.logger.Info("gateway trigger-heartbeat request", slog.String("url", url))

	httpReq, err := newJSONRequestWithContext(ctx, http.MethodPost, url, payload)
	if err != nil {
		return gatewayResponse{}, err
	}
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
		r.logger.Error("gateway trigger-heartbeat error", slog.String("url", url), slog.Int("status", resp.StatusCode), slog.String("body_prefix", truncate(string(respBody), 300)))
		return gatewayResponse{}, fmt.Errorf("agent gateway error: %s", strings.TrimSpace(string(respBody)))
	}

	var parsed gatewayResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		r.logger.Error("gateway trigger-heartbeat response parse failed", slog.String("body_prefix", truncate(string(respBody), 300)), slog.Any("error", err))
		return gatewayResponse{}, fmt.Errorf("failed to parse gateway response: %w", err)
	}
	return parsed, nil
}

func (r *Resolver) streamChat(ctx context.Context, payload gatewayRequest, req conversation.ChatRequest, chunkCh chan<- conversation.StreamChunk) error {
	url := r.gatewayBaseURL + "/chat/stream"
	r.logger.Info(
		"gateway stream request",
		slog.String("url", url),
		slog.Int("messages", len(payload.Messages)),
		slog.Int("attachments", len(payload.Attachments)),
	)
	httpReq, err := newJSONRequestWithContext(ctx, http.MethodPost, url, payload)
	if err != nil {
		return err
	}
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

	stored := false
	var dataBuf bytes.Buffer

	flushEvent := func() error {
		if dataBuf.Len() == 0 {
			return nil
		}
		out := append([]byte(nil), dataBuf.Bytes()...)
		dataBuf.Reset()
		if len(out) == 0 || bytes.Equal(bytes.TrimSpace(out), []byte("[DONE]")) {
			return nil
		}
		// Persist final messages before forwarding the "done"/"agent_end" event so the
		// next user turn can immediately see the assistant output in history.
		if !stored {
			if handled, storeErr := r.tryStoreStream(ctx, req, out); storeErr != nil {
				return storeErr
			} else if handled {
				stored = true
			}
		}
		chunkCh <- conversation.StreamChunk(out)
		return nil
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), gatewaySSEMaxLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if err := flushEvent(); err != nil {
				return err
			}
			continue
		}
		if len(line) > 0 && line[0] == ':' {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		part := bytes.TrimPrefix(line, []byte("data:"))
		// Backward-compat: older SSE writers used "data: <payload>" (note the space).
		// Only strip the first leading space for the *first* fragment to avoid corrupting
		// chunked payloads split inside JSON string values.
		if dataBuf.Len() == 0 && len(part) > 0 && part[0] == ' ' {
			part = part[1:]
		}
		if len(part) == 0 {
			continue
		}
		_, _ = dataBuf.Write(part)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("sse line too long (max %d bytes)", gatewaySSEMaxLineBytes)
		}
		return err
	}
	return flushEvent()
}

func newJSONRequestWithContext(ctx context.Context, method, url string, payload any) (*http.Request, error) {
	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		_ = pw.CloseWithError(enc.Encode(payload))
	}()
	req, err := http.NewRequestWithContext(ctx, method, url, pr)
	if err != nil {
		_ = pr.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// tryStoreStream attempts to extract final messages from a stream event and persist them.
func (r *Resolver) tryStoreStream(ctx context.Context, req conversation.ChatRequest, data []byte) (bool, error) {
	// data: {"type":"text_delta"|"agent_end"|"done", ...}
	var envelope struct {
		Type     string                      `json:"type"`
		Data     json.RawMessage             `json:"data"`
		Messages []conversation.ModelMessage `json:"messages"`
		Usage    json.RawMessage             `json:"usage,omitempty"`
		Usages   []json.RawMessage           `json:"usages,omitempty"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil {
		if (envelope.Type == "agent_end" || envelope.Type == "done") && len(envelope.Messages) > 0 {
			return true, r.storeRound(ctx, req, envelope.Messages, envelope.Usage, envelope.Usages)
		}
		if envelope.Type == "done" && len(envelope.Data) > 0 {
			var resp gatewayResponse
			if err := json.Unmarshal(envelope.Data, &resp); err == nil && len(resp.Messages) > 0 {
				return true, r.storeRound(ctx, req, resp.Messages, resp.Usage, resp.Usages)
			}
		}
	}

	// fallback: data: {messages: [...]}
	var resp gatewayResponse
	if err := json.Unmarshal(data, &resp); err == nil && len(resp.Messages) > 0 {
		return true, r.storeRound(ctx, req, resp.Messages, resp.Usage, resp.Usages)
	}
	return false, nil
}

// routeAndMergeAttachments applies CapabilityFallbackPolicy to split
// request attachments by model input modalities, then merges the results
// into a single []any for the gateway request.
func (r *Resolver) routeAndMergeAttachments(ctx context.Context, model models.GetResponse, req conversation.ChatRequest) []any {
	if len(req.Attachments) == 0 {
		return []any{}
	}
	typed := r.prepareGatewayAttachments(ctx, req)
	routed := routeAttachmentsByCapability(model.InputModalities, typed)
	// Convert unsupported attachments to tool file references.
	for i := range routed.Fallback {
		fallbackPath := strings.TrimSpace(routed.Fallback[i].FallbackPath)
		if fallbackPath == "" {
			// Cannot downgrade non-file payloads to tool file references.
			// Drop them explicitly to keep gateway contract deterministic.
			if r != nil && r.logger != nil {
				r.logger.Warn(
					"drop attachment without fallback path",
					slog.String("type", strings.TrimSpace(routed.Fallback[i].Type)),
					slog.String("transport", strings.TrimSpace(routed.Fallback[i].Transport)),
					slog.String("content_hash", strings.TrimSpace(routed.Fallback[i].ContentHash)),
					slog.Bool("has_payload", strings.TrimSpace(routed.Fallback[i].Payload) != ""),
				)
			}
			routed.Fallback[i] = gatewayAttachment{}
			continue
		}
		routed.Fallback[i].Type = "file"
		routed.Fallback[i].Transport = gatewayTransportToolFileRef
		routed.Fallback[i].Payload = fallbackPath
	}
	merged := make([]any, 0, len(routed.Native)+len(routed.Fallback))
	merged = append(merged, attachmentsToAny(routed.Native)...)
	for _, fb := range routed.Fallback {
		if fb.Type == "" || strings.TrimSpace(fb.Transport) == "" || strings.TrimSpace(fb.Payload) == "" {
			continue
		}
		merged = append(merged, fb)
	}
	if len(merged) == 0 {
		return []any{}
	}
	return merged
}

func (r *Resolver) prepareGatewayAttachments(ctx context.Context, req conversation.ChatRequest) []gatewayAttachment {
	if len(req.Attachments) == 0 {
		return nil
	}
	prepared := make([]gatewayAttachment, 0, len(req.Attachments))
	for _, raw := range req.Attachments {
		attachmentType := strings.ToLower(strings.TrimSpace(raw.Type))
		payload := strings.TrimSpace(raw.Base64)
		transport := ""
		fallbackPath := strings.TrimSpace(raw.Path)
		if payload != "" {
			transport = gatewayTransportInlineDataURL
		} else {
			rawURL := strings.TrimSpace(raw.URL)
			if isDataURL(rawURL) {
				payload = rawURL
				transport = gatewayTransportInlineDataURL
			} else if isLikelyPublicURL(rawURL) {
				payload = rawURL
				transport = gatewayTransportPublicURL
			} else if rawURL != "" && fallbackPath == "" {
				fallbackPath = rawURL
			}
		}
		item := gatewayAttachment{
			ContentHash:  strings.TrimSpace(raw.ContentHash),
			Type:         attachmentType,
			Mime:         strings.TrimSpace(raw.Mime),
			Size:         raw.Size,
			Name:         strings.TrimSpace(raw.Name),
			Transport:    transport,
			Payload:      payload,
			Metadata:     raw.Metadata,
			FallbackPath: fallbackPath,
		}
		item = normalizeGatewayAttachmentPayload(item)
		item = r.inlineImageAttachmentAssetIfNeeded(ctx, strings.TrimSpace(req.BotID), item)
		prepared = append(prepared, item)
	}
	return prepared
}

func normalizeGatewayAttachmentPayload(item gatewayAttachment) gatewayAttachment {
	if item.Transport != gatewayTransportInlineDataURL {
		return item
	}
	payload := strings.TrimSpace(item.Payload)
	if payload == "" {
		return item
	}
	if strings.HasPrefix(strings.ToLower(payload), "data:") {
		mime := strings.TrimSpace(item.Mime)
		if mime == "" || strings.EqualFold(mime, "application/octet-stream") {
			if extracted := attachmentpkg.MimeFromDataURL(payload); extracted != "" {
				item.Mime = extracted
			}
		}
		item.Payload = payload
		return item
	}
	mime := strings.TrimSpace(item.Mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	item.Payload = attachmentpkg.NormalizeBase64DataURL(payload, mime)
	return item
}

func isLikelyPublicURL(raw string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

func isDataURL(raw string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(trimmed, "data:")
}

func (r *Resolver) inlineImageAttachmentAssetIfNeeded(ctx context.Context, botID string, item gatewayAttachment) gatewayAttachment {
	if item.Type != "image" {
		return item
	}
	if strings.TrimSpace(item.Payload) != "" &&
		(item.Transport == gatewayTransportInlineDataURL || item.Transport == gatewayTransportPublicURL) {
		return item
	}
	contentHash := strings.TrimSpace(item.ContentHash)
	if contentHash == "" {
		return item
	}
	dataURL, mime, err := r.inlineAssetAsDataURL(ctx, botID, contentHash, item.Type, item.Mime)
	if err != nil {
		if r != nil && r.logger != nil {
			r.logger.Warn(
				"inline gateway image attachment failed",
				slog.Any("error", err),
				slog.String("bot_id", botID),
				slog.String("content_hash", contentHash),
			)
		}
		return item
	}
	item.Transport = gatewayTransportInlineDataURL
	item.Payload = dataURL
	if strings.TrimSpace(item.Mime) == "" {
		item.Mime = mime
	}
	return item
}

func (r *Resolver) inlineAssetAsDataURL(ctx context.Context, botID, contentHash, attachmentType, fallbackMime string) (string, string, error) {
	if r == nil || r.assetLoader == nil {
		return "", "", fmt.Errorf("gateway asset loader not configured")
	}
	reader, assetMime, err := r.assetLoader.OpenForGateway(ctx, botID, contentHash)
	if err != nil {
		return "", "", fmt.Errorf("open asset: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	mime := strings.TrimSpace(fallbackMime)
	if mime == "" {
		mime = strings.TrimSpace(assetMime)
	}
	dataURL, resolvedMime, err := encodeReaderAsDataURL(reader, gatewayInlineAttachmentMaxBytes, attachmentType, mime)
	if err != nil {
		return "", "", err
	}
	return dataURL, resolvedMime, nil
}

func encodeReaderAsDataURL(reader io.Reader, maxBytes int64, attachmentType, fallbackMime string) (string, string, error) {
	if reader == nil {
		return "", "", fmt.Errorf("reader is required")
	}
	if maxBytes <= 0 {
		return "", "", fmt.Errorf("max bytes must be greater than 0")
	}
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	head := make([]byte, 512)
	n, err := limited.Read(head)
	if err != nil && err != io.EOF {
		return "", "", fmt.Errorf("read asset: %w", err)
	}
	head = head[:n]

	mime := strings.TrimSpace(fallbackMime)
	if strings.EqualFold(strings.TrimSpace(attachmentType), "image") &&
		(strings.TrimSpace(mime) == "" || strings.EqualFold(strings.TrimSpace(mime), "application/octet-stream")) {
		detected := strings.TrimSpace(http.DetectContentType(head))
		if strings.HasPrefix(strings.ToLower(detected), "image/") {
			mime = detected
		}
	}
	if mime == "" {
		mime = "application/octet-stream"
	}

	var encoded strings.Builder
	encoded.Grow(len("data:") + len(mime) + len(";base64,"))
	encoded.WriteString("data:")
	encoded.WriteString(mime)
	encoded.WriteString(";base64,")

	encoder := base64.NewEncoder(base64.StdEncoding, &encoded)
	if len(head) > 0 {
		if _, err := encoder.Write(head); err != nil {
			_ = encoder.Close()
			return "", "", fmt.Errorf("encode asset head: %w", err)
		}
	}
	copied, err := io.Copy(encoder, limited)
	if err != nil {
		_ = encoder.Close()
		return "", "", fmt.Errorf("encode asset body: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", "", fmt.Errorf("finalize asset encoding: %w", err)
	}

	total := int64(len(head)) + copied
	if total > maxBytes {
		return "", "", fmt.Errorf(
			"asset too large to inline: %d > %d",
			total,
			maxBytes,
		)
	}
	return encoded.String(), mime, nil
}

// --- container resolution ---

func (r *Resolver) resolveContainerID(ctx context.Context, botID, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if r.queries != nil {
		pgBotID, err := parseResolverUUID(botID)
		if err == nil {
			row, err := r.queries.GetContainerByBotID(ctx, pgBotID)
			if err == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID
			}
		}
	}
	r.logger.Warn("no container found for bot, using fallback", slog.String("bot_id", botID))
	return "mcp-" + botID
}

// --- message loading ---

type messageWithUsage struct {
	Message           conversation.ModelMessage
	UsageInputTokens  *int
	UsageOutputTokens *int
}

func (r *Resolver) loadMessages(ctx context.Context, chatID string, maxContextMinutes int) ([]messageWithUsage, error) {
	if r.messageService == nil {
		return nil, nil
	}
	since := time.Now().UTC().Add(-time.Duration(maxContextMinutes) * time.Minute)
	msgs, err := r.messageService.ListActiveSince(ctx, chatID, since)
	if err != nil {
		return nil, err
	}
	var result []messageWithUsage
	for _, m := range msgs {
		var mm conversation.ModelMessage
		if err := json.Unmarshal(m.Content, &mm); err != nil {
			r.logger.Warn("loadMessages: content unmarshal failed, treating as raw text",
				slog.String("chat_id", chatID), slog.Any("error", err))
			mm = conversation.ModelMessage{Role: m.Role, Content: m.Content}
		} else {
			mm.Role = m.Role
		}
		var inputTokens *int
		var outputTokens *int
		if len(m.Usage) > 0 {
			var u gatewayUsage
			if json.Unmarshal(m.Usage, &u) == nil {
				inputTokens = u.InputTokens
				outputTokens = u.OutputTokens
			}
		}
		result = append(result, messageWithUsage{Message: mm, UsageInputTokens: inputTokens, UsageOutputTokens: outputTokens})
	}
	return result, nil
}

func estimateMessageTokens(msg conversation.ModelMessage) int {
	text := msg.TextContent()
	if len(text) == 0 {
		data, _ := json.Marshal(msg.Content)
		return len(data) / 4
	}
	return len(text) / 4
}

func trimMessagesByTokens(messages []messageWithUsage, maxTokens int) []conversation.ModelMessage {
	if maxTokens <= 0 || len(messages) == 0 {
		result := make([]conversation.ModelMessage, len(messages))
		for i, m := range messages {
			result[i] = m.Message
		}
		return result
	}

	// Scan from newest to oldest, accumulating per-message outputTokens from
	// stored usage data. Messages without usage (user / tool) are included for
	// free — the outputTokens of surrounding assistant turns already account
	// for the context they consumed.
	totalTokens := 0
	cutoff := 0
	messagesWithUsage := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].UsageOutputTokens != nil {
			totalTokens += *messages[i].UsageOutputTokens
			messagesWithUsage++
		}
		if totalTokens > maxTokens {
			cutoff = i + 1
			break
		}
	}

	// Keep provider-valid message order: a "tool" message must follow a preceding
	// assistant tool call. When history is head-trimmed, a leading tool message
	// may become orphaned and cause provider 400 errors.
	for cutoff < len(messages) && strings.EqualFold(strings.TrimSpace(messages[cutoff].Message.Role), "tool") {
		cutoff++
	}

	slog.Debug("trimMessagesByTokens",
		slog.Int("total_messages", len(messages)),
		slog.Int("messages_with_usage", messagesWithUsage),
		slog.Int("accumulated_output_tokens", totalTokens),
		slog.Int("max_tokens", maxTokens),
		slog.Int("cutoff_index", cutoff),
		slog.Int("kept_messages", len(messages)-cutoff),
	)

	result := make([]conversation.ModelMessage, 0, len(messages)-cutoff)
	for _, m := range messages[cutoff:] {
		result = append(result, m.Message)
	}
	return result
}

type memoryContextItem struct {
	Namespace string
	Item      memory.MemoryItem
}

func (r *Resolver) loadMemoryContextMessage(ctx context.Context, req conversation.ChatRequest) *conversation.ModelMessage {
	if r.memoryService == nil {
		return nil
	}
	if strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.ChatID) == "" {
		return nil
	}

	results := make([]memoryContextItem, 0, memoryContextLimitPerScope)
	seen := map[string]struct{}{}
	resp, err := r.memoryService.Search(ctx, memory.SearchRequest{
		Query: req.Query,
		BotID: req.BotID,
		Limit: memoryContextLimitPerScope,
		Filters: map[string]any{
			"namespace": sharedMemoryNamespace,
			"scopeId":   req.BotID,
			"bot_id":    req.BotID,
		},
		NoStats: true,
	})
	if err != nil {
		r.logger.Warn("memory search for context failed",
			slog.String("namespace", sharedMemoryNamespace),
			slog.Any("error", err),
		)
		return nil
	}
	for _, item := range resp.Results {
		key := strings.TrimSpace(item.ID)
		if key == "" {
			key = sharedMemoryNamespace + ":" + strings.TrimSpace(item.Memory)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, memoryContextItem{Namespace: sharedMemoryNamespace, Item: item})
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
	msg := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(payload),
	}
	return &msg
}

// --- store helpers ---

func (r *Resolver) persistUserMessage(ctx context.Context, req conversation.ChatRequest) error {
	if r.messageService == nil {
		return nil
	}
	if strings.TrimSpace(req.BotID) == "" {
		return fmt.Errorf("bot id is required for persistence")
	}
	text := strings.TrimSpace(req.Query)
	if text == "" && len(req.Attachments) == 0 {
		return nil
	}

	message := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(text),
	}
	content, err := json.Marshal(message)
	if err != nil {
		return err
	}
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)
	_, err = r.messageService.Persist(ctx, messagepkg.PersistInput{
		BotID:                   req.BotID,
		RouteID:                 req.RouteID,
		SenderChannelIdentityID: senderChannelIdentityID,
		SenderUserID:            senderUserID,
		Platform:                req.CurrentChannel,
		ExternalMessageID:       req.ExternalMessageID,
		Role:                    "user",
		Content:                 content,
		Metadata:                buildRouteMetadata(req),
		Assets:                  chatAttachmentsToAssetRefs(req.Attachments),
	})
	return err
}

func (r *Resolver) storeRound(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, usage json.RawMessage, usages []json.RawMessage) error {
	fullRound := make([]conversation.ModelMessage, 0, len(messages))
	roundUsages := make([]json.RawMessage, 0, len(usages))
	for i, m := range messages {
		if req.UserMessagePersisted && m.Role == "user" && strings.TrimSpace(m.TextContent()) == strings.TrimSpace(req.Query) {
			continue
		}
		fullRound = append(fullRound, m)
		if i < len(usages) {
			roundUsages = append(roundUsages, usages[i])
		}
	}
	if len(fullRound) == 0 {
		return nil
	}

	r.storeMessages(ctx, req, fullRound, usage, roundUsages)
	go r.storeMemory(context.WithoutCancel(ctx), req.BotID, fullRound)
	return nil
}

func (r *Resolver) storeMessages(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, usage json.RawMessage, usages []json.RawMessage) {
	if r.messageService == nil {
		return
	}
	if strings.TrimSpace(req.BotID) == "" {
		return
	}
	meta := buildRouteMetadata(req)
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)

	// Determine the last assistant message index for outbound asset attachment.
	lastAssistantIdx := -1
	if req.OutboundAssetCollector != nil {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" {
				lastAssistantIdx = i
				break
			}
		}
	}
	var outboundAssets []messagepkg.AssetRef
	if lastAssistantIdx >= 0 {
		outboundAssets = outboundAssetRefsToMessageRefs(req.OutboundAssetCollector())
	}

	for i, msg := range messages {
		content, err := json.Marshal(msg)
		if err != nil {
			r.logger.Warn("storeMessages: marshal failed", slog.Any("error", err))
			continue
		}
		messageSenderChannelIdentityID := ""
		messageSenderUserID := ""
		externalMessageID := ""
		sourceReplyToMessageID := ""
		assets := []messagepkg.AssetRef(nil)
		if msg.Role == "user" {
			messageSenderChannelIdentityID = senderChannelIdentityID
			messageSenderUserID = senderUserID
			externalMessageID = req.ExternalMessageID
			if strings.TrimSpace(msg.TextContent()) == strings.TrimSpace(req.Query) {
				assets = chatAttachmentsToAssetRefs(req.Attachments)
			}
		} else if strings.TrimSpace(req.ExternalMessageID) != "" {
			sourceReplyToMessageID = req.ExternalMessageID
		}
		if i == lastAssistantIdx && len(outboundAssets) > 0 {
			assets = append(assets, outboundAssets...)
		}
		var msgUsage json.RawMessage
		if i < len(usages) && len(usages[i]) > 0 && !isJSONNull(usages[i]) {
			msgUsage = usages[i]
		} else if i == len(messages)-1 && len(usage) > 0 {
			msgUsage = usage
		}
		if _, err := r.messageService.Persist(ctx, messagepkg.PersistInput{
			BotID:                   req.BotID,
			RouteID:                 req.RouteID,
			SenderChannelIdentityID: messageSenderChannelIdentityID,
			SenderUserID:            messageSenderUserID,
			Platform:                req.CurrentChannel,
			ExternalMessageID:       externalMessageID,
			SourceReplyToMessageID:  sourceReplyToMessageID,
			Role:                    msg.Role,
			Content:                 content,
			Metadata:                meta,
			Usage:                   msgUsage,
			Assets:                  assets,
		}); err != nil {
			r.logger.Warn("persist message failed", slog.Any("error", err))
		}
	}
}

func isJSONNull(data json.RawMessage) bool {
	return len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

// outboundAssetRefsToMessageRefs converts outbound asset refs from the streaming
// collector into message-level asset refs for persistence.
func outboundAssetRefsToMessageRefs(refs []conversation.OutboundAssetRef) []messagepkg.AssetRef {
	if len(refs) == 0 {
		return nil
	}
	result := make([]messagepkg.AssetRef, 0, len(refs))
	for _, ref := range refs {
		contentHash := strings.TrimSpace(ref.ContentHash)
		if contentHash == "" {
			continue
		}
		role := ref.Role
		if strings.TrimSpace(role) == "" {
			role = "attachment"
		}
		result = append(result, messagepkg.AssetRef{
			ContentHash: contentHash,
			Role:        role,
			Ordinal:     ref.Ordinal,
			Mime:        ref.Mime,
			SizeBytes:   ref.SizeBytes,
			StorageKey:  ref.StorageKey,
		})
	}
	return result
}

// chatAttachmentsToAssetRefs converts ChatAttachment slice to message AssetRef slice.
// Only attachments that carry a content_hash are included.
func chatAttachmentsToAssetRefs(attachments []conversation.ChatAttachment) []messagepkg.AssetRef {
	if len(attachments) == 0 {
		return nil
	}
	refs := make([]messagepkg.AssetRef, 0, len(attachments))
	for i, att := range attachments {
		contentHash := strings.TrimSpace(att.ContentHash)
		if contentHash == "" {
			continue
		}
		ref := messagepkg.AssetRef{
			ContentHash: contentHash,
			Role:        "attachment",
			Ordinal:     i,
			Mime:        strings.TrimSpace(att.Mime),
			SizeBytes:   att.Size,
		}
		if att.Metadata != nil {
			if sk, ok := att.Metadata["storage_key"].(string); ok {
				ref.StorageKey = sk
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func buildRouteMetadata(req conversation.ChatRequest) map[string]any {
	if strings.TrimSpace(req.RouteID) == "" && strings.TrimSpace(req.CurrentChannel) == "" {
		return nil
	}
	meta := map[string]any{}
	if strings.TrimSpace(req.RouteID) != "" {
		meta["route_id"] = req.RouteID
	}
	if strings.TrimSpace(req.CurrentChannel) != "" {
		meta["platform"] = req.CurrentChannel
	}
	return meta
}

func (r *Resolver) resolvePersistSenderIDs(ctx context.Context, req conversation.ChatRequest) (string, string) {
	channelIdentityID := strings.TrimSpace(req.SourceChannelIdentityID)
	userID := strings.TrimSpace(req.UserID)

	senderChannelIdentityID := ""
	if r.isExistingChannelIdentityID(ctx, channelIdentityID) {
		senderChannelIdentityID = channelIdentityID
	}

	senderUserID := ""
	if r.isExistingUserID(ctx, userID) {
		senderUserID = userID
	}
	if senderUserID == "" && senderChannelIdentityID != "" {
		if linked := r.linkedUserIDFromChannelIdentity(ctx, senderChannelIdentityID); linked != "" {
			senderUserID = linked
		}
	}
	return senderChannelIdentityID, senderUserID
}

func (r *Resolver) isExistingChannelIdentityID(ctx context.Context, id string) bool {
	if r.queries == nil {
		return false
	}
	pgID, err := parseResolverUUID(id)
	if err != nil {
		return false
	}
	_, err = r.queries.GetChannelIdentityByID(ctx, pgID)
	return err == nil
}

func (r *Resolver) isExistingUserID(ctx context.Context, id string) bool {
	if r.queries == nil {
		return false
	}
	pgID, err := parseResolverUUID(id)
	if err != nil {
		return false
	}
	_, err = r.queries.GetUserByID(ctx, pgID)
	return err == nil
}

func (r *Resolver) linkedUserIDFromChannelIdentity(ctx context.Context, channelIdentityID string) string {
	if r.queries == nil {
		return ""
	}
	pgID, err := parseResolverUUID(channelIdentityID)
	if err != nil {
		return ""
	}
	row, err := r.queries.GetChannelIdentityByID(ctx, pgID)
	if err != nil || !row.UserID.Valid {
		return ""
	}
	return row.UserID.String()
}

// resolveDisplayName returns the best available display name for the request identity:
// req.DisplayName if set, else channel identity's display_name, else linked user's display_name, else "User".
func (r *Resolver) resolveDisplayName(ctx context.Context, req conversation.ChatRequest) string {
	if name := strings.TrimSpace(req.DisplayName); name != "" {
		return name
	}
	if r.queries == nil {
		return "User"
	}
	channelIdentityID := strings.TrimSpace(req.SourceChannelIdentityID)
	if channelIdentityID == "" {
		return "User"
	}
	pgID, err := parseResolverUUID(channelIdentityID)
	if err != nil {
		return "User"
	}
	ci, err := r.queries.GetChannelIdentityByID(ctx, pgID)
	if err == nil && ci.DisplayName.Valid {
		if name := strings.TrimSpace(ci.DisplayName.String); name != "" {
			return name
		}
	}
	linkedUserID := r.linkedUserIDFromChannelIdentity(ctx, channelIdentityID)
	if linkedUserID == "" {
		return "User"
	}
	userPgID, err := parseResolverUUID(linkedUserID)
	if err != nil {
		return "User"
	}
	u, err := r.queries.GetUserByID(ctx, userPgID)
	if err != nil || !u.DisplayName.Valid {
		return "User"
	}
	if name := strings.TrimSpace(u.DisplayName.String); name != "" {
		return name
	}
	return "User"
}

func (r *Resolver) storeMemory(ctx context.Context, botID string, messages []conversation.ModelMessage) {
	if r.memoryService == nil {
		return
	}
	if strings.TrimSpace(botID) == "" {
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
	r.addMemory(ctx, botID, memMsgs, sharedMemoryNamespace, botID)
}

func (r *Resolver) addMemory(ctx context.Context, botID string, msgs []memory.Message, namespace, scopeID string) {
	filters := map[string]any{
		"namespace": namespace,
		"scopeId":   scopeID,
		"bot_id":    botID,
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

func (r *Resolver) selectChatModel(ctx context.Context, req conversation.ChatRequest, botSettings settings.Settings, cs conversation.Settings) (models.GetResponse, sqlc.LlmProvider, error) {
	if r.modelsService == nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("models service not configured")
	}
	modelID := strings.TrimSpace(req.Model)
	providerFilter := strings.TrimSpace(req.Provider)

	// Priority: request model > chat settings > bot settings.
	if modelID == "" && providerFilter == "" {
		if value := strings.TrimSpace(cs.ModelID); value != "" {
			modelID = value
		} else if value := strings.TrimSpace(botSettings.ChatModelID); value != "" {
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
		if matchesModelReference(m, modelID) {
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
	modelRef := strings.TrimSpace(modelID)
	if modelRef == "" {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("model id is required")
	}

	// Support both model UUID and model_id slug. UUID-formatted slugs still
	// work because we fall back to GetByModelID when UUID lookup misses.
	var model models.GetResponse
	var err error
	if _, parseErr := db.ParseUUID(modelRef); parseErr == nil {
		model, err = r.modelsService.GetByID(ctx, modelRef)
		if err == nil {
			goto resolved
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return models.GetResponse{}, sqlc.LlmProvider{}, err
		}
	}
	model, err = r.modelsService.GetByModelID(ctx, modelRef)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}

resolved:
	if model.Type != models.ModelTypeChat {
		return models.GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("model is not a chat model")
	}
	prov, err := models.FetchProviderByID(ctx, r.queries, model.LlmProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.LlmProvider{}, err
	}
	return model, prov, nil
}

func matchesModelReference(model models.GetResponse, modelRef string) bool {
	ref := strings.TrimSpace(modelRef)
	if ref == "" {
		return false
	}
	return model.ID == ref || model.ModelID == ref
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

// --- inbox ---

func (r *Resolver) markInboxRead(ctx context.Context, botID string, ids []string) {
	if r.inboxService == nil || len(ids) == 0 {
		return
	}
	if err := r.inboxService.MarkRead(ctx, botID, ids); err != nil {
		r.logger.Warn("failed to mark inbox items as read", slog.String("bot_id", botID), slog.Any("error", err))
	}
}

// --- settings ---

func (r *Resolver) loadBotSettings(ctx context.Context, botID string) (settings.Settings, error) {
	if r.settingsService == nil {
		return settings.Settings{}, fmt.Errorf("settings service not configured")
	}
	return r.settingsService.GetBot(ctx, botID)
}

// --- utility ---

func normalizeClientType(clientType string) (string, error) {
	ct := strings.ToLower(strings.TrimSpace(clientType))
	switch ct {
	case "openai-responses", "openai-completions", "anthropic-messages", "google-generative-ai":
		return ct, nil
	default:
		return "", fmt.Errorf("unsupported agent gateway client type: %s", clientType)
	}
}

func sanitizeMessages(messages []conversation.ModelMessage) []conversation.ModelMessage {
	cleaned := make([]conversation.ModelMessage, 0, len(messages))
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

func normalizeGatewaySkill(entry SkillEntry) (gatewaySkill, bool) {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return gatewaySkill{}, false
	}
	description := strings.TrimSpace(entry.Description)
	if description == "" {
		description = name
	}
	content := strings.TrimSpace(entry.Content)
	if content == "" {
		content = description
	}
	return gatewaySkill{
		Name:        name,
		Description: description,
		Content:     content,
		Metadata:    entry.Metadata,
	}, true
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

func nonNilModelMessages(m []conversation.ModelMessage) []conversation.ModelMessage {
	if m == nil {
		return []conversation.ModelMessage{}
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

func parseResolverUUID(id string) (pgtype.UUID, error) {
	if strings.TrimSpace(id) == "" {
		return pgtype.UUID{}, fmt.Errorf("empty id")
	}
	return db.ParseUUID(id)
}

// UserMessageMeta holds the structured metadata attached to every user
// message. It is the single source of truth shared by the YAML header
// (sent to the LLM) and the inbox content JSONB.
type UserMessageMeta struct {
	MessageID         string   `json:"message-id,omitempty"`
	ChannelIdentityID string   `json:"channel-identity-id"`
	DisplayName       string   `json:"display-name"`
	Channel           string   `json:"channel"`
	ConversationType  string   `json:"conversation-type"`
	ConversationName  string   `json:"conversation-name,omitempty"`
	Time              string   `json:"time"`
	AttachmentPaths   []string `json:"attachments"`
}

// BuildUserMessageMeta constructs a UserMessageMeta from the inbound
// parameters. Both FormatUserHeader and inbox content use this.
func BuildUserMessageMeta(messageID, channelIdentityID, displayName, channel, conversationType, conversationName string, attachmentPaths []string) UserMessageMeta {
	if attachmentPaths == nil {
		attachmentPaths = []string{}
	}
	return UserMessageMeta{
		MessageID:         messageID,
		ChannelIdentityID: channelIdentityID,
		DisplayName:       displayName,
		Channel:           channel,
		ConversationType:  conversationType,
		ConversationName:  conversationName,
		Time:              time.Now().UTC().Format(time.RFC3339),
		AttachmentPaths:   attachmentPaths,
	}
}

// ToMap returns the metadata as a map with the same keys used in the YAML
// header, suitable for storing as inbox content JSONB.
func (m UserMessageMeta) ToMap() map[string]any {
	result := map[string]any{
		"channel-identity-id": m.ChannelIdentityID,
		"display-name":        m.DisplayName,
		"channel":             m.Channel,
		"conversation-type":   m.ConversationType,
		"time":                m.Time,
		"attachments":         m.AttachmentPaths,
	}
	if m.MessageID != "" {
		result["message-id"] = m.MessageID
	}
	if m.ConversationName != "" {
		result["conversation-name"] = m.ConversationName
	}
	return result
}

// FormatUserHeader wraps a user query with YAML front-matter metadata so
// the LLM sees structured context (sender, channel, time, attachments)
// alongside the raw message. This must be the single source of truth for
// user-message formatting — the agent gateway must NOT add its own header.
func FormatUserHeader(messageID, channelIdentityID, displayName, channel, conversationType, conversationName string, attachmentPaths []string, query string) string {
	meta := BuildUserMessageMeta(messageID, channelIdentityID, displayName, channel, conversationType, conversationName, attachmentPaths)
	return FormatUserHeaderFromMeta(meta, query)
}

// FormatUserHeaderFromMeta formats a pre-built UserMessageMeta into the
// YAML front-matter string sent to the LLM.
func FormatUserHeaderFromMeta(meta UserMessageMeta, query string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	if meta.MessageID != "" {
		writeYAMLString(&sb, "message-id", meta.MessageID)
	}
	writeYAMLString(&sb, "channel-identity-id", meta.ChannelIdentityID)
	writeYAMLString(&sb, "display-name", meta.DisplayName)
	writeYAMLString(&sb, "channel", meta.Channel)
	writeYAMLString(&sb, "conversation-type", meta.ConversationType)
	if meta.ConversationName != "" {
		writeYAMLString(&sb, "conversation-name", meta.ConversationName)
	}
	writeYAMLString(&sb, "time", meta.Time)
	if len(meta.AttachmentPaths) > 0 {
		sb.WriteString("attachments:\n")
		for _, p := range meta.AttachmentPaths {
			sb.WriteString("  - ")
			sb.WriteString(p)
			sb.WriteByte('\n')
		}
	} else {
		sb.WriteString("attachments: []\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(query)
	return sb.String()
}

func writeYAMLString(sb *strings.Builder, key, value string) {
	sb.WriteString(key)
	sb.WriteString(": ")
	if value == "" || needsYAMLQuote(value) {
		sb.WriteByte('"')
		sb.WriteString(strings.ReplaceAll(value, `"`, `\"`))
		sb.WriteByte('"')
	} else {
		sb.WriteString(value)
	}
	sb.WriteByte('\n')
}

func needsYAMLQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, c := range s {
		if c == ':' || c == '#' || c == '"' || c == '\'' || c == '{' || c == '}' || c == '[' || c == ']' || c == ',' || c == '\n' {
			return true
		}
	}
	return false
}

// extractFileRefPaths collects container file paths from gateway attachments
// that use the tool_file_ref transport (files already written to the bot container).
func extractFileRefPaths(attachments []any) []string {
	var paths []string
	for _, att := range attachments {
		if ga, ok := att.(gatewayAttachment); ok && ga.Transport == gatewayTransportToolFileRef && strings.TrimSpace(ga.Payload) != "" {
			paths = append(paths, ga.Payload)
		}
	}
	return paths
}
