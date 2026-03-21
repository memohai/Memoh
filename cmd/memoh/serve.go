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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acl"
	agentpkg "github.com/memohai/memoh/internal/agent"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/discord"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/adapters/qq"
	"github.com/memohai/memoh/internal/channel/adapters/telegram"
	"github.com/memohai/memoh/internal/channel/adapters/wecom"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/inbound"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/command"
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
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/messaging"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/server"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/storage/providers/containerfs"
	"github.com/memohai/memoh/internal/subagent"
	ttspkg "github.com/memohai/memoh/internal/tts"
	ttsedge "github.com/memohai/memoh/internal/tts/adapter/edge"
	"github.com/memohai/memoh/internal/version"
	"github.com/memohai/memoh/internal/workspace"
)

func runServe() {
	fx.New(
		fx.Provide(
			provideConfig,
			boot.ProvideRuntimeConfig,
			provideLogger,
			provideContainerService,
			provideDBConn,
			provideDBQueries,
			provideWorkspaceManager,
			provideMemoryLLM,
			memprovider.NewService,
			provideMemoryProviderRegistry,
			models.NewService,
			bots.NewService,
			accounts.NewService,
			acl.NewService,
			settings.NewService,
			providers.NewService,
			searchproviders.NewService,
			policy.NewService,
			mcp.NewConnectionService,
			subagent.NewService,
			conversation.NewService,
			identities.NewService,
			bind.NewService,
			event.NewHub,
			provideTtsRegistry,
			ttspkg.NewService,
			provideTtsTempStore,
			provideEmailRegistry,
			emailpkg.NewService,
			emailpkg.NewOutboxService,
			provideEmailChatGateway,
			provideEmailTrigger,
			emailpkg.NewManager,
			provideRouteService,
			provideSessionService,
			provideMessageService,
			provideMediaService,
			local.NewRouteHub,
			provideChannelRegistry,
			channel.NewStore,
			provideChannelRouter,
			provideChannelManager,
			provideChannelLifecycleService,
			provideAgent,
			provideChatResolver,
			browsercontexts.NewService,
			provideScheduleTriggerer,
			provideHeartbeatSessionCreator,
			provideScheduleSessionCreator,
			schedule.NewService,
			provideHeartbeatTriggerer,
			heartbeat.NewService,
			provideContainerdHandler,
			provideFederationGateway,
			provideToolGatewayService,
			provideToolProviders,
			provideServerHandler(handlers.NewPingHandler),
			provideServerHandler(provideMemohAuthHandler),
			provideServerHandler(provideMemoryHandler),
			provideServerHandler(provideMessageHandler),
			provideServerHandler(provideSessionHandler),
			provideServerHandler(handlers.NewSwaggerHandler),
			provideServerHandler(handlers.NewProvidersHandler),
			provideServerHandler(handlers.NewSearchProvidersHandler),
			provideServerHandler(handlers.NewModelsHandler),
			provideServerHandler(handlers.NewSettingsHandler),
			provideServerHandler(handlers.NewACLHandler),
			provideServerHandler(handlers.NewBindHandler),
			provideServerHandler(handlers.NewScheduleHandler),
			provideServerHandler(handlers.NewHeartbeatHandler),
			provideServerHandler(handlers.NewSubagentHandler),
			provideServerHandler(handlers.NewChannelHandler),
			provideServerHandler(feishu.NewWebhookServerHandler),
			provideServerHandler(provideUsersHandler),
			provideServerHandler(handlers.NewMemoryProvidersHandler),
			provideServerHandler(handlers.NewTtsProvidersHandler),
			provideServerHandler(handlers.NewBotTtsHandler),
			provideServerHandler(handlers.NewEmailProvidersHandler),
			provideServerHandler(handlers.NewEmailBindingsHandler),
			provideServerHandler(handlers.NewEmailOutboxHandler),
			provideServerHandler(handlers.NewEmailWebhookHandler),
			provideServerHandler(provideEmailOAuthHandler),
			emailpkg.NewDBOAuthTokenStore,
			provideServerHandler(handlers.NewMCPHandler),
			provideServerHandler(handlers.NewMCPOAuthHandler),
			provideOAuthService,
			provideServerHandler(handlers.NewTokenUsageHandler),
			provideServerHandler(handlers.NewBrowserContextsHandler),
			provideServerHandler(provideCLIHandler),
			provideServerHandler(provideWebHandler),
			provideServerHandler(handlers.NewEmbeddedWebHandler),
			provideServer,
		),
		fx.Invoke(
			injectToolProviders,
			startMemoryProviderBootstrap,
			startScheduleService,
			startHeartbeatService,
			startChannelManager,
			startEmailManager,
			startContainerReconciliation,
			startTtsTempStoreCleanup,
			startServer,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger.With(slog.String("component", "fx"))}
		}),
	).Run()
}

