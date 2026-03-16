package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	stdpath "path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"golang.org/x/crypto/bcrypt"

	dbembed "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acl"
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
	"github.com/memohai/memoh/internal/inbox"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	mcpbrowser "github.com/memohai/memoh/internal/mcp/providers/browser"
	mcpcontacts "github.com/memohai/memoh/internal/mcp/providers/contacts"
	mcpcontainer "github.com/memohai/memoh/internal/mcp/providers/container"
	mcpemail "github.com/memohai/memoh/internal/mcp/providers/email"
	mcpinbox "github.com/memohai/memoh/internal/mcp/providers/inbox"
	mcpmemory "github.com/memohai/memoh/internal/mcp/providers/memory"
	mcpmessage "github.com/memohai/memoh/internal/mcp/providers/message"
	mcpschedule "github.com/memohai/memoh/internal/mcp/providers/schedule"
	mcpskill "github.com/memohai/memoh/internal/mcp/providers/skill"
	mcpsubagent "github.com/memohai/memoh/internal/mcp/providers/subagent"
	mcptts "github.com/memohai/memoh/internal/mcp/providers/tts"
	mcpweb "github.com/memohai/memoh/internal/mcp/providers/web"
	mcpwebfetch "github.com/memohai/memoh/internal/mcp/providers/webfetch"
	mcpfederation "github.com/memohai/memoh/internal/mcp/sources/federation"
	"github.com/memohai/memoh/internal/media"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	membuiltin "github.com/memohai/memoh/internal/memory/adapters/builtin"
	memmem0 "github.com/memohai/memoh/internal/memory/adapters/mem0"
	memopenviking "github.com/memohai/memoh/internal/memory/adapters/openviking"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/server"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/storage/providers/containerfs"
	"github.com/memohai/memoh/internal/subagent"
	ttspkg "github.com/memohai/memoh/internal/tts"
	ttsedge "github.com/memohai/memoh/internal/tts/adapter/edge"
	"github.com/memohai/memoh/internal/version"
	"github.com/memohai/memoh/internal/workspace"
)

func migrationsFS() fs.FS {
	sub, err := fs.Sub(dbembed.MigrationsFS, "migrations")
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err))
	}
	return sub
}

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "migrate":
		runMigrate(os.Args[2:])
	case "version":
		fmt.Printf("memoh-server %s\n", version.GetInfo())
	default:
		fmt.Fprintf(os.Stderr, "Usage: memoh-server <command>\n\nCommands:\n  serve     Start the server (default)\n  migrate   Run database migrations (up|down|version|force)\n  version   Print version information\n")
		os.Exit(1)
	}
}

