package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	stdpath "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acl"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/background"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	audiopkg "github.com/memohai/memoh/internal/audio"
	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/dingtalk"
	"github.com/memohai/memoh/internal/channel/adapters/discord"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
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
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	emailpkg "github.com/memohai/memoh/internal/email"
	emailgeneric "github.com/memohai/memoh/internal/email/adapters/generic"
	emailgmail "github.com/memohai/memoh/internal/email/adapters/gmail"
	emailmailgun "github.com/memohai/memoh/internal/email/adapters/mailgun"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/healthcheck"
	channelchecker "github.com/memohai/memoh/internal/healthcheck/checkers/channel"
	mcpchecker "github.com/memohai/memoh/internal/healthcheck/checkers/mcp"
	modelchecker "github.com/memohai/memoh/internal/healthcheck/checkers/model"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	mcpfederation "github.com/memohai/memoh/internal/mcp/sources/federation"
	"github.com/memohai/memoh/internal/media"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	membuiltin "github.com/memohai/memoh/internal/memory/adapters/builtin"
	memmem0 "github.com/memohai/memoh/internal/memory/adapters/mem0"
	memopenviking "github.com/memohai/memoh/internal/memory/adapters/openviking"
	"github.com/memohai/memoh/internal/memory/memllm"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/messaging"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/orchestration"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/registry"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/server"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/storage/providers/containerfs"
	"github.com/memohai/memoh/internal/storage/providers/fallback"
	"github.com/memohai/memoh/internal/storage/providers/localfs"
	"github.com/memohai/memoh/internal/version"
	"github.com/memohai/memoh/internal/workspace"
)

func provideServerHandler(fn any) any {
	return fx.Annotate(
		fn,
		fx.As(new(server.Handler)),
		fx.ResultTags(`group:"server_handlers"`),
	)
}

func provideLogger(cfg config.Config) *slog.Logger {
	logger.Init(cfg.Log.Level, cfg.Log.Format)
	return logger.L
}

func provideContainerService(lc fx.Lifecycle, log *slog.Logger, cfg config.Config, rc *boot.RuntimeConfig) (ctr.Service, error) {
	svc, cleanup, err := ctr.ProvideService(context.Background(), log, cfg, rc.ContainerBackend)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			cleanup()
			return nil
		},
	})
	return svc, nil
}

func provideDBConn(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	conn, err := db.Open(context.Background(), cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			conn.Close()
			return nil
		},
	})
	return conn, nil
}

func provideDBQueries(conn *pgxpool.Pool) *dbsqlc.Queries {
	return dbsqlc.New(conn)
}

func provideWorkspaceManager(log *slog.Logger, service ctr.Service, cfg config.Config, conn *pgxpool.Pool) *workspace.Manager {
	return workspace.NewManager(log, service, cfg.Workspace, cfg.Containerd.Namespace, conn)
}

func provideMemoryLLM(modelsService *models.Service, settingsService *settings.Service, queries *dbsqlc.Queries, log *slog.Logger) memprovider.LLM {
	return &lazyLLMClient{
		modelsService:   modelsService,
		settingsService: settingsService,
		queries:         queries,
		timeout:         models.DefaultProviderRequestTimeout,
		logger:          log,
	}
}

func provideMemoryProviderRegistry(log *slog.Logger, llm memprovider.LLM, chatService *conversation.Service, accountService *accounts.Service, manager *workspace.Manager, queries *dbsqlc.Queries, cfg config.Config) *memprovider.Registry {
	registry := memprovider.NewRegistry(log)
	fileRuntime := handlers.NewBuiltinMemoryRuntime(manager)
	fileStore := storefs.New(log, manager)
	registry.RegisterFactory(string(memprovider.ProviderBuiltin), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		runtime, err := membuiltin.NewBuiltinRuntimeFromConfig(log, providerConfig, fileRuntime, fileStore, queries, cfg)
		if err != nil {
			return nil, err
		}
		p := membuiltin.NewBuiltinProvider(log, runtime, chatService, accountService)
		p.SetLLM(llm)
		p.ApplyProviderConfig(providerConfig)
		return p, nil
	})
	registry.RegisterFactory(string(memprovider.ProviderMem0), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		return memmem0.NewMem0Provider(log, providerConfig, fileStore)
	})
	registry.RegisterFactory(string(memprovider.ProviderOpenViking), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		return memopenviking.NewOpenVikingProvider(log, providerConfig)
	})
	defaultProvider := membuiltin.NewBuiltinProvider(log, fileRuntime, chatService, accountService)
	defaultProvider.SetLLM(llm)
	registry.Register("__builtin_default__", defaultProvider)
	return registry
}

