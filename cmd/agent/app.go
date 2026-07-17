package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	stdpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/background"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/agentpayload"
	audiopkg "github.com/memohai/memoh/internal/audio"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/botbackup"
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
	ctr "github.com/memohai/memoh/internal/container"
	containerprovider "github.com/memohai/memoh/internal/container/provider"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/db"
	pgvectordb "github.com/memohai/memoh/internal/db/pgvector"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	emailpkg "github.com/memohai/memoh/internal/email"
	emailgeneric "github.com/memohai/memoh/internal/email/adapters/generic"
	emailgmail "github.com/memohai/memoh/internal/email/adapters/gmail"
	emailmailgun "github.com/memohai/memoh/internal/email/adapters/mailgun"
	"github.com/memohai/memoh/internal/fetchproviders"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/healthcheck"
	channelchecker "github.com/memohai/memoh/internal/healthcheck/checkers/channel"
	mcpchecker "github.com/memohai/memoh/internal/healthcheck/checkers/mcp"
	modelchecker "github.com/memohai/memoh/internal/healthcheck/checkers/model"
	"github.com/memohai/memoh/internal/heartbeat"
	hookspkg "github.com/memohai/memoh/internal/hooks"
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
	"github.com/memohai/memoh/internal/memory/wikistore"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/messaging"
	"github.com/memohai/memoh/internal/models"
	netctl "github.com/memohai/memoh/internal/network"
	netoverlay "github.com/memohai/memoh/internal/network/overlay"
	"github.com/memohai/memoh/internal/oauthclients"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
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
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
	"github.com/memohai/memoh/internal/userruntime"
	"github.com/memohai/memoh/internal/version"
	videopkg "github.com/memohai/memoh/internal/video"
	"github.com/memohai/memoh/internal/webhooktunnel"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
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
	svc, cleanup, err := containerprovider.ProvideService(context.Background(), log, cfg, rc.ContainerBackend)
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

func provideNetworkController(service ctr.Service, rc *boot.RuntimeConfig, networkService *netctl.Service, registry *netctl.Registry) netctl.Controller {
	runtime := netctl.NewContainerRuntimeFromBackend(rc.ContainerBackend, service)
	ctrl := netctl.NewController(runtime, networkService, registry)
	networkService.SetController(ctrl)
	return ctrl
}

func provideDBConn(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	conn, err := db.Open(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	if conn == nil {
		return nil, nil
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			conn.Close()
			return nil
		},
	})
	return conn, nil
}

func providePGVectorStore(lc fx.Lifecycle, log *slog.Logger, cfg config.Config) (*pgvectordb.Store, error) {
	if !cfg.PGVector.Enabled {
		return nil, nil
	}
	store, err := pgvectordb.Open(context.Background(), log, cfg.PGVector)
	if err != nil {
		log.Warn("pgvector store unavailable; semantic memory index disabled", slog.Any("error", err))
		return nil, nil
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			store.Close()
			return nil
		},
	})
	return store, nil
}

func providePostgresStore(conn *pgxpool.Pool) (*postgresstore.Store, error) {
	if conn == nil {
		return nil, nil
	}
	return postgresstore.New(conn)
}

func provideOverlayProviderRegistry(service ctr.Service, cfg config.Config, rc *boot.RuntimeConfig) *netctl.Registry {
	registry := netctl.NewRegistry()
	runtime := netctl.NewContainerRuntimeFromBackend(rc.ContainerBackend, service)
	if err := netoverlay.RegisterBuiltinProviders(registry, netoverlay.ProviderDeps{
		SidecarRuntime: service,
		Runtime:        runtime.Descriptor(),
		StateRoot:      cfg.Workspace.DataRoot,
	}); err != nil {
		panic(err)
	}
	return registry
}

func provideNetworkService(log *slog.Logger, queries dbstore.Queries, registry *netctl.Registry, service ctr.Service, rc *boot.RuntimeConfig, cfg config.Config) *netctl.Service {
	return netctl.NewService(log, queries, registry, service, rc.ContainerBackend, cfg.Workspace.CNIBinaryDir, cfg.Workspace.CNIConfigDir, cfg.Workspace.DataRoot)
}

func provideDBQueries(postgresStore *postgresstore.Store) (dbstore.Queries, error) {
	if postgresStore == nil {
		return nil, errors.New("postgres store not configured")
	}
	return postgresstore.NewQueriesWithPool(postgresStore.Pool(), postgresStore.SQLC()), nil
}

