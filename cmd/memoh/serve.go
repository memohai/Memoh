package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	agentruntime "github.com/memohai/memoh/internal/bun/runtime"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/discord"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/adapters/telegram"
	"github.com/memohai/memoh/internal/channel/adapters/wecom"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/inbound"
	"github.com/memohai/memoh/internal/channel/route"
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
	"github.com/memohai/memoh/internal/inbox"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	mcpcontacts "github.com/memohai/memoh/internal/mcp/providers/contacts"
	mcpcontainer "github.com/memohai/memoh/internal/mcp/providers/container"
	mcpemail "github.com/memohai/memoh/internal/mcp/providers/email"
	mcpinbox "github.com/memohai/memoh/internal/mcp/providers/inbox"
	mcpmemory "github.com/memohai/memoh/internal/mcp/providers/memory"
	mcpmessage "github.com/memohai/memoh/internal/mcp/providers/message"
	mcpschedule "github.com/memohai/memoh/internal/mcp/providers/schedule"
	mcpweb "github.com/memohai/memoh/internal/mcp/providers/web"
	mcpfederation "github.com/memohai/memoh/internal/mcp/sources/federation"
	"github.com/memohai/memoh/internal/media"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	membuiltin "github.com/memohai/memoh/internal/memory/adapters/builtin"
	memmem0 "github.com/memohai/memoh/internal/memory/adapters/mem0"
	memopenviking "github.com/memohai/memoh/internal/memory/adapters/openviking"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/preauth"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/server"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/storage/providers/containerfs"
	"github.com/memohai/memoh/internal/subagent"
	"github.com/memohai/memoh/internal/version"
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
			provideMCPManager,
			provideAgentRuntimeManager,
			provideMemoryLLM,
			memprovider.NewService,
			provideMemoryProviderRegistry,
			models.NewService,
			bots.NewService,
			accounts.NewService,
			settings.NewService,
			providers.NewService,
			searchproviders.NewService,
			policy.NewService,
			preauth.NewService,
			mcp.NewConnectionService,
			subagent.NewService,
			conversation.NewService,
			identities.NewService,
			bind.NewService,
			event.NewHub,
			inbox.NewService,
			provideEmailRegistry,
			emailpkg.NewService,
			emailpkg.NewOutboxService,
			provideEmailChatGateway,
			provideEmailTrigger,
			emailpkg.NewManager,
			provideRouteService,
			provideMessageService,
			provideMediaService,
			local.NewRouteHub,
			provideChannelRegistry,
			channel.NewStore,
			provideChannelRouter,
			provideChannelManager,
			provideChannelLifecycleService,
			provideChatResolver,
			provideScheduleTriggerer,
			schedule.NewService,
			provideContainerdHandler,
			provideFederationGateway,
			provideToolGatewayService,
			provideServerHandler(handlers.NewPingHandler),
			provideServerHandler(provideMemohAuthHandler),
			provideServerHandler(provideMemoryHandler),
			provideServerHandler(provideMessageHandler),
			provideServerHandler(handlers.NewSwaggerHandler),
			provideServerHandler(handlers.NewProvidersHandler),
			provideServerHandler(handlers.NewSearchProvidersHandler),
			provideServerHandler(handlers.NewModelsHandler),
			provideServerHandler(handlers.NewSettingsHandler),
			provideServerHandler(handlers.NewPreauthHandler),
			provideServerHandler(handlers.NewBindHandler),
			provideServerHandler(handlers.NewScheduleHandler),
			provideServerHandler(handlers.NewSubagentHandler),
			provideServerHandler(handlers.NewChannelHandler),
			provideServerHandler(feishu.NewWebhookServerHandler),
			provideServerHandler(provideUsersHandler),
			provideServerHandler(handlers.NewMemoryProvidersHandler),
			provideServerHandler(handlers.NewEmailProvidersHandler),
			provideServerHandler(handlers.NewEmailBindingsHandler),
			provideServerHandler(handlers.NewEmailOutboxHandler),
			provideServerHandler(handlers.NewEmailWebhookHandler),
			provideServerHandler(provideEmailOAuthHandler),
			emailpkg.NewDBOAuthTokenStore,
			provideServerHandler(handlers.NewMCPHandler),
			provideServerHandler(handlers.NewMCPOAuthHandler),
			provideOAuthService,
			provideServerHandler(handlers.NewInboxHandler),
			provideServerHandler(provideCLIHandler),
			provideServerHandler(provideWebHandler),
			provideServerHandler(handlers.NewEmbeddedWebHandler),
			provideServer,
		),
		fx.Invoke(
			startMemoryProviderBootstrap,
			startScheduleService,
			startChannelManager,
			startEmailManager,
			startContainerReconciliation,
			startAgentRuntime,
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
func provideMCPManager(log *slog.Logger, service ctr.Service, cfg config.Config, conn *pgxpool.Pool) *mcp.Manager {
	return mcp.NewManager(log, service, cfg.MCP, cfg.Containerd.Namespace, conn)
}

func provideAgentRuntimeManager(log *slog.Logger, cfg config.Config) *agentruntime.Manager {
	return agentruntime.NewManager(log, cfg)
}

func provideMemoryLLM(modelsService *models.Service, queries *dbsqlc.Queries, log *slog.Logger) memprovider.LLM {
	return &lazyLLMClient{modelsService: modelsService, queries: queries, timeout: 30 * time.Second, logger: log}
}

func provideMemoryProviderRegistry(log *slog.Logger, chatService *conversation.Service, accountService *accounts.Service, manager *mcp.Manager) *memprovider.Registry {
	registry := memprovider.NewRegistry(log)
	builtinRuntime := handlers.NewBuiltinMemoryRuntime(manager)
	registry.RegisterFactory(string(memprovider.ProviderBuiltin), func(_ string, _ map[string]any) (memprovider.Provider, error) {
		return membuiltin.NewBuiltinProvider(log, builtinRuntime, chatService, accountService), nil
	})
	registry.RegisterFactory(string(memprovider.ProviderMem0), func(_ string, config map[string]any) (memprovider.Provider, error) {
		return memmem0.NewMem0Provider(log, config)
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

func provideMessageService(log *slog.Logger, queries *dbsqlc.Queries, hub *event.Hub) *message.DBService {
	return message.NewService(log, queries, hub)
}

func provideScheduleTriggerer(resolver *flow.Resolver) schedule.Triggerer {
	return flow.NewScheduleGateway(resolver)
}

func provideChatResolver(log *slog.Logger, cfg config.Config, modelsService *models.Service, queries *dbsqlc.Queries, chatService *conversation.Service, msgService *message.DBService, settingsService *settings.Service, mediaService *media.Service, containerdHandler *handlers.ContainerdHandler, inboxService *inbox.Service, memoryRegistry *memprovider.Registry) *flow.Resolver {
	resolver := flow.NewResolver(log, modelsService, queries, chatService, msgService, settingsService, cfg.AgentGateway.BaseURL(), 120*time.Second)
	resolver.SetMemoryRegistry(memoryRegistry)
	resolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	resolver.SetGatewayAssetLoader(&gatewayAssetLoaderAdapter{media: mediaService})
	resolver.SetInboxService(inboxService)
	return resolver
}

func provideChannelRegistry(log *slog.Logger, hub *local.RouteHub, mediaService *media.Service) *channel.Registry {
	registry := channel.NewRegistry()
	tgAdapter := telegram.NewTelegramAdapter(log)
	tgAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(tgAdapter)
	discordAdapter := discord.NewDiscordAdapter(log)
	registry.MustRegister(discordAdapter)
	feishuAdapter := feishu.NewFeishuAdapter(log)
	feishuAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(feishuAdapter)
	registry.MustRegister(wecom.NewWeComAdapter(log))
	registry.MustRegister(local.NewCLIAdapter(hub))
	registry.MustRegister(local.NewWebAdapter(hub))
	return registry
}

func provideChannelRouter(log *slog.Logger, registry *channel.Registry, hub *local.RouteHub, routeService *route.DBService, msgService *message.DBService, resolver *flow.Resolver, identityService *identities.Service, botService *bots.Service, policyService *policy.Service, preauthService *preauth.Service, bindService *bind.Service, mediaService *media.Service, inboxService *inbox.Service, rc *boot.RuntimeConfig) *inbound.ChannelInboundProcessor {
	processor := inbound.NewChannelInboundProcessor(log, registry, routeService, msgService, resolver, identityService, botService, policyService, preauthService, bindService, rc.JwtSecret, 5*time.Minute)
	processor.SetMediaService(mediaService)
	processor.SetStreamObserver(local.NewRouteHubBroadcaster(hub))
	processor.SetInboxService(inboxService)
	return processor
}

func provideChannelManager(log *slog.Logger, registry *channel.Registry, channelStore *channel.Store, channelRouter *inbound.ChannelInboundProcessor) *channel.Manager {
	mgr := channel.NewManager(log, registry, channelStore, channelRouter)
	if mw := channelRouter.IdentityMiddleware(); mw != nil {
		mgr.Use(mw)
	}
	return mgr
}

func provideChannelLifecycleService(channelStore *channel.Store, channelManager *channel.Manager) *channel.Lifecycle {
	return channel.NewLifecycle(channelStore, channelManager)
}

func provideContainerdHandler(log *slog.Logger, service ctr.Service, manager *mcp.Manager, cfg config.Config, rc *boot.RuntimeConfig, botService *bots.Service, accountService *accounts.Service, policyService *policy.Service, queries *dbsqlc.Queries) *handlers.ContainerdHandler {
	return handlers.NewContainerdHandler(log, service, manager, cfg.MCP, cfg.Containerd.Namespace, rc.ContainerBackend, botService, accountService, policyService, queries)
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

func provideToolGatewayService(log *slog.Logger, _ config.Config, channelManager *channel.Manager, registry *channel.Registry, routeService *route.DBService, scheduleService *schedule.Service, _ *conversation.Service, _ *accounts.Service, settingsService *settings.Service, searchProviderService *searchproviders.Service, manager *mcp.Manager, containerdHandler *handlers.ContainerdHandler, mcpConnService *mcp.ConnectionService, mediaService *media.Service, inboxService *inbox.Service, memoryRegistry *memprovider.Registry, emailService *emailpkg.Service, emailManager *emailpkg.Manager, fedGateway *handlers.MCPFederationGateway, oauthService *mcp.OAuthService) *mcp.ToolGatewayService {
	fedGateway.SetOAuthService(oauthService)
	var assetResolver mcpmessage.AssetResolver
	if mediaService != nil {
		assetResolver = &mediaAssetResolverAdapter{media: mediaService}
	}
	messageExec := mcpmessage.NewExecutor(log, channelManager, channelManager, registry, assetResolver)
	contactsExec := mcpcontacts.NewExecutor(log, routeService)
	scheduleExec := mcpschedule.NewExecutor(log, scheduleService)
	memoryExec := mcpmemory.NewExecutor(log, memoryRegistry, settingsService)
	webExec := mcpweb.NewExecutor(log, settingsService, searchProviderService)
	inboxExec := mcpinbox.NewExecutor(log, inboxService)
	fsExec := mcpcontainer.NewExecutor(log, manager, config.DefaultDataMount)
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService)
	emailExec := mcpemail.NewExecutor(log, emailService, emailManager)
	svc := mcp.NewToolGatewayService(log, []mcp.ToolExecutor{messageExec, contactsExec, scheduleExec, memoryExec, webExec, fsExec, inboxExec, emailExec}, []mcp.ToolSource{fedSource})
	containerdHandler.SetToolGatewayService(svc)
	return svc
}

func provideMemoryHandler(log *slog.Logger, botService *bots.Service, accountService *accounts.Service, _ config.Config, manager *mcp.Manager, memoryRegistry *memprovider.Registry, settingsService *settings.Service, _ *handlers.ContainerdHandler) *handlers.MemoryHandler {
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

type memohAuthHandler struct{ inner *handlers.AuthHandler }

func (h *memohAuthHandler) Register(e *echo.Echo) {
	e.POST("/api/auth/login", h.inner.Login)
	e.POST("/api/auth/refresh", h.inner.Refresh)
}

func provideMediaService(log *slog.Logger, manager *mcp.Manager) *media.Service {
	provider := containerfs.New(manager)
	return media.NewService(log, provider)
}

func provideUsersHandler(log *slog.Logger, accountService *accounts.Service, identityService *identities.Service, botService *bots.Service, routeService *route.DBService, channelStore *channel.Store, channelLifecycle *channel.Lifecycle, channelManager *channel.Manager, registry *channel.Registry) *handlers.UsersHandler {
	return handlers.NewUsersHandler(log, accountService, identityService, botService, routeService, channelStore, channelLifecycle, channelManager, registry)
}

func provideCLIHandler(channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, resolver *flow.Resolver) *handlers.LocalChannelHandler {
	h := handlers.NewLocalChannelHandler(local.CLIType, channelManager, channelStore, chatService, hub, botService, accountService)
	h.SetResolver(resolver)
	return h
}

func provideWebHandler(channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, resolver *flow.Resolver) *handlers.LocalChannelHandler {
	h := handlers.NewLocalChannelHandler(local.WebType, channelManager, channelStore, chatService, hub, botService, accountService)
	h.SetResolver(resolver)
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
		"/inbox",
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

func startChannelManager(lc fx.Lifecycle, channelManager *channel.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error { channelManager.Start(ctx); return nil },
		OnStop:  func(stopCtx context.Context) error { cancel(); return channelManager.Shutdown(stopCtx) },
	})
}

func startContainerReconciliation(lc fx.Lifecycle, containerdHandler *handlers.ContainerdHandler, _ *mcp.ToolGatewayService) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error { go containerdHandler.ReconcileContainers(ctx); return nil }})
}

func startAgentRuntime(lc fx.Lifecycle, manager *agentruntime.Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error { return manager.Start(ctx) },
		OnStop:  func(ctx context.Context) error { return manager.Stop(ctx) },
	})
}