func provideServerHandler(fn any) any {
	return fx.Annotate(
		fn,
		fx.As(new(server.Handler)),
		fx.ResultTags(`group:"server_handlers"`),
	)
}

func provideConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
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
	lc.Append(fx.Hook{OnStop: func(_ context.Context) error { cleanup(); return nil }})
	return svc, nil
}

func provideDBConn(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	conn, err := db.Open(context.Background(), cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	lc.Append(fx.Hook{OnStop: func(_ context.Context) error { conn.Close(); return nil }})
	return conn, nil
}

func provideDBQueries(conn *pgxpool.Pool) *dbsqlc.Queries { return dbsqlc.New(conn) }
func provideWorkspaceManager(log *slog.Logger, service ctr.Service, cfg config.Config, conn *pgxpool.Pool) *workspace.Manager {
	return workspace.NewManager(log, service, cfg.Workspace, cfg.Containerd.Namespace, conn)
}

func provideMemoryLLM(modelsService *models.Service, queries *dbsqlc.Queries, log *slog.Logger) memprovider.LLM {
	return &lazyLLMClient{modelsService: modelsService, queries: queries, timeout: 30 * time.Second, logger: log}
}

func provideMemoryProviderRegistry(log *slog.Logger, chatService *conversation.Service, accountService *accounts.Service, manager *workspace.Manager, queries *dbsqlc.Queries, cfg config.Config) *memprovider.Registry {
	registry := memprovider.NewRegistry(log)
	builtinRuntime := handlers.NewBuiltinMemoryRuntime(manager)
	fileStore := storefs.New(log, manager)
	registry.RegisterFactory(string(memprovider.ProviderBuiltin), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		runtime, err := membuiltin.NewBuiltinRuntimeFromConfig(log, providerConfig, builtinRuntime, fileStore, queries, cfg)
		if err != nil {
			return nil, err
		}
		return membuiltin.NewBuiltinProvider(log, runtime, chatService, accountService), nil
	})
	registry.RegisterFactory(string(memprovider.ProviderMem0), func(_ string, config map[string]any) (memprovider.Provider, error) {
		return memmem0.NewMem0Provider(log, config, fileStore)
	})
	registry.RegisterFactory(string(memprovider.ProviderOpenViking), func(_ string, config map[string]any) (memprovider.Provider, error) {
		return memopenviking.NewOpenVikingProvider(log, config)
	})
	registry.Register("__builtin_default__", membuiltin.NewBuiltinProvider(log, builtinRuntime, chatService, accountService))
	return registry
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

func injectToolProviders(a *agentpkg.Agent, providers []agenttools.ToolProvider) {
	a.SetToolProviders(providers)
}

func provideChatResolver(log *slog.Logger, a *agentpkg.Agent, modelsService *models.Service, queries *dbsqlc.Queries, chatService *conversation.Service, msgService *message.DBService, settingsService *settings.Service, mediaService *media.Service, containerdHandler *handlers.ContainerdHandler, memoryRegistry *memprovider.Registry, sessionService *sessionpkg.Service, eventHub *event.Hub) *flow.Resolver {
	resolver := flow.NewResolver(log, modelsService, queries, chatService, msgService, settingsService, a, 120*time.Second)
	resolver.SetMemoryRegistry(memoryRegistry)
	resolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	resolver.SetGatewayAssetLoader(&gatewayAssetLoaderAdapter{media: mediaService})
	resolver.SetSessionService(sessionService)
	resolver.SetEventPublisher(eventHub)
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
	feishuAdapter := feishu.NewFeishuAdapter(log)
	feishuAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(feishuAdapter)
	registry.MustRegister(wecom.NewWeComAdapter(log))
	registry.MustRegister(local.NewCLIAdapter(hub))
	registry.MustRegister(local.NewWebAdapter(hub))
	return registry
}

func provideChannelRouter(log *slog.Logger, registry *channel.Registry, hub *local.RouteHub, routeService *route.DBService, sessionService *sessionpkg.Service, msgService *message.DBService, resolver *flow.Resolver, identityService *identities.Service, botService *bots.Service, aclService *acl.Service, policyService *policy.Service, bindService *bind.Service, mediaService *media.Service, ttsService *ttspkg.Service, settingsService *settings.Service, subagentService *subagent.Service, scheduleService *schedule.Service, mcpConnService *mcp.ConnectionService, modelsService *models.Service, providersService *providers.Service, memProvService *memprovider.Service, searchProvService *searchproviders.Service, browserCtxService *browsercontexts.Service, emailService *emailpkg.Service, emailOutboxService *emailpkg.OutboxService, heartbeatService *heartbeat.Service, queries *dbsqlc.Queries, containerdHandler *handlers.ContainerdHandler, manager *workspace.Manager, rc *boot.RuntimeConfig) *inbound.ChannelInboundProcessor {
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
	processor.SetACLService(aclService)
	processor.SetMediaService(mediaService)
	processor.SetStreamObserver(local.NewRouteHubBroadcaster(hub))
	processor.SetTtsService(ttsService, &settingsTtsModelResolver{settings: settingsService})
	processor.SetCommandHandler(command.NewHandler(
		log,
		&command.BotMemberRoleAdapter{BotService: botService},
		subagentService,
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
		&commandSkillLoaderAdapter{handler: containerdHandler},
		&commandContainerFSAdapter{manager: manager},
	))
	return processor
}

func provideChannelManager(log *slog.Logger, registry *channel.Registry, channelStore *channel.Store, channelRouter *inbound.ChannelInboundProcessor) *channel.Manager {
	mgr := channel.NewManager(log, registry, channelStore, channelRouter)
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
	callbackURL := "http://" + host + "/oauth/mcp/callback"
	return mcp.NewOAuthService(log, queries, callbackURL)
}

func provideToolGatewayService(log *slog.Logger, fedGateway *handlers.MCPFederationGateway, oauthService *mcp.OAuthService, mcpConnService *mcp.ConnectionService, containerdHandler *handlers.ContainerdHandler) *mcp.ToolGatewayService {
	fedGateway.SetOAuthService(oauthService)
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService)
	svc := mcp.NewToolGatewayService(log, []mcp.ToolSource{fedSource})
	containerdHandler.SetToolGatewayService(svc)
	return svc
}

