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
	"github.com/memohai/memoh/internal/agent/sessionmode"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/hooks"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/providers"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
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

type botPermissionChecker interface {
	HasBotPermission(ctx context.Context, botID, accountID, permission string) (bool, error)
}

// Resolver orchestrates chat with the internal agent.
type Resolver struct {
	agent              *agentpkg.Agent
	modelsService      *models.Service
	queries            dbstore.Queries
	memoryRegistry     *memprovider.Registry
	conversationSvc    ConversationSettingsReader
	messageService     messagepkg.Service
	settingsService    *settings.Service
	accountService     *accounts.Service
	sessionService     SessionService
	acpPool            acpPrompter
	compactionService  *compaction.Service
	eventPublisher     messageevent.Publisher
	skillLoader        SkillLoader
	assetLoader        gatewayAssetLoader
	channelStore       botChannelConfigReader
	botPermissions     botPermissionChecker
	pipeline           *pipelinepkg.Pipeline
	streamHTTPClient   *http.Client
	bgManager          *background.Manager
	toolApproval       *toolapproval.Service
	userInput          userInputService
	hookService        *hooks.Service
	memoryContextMu    sync.Mutex
	memoryContextCache *memprovider.MemoryContextCache
	acpPromptMu        sync.Mutex
	acpPromptHubs      map[string]*acpActivePromptHub
	// continueUserInputFn overrides the chat-flow resume after a user input
	// response; nil means storeUserInputResultAndContinue. Test seam.
	continueUserInputFn func(ctx context.Context, req userinput.Request, input UserInputResponseInput, result sdk.ToolResultPart, eventCh chan<- WSStreamEvent) error
	sessionTurnMu       sync.Mutex
	sessionTurnRefs     map[string]int // key: "botID:sessionID" → active turn refcount
	sessionTurnLocks    map[string]*sync.Mutex
	sessionCompactionMu sync.Mutex
	sessionCompactions  map[string]*sessionCompactionGate
	timeout             time.Duration
	memorySearchTimeout time.Duration
	clockLocation       *time.Location
	logger              *slog.Logger
}