func startServer(lc fx.Lifecycle, logger *slog.Logger, srv *memohServer, shutdowner fx.Shutdowner, cfg config.Config, queries *dbsqlc.Queries, botService *bots.Service, containerdHandler *handlers.ContainerdHandler, manager *mcp.Manager, mcpConnService *mcp.ConnectionService, toolGateway *mcp.ToolGatewayService, channelManager *channel.Manager) {
	fmt.Printf("Starting Memoh Agent %s\n", version.GetInfo())
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := ensureAdminUser(ctx, logger, queries, cfg); err != nil {
				return err
			}
			botService.SetContainerLifecycle(containerdHandler)
			botService.SetContainerReachability(func(ctx context.Context, botID string) error {
				_, err := manager.MCPClient(ctx, botID)
				return err
			})
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(mcpchecker.NewChecker(logger, mcpConnService, toolGateway)))
			botService.AddRuntimeChecker(healthcheck.NewRuntimeCheckerAdapter(channelchecker.NewChecker(logger, channelManager)))
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

func provideEmailTrigger(log *slog.Logger, service *emailpkg.Service, botInbox *inbox.Service, chatTriggerer emailpkg.ChatTriggerer) *emailpkg.Trigger {
	return emailpkg.NewTrigger(log, service, botInbox, chatTriggerer)
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
	dataRoot := pgtype.Text{String: cfg.MCP.DataRoot, Valid: cfg.MCP.DataRoot != ""}
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

func (a *mediaAssetResolverAdapter) GetByStorageKey(ctx context.Context, botID, storageKey string) (mcpmessage.AssetMeta, error) {
	if a == nil || a.media == nil {
		return mcpmessage.AssetMeta{}, errors.New("media service not configured")
	}
	asset, err := a.media.GetByStorageKey(ctx, botID, storageKey)
	if err != nil {
		return mcpmessage.AssetMeta{}, err
	}
	return mcpmessage.AssetMeta{ContentHash: asset.ContentHash, Mime: asset.Mime, SizeBytes: asset.SizeBytes, StorageKey: asset.StorageKey}, nil
}

func (a *mediaAssetResolverAdapter) IngestContainerFile(ctx context.Context, botID, containerPath string) (mcpmessage.AssetMeta, error) {
	if a == nil || a.media == nil {
		return mcpmessage.AssetMeta{}, errors.New("media service not configured")
	}
	asset, err := a.media.IngestContainerFile(ctx, botID, containerPath)
	if err != nil {
		return mcpmessage.AssetMeta{}, err
	}
	return mcpmessage.AssetMeta{ContentHash: asset.ContentHash, Mime: asset.Mime, SizeBytes: asset.SizeBytes, StorageKey: asset.StorageKey}, nil
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