func providePipeline() *pipelinepkg.Pipeline {
	return pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
}

func provideEventStore(log *slog.Logger, queries *dbsqlc.Queries) *pipelinepkg.EventStore {
	return pipelinepkg.NewEventStore(log, queries)
}

func provideDiscussDriver(log *slog.Logger, pipeline *pipelinepkg.Pipeline, eventStore *pipelinepkg.EventStore, agent *agentpkg.Agent, msgService *message.DBService) *pipelinepkg.DiscussDriver {
	return pipelinepkg.NewDiscussDriver(pipelinepkg.DiscussDriverDeps{
		Pipeline:       pipeline,
		EventStore:     eventStore,
		Agent:          agent,
		MessageService: msgService,
		Logger:         log,
	})
}

func provideRouteService(log *slog.Logger, queries *dbsqlc.Queries, chatService *conversation.Service) *route.DBService {
	return route.NewService(log, queries, chatService)
}

func provideSessionService(log *slog.Logger, queries *dbsqlc.Queries) *sessionpkg.Service {
	return sessionpkg.NewService(log, queries)
}

func provideMessageService(log *slog.Logger, queries *dbsqlc.Queries, hub *event.Hub) *message.DBService {
	return message.NewService(log, queries, hub)
}

func provideScheduleTriggerer(resolver *flow.Resolver) schedule.Triggerer {
	return flow.NewScheduleGateway(resolver)
}

func provideHeartbeatTriggerer(resolver *flow.Resolver) heartbeat.Triggerer {
	return flow.NewHeartbeatGateway(resolver)
}

type sessionCreatorAdapter struct {
	svc *sessionpkg.Service
}

func (a *sessionCreatorAdapter) CreateSession(ctx context.Context, botID, sessionType string) (string, error) {
	sess, err := a.svc.Create(ctx, sessionpkg.CreateInput{
		BotID: botID,
		Type:  sessionType,
	})
	if err != nil {
		return "", err
	}
	return sess.ID, nil
}

func provideHeartbeatSessionCreator(sessionService *sessionpkg.Service) heartbeat.SessionCreator {
	return &sessionCreatorAdapter{svc: sessionService}
}

func provideScheduleSessionCreator(sessionService *sessionpkg.Service) schedule.SessionCreator {
	return &sessionCreatorAdapter{svc: sessionService}
}

func provideAgent(log *slog.Logger, manager *workspace.Manager) *agentpkg.Agent {
	return agentpkg.New(agentpkg.Deps{
		BridgeProvider: manager,
		Logger:         log,
	})
}

func injectToolProviders(a *agentpkg.Agent, msgService *message.DBService, providers []agenttools.ToolProvider) {
	a.SetToolProviders(providers)
	for _, p := range providers {
		if sp, ok := p.(*agenttools.SpawnProvider); ok {
			sp.SetAgent(agentpkg.NewSpawnAdapter(a))
			sp.SetMessageService(msgService)
			sp.SetSystemPromptFunc(agentpkg.SpawnSystemPrompt)
			sp.SetModelCreator(agentpkg.SpawnModelCreatorFunc())
		}
	}
}

func provideChatResolver(log *slog.Logger, a *agentpkg.Agent, modelsService *models.Service, queries *dbsqlc.Queries, chatService *conversation.Service, msgService *message.DBService, settingsService *settings.Service, accountService *accounts.Service, mediaService *media.Service, containerdHandler *handlers.ContainerdHandler, memoryRegistry *memprovider.Registry, channelStore *channel.Store, routeService *route.DBService, sessionService *sessionpkg.Service, eventHub *event.Hub, compactionService *compaction.Service, pipeline *pipelinepkg.Pipeline, rc *boot.RuntimeConfig, bgManager *background.Manager) *flow.Resolver {
	resolver := flow.NewResolver(log, modelsService, queries, chatService, msgService, settingsService, accountService, a, rc.TimezoneLocation, 120*time.Second)
	resolver.SetMemoryRegistry(memoryRegistry)
	resolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	resolver.SetGatewayAssetLoader(&gatewayAssetLoaderAdapter{media: mediaService})
	resolver.SetChannelStore(channelStore)
	resolver.SetRouteService(routeService)
	resolver.SetSessionService(sessionService)
	resolver.SetEventPublisher(eventHub)
	resolver.SetCompactionService(compactionService)
	resolver.SetPipeline(pipeline)
	resolver.SetBackgroundManager(bgManager)
	bgManager.SetWakeFunc(func(botID, sessionID string) {
		resolver.TriggerBackgroundNotification(context.Background(), botID, sessionID)
	})
	return resolver
}

