package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/accounts"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/sqlc"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
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
	agent             *agentpkg.Agent
	modelsService     *models.Service
	queries           *sqlc.Queries
	memoryRegistry    *memprovider.Registry
	conversationSvc   ConversationSettingsReader
	messageService    messagepkg.Service
	settingsService   *settings.Service
	accountService    *accounts.Service
	sessionService    SessionService
	compactionService *compaction.Service
	eventPublisher    messageevent.Publisher
	skillLoader       SkillLoader
	assetLoader       gatewayAssetLoader
	timeout           time.Duration
	clockLocation     *time.Location
	logger            *slog.Logger
}

// NewResolver creates a Resolver that uses the internal agent directly.
func NewResolver(
	log *slog.Logger,
	modelsService *models.Service,
	queries *sqlc.Queries,
	conversationSvc ConversationSettingsReader,
	messageService messagepkg.Service,
	settingsService *settings.Service,
	accountService *accounts.Service,
	a *agentpkg.Agent,
	clockLocation *time.Location,
	timeout time.Duration,
) *Resolver {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if clockLocation == nil {
		clockLocation = time.UTC
	}
	return &Resolver{
		agent:           a,
		modelsService:   modelsService,
		queries:         queries,
		conversationSvc: conversationSvc,
		messageService:  messageService,
		settingsService: settingsService,
		accountService:  accountService,
		timeout:         timeout,
		clockLocation:   clockLocation,
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

// SetCompactionService configures the compaction service for context compaction.
func (r *Resolver) SetCompactionService(s *compaction.Service) {
	r.compactionService = s
}

type usageInfo struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
}

type resolvedContext struct {
	runConfig agentpkg.RunConfig
	model     models.GetResponse
	provider  sqlc.LlmProvider
	query     string // headerified query
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

	botSettings, err := r.loadBotSettings(ctx, req.BotID)
	if err != nil {
		return resolvedContext{}, err
	}
	loopDetectionEnabled := r.loadBotLoopDetectionEnabled(ctx, req.BotID)
	userTimezoneName, userClockLocation := r.resolveTimezone(ctx, req.BotID, req.UserID)

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
	clientType := provider.ClientType

	memoryMsg := r.loadMemoryContextMessage(ctx, req)
	reqMessages := pruneMessagesForGateway(nonNilModelMessages(req.Messages))
	if memoryMsg != nil {
		pruned, _ := pruneMessageForGateway(*memoryMsg)
		memoryMsg = &pruned
	}

	var messages []conversation.ModelMessage
	if r.conversationSvc != nil {
		loaded, loadErr := r.loadMessages(ctx, req.ChatID, req.SessionID, defaultMaxContextMinutes)
		if loadErr != nil {
			return resolvedContext{}, loadErr
		}
		loaded = pruneHistoryForGateway(loaded)
		loaded = dedupePersistedCurrentUserMessage(loaded, req)
		loaded = r.replaceCompactedMessages(ctx, loaded)
		messages = trimMessagesByTokens(r.logger, loaded, 0)
	}
	if memoryMsg != nil {
		messages = append(messages, *memoryMsg)
	}
	messages = append(messages, reqMessages...)
	messages = sanitizeMessages(messages)
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

	displayName := r.resolveDisplayName(ctx, req)
	mergedAttachments := r.routeAndMergeAttachments(ctx, chatModel, req)
	headerifiedQuery := FormatUserHeader(
		strings.TrimSpace(req.ExternalMessageID),
		strings.TrimSpace(req.SourceChannelIdentityID),
		displayName,
		req.CurrentChannel,
		strings.TrimSpace(req.ConversationType),
		strings.TrimSpace(req.ConversationName),
		extractAttachmentPaths(mergedAttachments),
		time.Now().In(userClockLocation),
		userTimezoneName,
		req.Query,
	)
	inlineImages := extractNativeImageParts(mergedAttachments)

	reasoningEffort := ""
	if chatModel.HasCompatibility(models.CompatReasoning) {
		if re := strings.TrimSpace(req.ReasoningEffort); re != "" {
			reasoningEffort = re
		} else if botSettings.ReasoningEnabled {
			reasoningEffort = botSettings.ReasoningEffort
		}
	}

	var reasoningConfig *models.ReasoningConfig
	if reasoningEffort != "" {
		reasoningConfig = &models.ReasoningConfig{
			Enabled: true,
			Effort:  reasoningEffort,
		}
	}

	authResolver := providers.NewService(nil, r.queries, "")
	creds, err := authResolver.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return resolvedContext{}, fmt.Errorf("resolve provider credentials: %w", err)
	}

	modelCfg := models.SDKModelConfig{
		ModelID:         chatModel.ModelID,
		ClientType:      clientType,
		APIKey:          creds.APIKey,
		CodexAccountID:  creds.CodexAccountID,
		BaseURL:         provider.BaseUrl,
		ReasoningConfig: reasoningConfig,
	}

	sdkModel := models.NewSDKChatModel(modelCfg)
	sdkMessages := modelMessagesToSDKMessages(nonNilModelMessages(messages))

	runCfg := agentpkg.RunConfig{
		Model:              sdkModel,
		ReasoningEffort:    reasoningEffort,
		Messages:           sdkMessages,
		Query:              headerifiedQuery,
		SupportsImageInput: chatModel.HasCompatibility(models.CompatVision),
		InlineImages:       inlineImages,
		Identity: agentpkg.SessionContext{
			BotID:             req.BotID,
			ChatID:            req.ChatID,
			SessionID:         req.SessionID,
			ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
			CurrentPlatform:   req.CurrentChannel,
			ReplyTarget:       strings.TrimSpace(req.ReplyTarget),
			ConversationType:  strings.TrimSpace(req.ConversationType),
			Timezone:          userTimezoneName,
			TimezoneLocation:  userClockLocation,
			SessionToken:      req.ChatToken,
		},
		Skills:        agentSkills,
		LoopDetection: agentpkg.LoopDetectionConfig{Enabled: loopDetectionEnabled},
	}

	return resolvedContext{runConfig: runCfg, model: chatModel, provider: provider, query: headerifiedQuery}, nil
}