// NewResolver creates a Resolver that uses the internal agent directly.
func NewResolver(
	log *slog.Logger,
	modelsService *models.Service,
	queries dbstore.Queries,
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
		agent:               a,
		modelsService:       modelsService,
		queries:             queries,
		conversationSvc:     conversationSvc,
		messageService:      messageService,
		settingsService:     settingsService,
		accountService:      accountService,
		streamHTTPClient:    streamHTTPClient,
		sessionTurnRefs:     make(map[string]int),
		sessionTurnLocks:    make(map[string]*sync.Mutex),
		timeout:             timeout,
		memorySearchTimeout: defaultMemorySearchTimeout,
		clockLocation:       clockLocation,
		logger:              log.With(slog.String("service", "conversation_resolver")),
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

func (r *Resolver) SetBotPermissionChecker(checker botPermissionChecker) {
	r.botPermissions = checker
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

// SetBackgroundManager configures the background task manager used for task
// summaries and background status tooling.
func (r *Resolver) SetBackgroundManager(m *background.Manager) {
	r.bgManager = m
}

func (r *Resolver) SetToolApprovalService(s *toolapproval.Service) {
	r.toolApproval = s
}

func (r *Resolver) SetHookService(s *hooks.Service) {
	r.hookService = s
}

func (r *Resolver) SetUserInputService(s *userinput.Service) {
	if s == nil {
		r.userInput = nil
		return
	}
	r.userInput = s
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

type resolvedContext struct {
	runConfig                   agentpkg.RunConfig
	model                       models.GetResponse
	provider                    sqlc.Provider
	query                       string // headerified persistable query
	userMessageAlreadyInContext bool
	injectedRecords             *[]conversation.InjectedMessageRecord
	estimatedTokens             int // estimated input token count for compaction
	compactableTokens           int // raw history eligible for compaction
	compactableTokensKnown      bool
	contextTokenBudget          int // token budget used to clamp compaction triggers
}

func (r *Resolver) resolve(ctx context.Context, req conversation.ChatRequest) (resolvedContext, error) {
	modelQuery := modelQueryText(req)
	if strings.TrimSpace(modelQuery) == "" && len(req.Attachments) == 0 {
		return resolvedContext{}, errors.New("query or attachments is required")
	}
	if strings.TrimSpace(req.BotID) == "" {
		return resolvedContext{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(req.ChatID) == "" {
		return resolvedContext{}, errors.New("chat id is required")
	}
	if err := r.rejectRequestedSkillsIfUnsupportedContext(ctx, req); err != nil {
		return resolvedContext{}, err
	}

	runCfg, chatModel, provider, err := r.buildBaseRunConfig(ctx, baseRunConfigParams{
		BotID:             req.BotID,
		ChatID:            req.ChatID,
		SessionID:         req.SessionID,
		RouteID:           req.RouteID,
		UserID:            req.UserID,
		ChannelIdentityID: req.SourceChannelIdentityID,
		CurrentPlatform:   req.CurrentChannel,
		ReplyTarget:       req.ReplyTarget,
		ConversationType:  req.ConversationType,
		SessionToken:      req.ChatToken,
		SessionType:       req.SessionType,
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
	usePipeline := r.pipeline != nil &&
		strings.TrimSpace(req.SessionID) != "" &&
		strings.TrimSpace(req.HistoryCutoffBeforeMessageID) == "" &&
		len(req.RequestedSkills) == 0
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
	var historyRecords []historyfrag.HistoryRecord
	var estimatedTokens int
	var compactableTokens int
	var compactableTokensKnown bool
	if usePipeline {
		messages = r.buildMessagesFromPipeline(ctx, req, contextTokenBudget)
	} else if r.conversationSvc != nil {
		historyFallback := historyScopeFallbackFromChatRequest(req)
		loaded, loadErr := r.loadHistoryRecords(ctx, historyFallback, req.SessionID, defaultMaxContextMinutes)
		if loadErr != nil {
			r.logger.Error("resolve: loadHistoryRecords failed",
				slog.String("bot_id", req.BotID),
				slog.Any("error", loadErr),
			)
			return resolvedContext{}, loadErr
		}
		loaded = pruneHistoryForGateway(loaded)
		loaded = filterMessagesBeforeID(loaded, req.HistoryCutoffBeforeMessageID)
		loaded = dedupePersistedCurrentUserMessage(loaded, req)
		loaded, loadErr = r.ensureRequiredHistoryMessage(ctx, loaded, req)
		if loadErr != nil {
			r.logger.Error("resolve: load required history message failed",
				slog.String("bot_id", req.BotID),
				slog.Any("error", loadErr),
			)
			return resolvedContext{}, loadErr
		}
		loaded, loadErr = r.replaceCompactedMessages(ctx, req.SessionID, compactionSummaryScope(req.BotID, req.ChatID, req.SessionID, req.ConversationType, req.ConversationName, req.ReplyTarget), loaded, strings.TrimSpace(req.HistoryCutoffBeforeMessageID) == "")
		if loadErr != nil {
			return resolvedContext{}, loadErr
		}
		messages, historyRecords, estimatedTokens = trimMessagesAndRecordsByTokens(r.logger, loaded, contextTokenBudget)
		// When context reaches the shared budget share, run synchronous
		// compaction before sending the request. contextTokenBudget is the
		// authoritative limit for how much context the user wants to send
		// to the LLM.
		compactionThreshold := 0
		if contextTokenBudget > 0 {
			compactionThreshold = contextTokenBudget * compactionBudgetThresholdPercent / 100
		}
		// The trigger only counts raw (compactable) rows: active summaries can
		// never be compacted away, so including them would make the trigger
		// self-sustaining once accumulated summaries cross the threshold.
		compactableTokens = totalCompactableHistoryTokens(loaded)
		compactableTokensKnown = true
		if compactionThreshold > 0 && compactableTokens >= compactionThreshold {
			r.logger.Warn("resolve: context reached compaction threshold, running synchronous compaction",
				slog.String("bot_id", req.BotID),
				slog.Int("estimated_tokens", estimatedTokens),
				slog.Int("compactable_tokens", compactableTokens),
				slog.Int("context_token_budget", contextTokenBudget),
				slog.Int("compaction_threshold", compactionThreshold),
			)
			// Reload and post-process only when this run actually produced a
			// summary. A noop (cooldown, in-flight, nothing markable) keeps
			// this turn's context untouched — possibly still above the
			// threshold — and the next turn re-evaluates.
			if res := r.runCompactionSync(ctx, req, compactableTokens, contextTokenBudget); res.Status == compaction.StatusOK {
				loaded, loadErr = r.loadHistoryRecords(ctx, historyFallback, req.SessionID, defaultMaxContextMinutes)
				if loadErr != nil {
					r.logger.Error("resolve: reload messages after compaction failed",
						slog.String("bot_id", req.BotID),
						slog.Any("error", loadErr),
					)
					return resolvedContext{}, loadErr
				}
				loaded = pruneHistoryForGateway(loaded)
				loaded = filterMessagesBeforeID(loaded, req.HistoryCutoffBeforeMessageID)
				loaded = dedupePersistedCurrentUserMessage(loaded, req)
				loaded, loadErr = r.ensureRequiredHistoryMessage(ctx, loaded, req)
				if loadErr != nil {
					r.logger.Error("resolve: reload required history message failed",
						slog.String("bot_id", req.BotID),
						slog.Any("error", loadErr),
					)
					return resolvedContext{}, loadErr
				}
				loaded, loadErr = r.replaceCompactedMessages(ctx, req.SessionID, compactionSummaryScope(req.BotID, req.ChatID, req.SessionID, req.ConversationType, req.ConversationName, req.ReplyTarget), loaded, strings.TrimSpace(req.HistoryCutoffBeforeMessageID) == "")
				if loadErr != nil {
					return resolvedContext{}, loadErr
				}
				compactableTokens = totalCompactableHistoryTokens(loaded)
				messages, historyRecords, estimatedTokens = trimMessagesAndRecordsByTokens(r.logger, loaded, contextTokenBudget)
				// Remove tool messages from the recent context — they are large
				// and unnecessary when we already have a summary. Keep only
				// user/assistant conversation turns.
				messages = stripToolMessages(messages)
			}
		}
	}
	if memoryMsg != nil {
		messages = append(messages, *memoryMsg)
	}
	if requestedSkillMsg := buildRequestedSkillContextMessage(req.RequestedSkills); requestedSkillMsg != nil {
		messages = append(messages, *requestedSkillMsg)
	}
	if !usePipeline && !req.ReusePersistedUserMessage {
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
	headerInput := UserMessageHeaderInput{
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
	}
	headerifiedQuery := ""
	if userQueryNeedsHeader(req, len(mergedAttachments)) {
		headerifiedQuery = FormatUserHeader(headerInput, req.Query)
	}
	headerifiedModelQuery := headerifiedQuery
	if strings.TrimSpace(modelQuery) != strings.TrimSpace(req.Query) {
		headerifiedModelQuery = FormatUserHeader(headerInput, modelQuery)
	}
	runCfg.ContextFrags = historyContextFragsForMessages(messages, historyRecords)
	runCfg.Messages = modelMessagesToSDKMessages(nonNilModelMessages(messages))
	// When using the pipeline the user message is already in the RC;
	// don't send it to the LLM again. headerifiedQuery is still kept
	// for storeRound so the user message gets persisted.
	if !usePipeline && !req.ReusePersistedUserMessage {
		runCfg.Query = headerifiedModelQuery
	}
	runCfg.InlineImages = extractNativeImageParts(mergedAttachments)
	runCfg.ContextScope = buildContextFragScope(req, displayName, runCfg.Identity)
	runCfg = runCfg.RefreshContextFrag()

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
		runConfig:                   runCfg,
		model:                       chatModel,
		provider:                    provider,
		query:                       headerifiedQuery,
		userMessageAlreadyInContext: usePipeline,
		injectedRecords:             injectedRecords,
		estimatedTokens:             estimatedTokens,
		compactableTokens:           compactableTokens,
		compactableTokensKnown:      compactableTokensKnown,
		contextTokenBudget:          contextTokenBudget,
	}, nil
}

// Chat sends a synchronous chat request and stores the result.
func (r *Resolver) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	if err := rejectReservedSkillMetadataIfPresent(req); err != nil {
		return conversation.ChatResponse{}, err
	}
	if err := r.rejectRequestedSkillsIfUnsupportedContext(ctx, req); err != nil {
		return conversation.ChatResponse{}, err
	}

	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	var err error
	if !req.UserMessagePersisted {
		req, err = r.applyUserMessageHook(ctx, req)
		if err != nil {
			return conversation.ChatResponse{}, err
		}
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	req.Query = rc.query

	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.RawQuery)

	cfg := rc.runConfig
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return conversation.ChatResponse{}, err
	}

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	storeReq := req
	roundMessages := prependTurnUserMessage(storeReq, outputMessages)
	if err := r.storeRoundWithOptions(ctx, storeReq, roundMessages, rc.model.ID, storeRoundOptions{
		SkipMemory: storeReq.SkipMemoryExtraction,
	}); err != nil {
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
	RouteID           string
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
	botInfo, loopDetectionEnabled := r.loadBotRuntimeInfo(ctx, p.BotID)
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

	authResolver := providers.NewService(nil, r.queries, "")
	authCtx := oauthctx.WithUserID(ctx, p.UserID)
	creds, err := authResolver.ResolveModelCredentials(authCtx, provider)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("resolve provider credentials: %w", err)
	}

	baseURL := providers.ProviderConfigString(provider, "base_url")
	chatCompletionsCompat := models.ResolveChatCompletionsCompat(
		baseURL,
		providers.ProviderConfigString(provider, "chat_completions_compat"),
	)

	reasoningConfig := resolveReasoningConfig(chatModel, botSettings, p.ReasoningEffort, provider.ClientType)
	reasoningEffort := ""
	if reasoningConfig != nil && reasoningConfig.Active {
		reasoningEffort = reasoningConfig.Effort
	}

	sdkModel := models.NewSDKChatModel(models.SDKModelConfig{
		ModelID:               chatModel.ModelID,
		ClientType:            provider.ClientType,
		APIKey:                creds.APIKey,
		CodexAccountID:        creds.CodexAccountID,
		BaseURL:               baseURL,
		ChatCompletionsCompat: chatCompletionsCompat,
		HTTPClient:            r.streamHTTPClient,
		ReasoningConfig:       reasoningConfig,
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
		Model:                 sdkModel,
		ReasoningEffort:       reasoningEffort,
		ReasoningActive:       reasoningConfig != nil && reasoningConfig.Active,
		ReasoningDisabled:     reasoningConfig != nil && reasoningConfig.Disabled,
		ReasoningAdaptive:     reasoningConfig != nil && reasoningConfig.Adaptive,
		ReasoningOffEffort:    offEffortOrEmpty(reasoningConfig),
		ChatCompletionsCompat: chatCompletionsCompat,
		PromptCacheTTL:        providers.ProviderConfigString(provider, "prompt_cache_ttl"),
		SessionType:           p.SessionType,
		SupportsImageInput:    supportsImageInputForModel(chatModel),
		SupportsToolCall:      chatModel.HasCompatibility(models.CompatToolCall),
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
		Bot:               botInfo,
		Skills:            agentSkills,
		LoopDetection:     agentpkg.LoopDetectionConfig{Enabled: loopDetectionEnabled},
		BackgroundManager: r.bgManager,
		ContextScope: contextfrag.Scope{
			BotID:             p.BotID,
			ChatID:            chatID,
			SessionID:         p.SessionID,
			ChannelIdentityID: strings.TrimSpace(p.ChannelIdentityID),
			Platform:          strings.TrimSpace(p.CurrentPlatform),
			ConversationType:  strings.TrimSpace(p.ConversationType),
			ReplyTarget:       strings.TrimSpace(p.ReplyTarget),
		},
	}
	if r.toolApproval != nil || r.userInput != nil {
		cfg.ToolApprovalHandler = r.buildToolApprovalHandler(p)
	}

	return cfg, chatModel, provider, nil
}

func (r *Resolver) canDeliverUserInputStream() bool {
	return r.userInput != nil
}

func (r *Resolver) canDeliverUserInputWS(eventCh chan<- WSStreamEvent) bool {
	return r.userInput != nil && eventCh != nil
}

func supportsImageInputForModel(m models.GetResponse) bool {
	return m.HasCompatibility(models.CompatVision)
}

const (
	reasoningEffortAdaptive = "adaptive"
	reasoningEffortDisable  = "disable"
)

// resolveReasoningConfig makes the single reasoning decision for a call, driven
// by the model's discovered thinking mode plus the user's settings/override.
//
//   - none:     no thinking; returns nil.
//   - adaptive: on/off; when active, Anthropic-style providers use adaptive
//     thinking plus the selected effort.
//   - toggle:   on/off, with per-message override taking precedence over the
//     bot's default.
func resolveReasoningConfig(chatModel models.GetResponse, botSettings settings.Settings, requestedEffort, clientType string) *models.ReasoningConfig {
	mode := chatModel.ResolveThinkingMode()
	if mode == models.ThinkingModeNone {
		return nil
	}

	effortLevels := effectiveReasoningEfforts(chatModel.Config.ReasoningEfforts, clientType)
	offEffort := offEffortFor(effortLevels)
	requested := strings.TrimSpace(requestedEffort)
	adaptive := mode == models.ThinkingModeAdaptive
	// Anthropic 4.6+ uses the effort/adaptive wire (no budget_tokens). Cloud
	// variants (bedrock/vertex/azure/openrouter) are missing
	// supports_adaptive_thinking in the LiteLLM registry but still advertise the
	// 4.6+ effort tiers, so promote them to adaptive here. This keeps them off the
	// legacy budget path, where budget_tokens is rejected with 400 on 4.7+.
	if !adaptive && clientType == string(models.ClientTypeAnthropicMessages) && anthropicEffortEra(effortLevels) {
		adaptive = true
	}

	switch {
	case reasoningEffortDisabled(requested):
		return &models.ReasoningConfig{Disabled: true, OffEffort: offEffort}
	case requested == reasoningEffortAdaptive:
		// Legacy "adaptive" override on a toggle model: treat as on (toggle has no
		// adaptive concept; send a normal effort).
		return &models.ReasoningConfig{Active: true, Adaptive: adaptive, Effort: pickEffort("", botSettings, effortLevels), OffEffort: offEffort}
	case requested != "":
		return &models.ReasoningConfig{Active: true, Adaptive: adaptive, Effort: pickEffort(requested, botSettings, effortLevels), OffEffort: offEffort}
	case botSettings.ReasoningEnabled:
		return &models.ReasoningConfig{Active: true, Adaptive: adaptive, Effort: pickEffort("", botSettings, effortLevels), OffEffort: offEffort}
	default:
		return &models.ReasoningConfig{Disabled: true, OffEffort: offEffort}
	}
}

// anthropicEffortEra reports whether an Anthropic model uses the 4.6+
// effort/adaptive thinking mechanism rather than the legacy
// thinking{type:"enabled", budget_tokens:N} path. Pre-4.6 Claude advertises only
// the implicit low/medium/high base; 4.6+ adds at least one of none/minimal/
// xhigh/max. Detecting any of those tiers catches the cloud-provider variants
// that the registry leaves without supports_adaptive_thinking.
func anthropicEffortEra(effortLevels []string) bool {
	for _, e := range effortLevels {
		switch e {
		case models.ReasoningEffortNone, models.ReasoningEffortMinimal,
			models.ReasoningEffortXHigh, models.ReasoningEffortMax:
			return true
		}
	}
	return false
}

// pickEffort resolves the effort to send when thinking is active: the
// per-message override (if a concrete tier) wins, then the bot default, then
// medium. Values outside the effective model+wire effort list are ignored so
// stale settings or command/API overrides cannot send a known-invalid wire value.
func pickEffort(requested string, botSettings settings.Settings, effortLevels []string) string {
	if e := strings.TrimSpace(requested); e != "" && e != reasoningEffortAdaptive && e != reasoningEffortDisable {
		if hasEffort(effortLevels, e) {
			return e
		}
	}
	if e := strings.TrimSpace(botSettings.ReasoningEffort); e != "" && hasEffort(effortLevels, e) {
		return e
	}
	if hasEffort(effortLevels, models.ReasoningEffortMedium) {
		return models.ReasoningEffortMedium
	}
	if len(effortLevels) > 0 {
		return effortLevels[0]
	}
	return models.ReasoningEffortMedium
}

// effectiveReasoningEfforts intersects the model's advertised effort levels
// with the wire format's accepted set. OpenAI-format clients reject "max", so
// it is excluded here. This is the primary filter; openAIWireEffort in
// models/sdk.go and the Twilight SDK provider layer act as defence-in-depth.
// Keep isOpenAIReasoningWire in sync with the frontend OPENAI_FORMAT_CLIENT_TYPES.
func effectiveReasoningEfforts(effortLevels []string, clientType string) []string {
	levels := effortLevels
	if len(levels) == 0 {
		levels = []string{models.ReasoningEffortLow, models.ReasoningEffortMedium, models.ReasoningEffortHigh}
	}
	out := make([]string, 0, len(levels))
	for _, e := range levels {
		if isOpenAIReasoningWire(clientType) && e == models.ReasoningEffortMax {
			continue
		}
		if !hasEffort(out, e) {
			out = append(out, e)
		}
	}
	return out
}

// isOpenAIReasoningWire returns true for client types whose wire format rejects
// "max" effort. Keep in sync with OPENAI_FORMAT_CLIENT_TYPES in reasoning-effort.ts.
func isOpenAIReasoningWire(clientType string) bool {
	switch models.ClientType(clientType) {
	case models.ClientTypeOpenAICompletions, models.ClientTypeOpenAIResponses, models.ClientTypeOpenAICodex:
		return true
	default:
		return false
	}
}

func hasEffort(effortLevels []string, effort string) bool {
	for _, e := range effortLevels {
		if e == effort {
			return true
		}
	}
	return false
}

// offEffortFor picks the effort an OpenAI-style provider should send to
// approximate "off": "none" when advertised, else "minimal" when advertised,
// else "" meaning the caller must omit reasoning_effort entirely. Returning a
// real tier (low/medium/high) here would *enable* thinking instead of disabling
// it — e.g. OpenRouter translates reasoning_effort:"low" into Anthropic extended
// thinking, so a toggle model that advertises only low/medium/high would keep
// reasoning on when the user selected Off. Omitting the field instead lets the
// provider default (thinking off for toggle/Anthropic-compat models) take over
// and also avoids sending an unsupported tier. effortLevels is ordered low→high.
func offEffortFor(effortLevels []string) string {
	if hasEffort(effortLevels, models.ReasoningEffortNone) {
		return models.ReasoningEffortNone
	}
	if hasEffort(effortLevels, models.ReasoningEffortMinimal) {
		return models.ReasoningEffortMinimal
	}
	return ""
}

func reasoningEffortDisabled(effort string) bool {
	return strings.TrimSpace(effort) == reasoningEffortDisable
}

func offEffortOrEmpty(rc *models.ReasoningConfig) string {
	if rc == nil {
		return ""
	}
	return rc.OffEffort
}

func (r *Resolver) buildToolApprovalHandler(p baseRunConfigParams) func(context.Context, sdk.ToolCall) (sdk.ToolApprovalResult, error) {
	return func(ctx context.Context, call sdk.ToolCall) (sdk.ToolApprovalResult, error) {
		if strings.TrimSpace(call.ToolName) == userinput.ToolNameAskUser {
			if err := userinput.ValidateAskUserInput(call.Input); err != nil {
				// Let the tool's Execute handler return an instructional tool result
				// to the model instead of creating a fake pending request.
				return sdk.ToolApprovalResult{Decision: sdk.ToolApprovalDecisionApproved}, nil
			}
			if r.userInput == nil {
				return r.limitToolApprovalResult(sdk.ToolApprovalResult{
					Decision: sdk.ToolApprovalDecisionRejected,
					Reason:   "user input service is not configured",
				}, call.ToolName), nil
			}
			if !isInteractiveApprovalSession(p.SessionType) {
				return r.limitToolApprovalResult(sdk.ToolApprovalResult{
					Decision: sdk.ToolApprovalDecisionRejected,
					Reason:   "user input requested in a non-interactive session",
				}, call.ToolName), nil
			}
			// No ExpiresAt here: chat-flow requests have no in-process
			// waiter — the run pauses and resumes whenever the user answers,
			// even much later. Only waiter-backed (ACP/MCP) requests expire.
			req, err := r.userInput.CreatePending(ctx, userinput.CreatePendingInput{
				BotID:                        p.BotID,
				SessionID:                    p.SessionID,
				RouteID:                      p.RouteID,
				ChannelIdentityID:            p.ChannelIdentityID,
				RequestedByChannelIdentityID: p.ChannelIdentityID,
				ToolCallID:                   call.ToolCallID,
				ToolName:                     call.ToolName,
				Input:                        call.Input,
				SourcePlatform:               p.CurrentPlatform,
				ReplyTarget:                  p.ReplyTarget,
				ConversationType:             p.ConversationType,
			})
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			if req.Status != userinput.StatusPending {
				return r.limitToolApprovalResult(sdk.ToolApprovalResult{
					Decision:   sdk.ToolApprovalDecisionRejected,
					ApprovalID: req.ID,
					Reason:     "ask_user request is already " + req.Status,
					Metadata:   userinput.DeferredMetadata(req),
				}, call.ToolName), nil
			}
			return sdk.ToolApprovalResult{
				Decision:   sdk.ToolApprovalDecisionDeferred,
				ApprovalID: req.ID,
				Metadata:   userinput.DeferredMetadata(req),
			}, nil
		}
		input := toolapproval.CreatePendingInput{
			BotID:                        p.BotID,
			SessionID:                    p.SessionID,
			RouteID:                      p.RouteID,
			ChannelIdentityID:            p.ChannelIdentityID,
			RequestedByChannelIdentityID: p.ChannelIdentityID,
			ToolCallID:                   call.ToolCallID,
			ToolName:                     call.ToolName,
			ToolInput:                    call.Input,
			SourcePlatform:               p.CurrentPlatform,
			ReplyTarget:                  p.ReplyTarget,
			ConversationType:             p.ConversationType,
		}
		forcedApprovalReason, forcedApproval := agentpkg.HookForcedApprovalReason(ctx)
		if r.toolApproval == nil {
			if forcedApproval {
				return r.limitToolApprovalResult(sdk.ToolApprovalResult{
					Decision: sdk.ToolApprovalDecisionRejected,
					Reason:   firstNonEmpty(forcedApprovalReason, "hook requested approval but tool approval is not configured"),
				}, call.ToolName), nil
			}
			return sdk.ToolApprovalResult{Decision: sdk.ToolApprovalDecisionApproved}, nil
		}
		if !forcedApproval {
			eval, err := r.toolApproval.EvaluatePolicy(ctx, input)
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			if eval.Decision == toolapproval.DecisionBypass {
				return sdk.ToolApprovalResult{Decision: sdk.ToolApprovalDecisionApproved}, nil
			}
		}
		if !isInteractiveApprovalSession(p.SessionType) {
			req, err := r.toolApproval.CreatePending(ctx, input)
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			reason := "tool execution requires approval, but this session type cannot request approval"
			rejected, err := r.toolApproval.Reject(ctx, req.ID, p.ChannelIdentityID, reason)
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			return r.limitToolApprovalResult(sdk.ToolApprovalResult{
				Decision:   sdk.ToolApprovalDecisionRejected,
				ApprovalID: rejected.ID,
				Reason:     reason,
				Metadata:   approvalResultMetadata(rejected),
			}, call.ToolName), nil
		}
		if forcedApproval {
			req, err := r.toolApproval.CreatePending(ctx, input)
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			return sdk.ToolApprovalResult{
				Decision:   sdk.ToolApprovalDecisionDeferred,
				ApprovalID: req.ID,
				Metadata:   approvalResultMetadata(req),
			}, nil
		}
		eval, err := r.toolApproval.Evaluate(ctx, input)
		if err != nil {
			return sdk.ToolApprovalResult{}, err
		}
		return sdk.ToolApprovalResult{
			Decision:   sdk.ToolApprovalDecisionDeferred,
			ApprovalID: eval.Request.ID,
			Metadata:   approvalResultMetadata(eval.Request),
		}, nil
	}
}

func approvalResultMetadata(req toolapproval.Request) map[string]any {
	return map[string]any{
		"short_id":     req.ShortID,
		"status":       req.Status,
		"tool_name":    req.ToolName,
		"operation":    req.Operation,
		"tool_call_id": req.ToolCallID,
	}
}

func isInteractiveApprovalSession(sessionType string) bool {
	return sessionmode.IsInteractive(sessionType)
}

func (r *Resolver) resolveRunConfigSessionDescriptor(ctx context.Context, sessionID string) (string, string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r == nil || r.sessionService == nil {
		return sessionpkg.TypeChat, sessionpkg.RuntimeModel
	}
	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("ResolveRunConfig: session lookup failed; falling back to chat session type",
				slog.String("session_id", sessionID),
				slog.Any("error", err),
			)
		}
		return sessionpkg.TypeChat, sessionpkg.RuntimeModel
	}
	typ := strings.TrimSpace(sess.Type)
	if typ == "" {
		typ = sessionpkg.TypeChat
	}
	runtimeType := strings.TrimSpace(sess.RuntimeType)
	if runtimeType == "" {
		if sessionpkg.IsACPRuntime(sess) {
			runtimeType = sessionpkg.RuntimeACPAgent
		} else {
			runtimeType = sessionpkg.RuntimeModel
		}
	}
	return typ, runtimeType
}

func (r *Resolver) resolveRunConfigSessionType(ctx context.Context, sessionID string) string {
	typ, _ := r.resolveRunConfigSessionDescriptor(ctx, sessionID)
	return typ
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

	sessionType, runtimeType := r.resolveRunConfigSessionDescriptor(ctx, sessionID)
	if runtimeType == sessionpkg.RuntimeACPAgent {
		cfg := agentpkg.RunConfig{
			SessionType: sessionType,
			Identity: agentpkg.SessionContext{
				BotID:             botID,
				ChatID:            botID,
				SessionID:         sessionID,
				ChannelIdentityID: strings.TrimSpace(channelIdentityID),
				CurrentPlatform:   currentPlatform,
				ReplyTarget:       strings.TrimSpace(replyTarget),
				ConversationType:  strings.TrimSpace(conversationType),
				SessionToken:      chatToken,
			},
		}
		return pipelinepkg.ResolveRunConfigResult{
			RunConfig:   cfg,
			RuntimeType: runtimeType,
		}, nil
	}
	cfg, chatModel, _, err := r.buildBaseRunConfig(ctx, baseRunConfigParams{
		BotID:             botID,
		SessionID:         sessionID,
		ChannelIdentityID: channelIdentityID,
		CurrentPlatform:   currentPlatform,
		ReplyTarget:       replyTarget,
		ConversationType:  conversationType,
		SessionToken:      chatToken,
		SessionType:       sessionType,
	})
	if err != nil {
		return pipelinepkg.ResolveRunConfigResult{}, err
	}

	cfg = r.prepareRunConfig(ctx, cfg)
	return pipelinepkg.ResolveRunConfigResult{
		RunConfig:   cfg,
		ModelID:     chatModel.ID,
		RuntimeType: runtimeType,
	}, nil
}

// prepareRunConfig generates the system prompt and appends the user message.
func (r *Resolver) prepareRunConfig(ctx context.Context, cfg agentpkg.RunConfig) agentpkg.RunConfig {
	beforePromptContext := r.runPromptHook(ctx, agentRunConfigView{
		BotID:        cfg.Identity.BotID,
		SessionID:    cfg.Identity.SessionID,
		ChatID:       cfg.Identity.ChatID,
		SessionType:  cfg.SessionType,
		MessageCount: len(cfg.Messages),
	}, hooks.EventBeforePromptBuild)
	var files []agentpkg.SystemFile
	limits := agentpkg.DefaultLimits()
	if r.agent != nil {
		limits = r.agent.Limits()
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
		Bot:                       cfg.Bot,
		Skills:                    cfg.Skills,
		Files:                     files,
		MaxFilesBytes:             limits.SystemFilesMaxBytes,
		Now:                       now,
		Timezone:                  cfg.Identity.Timezone,
		PlatformIdentitiesSection: platformIdentitiesSection,
	})
	if beforePromptContext != "" {
		cfg.System += "\n\n" + formatResolverHookContext(hooks.EventBeforePromptBuild, beforePromptContext)
	}
	afterPromptContext := r.runPromptHook(ctx, agentRunConfigView{
		BotID:        cfg.Identity.BotID,
		SessionID:    cfg.Identity.SessionID,
		ChatID:       cfg.Identity.ChatID,
		SessionType:  cfg.SessionType,
		MessageCount: len(cfg.Messages),
		SystemBytes:  len(cfg.System),
	}, hooks.EventAfterPromptBuild)
	if afterPromptContext != "" {
		cfg.System += "\n\n" + formatResolverHookContext(hooks.EventAfterPromptBuild, afterPromptContext)
	}

	if cfg.Query != "" {
		var extra []sdk.MessagePart
		for _, img := range cfg.InlineImages {
			if strings.TrimSpace(img.Image) != "" {
				extra = append(extra, img)
			}
		}
		cfg.Messages = append(cfg.Messages, sdk.UserMessage(cfg.Query, extra...))
		cfg.ContextQueryMaterialized = true
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
			cfg.ContextQueryMaterialized = true
		}
	}

	return cfg.RefreshContextFrag()
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

// userQueryNeedsHeader reports whether resolve should wrap req.Query in the
// user message header (sender/channel/time/attachment paths). Text always
// gets a header. An attachment-only message (no caption) must too: the header
// is the only thing telling the model who sent what file, and the headerified
// query is what makes the user turn (with its asset links) persist to history.
// The one deliberate exception is a no-prompt skill activation, whose stored
// user content must stay empty (its model text travels via ModelQuery, and
// display state via skill_activation metadata).
func userQueryNeedsHeader(req conversation.ChatRequest, attachmentCount int) bool {
	if strings.TrimSpace(req.Query) != "" {
		return true
	}
	return attachmentCount > 0 && req.UserMessageKind != conversation.UserMessageKindSkillActivation
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