func provideChannelRegistry(log *slog.Logger, hub *local.RouteHub, mediaService *media.Service) *channel.Registry {
	registry := channel.NewRegistry()

	tgAdapter := telegram.NewTelegramAdapter(log)
	tgAdapter.SetAssetOpener(mediaService)
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

	weixinAdapter := weixin.NewWeixinAdapter(log)
	weixinAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(weixinAdapter)
	registry.MustRegister(local.NewWebAdapter(hub))
	registry.MustRegister(misskey.NewMisskeyAdapter(log))

	return registry
}

func provideChannelRouter(
	log *slog.Logger,
	registry *channel.Registry,
	hub *local.RouteHub,
	routeService *route.DBService,
	sessionService *sessionpkg.Service,
	msgService *message.DBService,
	resolver *flow.Resolver,
	identityService *identities.Service,
	botService *bots.Service,
	aclService *acl.Service,
	policyService *policy.Service,
	bindService *bind.Service,
	mediaService *media.Service,
	audioService *audiopkg.Service,
	settingsService *settings.Service,
	scheduleService *schedule.Service,
	mcpConnService *mcp.ConnectionService,
	modelsService *models.Service,
	providersService *providers.Service,
	memProvService *memprovider.Service,
	searchProvService *searchproviders.Service,
	browserCtxService *browsercontexts.Service,
	emailService *emailpkg.Service,
	emailOutboxService *emailpkg.OutboxService,
	heartbeatService *heartbeat.Service,
	compactionService *compaction.Service,
	queries *dbsqlc.Queries,
	containerdHandler *handlers.ContainerdHandler,
	manager *workspace.Manager,
	pipeline *pipelinepkg.Pipeline,
	eventStore *pipelinepkg.EventStore,
	discussDriver *pipelinepkg.DiscussDriver,
	rc *boot.RuntimeConfig,
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

	processor := inbound.NewChannelInboundProcessor(log, registry, routeService, msgService, resolver, identityService, policyService, bindService, rc.JwtSecret, 5*time.Minute)
	processor.SetSessionEnsurer(&sessionEnsurerAdapter{svc: sessionService})
	processor.SetPipeline(pipeline, eventStore, discussDriver)
	discussDriver.SetResolver(resolver)
	discussDriver.SetBroadcaster(hub)
	processor.SetACLService(aclService)
	processor.SetMediaService(mediaService)
	processor.SetStreamObserver(local.NewRouteHubBroadcaster(hub))
	processor.SetDispatcher(inbound.NewRouteDispatcher(log))
	processor.SetSpeechService(audioService, &settingsSpeechModelResolver{settings: settingsService})
	processor.SetTranscriptionService(&settingsTranscriptionAdapter{audio: audioService}, &settingsTranscriptionModelResolver{settings: settingsService})
	processor.SetIMDisplayOptions(&settingsIMDisplayOptions{settings: settingsService})
	cmdHandler := command.NewHandler(
		log,
		&command.BotMemberRoleAdapter{BotService: botService},
		scheduleService,
		settingsService,
		mcpConnService,
		modelsService,
		providersService,
		memProvService,
		searchProvService,
		browserCtxService,
		emailService,
		emailOutboxService,
		heartbeatService,
		queries,
		aclService,
		&commandSkillLoaderAdapter{handler: containerdHandler},
		&commandContainerFSAdapter{manager: manager},
	)
	cmdHandler.SetCompactionService(compactionService, queries)
	processor.SetCommandHandler(cmdHandler)
	return processor
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

func provideContainerdHandler(log *slog.Logger, manager *workspace.Manager, cfg config.Config, rc *boot.RuntimeConfig, botService *bots.Service, accountService *accounts.Service, policyService *policy.Service) *handlers.ContainerdHandler {
	return handlers.NewContainerdHandler(log, manager, cfg.Workspace, rc.ContainerBackend, botService, accountService, policyService)
}

func provideFederationGateway(log *slog.Logger, containerdHandler *handlers.ContainerdHandler) *handlers.MCPFederationGateway {
	return handlers.NewMCPFederationGateway(log, containerdHandler)
}

func provideOAuthService(log *slog.Logger, queries *dbsqlc.Queries, cfg config.Config) *mcp.OAuthService {
	addr := strings.TrimSpace(cfg.Server.Addr)
	if addr == "" {
		addr = ":8080"
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	callbackURL := "http://" + host + "/api/oauth/mcp/callback"
	return mcp.NewOAuthService(log, queries, callbackURL)
}

func provideToolGatewayService(log *slog.Logger, fedGateway *handlers.MCPFederationGateway, oauthService *mcp.OAuthService, mcpConnService *mcp.ConnectionService, containerdHandler *handlers.ContainerdHandler) *mcp.ToolGatewayService {
	fedGateway.SetOAuthService(oauthService)
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService)
	svc := mcp.NewToolGatewayService(log, []mcp.ToolSource{fedSource})
	containerdHandler.SetToolGatewayService(svc)
	return svc
}

func provideBackgroundManager(log *slog.Logger) *background.Manager {
	return background.New(log)
}

func provideToolProviders(log *slog.Logger, cfg config.Config, channelManager *channel.Manager, registry *channel.Registry, routeService *route.DBService, scheduleService *schedule.Service, settingsService *settings.Service, searchProviderService *searchproviders.Service, manager *workspace.Manager, mediaService *media.Service, memoryRegistry *memprovider.Registry, emailService *emailpkg.Service, emailManager *emailpkg.Manager, fedGateway *handlers.MCPFederationGateway, mcpConnService *mcp.ConnectionService, modelsService *models.Service, browserContextService *browsercontexts.Service, queries *dbsqlc.Queries, audioService *audiopkg.Service, sessionService *sessionpkg.Service, bgManager *background.Manager) []agenttools.ToolProvider {
	var assetResolver messaging.AssetResolver
	if mediaService != nil {
		assetResolver = &mediaAssetResolverAdapter{media: mediaService}
	}
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService)
	return []agenttools.ToolProvider{
		agenttools.NewMessageProvider(log, channelManager, channelManager, registry, assetResolver),
		agenttools.NewContactsProvider(log, routeService),
		agenttools.NewScheduleProvider(log, scheduleService),
		agenttools.NewMemoryProvider(log, memoryRegistry, settingsService),
		agenttools.NewWebProvider(log, settingsService, searchProviderService),
		agenttools.NewContainerProvider(log, manager, bgManager, config.DefaultDataMount),
		agenttools.NewEmailProvider(log, emailService, emailManager),
		agenttools.NewWebFetchProvider(log),
		agenttools.NewSpawnProvider(log, settingsService, modelsService, queries, sessionService),
		agenttools.NewSkillProvider(log),
		agenttools.NewBrowserProvider(log, settingsService, browserContextService, manager, cfg.BrowserGateway),
		agenttools.NewTTSProvider(log, settingsService, audioService, channelManager, registry),
		agenttools.NewTranscriptionProvider(log, settingsService, audioService, mediaService),
		agenttools.NewImageGenProvider(log, settingsService, modelsService, queries, manager, config.DefaultDataMount),
		agenttools.NewFederationProvider(log, fedSource),
		agenttools.NewHistoryProvider(log, sessionService, queries),
	}
}