func provideAccountStore(postgresStore *postgresstore.Store) (dbstore.AccountStore, error) {
	if postgresStore == nil {
		return nil, errors.New("postgres account store not configured")
	}
	return postgresStore, nil
}

func provideUserRuntimeStore(postgresStore *postgresstore.Store) (dbstore.UserRuntimeStore, error) {
	if postgresStore == nil {
		return nil, errors.New("postgres user runtime store not configured")
	}
	return postgresStore, nil
}

func provideBotRemoteRuntimeBindingStore(postgresStore *postgresstore.Store) (dbstore.BotRemoteRuntimeBindingStore, error) {
	if postgresStore == nil {
		return nil, errors.New("postgres bot remote runtime binding store not configured")
	}
	return postgresStore, nil
}

func provideUserRuntimeHub(lc fx.Lifecycle, log *slog.Logger) *userruntime.Hub {
	hub := userruntime.NewHub(log)
	lc.Append(fx.Hook{OnStop: hub.Shutdown})
	return hub
}

func provideUserRuntimePipe() userruntime.Pipe {
	return userruntime.NewDirectPipe()
}

func provideAccountService(log *slog.Logger, accountStore dbstore.AccountStore, emailService *emailpkg.Service) *accounts.Service {
	svc := accounts.NewService(log, accountStore)
	svc.SetEmailProviderBootstrapper(emailService)
	return svc
}

// provideWikiStore wires the PostgreSQL memory wiki store. Returns a pointer
// so FX can inject nil-safe into providers that may run without a wiki store.
func provideWikiStore(postgresStore *postgresstore.Store) (*wikistore.Store, error) {
	if postgresStore == nil {
		return nil, errors.New("postgres wiki store not configured")
	}
	ws := wikistore.Store(wikistore.NewPostgres(postgresStore.SQLC()))
	return &ws, nil
}

func provideBridgeProvider(manage *workspace.Manager) bridge.Provider {
	return manage
}

type nativeWorkspaceBridgeProvider struct {
	manager *workspace.Manager
}

type workspaceTargetPolicyResolver struct {
	manager *workspace.Manager
}

func (r workspaceTargetPolicyResolver) ResolveWorkspaceTargetPolicy(ctx context.Context, botID, targetID string) (toolapproval.WorkspaceTargetPolicy, error) {
	resolved, err := r.manager.ResolveWorkspaceTarget(ctx, botID, targetID)
	if err != nil {
		return toolapproval.WorkspaceTargetPolicy{}, err
	}
	return toolapproval.WorkspaceTargetPolicy{
		TargetID:      resolved.TargetID,
		Kind:          resolved.Kind,
		Name:          resolved.Name,
		WorkspacePath: resolved.WorkspacePath,
		Config:        resolved.Approval,
	}, nil
}

func (p nativeWorkspaceBridgeProvider) MCPClient(ctx context.Context, botID string) (*bridge.Client, error) {
	return p.manager.NativeMCPClient(ctx, botID)
}

func providePluginBridgeProvider(provider bridge.Provider) pluginspkg.BridgeProvider {
	return pluginspkg.BridgeProvider{Provider: provider}
}

func provideHooksService(log *slog.Logger, provider bridge.Provider, pluginService *pluginspkg.Service) *hookspkg.Service {
	service := hookspkg.NewService(log, provider)
	service.SetPluginService(pluginService)
	return service
}

func provideWorkspaceManager(lc fx.Lifecycle, log *slog.Logger, service ctr.Service, networkController netctl.Controller, cfg config.Config, conn *pgxpool.Pool, queries dbstore.Queries, remote *workspace.RemoteWorkspaceService) (*workspace.Manager, error) {
	localSvc := workspace.NewLocalService(log, cfg.Local, cfg.Workspace.DataRoot)
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			localSvc.Close()
			return nil
		},
	})
	runtimeSvc := workspace.NewRuntimeRouter(service, localSvc)
	mgr := workspace.NewManager(log, runtimeSvc, networkController, cfg.Workspace, cfg.Containerd.Namespace, conn, queries)
	mgr.SetRemoteWorkspaceService(remote)
	tlsOpts, err := workspace.BridgeTLSRuntimeOptionsFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if tlsOpts != nil {
		mgr.SetBridgeTLS(tlsOpts)
	}
	return mgr, nil
}

func provideMemoryLLM(modelsService *models.Service, settingsService *settings.Service, queries dbstore.Queries, log *slog.Logger) memprovider.LLM {
	return &lazyLLMClient{
		modelsService:   modelsService,
		settingsService: settingsService,
		queries:         queries,
		timeout:         models.DefaultProviderRequestTimeout,
		logger:          log,
	}
}