func runMigrate(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: memoh-server migrate <up|down|version|force N>\n")
		os.Exit(1)
	}

	cfg, err := provideConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format)
	log := logger.L

	migrateCmd := args[0]
	var migrateArgs []string
	if len(args) > 1 {
		migrateArgs = args[1:]
	}

	if err := db.RunMigrate(log, cfg.Postgres, migrationsFS(), migrateCmd, migrateArgs); err != nil {
		log.Error("migration failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func runServe() {
	fx.New(
		fx.Provide(
			provideConfig,
			boot.ProvideRuntimeConfig,
			provideLogger,
			provideContainerService,
			provideDBConn,
			provideDBQueries,

			// container & workspace infrastructure
			provideWorkspaceManager,

			// memory pipeline
			provideMemoryLLM,
			memprovider.NewService,
			provideMemoryProviderRegistry,

			// domain services (auto-wired)
			models.NewService,
			bots.NewService,
			accounts.NewService,
			acl.NewService,
			settings.NewService,
			providers.NewService,
			searchproviders.NewService,
			browsercontexts.NewService,
			policy.NewService,
			mcp.NewConnectionService,
			subagent.NewService,
			conversation.NewService,
			identities.NewService,
			bind.NewService,
			event.NewHub,
			inbox.NewService,

			// tts infrastructure
			provideTtsRegistry,
			ttspkg.NewService,
			provideTtsTempStore,

			// email infrastructure
			emailpkg.NewDBOAuthTokenStore,
			provideEmailRegistry,
			emailpkg.NewService,
			emailpkg.NewOutboxService,
			provideEmailChatGateway,
			provideEmailTrigger,
			emailpkg.NewManager,

			// services requiring provide functions
			provideRouteService,
			provideMessageService,
			provideMediaService,

			// channel infrastructure
			local.NewRouteHub,
			provideChannelRegistry,
			channel.NewStore,
			provideChannelRouter,
			provideChannelManager,
			provideChannelLifecycleService,

			// conversation flow
			provideChatResolver,
			provideScheduleTriggerer,
			schedule.NewService,
			provideHeartbeatTriggerer,
			heartbeat.NewService,

			// containerd handler & tool gateway
			provideContainerdHandler,
			provideFederationGateway,
			provideToolGatewayService,

			// http handlers (group:"server_handlers")
			provideServerHandler(handlers.NewPingHandler),
			provideServerHandler(provideAuthHandler),
			provideServerHandler(provideMemoryHandler),
			provideServerHandler(provideMessageHandler),
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
			provideServerHandler(handlers.NewMCPHandler),
			provideServerHandler(handlers.NewMCPOAuthHandler),
			provideOAuthService,
			provideServerHandler(handlers.NewInboxHandler),
			provideServerHandler(handlers.NewTokenUsageHandler),
			provideServerHandler(handlers.NewBrowserContextsHandler),
			provideServerHandler(provideCLIHandler),
			provideServerHandler(provideWebHandler),

			provideServer,
		),
		fx.Invoke(
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

// ---------------------------------------------------------------------------
// fx helper
// ---------------------------------------------------------------------------

func provideServerHandler(fn any) any {
	return fx.Annotate(
		fn,
		fx.As(new(server.Handler)),
		fx.ResultTags(`group:"server_handlers"`),
	)
}

// ---------------------------------------------------------------------------
// infrastructure providers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// memory providers
// ---------------------------------------------------------------------------

func provideMemoryLLM(modelsService *models.Service, queries *dbsqlc.Queries, log *slog.Logger) memprovider.LLM {
	return &lazyLLMClient{
		modelsService: modelsService,
		queries:       queries,
		timeout:       30 * time.Second,
		logger:        log,
	}
}

func provideMemoryProviderRegistry(log *slog.Logger, chatService *conversation.Service, accountService *accounts.Service, manager *workspace.Manager, queries *dbsqlc.Queries, cfg config.Config) *memprovider.Registry {
	registry := memprovider.NewRegistry(log)
	fileRuntime := handlers.NewBuiltinMemoryRuntime(manager)
	fileStore := storefs.New(log, manager)
	registry.RegisterFactory(string(memprovider.ProviderBuiltin), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		runtime, err := membuiltin.NewBuiltinRuntimeFromConfig(log, providerConfig, fileRuntime, fileStore, queries, cfg)
		if err != nil {
			return nil, err
		}
		return membuiltin.NewBuiltinProvider(log, runtime, chatService, accountService), nil
	})
	registry.RegisterFactory(string(memprovider.ProviderMem0), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		return memmem0.NewMem0Provider(log, providerConfig, fileStore)
	})
	registry.RegisterFactory(string(memprovider.ProviderOpenViking), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		return memopenviking.NewOpenVikingProvider(log, providerConfig)
	})
	registry.Register("__builtin_default__", membuiltin.NewBuiltinProvider(log, fileRuntime, chatService, accountService))
	return registry
}

// ---------------------------------------------------------------------------
// domain service providers (interface adapters)
// ---------------------------------------------------------------------------

func provideRouteService(log *slog.Logger, queries *dbsqlc.Queries, chatService *conversation.Service) *route.DBService {
	return route.NewService(log, queries, chatService)
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

// ---------------------------------------------------------------------------
// conversation flow
// ---------------------------------------------------------------------------

func provideChatResolver(log *slog.Logger, cfg config.Config, modelsService *models.Service, queries *dbsqlc.Queries, chatService *conversation.Service, msgService *message.DBService, settingsService *settings.Service, mediaService *media.Service, containerdHandler *handlers.ContainerdHandler, inboxService *inbox.Service, memoryRegistry *memprovider.Registry) *flow.Resolver {
	resolver := flow.NewResolver(log, modelsService, queries, chatService, msgService, settingsService, cfg.AgentGateway.BaseURL(), 120*time.Second)
	resolver.SetMemoryRegistry(memoryRegistry)
	resolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	resolver.SetGatewayAssetLoader(&gatewayAssetLoaderAdapter{media: mediaService})
	resolver.SetInboxService(inboxService)
	return resolver
}

// ---------------------------------------------------------------------------
// channel providers
// ---------------------------------------------------------------------------

func provideChannelRegistry(log *slog.Logger, hub *local.RouteHub, mediaService *media.Service) *channel.Registry {
	registry := channel.NewRegistry()

	// Telegram
	tgAdapter := telegram.NewTelegramAdapter(log)
	tgAdapter.SetAssetOpener(mediaService)
	registry.MustRegister(tgAdapter)

	// Discord
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

func provideChannelRouter(
	log *slog.Logger,
	registry *channel.Registry,
	hub *local.RouteHub,
	routeService *route.DBService,
	msgService *message.DBService,
	resolver *flow.Resolver,
	identityService *identities.Service,
	botService *bots.Service,
	aclService *acl.Service,
	policyService *policy.Service,
	bindService *bind.Service,
	mediaService *media.Service,
	inboxService *inbox.Service,
	ttsService *ttspkg.Service,
	settingsService *settings.Service,
	subagentService *subagent.Service,
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
	queries *dbsqlc.Queries,
	containerdHandler *handlers.ContainerdHandler,
	manager *workspace.Manager,
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
	processor.SetACLService(aclService)
	processor.SetMediaService(mediaService)
	processor.SetStreamObserver(local.NewRouteHubBroadcaster(hub))
	processor.SetInboxService(inboxService)
	processor.SetTtsService(ttsService, &settingsTtsModelResolver{settings: settingsService})
	processor.SetCommandHandler(command.NewHandler(
		log,
		&command.BotMemberRoleAdapter{BotService: botService},
		subagentService,
		scheduleService,
		settingsService,
		mcpConnService,
		inboxService,
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

// ---------------------------------------------------------------------------
// containerd handler & tool gateway
// ---------------------------------------------------------------------------

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

func provideToolGatewayService(log *slog.Logger, cfg config.Config, channelManager *channel.Manager, registry *channel.Registry, routeService *route.DBService, scheduleService *schedule.Service, _ *conversation.Service, _ *accounts.Service, settingsService *settings.Service, searchProviderService *searchproviders.Service, manager *workspace.Manager, containerdHandler *handlers.ContainerdHandler, mcpConnService *mcp.ConnectionService, mediaService *media.Service, inboxService *inbox.Service, memoryRegistry *memprovider.Registry, emailService *emailpkg.Service, emailManager *emailpkg.Manager, fedGateway *handlers.MCPFederationGateway, oauthService *mcp.OAuthService, subagentService *subagent.Service, modelsService *models.Service, browserContextService *browsercontexts.Service, queries *dbsqlc.Queries, ttsService *ttspkg.Service) *mcp.ToolGatewayService {
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
	webFetchExec := mcpwebfetch.NewExecutor(log)
	subagentExec := mcpsubagent.NewExecutor(log, subagentService, settingsService, modelsService, queries, cfg.AgentGateway.BaseURL())
	skillExec := mcpskill.NewExecutor(log)
	browserExec := mcpbrowser.NewExecutor(log, settingsService, browserContextService, manager, cfg.BrowserGateway)
	ttsExec := mcptts.NewExecutor(log, settingsService, ttsService, channelManager, registry)

	svc := mcp.NewToolGatewayService(
		log,
		[]mcp.ToolExecutor{messageExec, contactsExec, scheduleExec, memoryExec, webExec, fsExec, inboxExec, emailExec, webFetchExec, subagentExec, skillExec, browserExec, ttsExec},
		[]mcp.ToolSource{fedSource},
	)
	containerdHandler.SetToolGatewayService(svc)
	return svc
}

// ---------------------------------------------------------------------------
// handler providers (interface adaptation / config extraction)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// email providers
// ---------------------------------------------------------------------------

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
		OnStop: func(stopCtx context.Context) error {
			cancel()
			emailManager.Stop(stopCtx)
			return nil
		},
	})
}

// ---------------------------------------------------------------------------
// server
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// lifecycle hooks
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// lazy LLM client
// ---------------------------------------------------------------------------

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

// skillLoaderAdapter bridges handlers.ContainerdHandler to flow.SkillLoader.
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
		entries[i] = flow.SkillEntry{
			Name:        item.Name,
			Description: item.Description,
			Content:     item.Content,
			Metadata:    item.Metadata,
		}
	}
	return entries, nil
}

// mediaAssetResolverAdapter bridges media.Service to the message tool's AssetResolver interface.
type mediaAssetResolverAdapter struct {
	media *media.Service
}

func (a *mediaAssetResolverAdapter) GetByStorageKey(ctx context.Context, botID, storageKey string) (mcpmessage.AssetMeta, error) {
	if a == nil || a.media == nil {
		return mcpmessage.AssetMeta{}, errors.New("media service not configured")
	}
	asset, err := a.media.GetByStorageKey(ctx, botID, storageKey)
	if err != nil {
		return mcpmessage.AssetMeta{}, err
	}
	return mcpmessage.AssetMeta{
		ContentHash: asset.ContentHash,
		Mime:        asset.Mime,
		SizeBytes:   asset.SizeBytes,
		StorageKey:  asset.StorageKey,
	}, nil
}

func (a *mediaAssetResolverAdapter) IngestContainerFile(ctx context.Context, botID, containerPath string) (mcpmessage.AssetMeta, error) {
	if a == nil || a.media == nil {
		return mcpmessage.AssetMeta{}, errors.New("media service not configured")
	}
	asset, err := a.media.IngestContainerFile(ctx, botID, containerPath)
	if err != nil {
		return mcpmessage.AssetMeta{}, err
	}
	return mcpmessage.AssetMeta{
		ContentHash: asset.ContentHash,
		Mime:        asset.Mime,
		SizeBytes:   asset.SizeBytes,
		StorageKey:  asset.StorageKey,
	}, nil
}

// gatewayAssetLoaderAdapter bridges media service to flow gateway asset loader.
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