func provideMemoryHandler(log *slog.Logger, botService *bots.Service, accountService *accounts.Service, _ config.Config, manager *workspace.Manager, memoryRegistry *memprovider.Registry, settingsService *settings.Service, _ *handlers.ContainerdHandler) *handlers.MemoryHandler {
	h := handlers.NewMemoryHandler(log, botService, accountService)
	h.SetMemoryRegistry(memoryRegistry)
	h.SetSettingsService(settingsService)
	h.SetMCPClientProvider(manager)
	return h
}

func provideAuthHandler(log *slog.Logger, accountService *accounts.Service, rc *boot.RuntimeConfig) *handlers.AuthHandler {
	return handlers.NewAuthHandler(log, accountService, rc.JwtSecret, rc.JwtExpiresIn)
}

func provideMessageHandler(log *slog.Logger, chatService *conversation.Service, msgService *message.DBService, mediaService *media.Service, botService *bots.Service, accountService *accounts.Service, hub *event.Hub) *handlers.MessageHandler {
	h := handlers.NewMessageHandler(log, chatService, msgService, botService, accountService, hub)
	h.SetMediaService(mediaService)
	return h
}

func provideSessionHandler(log *slog.Logger, sessionService *sessionpkg.Service, botService *bots.Service, accountService *accounts.Service) *handlers.SessionHandler {
	return handlers.NewSessionHandler(log, sessionService, botService, accountService)
}

func provideMediaService(log *slog.Logger, manager *workspace.Manager, cfg config.Config) *media.Service {
	primary := containerfs.New(manager)
	dataRoot := cfg.Workspace.DataRoot
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	secondary := localfs.New(filepath.Join(dataRoot, "media"))
	provider := fallback.New(primary, secondary)
	return media.NewService(log, provider)
}