func provideMemoryProviderRegistry(log *slog.Logger, llm memprovider.LLM, chatService *conversation.Service, accountService *accounts.Service, provider bridge.Provider, queries dbstore.Queries, vectorStore *pgvectordb.Store, wikiStore *wikistore.Store) *memprovider.Registry {
	registry := memprovider.NewRegistry(log)
	fileStore := storefs.New(log, provider)
	registry.RegisterFactory(string(memprovider.ProviderBuiltin), func(_ string, providerConfig map[string]any) (memprovider.Provider, error) {
		var ws wikistore.Store
		if wikiStore != nil {
			ws = *wikiStore
		}
		runtime, err := membuiltin.NewBuiltinRuntimeFromConfig(log, providerConfig, fileStore, queries, vectorStore, ws)
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
	// Default provider for bots without an explicit memory_provider_id. Uses the
	// graph runtime (PG nodes/edges as source of truth) when a wiki store is
	// wired; falls back to the file runtime otherwise (e.g. bootstrap before the
	// DB is ready).
	var defaultRuntime membuiltin.Runtime
	if wikiStore != nil {
		defaultRuntime = membuiltin.NewGraphRuntime(log, *wikiStore, fileStore)
	} else {
		defaultRuntime = membuiltin.NewFileRuntime(fileStore)
	}
	defaultProvider := membuiltin.NewBuiltinProvider(log, defaultRuntime, chatService, accountService)
	defaultProvider.SetLLM(llm)
	registry.Register("__builtin_default__", defaultProvider)
	return registry
}

func providePipeline() *pipelinepkg.Pipeline {
	return pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
}

func provideEventStore(log *slog.Logger, queries dbstore.Queries) *pipelinepkg.EventStore {
	return pipelinepkg.NewEventStore(log, queries)
}

func provideDiscussDriver(log *slog.Logger, pipeline *pipelinepkg.Pipeline, eventStore *pipelinepkg.EventStore, agent *agentpkg.Agent, msgService *message.DBService) *pipelinepkg.DiscussDriver {
	return pipelinepkg.NewDiscussDriver(pipelinepkg.DiscussDriverDeps{
		Pipeline:       pipeline,
		EventStore:     eventStore,
		Agent:          agent,
		MessageService: msgService,
		CursorStore:    eventStore,
		Logger:         log,
	})
}

func provideRouteService(log *slog.Logger, queries dbstore.Queries, chatService *conversation.Service) *route.DBService {
	return route.NewService(log, queries, chatService)
}

func provideSessionService(log *slog.Logger, queries dbstore.Queries, hub *event.Hub) *sessionpkg.Service {
	return sessionpkg.NewService(log, queries, hub)
}

func provideMessageService(log *slog.Logger, queries dbstore.Queries, hub *event.Hub) *message.DBService {
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

func provideAgent(log *slog.Logger, provider bridge.Provider, hookService *hookspkg.Service, cfg config.Config) *agentpkg.Agent {
	return agentpkg.New(agentpkg.Deps{
		BridgeProvider: provider,
		HookService:    hookService,
		Logger:         log,
		Limits:         agentLimitsFromConfig(cfg.Agent),
	})
}

func agentLimitsFromConfig(cfg config.AgentConfig) agentpkg.Limits {
	return agentpkg.LimitsFromValues(
		cfg.ToolOutputMaxBytes,
		cfg.ToolOutputMaxLines,
		cfg.SystemFilesMaxBytes,
	)
}

func injectToolProviders(a *agentpkg.Agent, msgService *message.DBService, hookService *hookspkg.Service, providers []agenttools.ToolProvider) {
	a.SetToolProviders(providers)
	for _, p := range providers {
		if cp, ok := p.(*agenttools.ContainerProvider); ok {
			cp.SetHookService(hookService)
		}
		if sp, ok := p.(*agenttools.SpawnProvider); ok {
			sp.SetAgent(agentpkg.NewSpawnAdapter(a))
			sp.SetMessageService(msgService)
			sp.SetSystemPromptFunc(agentpkg.SpawnSystemPrompt)
			sp.SetModelCreator(agentpkg.SpawnModelCreatorFunc())
			sp.SetHookService(hookService)
		}
	}
}

func provideACPRunner(log *slog.Logger, manager *workspace.Manager) *acpclient.Runner {
	return acpclient.NewRunner(log, manager)
}

func provideACPSessionPool(lc fx.Lifecycle, log *slog.Logger, runner *acpclient.Runner, botService *bots.Service, sessionService *sessionpkg.Service, toolGateway *mcp.ToolGatewayService, toolContexts *mcp.ToolSessionContextStore, toolApproval *toolapproval.Service, userInput *userinput.Service, containerdHandler *handlers.ContainerdHandler) *acpagent.SessionPool {
	pool := acpagent.NewSessionPool(log, runner, botService, sessionService)
	pool.SetToolGateway(toolGateway)
	pool.SetToolSessionContextStore(toolContexts)
	pool.SetToolApprovalService(toolApproval)
	pool.SetUserInputService(userInput)
	containerdHandler.SetACPRuntimeResolver(pool)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			pool.StartReaper(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			pool.CloseAll() //nolint:contextcheck // ACP shutdown must close subprocesses even after lifecycle ctx cancellation.
			return nil
		},
	})
	return pool
}

func provideChatResolver(log *slog.Logger, a *agentpkg.Agent, modelsService *models.Service, queries dbstore.Queries, chatService *conversation.Service, msgService *message.DBService, settingsService *settings.Service, accountService *accounts.Service, botService *bots.Service, mediaService *media.Service, containerdHandler *handlers.ContainerdHandler, workspaceManager *workspace.Manager, memoryRegistry *memprovider.Registry, channelStore *channel.Store, _ *route.DBService, sessionService *sessionpkg.Service, eventHub *event.Hub, compactionService *compaction.Service, pipeline *pipelinepkg.Pipeline, rc *boot.RuntimeConfig, bgManager *background.Manager, toolApproval *toolapproval.Service, userInput *userinput.Service, acpPool *acpagent.SessionPool, hookService *hookspkg.Service) *flow.Resolver {
	resolver := flow.NewResolver(log, modelsService, queries, chatService, msgService, settingsService, accountService, a, rc.TimezoneLocation, 120*time.Second)
	resolver.SetBotPermissionChecker(&botPermissionCheckerAdapter{bots: botService, accounts: accountService})
	resolver.SetWorkspaceTargetResolver(workspaceManager)
	resolver.SetHookService(hookService)
	if sessionService != nil {
		sessionService.SetHookService(hookService)
	}
	if compactionService != nil {
		compactionService.SetHookService(hookService)
	}
	if workspaceManager != nil {
		workspaceManager.SetHookService(hookService)
	}
	resolver.SetMemoryRegistry(memoryRegistry)
	resolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	resolver.SetGatewayAssetLoader(&gatewayAssetLoaderAdapter{media: mediaService})
	resolver.SetChannelStore(channelStore)
	resolver.SetSessionService(sessionService)
	resolver.SetEventPublisher(eventHub)
	resolver.SetCompactionService(compactionService)
	resolver.SetPipeline(pipeline)
	resolver.SetBackgroundManager(bgManager)
	if toolApproval != nil {
		toolApproval.SetHookService(hookService)
		toolApproval.SetWorkspaceTargetPolicyResolver(workspaceTargetPolicyResolver{manager: workspaceManager})
	}
	resolver.SetToolApprovalService(toolApproval)
	resolver.SetUserInputService(userInput)
	resolver.SetACPSessionPool(acpPool)
	if bgManager != nil {
		bgManager.SetEventFunc(func(evt background.TaskEvent) {
			if eventHub == nil {
				return
			}
			// The wire shape lives in internal/agentpayload — see its
			// BackgroundTask helper and the tests there that pin the
			// top-level `session_id` placement the per-session SSE handler
			// routes on.
			data, err := json.Marshal(agentpayload.BackgroundTask(evt))
			if err != nil {
				return
			}
			eventHub.Publish(event.Event{
				Type:  event.EventTypeBackgroundTask,
				BotID: evt.BotID,
				Data:  data,
			})
		})
	}
	return resolver
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
	resolver *flow.Resolver,
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

	processor := inbound.NewChannelInboundProcessor(log, registry, routeService, msgService, resolver, identityService, policyService, rc.JwtSecret, 5*time.Minute)
	processor.SetSessionEnsurer(&sessionEnsurerAdapter{svc: sessionService})
	processor.SetPipeline(pipeline, eventStore, discussDriver)
	discussDriver.SetResolver(resolver)
	discussDriver.SetRuntimeStreamer(resolver)
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

func provideContainerdHandler(log *slog.Logger, manager *workspace.Manager, cfg config.Config, rc *boot.RuntimeConfig, botService *bots.Service, accountService *accounts.Service, policyService *policy.Service, pluginService *pluginspkg.Service) *handlers.ContainerdHandler {
	h := handlers.NewContainerdHandler(log, manager, cfg.Workspace, rc.ContainerBackend, botService, accountService, policyService)
	h.SetPluginService(pluginService)
	return h
}

func provideBotBackupService(log *slog.Logger, conn *pgxpool.Pool, queries dbstore.Queries, botService *bots.Service, settingsService *settings.Service, aclService *acl.Service, channelStore *channel.Store, mcpService *mcp.ConnectionService, scheduleService *schedule.Service, emailService *emailpkg.Service, providerService *providers.Service, modelsService *models.Service, searchProviderService *searchproviders.Service, fetchProviderService *fetchproviders.Service, memoryProviderService *memprovider.Service, manager *workspace.Manager, acpPool *acpagent.SessionPool) *botbackup.Service {
	return botbackup.New(botbackup.Params{
		Logger:          log,
		DB:              conn,
		Queries:         queries,
		Bots:            botService,
		Settings:        settingsService,
		ACL:             aclService,
		Channels:        channelStore,
		MCP:             mcpService,
		Schedules:       scheduleService,
		Email:           emailService,
		Providers:       providerService,
		Models:          modelsService,
		SearchProviders: searchProviderService,
		FetchProviders:  fetchProviderService,
		MemoryProviders: memoryProviderService,
		Workspace:       manager,
		ACPRuntimes:     acpPool,
	})
}

func provideFederationGateway(log *slog.Logger, containerdHandler *handlers.ContainerdHandler) *handlers.MCPFederationGateway {
	return handlers.NewMCPFederationGateway(log, containerdHandler)
}

func provideOAuthService(log *slog.Logger, queries dbstore.Queries, cfg config.Config) *mcp.OAuthService {
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

func provideACPToolSource(log *slog.Logger, toolApproval *toolapproval.Service, userInput *userinput.Service, toolContexts *mcp.ToolSessionContextStore, cfg config.Config) *agenttools.NativeToolSource {
	limits := agentLimitsFromConfig(cfg.Agent)
	return agenttools.NewNativeToolSource(log, nil, agenttools.NativeToolSourceOptions{
		AllowAll:        true,
		Approval:        toolApproval,
		UserInput:       userInput,
		ToolEvents:      toolContexts,
		ToolOutputLimit: limits.ToolOutputLimit(),
	})
}

func injectACPToolProviders(source *agenttools.NativeToolSource, toolProviders []agenttools.ToolProvider) {
	if source != nil {
		source.SetProviders(acpToolProviders(toolProviders))
	}
}

func provideToolGatewayService(log *slog.Logger, fedGateway *handlers.MCPFederationGateway, oauthService *mcp.OAuthService, mcpConnService *mcp.ConnectionService, containerdHandler *handlers.ContainerdHandler, nativeSource *agenttools.NativeToolSource, toolContexts *mcp.ToolSessionContextStore, cfg config.Config) *mcp.ToolGatewayService {
	fedGateway.SetOAuthService(oauthService)
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService, mcpfederation.WithReservedToolName(agenttools.IsBuiltInToolName))
	limits := agentLimitsFromConfig(cfg.Agent)
	svc := mcp.NewToolGatewayService(log, []mcp.ToolSource{nativeSource, fedSource}, mcp.WithToolOutputLimit(limits.ToolOutputLimit()))
	containerdHandler.SetToolGatewayService(svc)
	containerdHandler.SetToolSessionContextStore(toolContexts)
	return svc
}

func acpToolProviders(providers []agenttools.ToolProvider) []agenttools.ToolProvider {
	filtered := make([]agenttools.ToolProvider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if _, ok := provider.(*agenttools.FederationProvider); ok {
			continue
		}
		filtered = append(filtered, provider)
	}
	return filtered
}

func provideBackgroundManager(log *slog.Logger) *background.Manager {
	return background.New(log)
}

func provideToolProviders(log *slog.Logger, channelManager *channel.Manager, registry *channel.Registry, routeService *route.DBService, scheduleService *schedule.Service, settingsService *settings.Service, searchProviderService *searchproviders.Service, fetchProviderService *fetchproviders.Service, manager *workspace.Manager, mediaService *media.Service, memoryRegistry *memprovider.Registry, emailService *emailpkg.Service, emailManager *emailpkg.Manager, fedGateway *handlers.MCPFederationGateway, mcpConnService *mcp.ConnectionService, modelsService *models.Service, queries dbstore.Queries, audioService *audiopkg.Service, videoService *videopkg.Service, sessionService *sessionpkg.Service, messageService *message.DBService, bgManager *background.Manager, hookService *hookspkg.Service) []agenttools.ToolProvider {
	var assetResolver messaging.AssetResolver
	if mediaService != nil {
		assetResolver = &mediaAssetResolverAdapter{media: mediaService}
	}
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService, mcpfederation.WithReservedToolName(agenttools.IsBuiltInToolName))
	return []agenttools.ToolProvider{
		agenttools.NewAskUserProvider(log),
		agenttools.NewMessageProvider(log, channelManager, channelManager, registry, assetResolver),
		agenttools.NewContactsProvider(log, routeService),
		agenttools.NewScheduleProvider(log, scheduleService),
		agenttools.NewMemoryProvider(log, memoryRegistry, settingsService),
		agenttools.NewWebProvider(log, settingsService, searchProviderService),
		agenttools.NewContainerProvider(log, manager, bgManager, config.DefaultDataMount, hookService),
		agenttools.NewBackgroundProvider(log, bgManager),
		agenttools.NewBrowserProvider(log, settingsService, nativeWorkspaceBridgeProvider{manager: manager}, manager, config.DefaultDataMount),
		agenttools.NewEmailProvider(log, emailService, emailManager),
		agenttools.NewWebFetchProvider(log, settingsService, fetchProviderService),
		agenttools.NewSpawnProvider(log, settingsService, modelsService, queries, sessionService, bgManager),
		agenttools.NewSkillProvider(log),
		agenttools.NewTTSProvider(log, settingsService, audioService, channelManager, registry),
		agenttools.NewTranscriptionProvider(log, settingsService, audioService, mediaService),
		agenttools.NewImageGenProvider(log, settingsService, modelsService, queries, manager, config.DefaultDataMount),
		agenttools.NewVideoGenProvider(log, settingsService, videoService, bgManager, manager, config.DefaultDataMount),
		agenttools.NewFederationProvider(log, fedSource),
		agenttools.NewHistoryProvider(log, sessionService, messageService, queries),
	}
}

func provideMemoryHandler(log *slog.Logger, botService *bots.Service, accountService *accounts.Service, _ config.Config, memoryRegistry *memprovider.Registry, settingsService *settings.Service, _ *handlers.ContainerdHandler) *handlers.MemoryHandler {
	h := handlers.NewMemoryHandler(log, botService, accountService)
	h.SetMemoryRegistry(memoryRegistry)
	h.SetSettingsService(settingsService)
	return h
}

func provideAuthHandler(log *slog.Logger, accountService *accounts.Service, rc *boot.RuntimeConfig) *handlers.AuthHandler {
	return handlers.NewAuthHandler(log, accountService, rc.JwtSecret, rc.JwtExpiresIn)
}

func provideMessageHandler(log *slog.Logger, chatService *conversation.Service, msgService *message.DBService, sessionService *sessionpkg.Service, mediaService *media.Service, botService *bots.Service, accountService *accounts.Service, hub *event.Hub, toolApproval *toolapproval.Service, userInput *userinput.Service, bgManager *background.Manager) *handlers.MessageHandler {
	h := handlers.NewMessageHandler(log, chatService, msgService, sessionService, botService, accountService, hub)
	h.SetMediaService(mediaService)
	h.SetToolApprovalService(toolApproval)
	h.SetUserInputService(userInput)
	h.SetBackgroundManager(bgManager)
	return h
}

func provideSessionHandler(log *slog.Logger, sessionService *sessionpkg.Service, acpPool *acpagent.SessionPool, botService *bots.Service, accountService *accounts.Service) *handlers.SessionHandler {
	return handlers.NewSessionHandler(log, sessionService, acpPool, botService, accountService)
}

func provideMediaService(log *slog.Logger, provider bridge.Provider, cfg config.Config) *media.Service {
	primary := containerfs.New(provider)
	dataRoot := cfg.Workspace.DataRoot
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	secondary := localfs.New(filepath.Join(dataRoot, "media"))
	storageProvider := fallback.New(primary, secondary)
	return media.NewService(log, storageProvider)
}

func provideUsersHandler(log *slog.Logger, accountService *accounts.Service, botService *bots.Service, routeService *route.DBService, channelStore *channel.Store, channelLifecycle *channel.Lifecycle, channelManager *channel.Manager, registry *channel.Registry, workspaceManager *workspace.Manager, acpPool *acpagent.SessionPool) *handlers.UsersHandler {
	handler := handlers.NewUsersHandler(log, accountService, botService, routeService, channelStore, channelLifecycle, channelManager, registry, workspaceManager)
	handler.SetACPRuntimeCloser(acpPool)
	return handler
}

func provideACPCodexOAuthHandler(providersService *providers.Service, botService *bots.Service, accountService *accounts.Service, workspaceManager *workspace.Manager) *handlers.ACPCodexOAuthHandler {
	return handlers.NewACPCodexOAuthHandler(providersService, botService, accountService, workspaceManager, defaultACPCodexOAuthCallbackURL())
}

func provideACPCodexOAuthServerHandler(handler *handlers.ACPCodexOAuthHandler) *handlers.ACPCodexOAuthHandler {
	return handler
}

func provideACPClaudeCodeOAuthHandler(botService *bots.Service, accountService *accounts.Service, workspaceManager *workspace.Manager) *handlers.ACPClaudeCodeOAuthHandler {
	return handlers.NewACPClaudeCodeOAuthHandler(botService, accountService, workspaceManager)
}

func provideACPClaudeCodeOAuthServerHandler(handler *handlers.ACPClaudeCodeOAuthHandler) *handlers.ACPClaudeCodeOAuthHandler {
	return handler
}

func provideProviderOAuthHandler(providersService *providers.Service, acpCodexOAuthHandler *handlers.ACPCodexOAuthHandler) *handlers.ProviderOAuthHandler {
	handler := handlers.NewProviderOAuthHandler(providersService)
	handler.SetACPCodexOAuthHandler(acpCodexOAuthHandler)
	return handler
}

func provideWebHandler(channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, sessionService *sessionpkg.Service, resolver *flow.Resolver, mediaService *media.Service, audioService *audiopkg.Service, settingsService *settings.Service, rc *boot.RuntimeConfig, commandHandler *command.Handler, containerdHandler *handlers.ContainerdHandler) *handlers.LocalChannelHandler {
	h := handlers.NewLocalChannelHandler(local.WebType, channelManager, channelStore, chatService, hub, botService, accountService, sessionService)
	h.SetResolver(resolver)
	h.SetCommandHandler(commandHandler)
	h.SetRuntimeSkillResolver(containerdHandler)
	h.SetAuthTokenConfig(rc.JwtSecret, rc.JwtExpiresIn)
	h.SetMediaService(mediaService)
	h.SetSpeechService(audioService, &settingsSpeechModelResolver{settings: settingsService})
	return h
}

func provideAudioRegistry() *audiopkg.Registry {
	return audiopkg.NewRegistry()
}

func provideVideoRegistry() *videopkg.Registry {
	return videopkg.NewRegistry()
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

func provideEmailRegistry(log *slog.Logger, tokenStore *emailpkg.DBOAuthTokenStore, oauthClients *oauthclients.Registry) *emailpkg.Registry {
	reg := emailpkg.NewRegistry()
	reg.Register(emailgeneric.New(log))
	reg.Register(emailmailgun.New(log))
	reg.Register(emailgmail.New(log, tokenStore, oauthClients))
	return reg
}

func provideProvidersService(log *slog.Logger, queries dbstore.Queries, cfg config.Config) *providers.Service {
	return providers.NewService(log, queries, defaultProviderOAuthCallbackURL(), cfg.Registry.ProvidersPath())
}

func defaultProviderOAuthCallbackURL() string {
	return "http://localhost:1455/auth/callback"
}

func defaultACPCodexOAuthCallbackURL() string {
	return defaultProviderOAuthCallbackURL()
}

func provideEmailOAuthHandler(log *slog.Logger, service *emailpkg.Service, tokenStore *emailpkg.DBOAuthTokenStore, oauthClients *oauthclients.Registry, cfg config.Config) *handlers.EmailOAuthHandler {
	addr := strings.TrimSpace(cfg.Server.Addr)
	if addr == "" {
		addr = ":8080"
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	callbackURL := "http://" + host + "/api/email/oauth/callback"
	return handlers.NewEmailOAuthHandler(log, service, tokenStore, oauthClients, callbackURL)
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

func startRegistrySync(lc fx.Lifecycle, log *slog.Logger, cfg config.Config, queries dbstore.Queries) {
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
			defs = providerBootstrapDefinitions(defs)
			if len(defs) == 0 {
				return nil
			}
			return registry.Sync(ctx, log, queries, defs)
		},
	})
}

func providerBootstrapDefinitions(defs []registry.ProviderDefinition) []registry.ProviderDefinition {
	return defs
}

func startAudioProviderBootstrap(lc fx.Lifecycle, log *slog.Logger, queries dbstore.Queries, registry *audiopkg.Registry) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := audiopkg.SyncRegistry(ctx, log, queries, registry); err != nil {
				log.Warn("audio registry bootstrap failed", slog.Any("error", err))
			}
			return nil
		},
	})
}