func provideToolProviders(log *slog.Logger, cfg config.Config, channelManager *channel.Manager, registry *channel.Registry, routeService *route.DBService, scheduleService *schedule.Service, settingsService *settings.Service, searchProviderService *searchproviders.Service, manager *workspace.Manager, mediaService *media.Service, memoryRegistry *memprovider.Registry, emailService *emailpkg.Service, emailManager *emailpkg.Manager, fedGateway *handlers.MCPFederationGateway, mcpConnService *mcp.ConnectionService, subagentService *subagent.Service, modelsService *models.Service, browserContextService *browsercontexts.Service, queries *dbsqlc.Queries, ttsService *ttspkg.Service, sessionService *sessionpkg.Service) []agenttools.ToolProvider {
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
		agenttools.NewContainerProvider(log, manager, config.DefaultDataMount),
		agenttools.NewReadMediaProvider(log, manager, config.DefaultDataMount),
		agenttools.NewEmailProvider(log, emailService, emailManager),
		agenttools.NewWebFetchProvider(log),
		agenttools.NewSubagentProvider(log, subagentService, settingsService, modelsService, queries, ""),
		agenttools.NewSkillProvider(log),
		agenttools.NewBrowserProvider(log, settingsService, browserContextService, manager, cfg.BrowserGateway),
		agenttools.NewTTSProvider(log, settingsService, ttsService, channelManager, registry),
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

func provideMemohAuthHandler(log *slog.Logger, accountService *accounts.Service, rc *boot.RuntimeConfig) *memohAuthHandler {
	return &memohAuthHandler{inner: handlers.NewAuthHandler(log, accountService, rc.JwtSecret, rc.JwtExpiresIn)}
}

func provideMessageHandler(log *slog.Logger, chatService *conversation.Service, msgService *message.DBService, mediaService *media.Service, botService *bots.Service, accountService *accounts.Service, hub *event.Hub) *handlers.MessageHandler {
	h := handlers.NewMessageHandler(log, chatService, msgService, botService, accountService, hub)
	h.SetMediaService(mediaService)
	return h
}

func provideSessionHandler(log *slog.Logger, sessionService *sessionpkg.Service, botService *bots.Service, accountService *accounts.Service) *handlers.SessionHandler {
	return handlers.NewSessionHandler(log, sessionService, botService, accountService)
}

type memohAuthHandler struct{ inner *handlers.AuthHandler }

func (h *memohAuthHandler) Register(e *echo.Echo) {
	e.POST("/api/auth/login", h.inner.Login)
	e.POST("/api/auth/refresh", h.inner.Refresh)
}

func provideMediaService(log *slog.Logger, manager *workspace.Manager) *media.Service {
	provider := containerfs.New(manager)
	return media.NewService(log, provider)
}

func provideUsersHandler(log *slog.Logger, accountService *accounts.Service, identityService *identities.Service, botService *bots.Service, routeService *route.DBService, channelStore *channel.Store, channelLifecycle *channel.Lifecycle, channelManager *channel.Manager, registry *channel.Registry) *handlers.UsersHandler {
	return handlers.NewUsersHandler(log, accountService, identityService, botService, routeService, channelStore, channelLifecycle, channelManager, registry)
}

func provideCLIHandler(channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, resolver *flow.Resolver, mediaService *media.Service, ttsService *ttspkg.Service, settingsService *settings.Service) *handlers.LocalChannelHandler {
	h := handlers.NewLocalChannelHandler(local.CLIType, channelManager, channelStore, chatService, hub, botService, accountService)
	h.SetResolver(resolver)
	h.SetMediaService(mediaService)
	h.SetTtsService(ttsService, &settingsTtsModelResolver{settings: settingsService})
	return h
}

func provideWebHandler(channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, resolver *flow.Resolver, mediaService *media.Service, ttsService *ttspkg.Service, settingsService *settings.Service) *handlers.LocalChannelHandler {
	h := handlers.NewLocalChannelHandler(local.WebType, channelManager, channelStore, chatService, hub, botService, accountService)
	h.SetResolver(resolver)
	h.SetMediaService(mediaService)
	h.SetTtsService(ttsService, &settingsTtsModelResolver{settings: settingsService})
	return h
}

type serverParams struct {
	fx.In
	Logger            *slog.Logger
	RuntimeConfig     *boot.RuntimeConfig
	Config            config.Config
	ServerHandlers    []server.Handler `group:"server_handlers"`
	ContainerdHandler *handlers.ContainerdHandler
}

type memohServer struct {
	echo *echo.Echo
	addr string
}

var (
	memohJWTExactSkipPaths = map[string]struct{}{
		"/":                       {},
		"/ping":                   {},
		"/health":                 {},
		"/api/swagger.json":       {},
		"/api/auth/login":         {},
		"/logo.png":               {},
		"/channels/telegram.webp": {},
		"/channels/feishu.png":    {},
	}
	memohJWTPrefixSkipPaths = []string{
		"/assets/",
		"/api/docs",
		"/channels/feishu/webhook/",
		"/email/mailgun/webhook/",
		"/email/oauth/callback",
	}
	memohSPABackendPrefixes = []string{
		"/api",
		"/auth",
		"/channels",
		"/containers",
		"/users",
		"/bots",
		"/models",
		"/providers",
		"/search_providers",
		"/email-providers",
		"/email",
		"/settings",
		"/memory",
		"/message",
		"/mcp",
		"/schedule",
		"/bind",
		"/preauth",
		"/subagents",
		"/ping",
		"/health",
	}
	memohAPIRewriteBypassExact = map[string]struct{}{
		"/api/swagger.json": {},
	}
	memohAPIRewriteBypassPrefixes = []string{
		"/api/docs",
		"/api/auth/",
	}
)

func (s *memohServer) Start() error                   { return s.echo.Start(s.addr) }
func (s *memohServer) Stop(ctx context.Context) error { return s.echo.Shutdown(ctx) }

func provideServer(params serverParams) *memohServer {
	allHandlers := make([]server.Handler, 0, len(params.ServerHandlers)+1)
	allHandlers = append(allHandlers, params.ServerHandlers...)
	allHandlers = append(allHandlers, params.ContainerdHandler)

	addr := params.RuntimeConfig.ServerAddr
	if addr == "" {
		addr = ":8080"
	}
	e := echo.New()
	e.HideBanner = true
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rewriteAPIPathForMemoh(c.Request())
			return next(c)
		}
	})
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true,
		LogURI:    true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			params.Logger.Info("request",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
				slog.String("remote_ip", c.RealIP()),
			)
			return nil
		},
	}))
	e.Use(auth.JWTMiddleware(params.Config.Auth.JWTSecret, func(c echo.Context) bool {
		return shouldSkipJWTForMemoh(c.Request().URL.Path)
	}))
	for _, h := range allHandlers {
		if h != nil {
			h.Register(e)
		}
	}
	return &memohServer{echo: e, addr: addr}
}

