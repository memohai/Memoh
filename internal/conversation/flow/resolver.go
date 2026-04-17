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
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/accounts"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/sqlc"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
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
	Path        string
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

type botChannelConfigReader interface {
	ListBotConfigs(ctx context.Context, botID string) ([]channel.ChannelConfig, error)
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
	routeService      RouteService
	compactionService *compaction.Service
	eventPublisher    messageevent.Publisher
	skillLoader       SkillLoader
	assetLoader       gatewayAssetLoader
	channelStore      botChannelConfigReader
	pipeline          *pipelinepkg.Pipeline
	streamHTTPClient  *http.Client
	bgManager         *background.Manager
	outboundFn        func(ctx context.Context, botID, channelType, target, text string) error
	bgNotifDeferred   sync.Map // key: "botID:sessionID" → wake arrived while a session turn was active
	sessionTurnMu     sync.Mutex
	sessionTurnRefs   map[string]int // key: "botID:sessionID" → active turn refcount
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
	// HTTP client with timeouts for LLM provider streaming.
	// - DialTimeout: fail fast on connection issues
	// - ResponseHeaderTimeout: catch servers that accept TCP but never respond
	// - Timeout: overall request lifetime cap (prevents stuck SSE body reads)
	streamHTTPClient := &http.Client{
		Timeout: 10 * time.Minute, // overall cap, matches resolver timeout
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	return &Resolver{
		agent:            a,
		modelsService:    modelsService,
		queries:          queries,
		conversationSvc:  conversationSvc,
		messageService:   messageService,
		settingsService:  settingsService,
		accountService:   accountService,
		streamHTTPClient: streamHTTPClient,
		sessionTurnRefs:  make(map[string]int),
		timeout:          timeout,
		clockLocation:    clockLocation,
		logger:           log.With(slog.String("service", "conversation_resolver")),
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

// SetChannelStore configures the bot channel config store used to load
// platform identity metadata for system prompt generation.
func (r *Resolver) SetChannelStore(store botChannelConfigReader) {
	r.channelStore = store
}

// SetCompactionService configures the compaction service for context compaction.
func (r *Resolver) SetCompactionService(s *compaction.Service) {
	r.compactionService = s
}

// SetBackgroundManager configures the background task manager so that
// background exec notifications are injected into the agent loop.
func (r *Resolver) SetBackgroundManager(m *background.Manager) {
	r.bgManager = m
}

// SetOutboundFn configures the function used to deliver background notification
// responses to the user. The agent's text output is delivered through the same
// path as normal responses.
func (r *Resolver) SetOutboundFn(fn func(ctx context.Context, botID, channelType, target, text string) error) {
	r.outboundFn = fn
}

// SetPipeline configures the DCP pipeline for RC-based context assembly.
// When set, resolve() will use RC from the pipeline instead of loading
// history from bot_history_messages for sessions that have pipeline data.
func (r *Resolver) SetPipeline(p *pipelinepkg.Pipeline) {
	r.pipeline = p
}

// Pipeline returns the configured pipeline, or nil.
func (r *Resolver) Pipeline() *pipelinepkg.Pipeline {
	return r.pipeline
}

// InlineImageAttachments resolves image content hashes to sdk.ImagePart values
// using the configured asset loader. Intended for the discuss driver to inline
// images from new RC segments before calling the LLM.
func (r *Resolver) InlineImageAttachments(ctx context.Context, botID string, refs []pipelinepkg.ImageAttachmentRef) []sdk.ImagePart {
	if r == nil || r.assetLoader == nil || len(refs) == 0 {
		return nil
	}
	var parts []sdk.ImagePart
	for _, ref := range refs {
		contentHash := strings.TrimSpace(ref.ContentHash)
		if contentHash == "" {
			continue
		}
		dataURL, mime, err := r.inlineAssetAsDataURL(ctx, botID, contentHash, "image", strings.TrimSpace(ref.Mime))
		if err != nil {
			if r.logger != nil {
				r.logger.Warn(
					"inline discuss image attachment failed",
					slog.Any("error", err),
					slog.String("bot_id", botID),
					slog.String("content_hash", contentHash),
				)
			}
			continue
		}
		parts = append(parts, sdk.ImagePart{
			Image:     dataURL,
			MediaType: mime,
		})
	}
	return parts
}

type usageInfo struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
}

type resolvedContext struct {
	runConfig       agentpkg.RunConfig
	model           models.GetResponse
	provider        sqlc.Provider
	query           string // headerified query
	injectedRecords *[]conversation.InjectedMessageRecord
	estimatedTokens int // estimated input token count for compaction
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

	runCfg, chatModel, provider, err := r.buildBaseRunConfig(ctx, baseRunConfigParams{
		BotID:             req.BotID,
		ChatID:            req.ChatID,
		SessionID:         req.SessionID,
		UserID:            req.UserID,
		ChannelIdentityID: req.SourceChannelIdentityID,
		CurrentPlatform:   req.CurrentChannel,
		ReplyTarget:       req.ReplyTarget,
		ConversationType:  req.ConversationType,
		SessionToken:      req.ChatToken,
		Model:             req.Model,
		Provider:          req.Provider,
		ReasoningEffort:   req.ReasoningEffort,
	})
	if err != nil {
		r.logger.Error("resolve: buildBaseRunConfig failed",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return resolvedContext{}, err
	}
	memoryMsg := r.loadMemoryContextMessage(ctx, req)
	reqMessages := pruneMessagesForGateway(nonNilModelMessages(req.Messages))
	if memoryMsg != nil {
		pruned, _ := pruneMessageForGateway(*memoryMsg)
		memoryMsg = &pruned
	}

	// When the DCP pipeline has data for this session, build context from
	// the rendered event stream (RC) + bot turn responses (TR) instead of
	// loading raw history from bot_history_messages. The current inbound
	// message is already in the RC, so it must not be appended again.
	usePipeline := r.pipeline != nil && strings.TrimSpace(req.SessionID) != ""
	if usePipeline {
		if _, loaded := r.pipeline.GetIC(strings.TrimSpace(req.SessionID)); !loaded {
			usePipeline = false
		}
	}

	contextTokenBudget := 0
	if chatModel.Config.ContextWindow != nil && *chatModel.Config.ContextWindow > 0 {
		contextTokenBudget = *chatModel.Config.ContextWindow
	}

	var messages []conversation.ModelMessage
	var estimatedTokens int
	if usePipeline {
		messages = r.buildMessagesFromPipeline(ctx, req, contextTokenBudget)
	} else if r.conversationSvc != nil {
		loaded, loadErr := r.loadMessages(ctx, req.ChatID, req.SessionID, defaultMaxContextMinutes)
		if loadErr != nil {
			r.logger.Error("resolve: loadMessages failed",
				slog.String("bot_id", req.BotID),
				slog.Any("error", loadErr),
			)
			return resolvedContext{}, loadErr
		}
		loaded = pruneHistoryForGateway(loaded)
		loaded = dedupePersistedCurrentUserMessage(loaded, req)
		loaded = r.replaceCompactedMessages(ctx, loaded)
		messages, estimatedTokens = trimMessagesByTokens(r.logger, loaded, contextTokenBudget)
		// When context reaches 70% of the contextTokenBudget (the user-configured
		// budget cap), run synchronous compaction before sending the request.
		// contextTokenBudget is the authoritative limit for how much context
		// the user wants to send to the LLM. We compact at 70% to keep the
		// context healthy and avoid edge-case timeouts.
		compactionThreshold := 0
		if contextTokenBudget > 0 {
			compactionThreshold = contextTokenBudget * 70 / 100
		}
		if compactionThreshold > 0 && estimatedTokens >= compactionThreshold {
			r.logger.Warn("resolve: context reached compaction threshold, running synchronous compaction",
				slog.String("bot_id", req.BotID),
				slog.Int("estimated_tokens", estimatedTokens),
				slog.Int("context_token_budget", contextTokenBudget),
				slog.Int("compaction_threshold", compactionThreshold),
			)
			r.runCompactionSync(ctx, req, estimatedTokens)
			// Reload messages after compaction.
			loaded, loadErr = r.loadMessages(ctx, req.ChatID, req.SessionID, defaultMaxContextMinutes)
			if loadErr != nil {
				r.logger.Error("resolve: reload messages after compaction failed",
					slog.String("bot_id", req.BotID),
					slog.Any("error", loadErr),
				)
				return resolvedContext{}, loadErr
			}
			loaded = pruneHistoryForGateway(loaded)
			loaded = dedupePersistedCurrentUserMessage(loaded, req)
			loaded = r.replaceCompactedMessages(ctx, loaded)
			messages, estimatedTokens = trimMessagesByTokens(r.logger, loaded, contextTokenBudget)
			// Remove tool messages from the recent context — they are large
			// and unnecessary when we already have a summary. Keep only
			// user/assistant conversation turns.
			messages = stripToolMessages(messages)
		}
		_ = estimatedTokens
	}
	if memoryMsg != nil {
		messages = append(messages, *memoryMsg)
	}
	if !usePipeline {
		messages = append(messages, reqMessages...)
	}
	messages = sanitizeMessages(messages)
	// Strip tool messages and tool-call-only assistant messages from context.
	// Tool outputs are large and waste tokens; the LLM doesn't need raw tool
	// results when summaries and memory tools are available for lookup.
	if len(messages) > 10 {
		messages = stripToolMessages(messages)
	}
	messages = repairToolCallClosures(messages, syntheticToolClosureError)

	displayName := r.resolveDisplayName(ctx, req)
	mergedAttachments := r.routeAndMergeAttachments(ctx, chatModel, req)

	tz := runCfg.Identity.TimezoneLocation
	if tz == nil {
		tz = time.UTC
	}
	headerifiedQuery := FormatUserHeader(UserMessageHeaderInput{
		MessageID:         strings.TrimSpace(req.ExternalMessageID),
		ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
		DisplayName:       displayName,
		Channel:           req.CurrentChannel,
		ConversationType:  strings.TrimSpace(req.ConversationType),
		ConversationName:  strings.TrimSpace(req.ConversationName),
		Target:            strings.TrimSpace(req.ReplyTarget),
		AttachmentPaths:   extractAttachmentPaths(mergedAttachments),
		Time:              time.Now().In(tz),
		Timezone:          runCfg.Identity.Timezone,
	}, req.Query)
	runCfg.Messages = modelMessagesToSDKMessages(nonNilModelMessages(messages))
	// When using the pipeline the user message is already in the RC;
	// don't send it to the LLM again. headerifiedQuery is still kept
	// for storeRound so the user message gets persisted.
	if !usePipeline {
		runCfg.Query = headerifiedQuery
	}
	runCfg.InlineImages = extractNativeImageParts(mergedAttachments)

	var injectedRecords *[]conversation.InjectedMessageRecord
	if req.InjectCh != nil {
		agentInjectCh := make(chan agentpkg.InjectMessage, cap(req.InjectCh))
		go func() {
			for msg := range req.InjectCh {
				agentMsg := agentpkg.InjectMessage{
					Text:            msg.Text,
					HeaderifiedText: msg.HeaderifiedText,
				}
				// Inline any image attachments from the injected message so the
				// model receives them as vision input alongside the text.
				if runCfg.SupportsImageInput && len(msg.Attachments) > 0 {
					agentMsg.ImageParts = r.inlineInjectAttachments(ctx, req.BotID, msg.Attachments)
				}
				agentInjectCh <- agentMsg
			}
			close(agentInjectCh)
		}()
		runCfg.InjectCh = agentInjectCh

		records := make([]conversation.InjectedMessageRecord, 0)
		injectedRecords = &records
		var recMu sync.Mutex
		runCfg.InjectedRecorder = func(headerifiedText string, insertAfter int) {
			recMu.Lock()
			*injectedRecords = append(*injectedRecords, conversation.InjectedMessageRecord{
				HeaderifiedText: headerifiedText,
				InsertAfter:     insertAfter,
			})
			recMu.Unlock()
		}
	}

	return resolvedContext{
		runConfig:       runCfg,
		model:           chatModel,
		provider:        provider,
		query:           headerifiedQuery,
		injectedRecords: injectedRecords,
		estimatedTokens: estimatedTokens,
	}, nil
}

// Chat sends a synchronous chat request and stores the result.
func (r *Resolver) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()

	rc, err := r.resolve(ctx, req)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
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

// baseRunConfigParams holds parameters for buildBaseRunConfig that differ
// between chat and discuss callers.
type baseRunConfigParams struct {
	BotID             string
	ChatID            string
	SessionID         string
	UserID            string
	ChannelIdentityID string
	CurrentPlatform   string
	ReplyTarget       string
	ConversationType  string
	SessionToken      string //nolint:gosec // session credential material, not a hardcoded secret
	SessionType       string
	Model             string
	Provider          string
	ReasoningEffort   string // caller-provided override (empty = use bot default)
}

// buildBaseRunConfig creates a RunConfig with model, credentials, skills,
// identity and system prompt — everything except Messages/Query/InlineImages.
// Both resolve() and ResolveRunConfig() delegate to this shared builder.
func (r *Resolver) buildBaseRunConfig(ctx context.Context, p baseRunConfigParams) (agentpkg.RunConfig, models.GetResponse, sqlc.Provider, error) {
	botSettings, err := r.loadBotSettings(ctx, p.BotID)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, err
	}
	loopDetectionEnabled := r.loadBotLoopDetectionEnabled(ctx, p.BotID)
	userTimezoneName, userClockLocation := r.resolveTimezone(ctx, p.BotID, p.UserID)

	chatID := p.ChatID
	if chatID == "" {
		chatID = p.BotID
	}

	req := buildModelSelectionRequest(p, chatID)

	chatModel, provider, err := r.selectChatModel(ctx, req, botSettings, conversation.Settings{})
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, err
	}

	reasoningEffort := p.ReasoningEffort
	if reasoningEffort == "" && chatModel.HasCompatibility(models.CompatReasoning) && botSettings.ReasoningEnabled {
		reasoningEffort = botSettings.ReasoningEffort
	}
	var reasoningConfig *models.ReasoningConfig
	if reasoningEffort != "" {
		reasoningConfig = &models.ReasoningConfig{Enabled: true, Effort: reasoningEffort}
	}

	authResolver := providers.NewService(nil, r.queries, "")
	authCtx := oauthctx.WithUserID(ctx, p.UserID)
	creds, err := authResolver.ResolveModelCredentials(authCtx, provider)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("resolve provider credentials: %w", err)
	}

	sdkModel := models.NewSDKChatModel(models.SDKModelConfig{
		ModelID:         chatModel.ModelID,
		ClientType:      provider.ClientType,
		APIKey:          creds.APIKey,
		CodexAccountID:  creds.CodexAccountID,
		BaseURL:         providers.ProviderConfigString(provider, "base_url"),
		HTTPClient:      r.streamHTTPClient,
		ReasoningConfig: reasoningConfig,
	})

	var agentSkills []agentpkg.SkillEntry
	if r.skillLoader != nil {
		entries, skillErr := r.skillLoader.LoadSkills(ctx, p.BotID)
		if skillErr != nil {
			r.logger.Warn("failed to load skills", slog.String("bot_id", p.BotID), slog.Any("error", skillErr))
		} else {
			for _, e := range entries {
				if skill, ok := normalizeGatewaySkill(e); ok {
					agentSkills = append(agentSkills, skill)
				}
			}
		}
	}
	if agentSkills == nil {
		agentSkills = []agentpkg.SkillEntry{}
	}

	cfg := agentpkg.RunConfig{
		Model:              sdkModel,
		ReasoningEffort:    reasoningEffort,
		SessionType:        p.SessionType,
		SupportsImageInput: chatModel.HasCompatibility(models.CompatVision),
		SupportsToolCall:   chatModel.HasCompatibility(models.CompatToolCall),
		Identity: agentpkg.SessionContext{
			BotID:             p.BotID,
			ChatID:            chatID,
			SessionID:         p.SessionID,
			ChannelIdentityID: strings.TrimSpace(p.ChannelIdentityID),
			CurrentPlatform:   p.CurrentPlatform,
			ReplyTarget:       strings.TrimSpace(p.ReplyTarget),
			ConversationType:  strings.TrimSpace(p.ConversationType),
			Timezone:          userTimezoneName,
			TimezoneLocation:  userClockLocation,
			SessionToken:      p.SessionToken,
		},
		Skills:            agentSkills,
		LoopDetection:     agentpkg.LoopDetectionConfig{Enabled: loopDetectionEnabled},
		BackgroundManager: r.bgManager,
	}

	return cfg, chatModel, provider, nil
}