func provideUsersHandler(log *slog.Logger, accountService *accounts.Service, identityService *identities.Service, botService *bots.Service, routeService *route.DBService, channelStore *channel.Store, channelLifecycle *channel.Lifecycle, channelManager *channel.Manager, registry *channel.Registry) *handlers.UsersHandler {
	return handlers.NewUsersHandler(log, accountService, identityService, botService, routeService, channelStore, channelLifecycle, channelManager, registry)
}

func provideWebHandler(channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, resolver *flow.Resolver, mediaService *media.Service, audioService *audiopkg.Service, settingsService *settings.Service) *handlers.LocalChannelHandler {
	h := handlers.NewLocalChannelHandler(local.WebType, channelManager, channelStore, chatService, hub, botService, accountService)
	h.SetResolver(resolver)
	h.SetMediaService(mediaService)
	h.SetSpeechService(audioService, &settingsSpeechModelResolver{settings: settingsService})
	return h
}

func provideAudioRegistry() *audiopkg.Registry {
	return audiopkg.NewRegistry()
}

func provideAudioTempStore() (*audiopkg.TempStore, error) {
	return audiopkg.NewTempStore(os.TempDir())
}

func startAudioTempStoreCleanup(lc fx.Lifecycle, store *audiopkg.TempStore) {
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go store.StartCleanup(done)
			return nil
		},
		OnStop: func(_ context.Context) error {
			close(done)
			return nil
		},
	})
}

func startBackgroundTaskCleanup(lc fx.Lifecycle, mgr *background.Manager) {
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go mgr.StartCleanupLoop(done, background.DefaultCleanupInterval, background.DefaultTaskRetention)
			return nil
		},
		OnStop: func(_ context.Context) error {
			close(done)
			return nil
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
	return inbound.SessionResult{ID: sess.ID, Type: sess.Type}, nil
}

func (a *sessionEnsurerAdapter) GetActiveSession(ctx context.Context, routeID string) (inbound.SessionResult, error) {
	sess, err := a.svc.GetActiveForRoute(ctx, routeID)
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inbound.SessionResult{ID: sess.ID, Type: sess.Type}, nil
}

func (a *sessionEnsurerAdapter) CreateNewSession(ctx context.Context, botID, routeID, channelType, sessionType string) (inbound.SessionResult, error) {
	sess, err := a.svc.CreateNewSession(ctx, botID, routeID, channelType, sessionType)
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inbound.SessionResult{ID: sess.ID, Type: sess.Type}, nil
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

type inboundTranscriptionResult struct {
	text string
}

func (r inboundTranscriptionResult) GetText() string { return r.text }

func (a *settingsTranscriptionAdapter) Transcribe(ctx context.Context, modelID string, audio []byte, filename string, contentType string, overrideCfg map[string]any) (inbound.TranscriptionResult, error) {
	result, err := a.audio.Transcribe(ctx, modelID, audio, filename, contentType, overrideCfg)
	if err != nil {
		return nil, err
	}
	return inboundTranscriptionResult{text: result.Text}, nil
}

func provideEmailRegistry(log *slog.Logger, tokenStore *emailpkg.DBOAuthTokenStore) *emailpkg.Registry {
	reg := emailpkg.NewRegistry()
	reg.Register(emailgeneric.New(log))
	reg.Register(emailmailgun.New(log))
	reg.Register(emailgmail.New(log, tokenStore))
	return reg
}

func provideProvidersService(log *slog.Logger, queries *dbsqlc.Queries, _ config.Config) *providers.Service {
	return providers.NewService(log, queries, defaultProviderOAuthCallbackURL())
}

func defaultProviderOAuthCallbackURL() string {
	return "http://localhost:1455/auth/callback"
}

func provideEmailOAuthHandler(log *slog.Logger, service *emailpkg.Service, tokenStore *emailpkg.DBOAuthTokenStore, cfg config.Config) *handlers.EmailOAuthHandler {
	addr := strings.TrimSpace(cfg.Server.Addr)
	if addr == "" {
		addr = ":8080"
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	callbackURL := "http://" + host + "/api/email/oauth/callback"
	return handlers.NewEmailOAuthHandler(log, service, tokenStore, callbackURL)
}

func provideEmailChatGateway(resolver *flow.Resolver, queries *dbsqlc.Queries, cfg config.Config, log *slog.Logger) emailpkg.ChatTriggerer {
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

type serverParams struct {
	fx.In

	Logger            *slog.Logger
	RuntimeConfig     *boot.RuntimeConfig
	Config            config.Config
	ServerHandlers    []server.Handler `group:"server_handlers"`
	ContainerdHandler *handlers.ContainerdHandler
}

func provideServer(params serverParams) *server.Server {
	allHandlers := make([]server.Handler, 0, len(params.ServerHandlers)+1)
	allHandlers = append(allHandlers, params.ServerHandlers...)
	allHandlers = append(allHandlers, params.ContainerdHandler)
	return server.NewServer(params.Logger, params.RuntimeConfig.ServerAddr, params.Config.Auth.JWTSecret, allHandlers...)
}

func startRegistrySync(lc fx.Lifecycle, log *slog.Logger, cfg config.Config, queries *dbsqlc.Queries) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			defs, err := registry.Load(log, cfg.Registry.ProvidersPath())
			if err != nil {
				log.Warn("registry: failed to load provider definitions", slog.Any("error", err))
				return nil
			}
			if len(defs) == 0 {
				return nil
			}
			return registry.Sync(ctx, log, queries, defs)
		},
	})
}

