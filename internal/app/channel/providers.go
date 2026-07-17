// Package appchannel is the Channel boundary composition root: channel
// registry/manager/processor, discuss pipeline, email, and webhook
// tunnel assembly (spec §7.1).
package appchannel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	stdpath "path"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/agent/turn"
	audiopkg "github.com/memohai/memoh/internal/audio"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/dingtalk"
	"github.com/memohai/memoh/internal/channel/adapters/discord"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
	"github.com/memohai/memoh/internal/channel/adapters/line"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/adapters/matrix"
	"github.com/memohai/memoh/internal/channel/adapters/misskey"
	"github.com/memohai/memoh/internal/channel/adapters/qq"
	slackadapter "github.com/memohai/memoh/internal/channel/adapters/slack"
	"github.com/memohai/memoh/internal/channel/adapters/telegram"
	"github.com/memohai/memoh/internal/channel/adapters/wechatoa"
	"github.com/memohai/memoh/internal/channel/adapters/wecom"
	"github.com/memohai/memoh/internal/channel/adapters/weixin"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/inbound"
	"github.com/memohai/memoh/internal/channel/publicmedia"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/channelaccess"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	dbstore "github.com/memohai/memoh/internal/db/store"
	emailpkg "github.com/memohai/memoh/internal/email"
	emailgeneric "github.com/memohai/memoh/internal/email/adapters/generic"
	emailgmail "github.com/memohai/memoh/internal/email/adapters/gmail"
	emailmailgun "github.com/memohai/memoh/internal/email/adapters/mailgun"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/media"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthclients"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/userinput"
	"github.com/memohai/memoh/internal/webhooktunnel"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func providePipeline() *pipelinepkg.Pipeline {
	return pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
}

func provideEventStore(log *slog.Logger, queries dbstore.Queries) *pipelinepkg.EventStore {
	return pipelinepkg.NewEventStore(log, queries)
}

func provideDiscussDriver(log *slog.Logger, pipeline *pipelinepkg.Pipeline, eventStore *pipelinepkg.EventStore, msgService *message.DBService) *pipelinepkg.DiscussDriver {
	return pipelinepkg.NewDiscussDriver(pipelinepkg.DiscussDriverDeps{
		Pipeline:       pipeline,
		EventStore:     eventStore,
		MessageService: msgService,
		CursorStore:    eventStore,
		Logger:         log,
	})
}

func provideRouteService(log *slog.Logger, queries dbstore.Queries, chatService *conversation.Service) *route.DBService {
	return route.NewService(log, queries, chatService)
}

func provideChannelRegistry(log *slog.Logger, cfg config.Config, hub *local.RouteHub, mediaService *media.Service, tunnelManager *webhooktunnel.Manager, userInput *userinput.Service) *channel.Registry {
	registry := channel.NewRegistry()

	tgAdapter := telegram.NewTelegramAdapter(log)
	tgAdapter.SetAssetOpener(mediaService)
	// Telegram's ask_user buttons drive the durable interaction state.
	tgAdapter.SetUserInputService(userInput)
	registry.MustRegister(tgAdapter)

	discordAdapter := discord.NewDiscordAdapter(log)
	discordAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(discordAdapter)

	qqAdapter := qq.NewQQAdapter(log)
	qqAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(qqAdapter)

	matrixAdapter := matrix.NewMatrixAdapter(log)
	matrixAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(matrixAdapter)

	feishuAdapter := feishu.NewFeishuAdapter(log)
	feishuAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(feishuAdapter)

	slackAdapter := slackadapter.NewSlackAdapter(log)
	slackAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(slackAdapter)

	registry.MustRegister(wecom.NewWeComAdapter(log))

	dingTalkAdapter := dingtalk.NewDingTalkAdapter(log)
	registry.MustRegister(dingTalkAdapter)
	registry.MustRegister(wechatoa.NewWeChatOAAdapter(log))
	lineAdapter := line.NewAdapter(log)
	lineAdapter.SetPublicBaseURLProvider(newPublicMediaBaseProvider(cfg, tunnelManager))
	registry.MustRegister(lineAdapter)

	weixinAdapter := weixin.NewWeixinAdapter(log)
	weixinAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(weixinAdapter)
	registry.MustRegister(local.NewWebAdapter(hub))
	registry.MustRegister(misskey.NewMisskeyAdapter(log))

	return registry
}