func buildModelSelectionRequest(p baseRunConfigParams, chatID string) conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:          p.BotID,
		ChatID:         chatID,
		SessionID:      p.SessionID,
		CurrentChannel: p.CurrentPlatform,
		Model:          p.Model,
		Provider:       p.Provider,
	}
}

// ResolveRunConfig builds a complete RunConfig (model, system prompt, tools,
// identity) for a bot+session without loading messages or requiring a query.
// The caller is responsible for filling RunConfig.Messages.
// Used by the discuss driver to reuse the resolver's model/tools/prompt pipeline.
func (r *Resolver) ResolveRunConfig(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (pipelinepkg.ResolveRunConfigResult, error) {
	if strings.TrimSpace(botID) == "" {
		return pipelinepkg.ResolveRunConfigResult{}, errors.New("bot id is required")
	}

	cfg, chatModel, _, err := r.buildBaseRunConfig(ctx, baseRunConfigParams{
		BotID:             botID,
		SessionID:         sessionID,
		ChannelIdentityID: channelIdentityID,
		CurrentPlatform:   currentPlatform,
		ReplyTarget:       replyTarget,
		ConversationType:  conversationType,
		SessionToken:      chatToken,
		SessionType:       "discuss",
	})
	if err != nil {
		return pipelinepkg.ResolveRunConfigResult{}, err
	}

	cfg = r.prepareRunConfig(ctx, cfg)
	return pipelinepkg.ResolveRunConfigResult{
		RunConfig: cfg,
		ModelID:   chatModel.ID,
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
	platformIdentitiesSection := ""
	if r.channelStore != nil {
		channelConfigs, err := r.channelStore.ListBotConfigs(ctx, cfg.Identity.BotID)
		if err != nil {
			r.logger.Warn("load bot platform identities failed",
				slog.String("bot_id", cfg.Identity.BotID),
				slog.Any("error", err),
			)
		} else {
			platformIdentitiesSection = buildPlatformIdentitiesSection(channelConfigs)
		}
	}
	cfg.System = agentpkg.GenerateSystemPrompt(agentpkg.SystemPromptParams{
		SessionType:               cfg.SessionType,
		Skills:                    cfg.Skills,
		Files:                     files,
		Now:                       now,
		Timezone:                  cfg.Identity.Timezone,
		SupportsImageInput:        supportsImageInput,
		PlatformIdentitiesSection: platformIdentitiesSection,
	})

	if cfg.Query != "" {
		var extra []sdk.MessagePart
		for _, img := range cfg.InlineImages {
			if strings.TrimSpace(img.Image) != "" {
				extra = append(extra, img)
			}
		}
		cfg.Messages = append(cfg.Messages, sdk.UserMessage(cfg.Query, extra...))
	} else if len(cfg.InlineImages) > 0 {
		// Pipeline path: the user query is already embedded in the RC messages,
		// but image parts are not rendered by the pipeline renderer. Inject the
		// inline images into the last user message so the model receives them.
		imageParts := make([]sdk.MessagePart, 0, len(cfg.InlineImages))
		for _, img := range cfg.InlineImages {
			if strings.TrimSpace(img.Image) != "" {
				imageParts = append(imageParts, img)
			}
		}
		if len(imageParts) > 0 {
			injected := false
			for i := len(cfg.Messages) - 1; i >= 0; i-- {
				if cfg.Messages[i].Role == sdk.MessageRoleUser {
					cfg.Messages[i].Content = append(cfg.Messages[i].Content, imageParts...)
					injected = true
					break
				}
			}
			if !injected {
				cfg.Messages = append(cfg.Messages, sdk.UserMessage("", imageParts...))
			}
		}
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
		Path:        strings.TrimSpace(entry.Path),
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
