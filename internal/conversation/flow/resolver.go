package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/inbox"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

const (
	defaultMaxContextMinutes = 24 * 60
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

// Resolver orchestrates chat with the internal agent.
type Resolver struct {
	agent           *agentpkg.Agent
	modelsService   *models.Service
	queries         *sqlc.Queries
	memoryRegistry  *memprovider.Registry
	conversationSvc ConversationSettingsReader
	messageService  messagepkg.Service
	settingsService *settings.Service
	inboxService    *inbox.Service
	skillLoader     SkillLoader
	assetLoader     gatewayAssetLoader
	timeout         time.Duration
	logger          *slog.Logger
}

// NewResolver creates a Resolver that uses the internal agent directly.
func NewResolver(
	log *slog.Logger,
	modelsService *models.Service,
	queries *sqlc.Queries,
	conversationSvc ConversationSettingsReader,
	messageService messagepkg.Service,
	settingsService *settings.Service,
	a *agentpkg.Agent,
	timeout time.Duration,
) *Resolver {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Resolver{
		agent:           a,
		modelsService:   modelsService,
		queries:         queries,
		conversationSvc: conversationSvc,
		messageService:  messageService,
		settingsService: settingsService,
		timeout:         timeout,
		logger:          log.With(slog.String("service", "conversation_resolver")),
	}
}

// SetMemoryRegistry sets the provider registry for memory operations.
func (r *Resolver) SetMemoryRegistry(registry *memprovider.Registry) {
	r.memoryRegistry = registry
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

type usageInfo struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
}

type resolvedContext struct {
	runConfig    agentpkg.RunConfig
	model        models.GetResponse
	provider     sqlc.LlmProvider
	inboxItemIDs []string
	query        string
}

func (r *Resolver) resolve(ctx context.Context, req conversation.ChatRequest) (resolvedContext, error) {
	if strings.TrimSpace(req.Query) == "" && len(req.Attachments) == 0 {
		return resolvedContext{}, errors.New("query or attachments is required")
	}
	if strings.TrimSpace(req.BotID) == "" {
		return resolvedContext{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(req.ChatID) == "" {
		return resolvedContext{}, errors.New("chat id is required")
	}

	skipHistory := req.MaxContextLoadTime < 0

	botSettings, err := r.loadBotSettings(ctx, req.BotID)
	if err != nil {
		return resolvedContext{}, err
	}
	loopDetectionEnabled := r.loadBotLoopDetectionEnabled(ctx, req.BotID)

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
	const systemPromptReserve = 4096
	overhead += systemPromptReserve

	historyBudget := maxTokens - overhead
	if maxTokens > 0 && historyBudget <= 0 {
		historyBudget = 1
	} else if historyBudget < 0 {
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
		loaded = dedupePersistedCurrentUserMessage(loaded, req)
		messages = trimMessagesByTokens(r.logger, loaded, historyBudget)
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
	var agentSkills []agentpkg.SkillEntry
	if r.skillLoader != nil {
		entries, err := r.skillLoader.LoadSkills(ctx, req.BotID)
		if err != nil {
			r.logger.Warn("failed to load usable skills", slog.String("bot_id", req.BotID), slog.Any("error", err))
		} else {
			agentSkills = make([]agentpkg.SkillEntry, 0, len(entries))
			for _, e := range entries {
				skill, ok := normalizeGatewaySkill(e)
				if !ok {
					continue
				}
				agentSkills = append(agentSkills, skill)
			}
		}
	}
	if agentSkills == nil {
		agentSkills = []agentpkg.SkillEntry{}
	}

	var agentInbox []agentpkg.InboxItem
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
			agentInbox = make([]agentpkg.InboxItem, 0, len(items))
			inboxItemIDs = make([]string, 0, len(items))
			for _, item := range items {
				agentInbox = append(agentInbox, agentpkg.InboxItem{
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

	displayName := r.resolveDisplayName(ctx, req)
	headerifiedQuery := FormatUserHeader(
		strings.TrimSpace(req.ExternalMessageID),
		strings.TrimSpace(req.SourceChannelIdentityID),
		displayName,
		req.CurrentChannel,
		strings.TrimSpace(req.ConversationType),
		strings.TrimSpace(req.ConversationName),
		extractFileRefPaths(r.routeAndMergeAttachments(ctx, chatModel, req)),
		req.Query,
	)

	reasoningEffort := ""
	if chatModel.SupportsReasoning && botSettings.ReasoningEnabled {
		reasoningEffort = botSettings.ReasoningEffort
	}

	var reasoningConfig *agentpkg.ReasoningConfig
	if reasoningEffort != "" {
		reasoningConfig = &agentpkg.ReasoningConfig{
			Enabled: true,
			Effort:  reasoningEffort,
		}
	}

	modelCfg := agentpkg.ModelConfig{
		ModelID:         chatModel.ModelID,
		ClientType:      clientType,
		InputModalities: chatModel.InputModalities,
		APIKey:          provider.ApiKey,
		BaseURL:         provider.BaseUrl,
		ReasoningConfig: reasoningConfig,
	}

	sdkModel := agentpkg.CreateModel(modelCfg)
	sdkMessages := modelMessagesToSDKMessages(nonNilModelMessages(messages))

	runCfg := agentpkg.RunConfig{
		Model:              sdkModel,
		ReasoningEffort:    reasoningEffort,
		Messages:           sdkMessages,
		Query:              headerifiedQuery,
		SupportsImageInput: chatModel.HasInputModality(models.ModelInputImage),
		Channels:           nonNilStrings(req.Channels),
		CurrentChannel:     req.CurrentChannel,
		Identity: agentpkg.SessionContext{
			BotID:             req.BotID,
			ChatID:            req.ChatID,
			ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
			DisplayName:       displayName,
			CurrentPlatform:   req.CurrentChannel,
			ReplyTarget:       strings.TrimSpace(req.ReplyTarget),
			ConversationType:  strings.TrimSpace(req.ConversationType),
			SessionToken:      req.ChatToken,
		},
		Skills:            agentSkills,
		EnabledSkillNames: nonNilStrings(skills),
		Inbox:             agentInbox,
		LoopDetection:     agentpkg.LoopDetectionConfig{Enabled: loopDetectionEnabled},
		ActiveContextTime: maxCtx,
	}

	return resolvedContext{runConfig: runCfg, model: chatModel, provider: provider, inboxItemIDs: inboxItemIDs, query: headerifiedQuery}, nil
}

// Chat sends a synchronous chat request and stores the result.
func (r *Resolver) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	req.Query = rc.query

	cfg := rc.runConfig
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return conversation.ChatResponse{}, err
	}

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	roundMessages := prependUserMessage(req.Query, outputMessages)
	usageJSON, _ := json.Marshal(result.Usage)
	if err := r.storeRound(ctx, req, roundMessages, usageJSON, nil, rc.model.ID); err != nil {
		return conversation.ChatResponse{}, err
	}
	r.markInboxRead(ctx, req.BotID, rc.inboxItemIDs)
	return conversation.ChatResponse{
		Messages: outputMessages,
		Skills:   result.Skills,
		Model:    rc.model.ModelID,
		Provider: string(rc.model.ClientType),
	}, nil
}

// prepareRunConfig generates the system prompt and appends the user message.
func (r *Resolver) prepareRunConfig(ctx context.Context, cfg agentpkg.RunConfig) agentpkg.RunConfig {
	supportsImageInput := cfg.SupportsImageInput
	var files []agentpkg.SystemFile
	if r.agent != nil {
		fs := agentpkg.NewFSClient(nil, cfg.Identity.BotID)
		files = fs.LoadSystemFiles(ctx)
	}

	cfg.System = agentpkg.GenerateSystemPrompt(agentpkg.SystemPromptParams{
		Skills:             cfg.Skills,
		EnabledSkills:      nil,
		Files:              files,
		Inbox:              cfg.Inbox,
		SupportsImageInput: supportsImageInput,
	})

	if cfg.Query != "" {
		cfg.Messages = append(cfg.Messages, sdk.UserMessage(cfg.Query))
	}

	return cfg
}

func normalizeGatewaySkill(entry SkillEntry) (agentpkg.SkillEntry, bool) {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return agentpkg.SkillEntry{}, false
	}
	description := strings.TrimSpace(entry.Description)
	if description == "" {
		description = name
	}
	content := strings.TrimSpace(entry.Content)
	if content == "" {
		content = description
	}
	return agentpkg.SkillEntry{
		Name:        name,
		Description: description,
		Content:     content,
		Metadata:    entry.Metadata,
	}, true
}

func normalizeUserMessageContent(msg conversation.ModelMessage) conversation.ModelMessage {
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		return msg
	}
	normalized, changed := normalizeUserContentParts(msg.Content)
	if !changed {
		return msg
	}
	msg.Content = normalized
	return msg
}

func normalizeUserContentParts(content json.RawMessage) (json.RawMessage, bool) {
	if len(content) == 0 {
		return nil, false
	}
	var parts []map[string]any
	if err := json.Unmarshal(content, &parts); err != nil || len(parts) == 0 {
		return nil, false
	}

	changed := false
	rebuilt := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(strings.ToLower(readAnyString(part["type"])))
		switch partType {
		case "image":
			normalized, ok, didChange := normalizeUserImagePart(part)
			if didChange {
				changed = true
			}
			if ok {
				rebuilt = append(rebuilt, normalized)
			}
		default:
			rebuilt = append(rebuilt, part)
		}
	}
	if !changed {
		return nil, false
	}
	if len(rebuilt) == 0 {
		rebuilt = append(rebuilt, map[string]any{
			"type": "text",
			"text": "[User sent an attachment]",
		})
	}
	data, err := json.Marshal(rebuilt)
	if err != nil {
		return nil, false
	}
	return data, true
}

func normalizeUserImagePart(part map[string]any) (map[string]any, bool, bool) {
	raw, ok := part["image"]
	if !ok {
		return nil, false, true
	}
	if image, ok := raw.(string); ok && strings.TrimSpace(image) != "" {
		return part, true, false
	}
	bytes, ok := anyIndexedByteObject(raw)
	if !ok {
		return nil, false, true
	}
	cloned := cloneAnyMap(part)
	mediaType := strings.TrimSpace(readAnyString(cloned["mediaType"]))
	encoded := base64.StdEncoding.EncodeToString(bytes)
	if mediaType != "" {
		cloned["image"] = "data:" + mediaType + ";base64," + encoded
	} else {
		cloned["image"] = encoded
	}
	return cloned, true, true
}

func cloneAnyMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func readAnyString(value any) string {
	text, _ := value.(string)
	return text
}

func anyIndexedByteObject(value any) ([]byte, bool) {
	obj, ok := value.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil, false
	}
	indexes := make([]int, 0, len(obj))
	values := make(map[int]byte, len(obj))
	for key, raw := range obj {
		idx, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil || idx < 0 {
			return nil, false
		}
		byteValue, ok := anyNumberToByte(raw)
		if !ok {
			return nil, false
		}
		indexes = append(indexes, idx)
		values[idx] = byteValue
	}
	sort.Ints(indexes)
	if indexes[len(indexes)-1]+1 != len(indexes) {
		return nil, false
	}
	bytes := make([]byte, len(indexes))
	for _, idx := range indexes {
		bytes[idx] = values[idx]
	}
	return bytes, true
}

func anyNumberToByte(value any) (byte, bool) {
	floatValue, ok := value.(float64)
	if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
		return 0, false
	}
	if floatValue < 0 || floatValue > 255 || math.Trunc(floatValue) != floatValue {
		return 0, false
	}
	parsed, err := strconv.ParseUint(strconv.FormatFloat(floatValue, 'f', 0, 64), 10, 8)
	if err != nil {
		return 0, false
	}
	return byte(parsed), true
}

// extractFileRefPaths collects container file paths from gateway attachments
// that use the tool_file_ref transport.
func extractFileRefPaths(attachments []any) []string {
	var paths []string
	for _, att := range attachments {
		if ga, ok := att.(gatewayAttachment); ok && ga.Transport == gatewayTransportToolFileRef && strings.TrimSpace(ga.Payload) != "" {
			paths = append(paths, ga.Payload)
		}
	}
	return paths
}