type publicMediaBaseProvider struct {
	tunnel *webhooktunnel.Manager
	signer *publicmedia.Signer
}

func newPublicMediaBaseProvider(cfg config.Config, tunnel *webhooktunnel.Manager) *publicMediaBaseProvider {
	return &publicMediaBaseProvider{
		tunnel: tunnel,
		signer: publicmedia.NewSigner(cfg.Auth.JWTSecret, publicmedia.SignedURLTTL),
	}
}

func (p *publicMediaBaseProvider) PublicBaseURL() string {
	if p == nil {
		return ""
	}
	if p.tunnel != nil {
		return p.tunnel.PublicBaseURL()
	}
	return ""
}

func (p *publicMediaBaseProvider) SignPublicMediaPath(path string) (string, bool) {
	if p == nil || p.signer == nil {
		return "", false
	}
	return p.signer.SignPath(path, time.Now().UTC())
}

func provideChannelRouter(
	log *slog.Logger,
	registry *channel.Registry,
	hub *local.RouteHub,
	routeService *route.DBService,
	sessionService *sessionpkg.Service,
	msgService *message.DBService,
	turnService turn.Service,
	identityService *identities.Service,
	botService *bots.Service,
	accountService *accounts.Service,
	aclService *acl.Service,
	policyService *policy.Service,
	mediaService *media.Service,
	audioService *audiopkg.Service,
	settingsService *settings.Service,
	pipeline *pipelinepkg.Pipeline,
	eventStore *pipelinepkg.EventStore,
	discussDriver *pipelinepkg.DiscussDriver,
	rc *boot.RuntimeConfig,
	cmdHandler *command.Handler,
	containerdHandler *handlers.ContainerdHandler,
) *inbound.ChannelInboundProcessor {
	adapter, ok := registry.Get(qq.Type)
	if !ok {
		panic("qq adapter not registered")
	}
	qqAdapter, ok := adapter.(*qq.QQAdapter)
	if !ok {
		panic("qq adapter has unexpected type")
	}
	qqAdapter.SetChannelIdentityResolver(identityService)
	qqAdapter.SetRouteResolver(routeService)

	processor := inbound.NewChannelInboundProcessor(log, registry, routeService, msgService, turnService, identityService, policyService, rc.JwtSecret, 5*time.Minute)
	processor.SetSessionEnsurer(&sessionEnsurerAdapter{svc: sessionService})
	processor.SetPipeline(pipeline, eventStore, discussDriver)
	discussDriver.SetTurnService(turnService)
	discussDriver.SetBroadcaster(hub)
	processor.SetACLService(aclService)
	processor.SetMediaService(mediaService)
	processor.SetStreamObserver(local.NewRouteHubBroadcaster(hub))
	processor.SetDispatcher(inbound.NewRouteDispatcher(log))
	processor.SetSpeechService(audioService, &settingsSpeechModelResolver{settings: settingsService})
	processor.SetTranscriptionService(&settingsTranscriptionAdapter{audio: audioService}, &settingsTranscriptionModelResolver{settings: settingsService})
	processor.SetIMDisplayOptions(&settingsIMDisplayOptions{settings: settingsService})
	processor.SetDefaultChatRuntime(&settingsDefaultChatRuntime{settings: settingsService})
	processor.SetACPAgentSetupReader(&botACPAgentSetupReader{bots: botService})
	processor.SetBotPermissionChecker(&botPermissionCheckerAdapter{bots: botService, accounts: accountService})
	processor.SetCommandHandler(cmdHandler)
	processor.SetRequestedSkillResolver(containerdHandler)
	return processor
}