func startAudioProviderBootstrap(lc fx.Lifecycle, log *slog.Logger, queries *dbsqlc.Queries, registry *audiopkg.Registry) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := audiopkg.SyncRegistry(ctx, log, queries, registry); err != nil {
				log.Warn("audio registry bootstrap failed", slog.Any("error", err))
			}
			return nil
		},
	})
}

func startMemoryProviderBootstrap(lc fx.Lifecycle, log *slog.Logger, mpService *memprovider.Service, registry *memprovider.Registry) {
	mpService.SetRegistry(registry)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			resp, err := mpService.EnsureDefault(ctx)
			if err != nil {
				log.Warn("failed to ensure default memory provider", slog.Any("error", err))
				return nil
			}
			if _, regErr := registry.Instantiate(resp.ID, resp.Provider, resp.Config); regErr != nil {
				log.Warn("failed to instantiate default memory provider", slog.Any("error", regErr))
			} else {
				log.Info("default memory provider ready", slog.String("id", resp.ID), slog.String("provider", resp.Provider))
			}
			return nil
		},
	})
}

func startSearchProviderBootstrap(lc fx.Lifecycle, log *slog.Logger, spService *searchproviders.Service) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := spService.EnsureDefaults(ctx); err != nil {
				log.Warn("failed to ensure default search providers", slog.Any("error", err))
			}
			return nil
		},
	})
}

func startScheduleService(lc fx.Lifecycle, scheduleService *schedule.Service) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return scheduleService.Bootstrap(ctx)
		},
	})
}

func startHeartbeatService(lc fx.Lifecycle, heartbeatService *heartbeat.Service) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return heartbeatService.Bootstrap(ctx)
		},
	})
}

func startOrchestrationRuntime(lc fx.Lifecycle, orchestrationService *orchestration.Service) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	builtinVerifierEnabled := orchestrationBuiltinVerifierEnabled()
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			workerCount := 4
			if builtinVerifierEnabled {
				workerCount++
			}
			wg.Add(workerCount)
			go func() {
				defer wg.Done()
				orchestrationService.RunPlannerLoop(ctx)
			}()
			go func() {
				defer wg.Done()
				orchestrationService.RunSchedulerLoop(ctx)
			}()
			go func() {
				defer wg.Done()
				orchestrationService.RunRecoveryLoop(ctx)
			}()
			go func() {
				defer wg.Done()
				orchestrationService.RunVerificationRecoveryLoop(ctx)
			}()
			if builtinVerifierEnabled {
				go func() {
					defer wg.Done()
					orchestrationService.RunVerifierLoop(ctx)
				}()
			}
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				wg.Wait()
			}()
			select {
			case <-stopCtx.Done():
				return stopCtx.Err()
			case <-done:
				return nil
			}
		},
	})
}

func orchestrationBuiltinVerifierEnabled() bool {
	value := strings.TrimSpace(os.Getenv("MEMOH_ORCHESTRATION_BUILTIN_VERIFYD"))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "", "0", "false", "no", "off":
		return false
	default:
		return false
	}
}

func wireResolverOutbound(resolver *flow.Resolver, channelManager *channel.Manager) {
	resolver.SetOutboundFn(func(ctx context.Context, botID, channelType, target, text string) error {
		return channelManager.Send(ctx, botID, channel.ChannelType(channelType), channel.SendRequest{
			Target:  target,
			Message: channel.Message{Text: text},
		})
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

func startContainerReconciliation(lc fx.Lifecycle, manager *workspace.Manager, _ *handlers.ContainerdHandler, _ *mcp.ToolGatewayService) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go manager.ReconcileContainers(ctx)
			return nil
		},
	})
}