func startScheduleService(lc fx.Lifecycle, scheduleService *schedule.Service) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error { return scheduleService.Bootstrap(ctx) }})
}

func startHeartbeatService(lc fx.Lifecycle, heartbeatService *heartbeat.Service) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error { return heartbeatService.Bootstrap(ctx) }})
}

func startChannelManager(lc fx.Lifecycle, channelManager *channel.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error { channelManager.Start(ctx); return nil },
		OnStop:  func(stopCtx context.Context) error { cancel(); return channelManager.Shutdown(stopCtx) },
	})
}

func startContainerReconciliation(lc fx.Lifecycle, manager *workspace.Manager, _ *handlers.ContainerdHandler, _ *mcp.ToolGatewayService) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error { go manager.ReconcileContainers(ctx); return nil }})
}

func startServer(lc fx.Lifecycle, logger *slog.Logger, srv *memohServer, shutdowner fx.Shutdowner, cfg config.Config, queries *dbsqlc.Queries, botService *bots.Service, _ *handlers.ContainerdHandler, manager *workspace.Manager, mcpConnService *mcp.ConnectionService, toolGateway *mcp.ToolGatewayService, channelManager *channel.Manager, modelsService *models.Service) {
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
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(mcpchecker.NewChecker(logger, mcpConnService, toolGateway)))
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(channelchecker.NewChecker(logger, channelManager)))
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(modelchecker.NewChecker(logger, modelchecker.NewQueriesLookup(queries), modelsService)))
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