func provideCommandHandler(
	log *slog.Logger,
	botService *bots.Service,
	channelAccessService *channelaccess.Service,
	scheduleService *schedule.Service,
	settingsService *settings.Service,
	mcpConnService *mcp.ConnectionService,
	modelsService *models.Service,
	providersService *providers.Service,
	memProvService *memprovider.Service,
	searchProvService *searchproviders.Service,
	emailService *emailpkg.Service,
	emailOutboxService *emailpkg.OutboxService,
	heartbeatService *heartbeat.Service,
	queries dbstore.Queries,
	aclService *acl.Service,
	containerdHandler *handlers.ContainerdHandler,
	provider bridge.Provider,
	compactionService *compaction.Service,
) *command.Handler {
	cmdHandler := command.NewHandler(
		log,
		&command.BotMemberRoleAdapter{BotService: botService, ManageResolver: channelAccessService},
		scheduleService,
		settingsService,
		mcpConnService,
		modelsService,
		providersService,
		memProvService,
		searchProvService,
		emailService,
		emailOutboxService,
		heartbeatService,
		queries,
		aclService,
		&commandSkillLoaderAdapter{handler: containerdHandler},
		&commandContainerFSAdapter{provider: provider},
	)
	cmdHandler.SetCompactionService(compactionService, queries)
	cmdHandler.SetLinkConsumer(channelAccessService)
	return cmdHandler
}

func provideChannelManager(log *slog.Logger, registry *channel.Registry, channelStore *channel.Store, channelRouter *inbound.ChannelInboundProcessor, mediaService *media.Service) *channel.Manager {
	if adapter, ok := registry.Get(matrix.Type); ok {
		if matrixAdapter, ok := adapter.(*matrix.MatrixAdapter); ok {
			matrixAdapter.SetSyncStateSaver(channelStore.SaveMatrixSyncSinceToken)
		}
	}
	mgr := channel.NewManager(log, registry, channelStore, channelRouter)
	mgr.SetAttachmentStore(mediaService)
	if mw := channelRouter.IdentityMiddleware(); mw != nil {
		mgr.Use(mw)
	}
	channelRouter.SetReactor(mgr)
	return mgr
}

func provideChannelLifecycleService(channelStore *channel.Store, channelManager *channel.Manager) *channel.Lifecycle {
	return channel.NewLifecycle(channelStore, channelManager)
}

func startWebhookTunnel(lc fx.Lifecycle, manager *webhooktunnel.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			return manager.Start(ctx)
		},
		OnStop: func(stopCtx context.Context) error {
			err := manager.Stop(stopCtx)
			cancel()
			return err
		},
	})
}

func startWebhookTunnelListener(lc fx.Lifecycle, log *slog.Logger, cfg config.Config, store *channel.Store, channelManager *channel.Manager, mediaService *media.Service) {
	if cfg.WebhookTunnel.EffectiveMode() == config.WebhookTunnelModeDisabled {
		return
	}
	addr := strings.TrimSpace(cfg.WebhookTunnel.ListenAddr)
	if addr == "" {
		addr = webhooktunnel.DefaultListenAddr
	}
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("1M"))
	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok\n")
	})
	channel.NewWebhookServerHandler(log, store, channelManager).Register(e)
	// This listener is only started for tunnel modes. Its public base URL is
	// resolved from either configured public_base_url or the running tunnel, so
	// the configured-public-base gate used by the main server is intentionally
	// not applied here.
	handlers.NewPublicMediaHandler(log, mediaService, cfg.Auth.JWTSecret).Register(e)
	logger := log.With(slog.String("component", "webhook_tunnel_listener"), slog.String("addr", addr))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
			if err != nil {
				return fmt.Errorf("webhook tunnel listener: %w", err)
			}
			go func() {
				logger.Info("webhook tunnel listener started")
				if err := e.Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error("webhook tunnel listener failed", slog.Any("error", err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return e.Shutdown(ctx)
		},
	})
}

type sessionEnsurerAdapter struct {
	svc *sessionpkg.Service
}

func (a *sessionEnsurerAdapter) EnsureActiveSession(ctx context.Context, botID, routeID, channelType string) (inbound.SessionResult, error) {
	sess, err := a.svc.EnsureActiveSession(ctx, botID, routeID, channelType)
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inboundSessionResult(sess), nil
}

func (a *sessionEnsurerAdapter) GetActiveSession(ctx context.Context, routeID string) (inbound.SessionResult, error) {
	sess, err := a.svc.GetActiveForRoute(ctx, routeID)
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inboundSessionResult(sess), nil
}