func startServer(lc fx.Lifecycle, logger *slog.Logger, srv *server.Server, shutdowner fx.Shutdowner, cfg config.Config, queries *dbsqlc.Queries, botService *bots.Service, _ *handlers.ContainerdHandler, manager *workspace.Manager, mcpConnService *mcp.ConnectionService, toolGateway *mcp.ToolGatewayService, channelManager *channel.Manager, modelsService *models.Service) {
	fmt.Printf("Starting Memoh Agent %s\n", version.GetInfo())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := ensureAdminUser(ctx, logger, queries, cfg); err != nil {
				return err
			}
			botService.SetContainerLifecycle(manager)
			botService.SetContainerReachability(func(ctx context.Context, botID string) error {
				_, err := manager.MCPClient(ctx, botID)
				return err
			})
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(
				mcpchecker.NewChecker(logger, mcpConnService, toolGateway),
			))
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(
				channelchecker.NewChecker(logger, channelManager),
			))
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(
				modelchecker.NewChecker(logger, modelchecker.NewQueriesLookup(queries), modelsService),
			))

			go func() {
				if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error("server failed", slog.Any("error", err))
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := srv.Stop(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("server stop: %w", err)
			}
			return nil
		},
	})
}

func ensureAdminUser(ctx context.Context, log *slog.Logger, queries *dbsqlc.Queries, cfg config.Config) error {
	if queries == nil {
		return errors.New("db queries not configured")
	}
	count, err := queries.CountAccounts(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	username := strings.TrimSpace(cfg.Admin.Username)
	password := strings.TrimSpace(cfg.Admin.Password)
	email := strings.TrimSpace(cfg.Admin.Email)
	if username == "" || password == "" {
		return errors.New("admin username/password required in config.toml")
	}
	if password == "change-your-password-here" {
		log.Warn("admin password uses default placeholder; please update config.toml")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user, err := queries.CreateUser(ctx, dbsqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	emailValue := pgtype.Text{Valid: false}
	if email != "" {
		emailValue = pgtype.Text{String: email, Valid: true}
	}
	displayName := pgtype.Text{String: username, Valid: true}
	dataRoot := pgtype.Text{String: cfg.Workspace.DataRoot, Valid: cfg.Workspace.DataRoot != ""}

	_, err = queries.CreateAccount(ctx, dbsqlc.CreateAccountParams{
		UserID:       user.ID,
		Username:     pgtype.Text{String: username, Valid: true},
		Email:        emailValue,
		PasswordHash: pgtype.Text{String: string(hashed), Valid: true},
		Role:         "admin",
		DisplayName:  displayName,
		AvatarUrl:    pgtype.Text{Valid: false},
		IsActive:     true,
		DataRoot:     dataRoot,
	})
	if err != nil {
		return err
	}
	log.Info("Admin user created", slog.String("username", username))
	return nil
}

type lazyLLMClient struct {
	modelsService   *models.Service
	settingsService *settings.Service
	queries         *dbsqlc.Queries
	timeout         time.Duration
	logger          *slog.Logger
}

func (c *lazyLLMClient) Extract(ctx context.Context, req memprovider.ExtractRequest) (memprovider.ExtractResponse, error) {
	client, err := c.resolve(ctx, req.BotID)
	if err != nil {
		return memprovider.ExtractResponse{}, err
	}
	return client.Extract(ctx, req)
}

func (c *lazyLLMClient) Decide(ctx context.Context, req memprovider.DecideRequest) (memprovider.DecideResponse, error) {
	client, err := c.resolve(ctx, req.BotID)
	if err != nil {
		return memprovider.DecideResponse{}, err
	}
	return client.Decide(ctx, req)
}

func (c *lazyLLMClient) Compact(ctx context.Context, req memprovider.CompactRequest) (memprovider.CompactResponse, error) {
	client, err := c.resolve(ctx, "")
	if err != nil {
		return memprovider.CompactResponse{}, err
	}
	return client.Compact(ctx, req)
}

func (c *lazyLLMClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	client, err := c.resolve(ctx, "")
	if err != nil {
		return "", err
	}
	return client.DetectLanguage(ctx, text)
}

func (c *lazyLLMClient) resolve(ctx context.Context, botID string) (memprovider.LLM, error) {
	if c.modelsService == nil || c.queries == nil {
		return nil, errors.New("models service not configured")
	}

	chatModelID := ""
	if c.settingsService != nil && strings.TrimSpace(botID) != "" {
		if botSettings, err := c.settingsService.GetBot(ctx, botID); err == nil {
			if id := strings.TrimSpace(botSettings.CompactionModelID); id != "" {
				chatModelID = id
			} else if id := strings.TrimSpace(botSettings.ChatModelID); id != "" {
				chatModelID = id
			}
		}
	}

	memoryModel, memoryProvider, err := models.SelectMemoryModelForBot(ctx, c.modelsService, c.queries, chatModelID)
	if err != nil {
		return nil, err
	}
	return memllm.New(memllm.Config{
		ModelID:    memoryModel.ModelID,
		BaseURL:    strings.TrimRight(providers.ProviderConfigString(memoryProvider, "base_url"), "/"),
		APIKey:     providers.ProviderConfigString(memoryProvider, "api_key"),
		ClientType: memoryProvider.ClientType,
		Timeout:    c.timeout,
	}), nil
}

type skillLoaderAdapter struct {
	handler *handlers.ContainerdHandler
}

func (a *skillLoaderAdapter) LoadSkills(ctx context.Context, botID string) ([]flow.SkillEntry, error) {
	items, err := a.handler.LoadSkills(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries := make([]flow.SkillEntry, len(items))
	for i, item := range items {
		skillPath := ""
		if item.SourcePath != "" {
			skillPath = stdpath.Dir(item.SourcePath)
		}
		entries[i] = flow.SkillEntry{
			Name:        item.Name,
			Description: item.Description,
			Content:     item.Content,
			Path:        skillPath,
			Metadata:    item.Metadata,
		}
	}
	return entries, nil
}

type mediaAssetResolverAdapter struct {
	media *media.Service
}

func (a *mediaAssetResolverAdapter) Stat(ctx context.Context, botID, contentHash string) (media.Asset, error) {
	if a == nil || a.media == nil {
		return media.Asset{}, errors.New("media service not configured")
	}
	return a.media.Stat(ctx, botID, contentHash)
}

func (a *mediaAssetResolverAdapter) Open(ctx context.Context, botID, contentHash string) (io.ReadCloser, media.Asset, error) {
	if a == nil || a.media == nil {
		return nil, media.Asset{}, errors.New("media service not configured")
	}
	return a.media.Open(ctx, botID, contentHash)
}

func (a *mediaAssetResolverAdapter) Ingest(ctx context.Context, input media.IngestInput) (media.Asset, error) {
	if a == nil || a.media == nil {
		return media.Asset{}, errors.New("media service not configured")
	}
	return a.media.Ingest(ctx, input)
}

func (a *mediaAssetResolverAdapter) GetByStorageKey(ctx context.Context, botID, storageKey string) (messaging.AssetMeta, error) {
	if a == nil || a.media == nil {
		return messaging.AssetMeta{}, errors.New("media service not configured")
	}
	return a.media.GetByStorageKey(ctx, botID, storageKey)
}

func (a *mediaAssetResolverAdapter) AccessPath(asset media.Asset) string {
	if a == nil || a.media == nil {
		return ""
	}
	return a.media.AccessPath(asset)
}

func (a *mediaAssetResolverAdapter) IngestContainerFile(ctx context.Context, botID, containerPath string) (messaging.AssetMeta, error) {
	if a == nil || a.media == nil {
		return messaging.AssetMeta{}, errors.New("media service not configured")
	}
	return a.media.IngestContainerFile(ctx, botID, containerPath)
}

type gatewayAssetLoaderAdapter struct {
	media *media.Service
}

func (a *gatewayAssetLoaderAdapter) OpenForGateway(ctx context.Context, botID, contentHash string) (io.ReadCloser, string, error) {
	if a == nil || a.media == nil {
		return nil, "", errors.New("media service not configured")
	}
	reader, asset, err := a.media.Open(ctx, botID, contentHash)
	if err != nil {
		return nil, "", err
	}
	return reader, strings.TrimSpace(asset.Mime), nil
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

type commandContainerFSAdapter struct {
	manager *workspace.Manager
}

func (a *commandContainerFSAdapter) ListDir(ctx context.Context, botID, dirPath string) ([]command.FSEntry, error) {
	client, err := a.manager.MCPClient(ctx, botID)
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
	client, err := a.manager.MCPClient(ctx, botID)
	if err != nil {
		return "", err
	}
	resp, err := client.ReadFile(ctx, filePath, 0, 0)
	if err != nil {
		return "", err
	}
	return resp.GetContent(), nil
}