func shouldSkipJWTForMemoh(path string) bool {
	if _, ok := memohJWTExactSkipPaths[path]; ok {
		return true
	}
	if hasAnyPrefix(path, memohJWTPrefixSkipPaths) {
		return true
	}
	// Treat non-backend, extension-less paths as SPA routes (e.g. /chat, /settings/profile).
	return shouldServeSPARouteForMemoh(path)
}

func shouldServeSPARouteForMemoh(path string) bool {
	if path == "" || path == "/" {
		return true
	}
	if strings.Contains(path, ".") {
		return false
	}
	if hasAnyPrefix(path, memohSPABackendPrefixes) {
		return false
	}
	return true
}

func rewriteAPIPathForMemoh(r *http.Request) {
	if r == nil || r.URL == nil {
		return
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/") {
		return
	}
	if _, ok := memohAPIRewriteBypassExact[path]; ok {
		return
	}
	if hasAnyPrefix(path, memohAPIRewriteBypassPrefixes) {
		return
	}
	rewritten := strings.TrimPrefix(path, "/api")
	if rewritten == "" {
		rewritten = "/"
	}
	r.URL.Path = rewritten
}

func hasAnyPrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func provideTtsRegistry(log *slog.Logger) *ttspkg.Registry {
	reg := ttspkg.NewRegistry()
	reg.Register(ttsedge.NewEdgeAdapter(log))
	return reg
}

