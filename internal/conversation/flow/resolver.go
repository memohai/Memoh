package flow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/sqlc"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
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
	agent             *agentpkg.Agent
	modelsService     *models.Service
	queries           *sqlc.Queries
	memoryRegistry    *memprovider.Registry
	conversationSvc   ConversationSettingsReader
	messageService    messagepkg.Service
	settingsService   *settings.Service
	sessionService    SessionService
	compactionService *compaction.Service
	eventPublisher    messageevent.Publisher
	skillLoader       SkillLoader
	assetLoader       gatewayAssetLoader
	timeout           time.Duration
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

// SetCompactionService configures the compaction service for context compaction.
func (r *Resolver) SetCompactionService(s *compaction.Service) {
	r.compactionService = s
}

type usageInfo struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
}

// --- resolved context (shared by Chat / StreamChat / TriggerSchedule) ---

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

	skipHistory := req.MaxContextLoadTime < 0

	botSettings, err := r.loadBotSettings(ctx, req.BotID)
	if err != nil {
		return resolvedContext{}, err
	}
	loopDetectionEnabled := r.loadBotLoopDetectionEnabled(ctx, req.BotID)

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
	clientType := provider.ClientType

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
		loaded, loadErr := r.loadMessages(ctx, req.ChatID, req.SessionID, maxCtx)
		if loadErr != nil {
			return resolvedContext{}, loadErr
		}
		loaded = pruneHistoryForGateway(loaded)
		loaded = dedupePersistedCurrentUserMessage(loaded, req)
		loaded = r.replaceCompactedMessages(ctx, loaded)
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
	var agentSkills []agentpkg.SkillEntry
	if r.skillLoader != nil {
		entries, err := r.skillLoader.LoadSkills(ctx, req.BotID)
		if err != nil {
			r.logger.Warn("failed to load usable skills", slog.String("bot_id", req.BotID), slog.Any("error", err))
		} else {
			agentSkills = make([]agentpkg.SkillEntry, 0, len(entries))
			for _, e := range entries {
				if strings.TrimSpace(e.Name) == "" {
					continue
				}
				agentSkills = append(agentSkills, agentpkg.SkillEntry{
					Name:        e.Name,
					Description: e.Description,
					Content:     e.Content,
					Metadata:    e.Metadata,
				})
			}
		}
	}
	if agentSkills == nil {
		agentSkills = []agentpkg.SkillEntry{}
	}

	displayName := r.resolveDisplayName(ctx, req)

	headerifiedQuery := FormatUserHeader(
		strings.TrimSpace(req.ExternalMessageID),
		strings.TrimSpace(req.SourceChannelIdentityID),
		displayName,
		req.CurrentChannel,
		strings.TrimSpace(req.ConversationType),
		strings.TrimSpace(req.ConversationName),
		nil, // attachments paths handled separately
		req.Query,
	)

	reasoningEffort := ""
	if chatModel.HasCompatibility(models.CompatReasoning) && botSettings.ReasoningEnabled {
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
		SupportsImageInput: chatModel.HasCompatibility(models.CompatVision),
		Identity: agentpkg.SessionContext{
			BotID:             req.BotID,
			ChatID:            req.ChatID,
			SessionID:         req.SessionID,
			ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
			CurrentPlatform:   req.CurrentChannel,
			ReplyTarget:       strings.TrimSpace(req.ReplyTarget),
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
		fs := agentpkg.NewFSClient(r.agent.BridgeProvider(), cfg.Identity.BotID)
		files = fs.LoadSystemFiles(ctx)
	}

	cfg.System = agentpkg.GenerateSystemPrompt(agentpkg.SystemPromptParams{
		SessionType:        cfg.SessionType,
		Skills:             cfg.Skills,
		Files:              files,
		SupportsImageInput: supportsImageInput,
	})

	if cfg.Query != "" {
		cfg.Messages = append(cfg.Messages, sdk.UserMessage(cfg.Query))
	}

	return cfg
}