func startVideoProviderBootstrap(lc fx.Lifecycle, log *slog.Logger, queries dbstore.Queries, registry *videopkg.Registry) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := videopkg.SyncRegistry(ctx, log, queries, registry); err != nil {
				log.Warn("video registry bootstrap failed", slog.Any("error", err))
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
			count, regErr := mpService.InstantiateAll(ctx)
			if regErr != nil {
				log.Warn("failed to instantiate memory providers", slog.Any("error", regErr))
			} else {
				log.Info("memory providers ready", slog.Int("count", count), slog.String("default_id", resp.ID), slog.String("default_provider", resp.Provider))
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

func startFetchProviderBootstrap(lc fx.Lifecycle, log *slog.Logger, fpService *fetchproviders.Service) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := fpService.EnsureDefaults(ctx); err != nil {
				log.Warn("failed to ensure default fetch providers", slog.Any("error", err))
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

func startServer(lc fx.Lifecycle, logger *slog.Logger, srv *server.Server, shutdowner fx.Shutdowner, cfg config.Config, queries dbstore.Queries, accountStore dbstore.AccountStore, emailService *emailpkg.Service, botService *bots.Service, _ *handlers.ContainerdHandler, manager *workspace.Manager, mcpConnService *mcp.ConnectionService, toolGateway *mcp.ToolGatewayService, channelManager *channel.Manager, modelsService *models.Service) {
	fmt.Printf("Starting Memoh Agent %s\n", version.GetInfo())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := ensureAdminUser(ctx, logger, accountStore, emailService, cfg); err != nil {
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

func ensureAdminUser(ctx context.Context, log *slog.Logger, accountStore dbstore.AccountStore, emailService *emailpkg.Service, cfg config.Config) error {
	if accountStore == nil {
		return errors.New("account store not configured")
	}
	count, err := accountStore.CountAccounts(ctx)
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

	user, err := accountStore.CreateUser(ctx, dbstore.CreateUserInput{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	_, err = accountStore.CreateAccount(ctx, dbstore.CreateAccountInput{
		UserID:       user.ID,
		Username:     username,
		Email:        email,
		PasswordHash: string(hashed),
		Role:         "admin",
		DisplayName:  username,
		IsActive:     true,
		DataRoot:     cfg.Workspace.DataRoot,
	})
	if err != nil {
		return err
	}
	if emailService != nil {
		if err := emailService.EnsureDefaultGmailProvider(ctx, user.ID); err != nil {
			return fmt.Errorf("ensure admin gmail provider: %w", err)
		}
	}
	log.Info("Admin user created", slog.String("username", username))
	return nil
}

type lazyLLMClient struct {
	modelsService   *models.Service
	settingsService *settings.Service
	queries         dbstore.Queries
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
	client, err := c.resolve(ctx, req.BotID)
	if err != nil {
		return memprovider.CompactResponse{}, err
	}
	return client.Compact(ctx, req)
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
		ModelID:        memoryModel.ModelID,
		BaseURL:        strings.TrimRight(providers.ProviderConfigString(memoryProvider, "base_url"), "/"),
		APIKey:         providers.ProviderConfigString(memoryProvider, "api_key"),
		ClientType:     memoryProvider.ClientType,
		Timeout:        c.timeout,
		PromptCacheTTL: providers.ProviderConfigString(memoryProvider, "prompt_cache_ttl"),
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

func (a *mediaAssetResolverAdapter) AccessPath(ctx context.Context, asset media.Asset) string {
	if a == nil || a.media == nil {
		return ""
	}
	return a.media.AccessPath(ctx, asset)
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

func (a *gatewayAssetLoaderAdapter) AccessPathForGateway(ctx context.Context, botID, contentHash string) (string, error) {
	if a == nil || a.media == nil {
		return "", errors.New("media service not configured")
	}
	asset, err := a.media.Resolve(ctx, botID, contentHash)
	if err != nil {
		return "", err
	}
	accessPath, err := a.media.EnsureAccessPath(ctx, asset)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(accessPath), nil
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