func provideTtsTempStore() (*ttspkg.TempStore, error) {
	return ttspkg.NewTempStore(os.TempDir())
}

func startTtsTempStoreCleanup(lc fx.Lifecycle, store *ttspkg.TempStore) {
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

// settingsTtsModelResolver adapts settings.Service to the ttsModelResolver interface
// expected by ChannelInboundProcessor and LocalChannelHandler.
type sessionEnsurerAdapter struct {
	svc *sessionpkg.Service
}

func (a *sessionEnsurerAdapter) EnsureActiveSession(ctx context.Context, botID, routeID, channelType string) (inbound.SessionResult, error) {
	sess, err := a.svc.EnsureActiveSession(ctx, botID, routeID, channelType)
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inbound.SessionResult{ID: sess.ID}, nil
}

func (a *sessionEnsurerAdapter) CreateNewSession(ctx context.Context, botID, routeID, channelType string) (inbound.SessionResult, error) {
	sess, err := a.svc.CreateNewSession(ctx, botID, routeID, channelType)
	if err != nil {
		return inbound.SessionResult{}, err
	}
	return inbound.SessionResult{ID: sess.ID}, nil
}

type settingsTtsModelResolver struct {
	settings *settings.Service
}

func (r *settingsTtsModelResolver) ResolveTtsModelID(ctx context.Context, botID string) (string, error) {
	s, err := r.settings.GetBot(ctx, botID)
	if err != nil {
		return "", err
	}
	return s.TtsModelID, nil
}

func provideEmailRegistry(log *slog.Logger, tokenStore *emailpkg.DBOAuthTokenStore) *emailpkg.Registry {
	reg := emailpkg.NewRegistry()
	reg.Register(emailgeneric.New(log))
	reg.Register(emailmailgun.New(log))
	reg.Register(emailgmail.New(log, tokenStore))
	return reg
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
	callbackURL := "http://" + host + "/email/oauth/callback"
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
		OnStop: func(stopCtx context.Context) error { cancel(); emailManager.Stop(stopCtx); return nil },
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
	user, err := queries.CreateUser(ctx, dbsqlc.CreateUserParams{IsActive: true, Metadata: []byte("{}")})
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
		UserID: user.ID, Username: pgtype.Text{String: username, Valid: true}, Email: emailValue,
		PasswordHash: pgtype.Text{String: string(hashed), Valid: true}, Role: "admin",
		DisplayName: displayName, AvatarUrl: pgtype.Text{Valid: false}, IsActive: true, DataRoot: dataRoot,
	})
	if err != nil {
		return err
	}
	log.Info("Admin user created", slog.String("username", username))
	return nil
}

type lazyLLMClient struct {
	modelsService *models.Service
	queries       *dbsqlc.Queries
	timeout       time.Duration
	logger        *slog.Logger
}