func (a *sessionEnsurerAdapter) CreateNewSession(ctx context.Context, botID, routeID, channelType string, spec inbound.NewSessionSpec) (inbound.SessionResult, error) {
	createdByUserID := newSessionCreatedByUserID(spec)
	sess, err := a.svc.CreateNewSessionWithInput(ctx, sessionpkg.CreateInput{
		BotID:           botID,
		RouteID:         routeID,
		ChannelType:     channelType,
		Type:            spec.Type,
		SessionMode:     spec.Mode,
		RuntimeType:     spec.Runtime,
		Metadata:        spec.Metadata,
		RuntimeMetadata: spec.Metadata,
		Title:           spec.Title,
		CreatedByUserID: createdByUserID,
	})
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inboundSessionResult(sess), nil
}

func newSessionCreatedByUserID(spec inbound.NewSessionSpec) string {
	if userID := strings.TrimSpace(spec.CreatedByUserID); userID != "" {
		return userID
	}
	return strings.TrimSpace(spec.RuntimeOwnerAccountID)
}

func inboundSessionResult(sess sessionpkg.Session) inbound.SessionResult {
	return inbound.SessionResult{
		ID:                    sess.ID,
		Type:                  sess.Type,
		Mode:                  sess.SessionMode,
		Runtime:               sess.RuntimeType,
		RuntimeOwnerAccountID: sessionRuntimeOwnerAccountID(sess),
	}
}

func sessionRuntimeOwnerAccountID(sess sessionpkg.Session) string {
	if value := runtimeMetadataString(sess.RuntimeMetadata, "runtime_owner_account_id"); value != "" {
		return value
	}
	return runtimeMetadataString(sess.Metadata, "runtime_owner_account_id")
}

func runtimeMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

type settingsSpeechModelResolver struct {
	settings *settings.Service
}

func (r *settingsSpeechModelResolver) ResolveSpeechModelID(ctx context.Context, botID string) (string, error) {
	s, err := r.settings.GetBot(ctx, botID)
	if err != nil {
		return "", err
	}
	return s.TtsModelID, nil
}

type settingsIMDisplayOptions struct {
	settings *settings.Service
}

func (r *settingsIMDisplayOptions) ShowToolCallsInIM(ctx context.Context, botID string) (bool, error) {
	s, err := r.settings.GetBot(ctx, botID)
	if err != nil {
		return false, err
	}
	return s.ShowToolCallsInIM, nil
}

type settingsDefaultChatRuntime struct {
	settings *settings.Service
}

func (r *settingsDefaultChatRuntime) DefaultChatRuntime(ctx context.Context, botID string) (inbound.DefaultChatRuntimeSettings, error) {
	s, err := r.settings.GetBot(ctx, botID)
	if err != nil {
		return inbound.DefaultChatRuntimeSettings{}, err
	}
	return inbound.DefaultChatRuntimeSettings{
		Runtime:     s.ChatRuntime,
		ACPAgentID:  s.ChatACPAgentID,
		ProjectPath: s.ChatACPProjectPath,
		ProjectMode: s.ChatACPProjectMode,
	}, nil
}

type botACPAgentSetupReader struct {
	bots *bots.Service
}

func (r *botACPAgentSetupReader) ACPAgentSetupMetadata(ctx context.Context, botID string) (map[string]any, error) {
	if r == nil || r.bots == nil {
		return nil, errors.New("bot setup reader not configured")
	}
	bot, err := r.bots.Get(ctx, botID)
	if err != nil {
		return nil, err
	}
	return bot.Metadata, nil
}

type botPermissionCheckerAdapter struct {
	bots     *bots.Service
	accounts *accounts.Service
}

func (a *botPermissionCheckerAdapter) HasBotPermission(ctx context.Context, botID, accountID, permission string) (bool, error) {
	if a == nil || a.bots == nil || a.accounts == nil {
		return false, errors.New("bot permission services not configured")
	}
	isAdmin, err := a.accounts.IsAdmin(ctx, accountID)
	if err != nil {
		return false, err
	}
	perms, err := a.bots.ResolveUserPermissions(ctx, botID, accountID, isAdmin)
	if err != nil {
		return false, err
	}
	return bots.HasPermission(perms, permission), nil
}

type settingsTranscriptionModelResolver struct {
	settings *settings.Service
}

func (r *settingsTranscriptionModelResolver) ResolveTranscriptionModelID(ctx context.Context, botID string) (string, error) {
	s, err := r.settings.GetBot(ctx, botID)
	if err != nil {
		return "", err
	}
	return s.TranscriptionModelID, nil
}