// Chat sends a synchronous chat request and stores the result.
func (r *Resolver) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	req.Query = rc.query

	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.Query)

	cfg := rc.runConfig
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return conversation.ChatResponse{}, err
	}

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	roundMessages := prependUserMessage(req.Query, outputMessages)
	if err := r.storeRound(ctx, req, roundMessages, rc.model.ID); err != nil {
		return conversation.ChatResponse{}, err
	}

	if result.Usage != nil {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, result.Usage.InputTokens)
	}

	return conversation.ChatResponse{
		Messages: outputMessages,
		Model:    rc.model.ModelID,
		Provider: rc.provider.ClientType,
	}, nil
}

// prepareRunConfig generates the system prompt and appends the user message.
func (r *Resolver) prepareRunConfig(ctx context.Context, cfg agentpkg.RunConfig) agentpkg.RunConfig {
	supportsImageInput := cfg.SupportsImageInput
	var files []agentpkg.SystemFile
	if r.agent != nil {
		nowFn := time.Now
		if cfg.Identity.TimezoneLocation != nil {
			nowFn = func() time.Time { return time.Now().In(cfg.Identity.TimezoneLocation) }
		}
		fs := agentpkg.NewFSClient(r.agent.BridgeProvider(), cfg.Identity.BotID, nowFn)
		files = fs.LoadSystemFiles(ctx)
	}

	now := time.Now().UTC()
	if cfg.Identity.TimezoneLocation != nil {
		now = now.In(cfg.Identity.TimezoneLocation)
	}
	cfg.System = agentpkg.GenerateSystemPrompt(agentpkg.SystemPromptParams{
		SessionType:        cfg.SessionType,
		Skills:             cfg.Skills,
		Files:              files,
		Now:                now,
		Timezone:           cfg.Identity.Timezone,
		SupportsImageInput: supportsImageInput,
	})

	if cfg.Query != "" {
		var extra []sdk.MessagePart
		for _, img := range cfg.InlineImages {
			if strings.TrimSpace(img.Image) != "" {
				extra = append(extra, img)
			}
		}
		cfg.Messages = append(cfg.Messages, sdk.UserMessage(cfg.Query, extra...))
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

// extractAttachmentPaths collects container file paths from ALL gateway
// attachments — both tool_file_ref (fallback) and native images that carry a
// FallbackPath. This ensures the YAML user header always lists every
// attachment the user sent, regardless of whether the model consumes the
// image natively or via the read_media tool.
func extractAttachmentPaths(attachments []any) []string {
	var paths []string
	for _, att := range attachments {
		ga, ok := att.(gatewayAttachment)
		if !ok {
			continue
		}
		if ga.Transport == gatewayTransportToolFileRef && strings.TrimSpace(ga.Payload) != "" {
			paths = append(paths, ga.Payload)
		} else if strings.TrimSpace(ga.FallbackPath) != "" {
			paths = append(paths, ga.FallbackPath)
		}
	}
	return paths
}

// extractNativeImageParts returns sdk.ImagePart entries for attachments that
// the model can consume as inline multimodal input (vision-capable images with
// an inline data URL or public URL payload).
func extractNativeImageParts(attachments []any) []sdk.ImagePart {
	var parts []sdk.ImagePart
	for _, att := range attachments {
		ga, ok := att.(gatewayAttachment)
		if !ok || ga.Type != "image" {
			continue
		}
		transport := strings.ToLower(strings.TrimSpace(ga.Transport))
		if transport != gatewayTransportInlineDataURL && transport != gatewayTransportPublicURL {
			continue
		}
		payload := strings.TrimSpace(ga.Payload)
		if payload == "" {
			continue
		}
		parts = append(parts, sdk.ImagePart{
			Image:     payload,
			MediaType: strings.TrimSpace(ga.Mime),
		})
	}
	return parts
}