func (c *lazyLLMClient) Extract(ctx context.Context, req memprovider.ExtractRequest) (memprovider.ExtractResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memprovider.ExtractResponse{}, err
	}
	return client.Extract(ctx, req)
}

func (c *lazyLLMClient) Decide(ctx context.Context, req memprovider.DecideRequest) (memprovider.DecideResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memprovider.DecideResponse{}, err
	}
	return client.Decide(ctx, req)
}

func (c *lazyLLMClient) Compact(ctx context.Context, req memprovider.CompactRequest) (memprovider.CompactResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memprovider.CompactResponse{}, err
	}
	return client.Compact(ctx, req)
}

func (c *lazyLLMClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return "", err
	}
	return client.DetectLanguage(ctx, text)
}

func (c *lazyLLMClient) resolve(ctx context.Context) (memprovider.LLM, error) {
	if c.modelsService == nil || c.queries == nil {
		return nil, errors.New("models service not configured")
	}
	botID := ""
	memoryModel, memoryProvider, err := models.SelectMemoryModelForBot(ctx, c.modelsService, c.queries, botID)
	if err != nil {
		return nil, err
	}
	clientType := string(memoryModel.ClientType)
	switch clientType {
	case "openai-responses", "openai-completions", "anthropic-messages", "google-generative-ai":
	default:
		return nil, fmt.Errorf("memory model client type not supported: %s", clientType)
	}
	_ = memoryProvider
	_ = memoryModel
	return nil, errors.New("memory llm runtime is not available")
}

type skillLoaderAdapter struct{ handler *handlers.ContainerdHandler }

func (a *skillLoaderAdapter) LoadSkills(ctx context.Context, botID string) ([]flow.SkillEntry, error) {
	items, err := a.handler.LoadSkills(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries := make([]flow.SkillEntry, len(items))
	for i, item := range items {
		entries[i] = flow.SkillEntry{Name: item.Name, Description: item.Description, Content: item.Content, Metadata: item.Metadata}
	}
	return entries, nil
}

type mediaAssetResolverAdapter struct{ media *media.Service }

func (a *mediaAssetResolverAdapter) GetByStorageKey(ctx context.Context, botID, storageKey string) (messaging.AssetMeta, error) {
	if a == nil || a.media == nil {
		return messaging.AssetMeta{}, errors.New("media service not configured")
	}
	asset, err := a.media.GetByStorageKey(ctx, botID, storageKey)
	if err != nil {
		return messaging.AssetMeta{}, err
	}
	return messaging.AssetMeta{ContentHash: asset.ContentHash, Mime: asset.Mime, SizeBytes: asset.SizeBytes, StorageKey: asset.StorageKey}, nil
}

func (a *mediaAssetResolverAdapter) IngestContainerFile(ctx context.Context, botID, containerPath string) (messaging.AssetMeta, error) {
	if a == nil || a.media == nil {
		return messaging.AssetMeta{}, errors.New("media service not configured")
	}
	asset, err := a.media.IngestContainerFile(ctx, botID, containerPath)
	if err != nil {
		return messaging.AssetMeta{}, err
	}
	return messaging.AssetMeta{ContentHash: asset.ContentHash, Mime: asset.Mime, SizeBytes: asset.SizeBytes, StorageKey: asset.StorageKey}, nil
}

type gatewayAssetLoaderAdapter struct{ media *media.Service }

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

// commandSkillLoaderAdapter bridges handlers.ContainerdHandler to command.SkillLoader.
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

// commandContainerFSAdapter bridges workspace.Manager to command.ContainerFS.
type commandContainerFSAdapter struct {
	manager *workspace.Manager
}

func (a *commandContainerFSAdapter) ListDir(ctx context.Context, botID, dirPath string) ([]command.FSEntry, error) {
	client, err := a.manager.MCPClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries, err := client.ListDir(ctx, dirPath, false)
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