type settingsTranscriptionAdapter struct {
	audio *audiopkg.Service
}

func (a *settingsTranscriptionAdapter) Transcribe(ctx context.Context, modelID string, audio []byte, filename string, contentType string, overrideCfg map[string]any) (inbound.TranscriptionResult, error) {
	result, err := a.audio.Transcribe(ctx, modelID, audio, filename, contentType, overrideCfg)
	if err != nil {
		return nil, err
	}
	return inboundTranscriptionResult{text: result.Text}, nil
}

func provideEmailRegistry(log *slog.Logger, tokenStore *emailpkg.DBOAuthTokenStore, oauthClients *oauthclients.Registry) *emailpkg.Registry {
	reg := emailpkg.NewRegistry()
	reg.Register(emailgeneric.New(log))
	reg.Register(emailmailgun.New(log))
	reg.Register(emailgmail.New(log, tokenStore, oauthClients))
	return reg
}

func provideEmailChatGateway(resolver *flow.Resolver, queries dbstore.Queries, cfg config.Config, log *slog.Logger) emailpkg.ChatTriggerer {
	return flow.NewEmailChatGateway(resolver, queries, cfg.Auth.JWTSecret, log)
}

func provideEmailTrigger(log *slog.Logger, service *emailpkg.Service, chatTriggerer emailpkg.ChatTriggerer) *emailpkg.Trigger {
	return emailpkg.NewTrigger(log, service, chatTriggerer)
}

func startEmailManager(lc fx.Lifecycle, emailManager *emailpkg.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				if err := emailManager.Start(ctx); err != nil {
					slog.Default().Error("email manager start failed", slog.Any("error", err))
				}
			}()
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			emailManager.Stop(stopCtx)
			return nil
		},
	})
}

func startChannelManager(lc fx.Lifecycle, channelManager *channel.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			channelManager.Start(ctx)
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			return channelManager.Shutdown(stopCtx)
		},
	})
}

type commandSkillLoaderAdapter struct {
	handler *handlers.ContainerdHandler
}

func (a *commandSkillLoaderAdapter) LoadSkills(ctx context.Context, botID string) ([]command.Skill, error) {
	items, err := a.handler.LoadSkills(ctx, botID)
	if err != nil {
		return nil, err
	}
	skills := make([]command.Skill, len(items))
	for i, item := range items {
		skills[i] = command.Skill{Name: item.Name, Description: item.Description}
	}
	return skills, nil
}

// ListRuntimeSkills exposes the runtime-usable safe catalog (the same list the
// Web slash picker shows) as the command layer's optional RuntimeSkillLister
// capability, upgrading /skill list to tap-to-activate rows.
func (a *commandSkillLoaderAdapter) ListRuntimeSkills(ctx context.Context, botID string) ([]command.Skill, error) {
	items, err := a.handler.ListSafeSkillCatalog(ctx, botID)
	if err != nil {
		return nil, err
	}
	skills := make([]command.Skill, len(items))
	for i, item := range items {
		skills[i] = command.Skill{Name: item.Name, Description: item.Description}
	}
	return skills, nil
}

type commandContainerFSAdapter struct {
	provider bridge.Provider
}

func (a *commandContainerFSAdapter) ListDir(ctx context.Context, botID, dirPath string) ([]command.FSEntry, error) {
	client, err := a.provider.MCPClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries, err := client.ListDirAll(ctx, dirPath, false)
	if err != nil {
		return nil, err
	}
	result := make([]command.FSEntry, len(entries))
	for i, e := range entries {
		name := stdpath.Base(e.GetPath())
		result[i] = command.FSEntry{Name: name, IsDir: e.GetIsDir(), Size: e.GetSize()}
	}
	return result, nil
}

func (a *commandContainerFSAdapter) ReadFile(ctx context.Context, botID, filePath string) (string, error) {
	client, err := a.provider.MCPClient(ctx, botID)
	if err != nil {
		return "", err
	}
	resp, err := client.ReadFile(ctx, filePath, 0, 0)
	if err != nil {
		return "", err
	}
	return resp.GetContent(), nil
}

type inboundTranscriptionResult struct {
	text string
}

func (r inboundTranscriptionResult) GetText() string { return r.text }
