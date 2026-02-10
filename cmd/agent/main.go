package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/adapters/telegram"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/contacts"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/embeddings"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/history"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/preauth"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/router"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/server"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/subagent"
	"github.com/memohai/memoh/internal/users"
	"github.com/memohai/memoh/internal/version"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func provideConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %v\n", err)
	}
	return cfg, nil
}

func provideLogger(cfg config.Config) *slog.Logger {
	logger.Init(cfg.Log.Level, cfg.Log.Format)
	return logger.L
}

func provideContainerdClient(lc fx.Lifecycle, runtimeConfig *boot.RuntimeConfig) (*containerd.Client, error) {
	factory := ctr.DefaultClientFactory{SocketPath: runtimeConfig.ContainerdSocketPath}
	client, err := factory.New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("connect containerd: %w", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if err := client.Close(); err != nil {
				return fmt.Errorf("close containerd client: %w", err)
			}
			return nil
		},
	})
	return client, nil
}

func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			boot.ProvideRuntimeConfig,
			provideLogger,

			// misc
			provideContainerdClient,
			provideDBConn,
			provideDBQueries,

			fx.Annotate(ctr.NewDefaultService, fx.As(new(ctr.Service))),
			mcp.NewManager,

			provideMemoryLLM,
			provideEmbeddingsResolver,
			provideEmbeddingSetup,
			provideTextEmbedderForMemory,
			provideQdrantStore,
			memory.NewBM25Indexer,
			provideChatResolver,
			local.NewSessionHub,
			provideChannelRegistry,

			provideChannelRouter,
			provideChannelManager,

			chat.NewScheduleGateway,
			fx.Annotate(func(scheduleGateway *chat.ScheduleGateway) schedule.Triggerer {
				return scheduleGateway
			}, fx.As(new(schedule.Triggerer))),

			models.NewService,
			bots.NewService,
			users.NewService,
			providers.NewService,
			settings.NewService,
			history.NewService,
			contacts.NewService,
			preauth.NewService,
			mcp.NewConnectionService,
			subagent.NewService,
			schedule.NewService,
			channel.NewService,
			policy.NewService,
			provideMemoryService,

			provideServerHandler(handlers.NewPingHandler),
			provideServerHandler(handlers.NewAuthHandler),
			provideServerHandler(handlers.NewMemoryHandler),
			provideServerHandler(handlers.NewEmbeddingsHandler),
			provideServerHandler(handlers.NewChatHandler),
			provideServerHandler(handlers.NewSwaggerHandler),
			provideServerHandler(handlers.NewProvidersHandler),
			provideServerHandler(handlers.NewModelsHandler),
			provideServerHandler(handlers.NewSettingsHandler),
			provideServerHandler(handlers.NewHistoryHandler),
			provideServerHandler(handlers.NewContactsHandler),
			provideServerHandler(handlers.NewPreauthHandler),
			provideServerHandler(handlers.NewScheduleHandler),
			provideServerHandler(handlers.NewSubagentHandler),
			handlers.NewContainerdHandler,
			provideServerHandler(handlers.NewContainerdHandler),
			provideServerHandler(handlers.NewChannelHandler),
			provideServerHandler(handlers.NewUsersHandler),
			provideServerHandler(handlers.NewMCPHandler),
			provideServerHandler(provideCLIHandler),
			provideServerHandler(provideWebHandler),

			provideServer,
		),
		fx.Invoke(
			startMemoryWarmup,
			startScheduleService,
			startChannelManager,
			startServer,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			l := &fxevent.SlogLogger{Logger: logger.With(slog.String("component", "fx"))}
			// l.UseLogLevel(slog.LevelInfo)
			return l
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

func provideDBConn(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	ctx := context.Background() // TODO: use timeout context

	conn, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			conn.Close()
			return nil
		},
	})
	return conn, nil
}

func provideDBQueries(conn *pgxpool.Pool) *dbsqlc.Queries {
	return dbsqlc.New(conn)
}

func provideEmbeddingsResolver(log *slog.Logger, modelsService *models.Service, queries *dbsqlc.Queries) *embeddings.Resolver {
	return embeddings.NewResolver(log, modelsService, queries, 10*time.Second)
}

type embeddingSetup struct {
	Vectors            map[string]int
	TextModel          models.GetResponse
	MultimodalModel    models.GetResponse
	HasEmbeddingModels bool
}

func provideEmbeddingSetup(log *slog.Logger, modelsService *models.Service) (embeddingSetup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vectors, textModel, multimodalModel, hasEmbeddingModels, err := embeddings.CollectEmbeddingVectors(ctx, modelsService)
	if err != nil {
		return embeddingSetup{}, fmt.Errorf("embedding models: %w", err)
	}
	if hasEmbeddingModels && multimodalModel.ModelID == "" {
		log.Warn("No multimodal embedding model configured. Multimodal embedding features will be limited.")
	}
	return embeddingSetup{
		Vectors:            vectors,
		TextModel:          textModel,
		MultimodalModel:    multimodalModel,
		HasEmbeddingModels: hasEmbeddingModels,
	}, nil
}

func provideTextEmbedderForMemory(resolver *embeddings.Resolver, setup embeddingSetup, log *slog.Logger) embeddings.Embedder {
	return buildTextEmbedder(resolver, setup.TextModel, setup.HasEmbeddingModels, log)
}

func provideMemoryService(log *slog.Logger, llm memory.LLM, embedder embeddings.Embedder, store *memory.QdrantStore, resolver *embeddings.Resolver, bm25Indexer *memory.BM25Indexer, setup embeddingSetup) *memory.Service {
	return memory.NewService(log, llm, embedder, store, resolver, bm25Indexer, setup.TextModel.ModelID, setup.MultimodalModel.ModelID)
}

func provideChatResolver(log *slog.Logger, cfg config.Config, modelsService *models.Service, queries *dbsqlc.Queries, memoryService *memory.Service, historyService *history.Service, settingsService *settings.Service, mcpConnectionsService *mcp.ConnectionService, containerdHandler *handlers.ContainerdHandler) *chat.Resolver {
	chatResolver := chat.NewResolver(log, modelsService, queries, memoryService, historyService, settingsService, mcpConnectionsService, cfg.AgentGateway.BaseURL(), 120*time.Second)
	chatResolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	return chatResolver
}

func provideChannelRegistry(log *slog.Logger, sessionHub *local.SessionHub) *channel.Registry {
	registry := channel.NewRegistry()
	registry.MustRegister(telegram.NewTelegramAdapter(log))
	registry.MustRegister(feishu.NewFeishuAdapter(log))
	registry.MustRegister(local.NewCLIAdapter(sessionHub))
	registry.MustRegister(local.NewWebAdapter(sessionHub))
	return registry
}

func provideChannelRouter(log *slog.Logger, registry *channel.Registry, channelService *channel.Service, chatResolver *chat.Resolver, contactsService *contacts.Service, policyService *policy.Service, preauthService *preauth.Service, cfg config.Config) *router.ChannelInboundProcessor {
	return router.NewChannelInboundProcessor(log, registry, channelService, chatResolver, contactsService, policyService, preauthService, cfg.Auth.JWTSecret, 5*time.Minute)
}

func provideChannelManager(log *slog.Logger, registry *channel.Registry, channelService *channel.Service, channelRouter *router.ChannelInboundProcessor) *channel.Manager {
	channelManager := channel.NewManager(log, registry, channelService, channelRouter)
	if mw := channelRouter.IdentityMiddleware(); mw != nil {
		channelManager.Use(mw)
	}
	return channelManager
}

func provideCLIHandler(channelManager *channel.Manager, channelService *channel.Service, sessionHub *local.SessionHub, botService *bots.Service, usersService *users.Service) *handlers.LocalChannelHandler {
	return handlers.NewLocalChannelHandler(local.CLIType, channelManager, channelService, sessionHub, botService, usersService)
}

func provideWebHandler(channelManager *channel.Manager, channelService *channel.Service, sessionHub *local.SessionHub, botService *bots.Service, usersService *users.Service) *handlers.LocalChannelHandler {
	return handlers.NewLocalChannelHandler(local.WebType, channelManager, channelService, sessionHub, botService, usersService)
}

type serverParams struct {
	fx.In

	Logger         *slog.Logger
	RuntimeConfig  *boot.RuntimeConfig
	Config         config.Config
	ServerHandlers []server.Handler `group:"server_handlers"`
}

func provideServer(params serverParams) *server.Server {
	return server.NewServer(params.Logger, params.RuntimeConfig.ServerAddr, params.Config.Auth.JWTSecret, params.ServerHandlers...)
}

func startMemoryWarmup(lc fx.Lifecycle, memoryService *memory.Service, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := memoryService.WarmupBM25(context.Background(), 200); err != nil {
					logger.Warn("bm25 warmup failed", slog.Any("error", err))
				}
			}()
			return nil
		},
	})

}

func startChannelManager(lc fx.Lifecycle, channelManager *channel.Manager, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			channelManager.Start(ctx)
			return nil
		},
	})
}

func startScheduleService(lc fx.Lifecycle, scheduleService *schedule.Service, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return scheduleService.Bootstrap(ctx)
		},
	})
}

func startServer(
	lc fx.Lifecycle,
	logger *slog.Logger,
	srv *server.Server,
	shutdowner fx.Shutdowner,
	cfg config.Config,
	queries *dbsqlc.Queries,
	scheduleService *schedule.Service,
	channelManager *channel.Manager,
	botService *bots.Service,
	containerdHandler *handlers.ContainerdHandler,
) {
	fmt.Printf("Starting Memoh Agent %s\n", version.GetInfo())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {

			if err := ensureAdminUser(ctx, logger, queries, cfg); err != nil {
				return err
			}

			botService.SetContainerLifecycle(containerdHandler)

			go func() {
				if err := srv.Start(); err != nil { // block until server is stopped
					logger.Error("server failed", slog.Any("error", err))
					_ = shutdowner.Shutdown() // shutdown the application if the server fails to start
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// graceful shutdown
			if err := srv.Stop(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("server stop: %w", err)
			}
			return nil
		},
	})
}

func buildTextEmbedder(resolver *embeddings.Resolver, textModel models.GetResponse, hasModels bool, log *slog.Logger) embeddings.Embedder {
	if !hasModels {
		return nil
	}
	if textModel.ModelID == "" || textModel.Dimensions <= 0 {
		log.Warn("No text embedding model configured. Text embedding features will be limited.")
		return nil
	}
	return &embeddings.ResolverTextEmbedder{
		Resolver: resolver,
		ModelID:  textModel.ModelID,
		Dims:     textModel.Dimensions,
	}
}

func provideQdrantStore(log *slog.Logger, cfgAll config.Config, setup embeddingSetup) (*memory.QdrantStore, error) {
	cfg := cfgAll.Qdrant
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if setup.HasEmbeddingModels && len(setup.Vectors) > 0 {
		store, err := memory.NewQdrantStoreWithVectors(
			log,
			cfg.BaseURL,
			cfg.APIKey,
			cfg.Collection,
			setup.Vectors,
			"sparse_hash",
			timeout,
		)
		if err != nil {
			return nil, fmt.Errorf("qdrant named vectors init: %w", err)
		}
		return store, nil
	}
	store, err := memory.NewQdrantStore(
		log,
		cfg.BaseURL,
		cfg.APIKey,
		cfg.Collection,
		setup.TextModel.Dimensions,
		"sparse_hash",
		timeout,
	)
	if err != nil {
		return nil, fmt.Errorf("qdrant init: %w", err)
	}
	return store, nil
}

func ensureAdminUser(ctx context.Context, log *slog.Logger, queries *dbsqlc.Queries, cfg config.Config) error {
	if queries == nil {
		return fmt.Errorf("db queries not configured")
	}
	count, err := queries.CountUsers(ctx)
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
		return fmt.Errorf("admin username/password required in config.toml")
	}
	if password == "change-your-password-here" {
		log.Warn("admin password uses default placeholder; please update config.toml")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	emailValue := pgtype.Text{Valid: false}
	if email != "" {
		emailValue = pgtype.Text{String: email, Valid: true}
	}
	displayName := pgtype.Text{String: username, Valid: true}
	dataRoot := pgtype.Text{String: cfg.MCP.DataRoot, Valid: cfg.MCP.DataRoot != ""}

	_, err = queries.CreateUser(ctx, dbsqlc.CreateUserParams{
		Username:     username,
		Email:        emailValue,
		PasswordHash: string(hashed),
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

func provideMemoryLLM(modelsService *models.Service, queries *dbsqlc.Queries, log *slog.Logger) memory.LLM {
	return &lazyLLMClient{
		modelsService: modelsService,
		queries:       queries,
		timeout:       30 * time.Second,
		logger:        log,
	}
}

type lazyLLMClient struct {
	modelsService *models.Service
	queries       *dbsqlc.Queries
	timeout       time.Duration
	logger        *slog.Logger
}

func (c *lazyLLMClient) Extract(ctx context.Context, req memory.ExtractRequest) (memory.ExtractResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memory.ExtractResponse{}, err
	}
	return client.Extract(ctx, req)
}

func (c *lazyLLMClient) Decide(ctx context.Context, req memory.DecideRequest) (memory.DecideResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return memory.DecideResponse{}, err
	}
	return client.Decide(ctx, req)
}

func (c *lazyLLMClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return "", err
	}
	return client.DetectLanguage(ctx, text)
}

func (c *lazyLLMClient) resolve(ctx context.Context) (memory.LLM, error) {
	if c.modelsService == nil || c.queries == nil {
		return nil, fmt.Errorf("models service not configured")
	}
	memoryModel, memoryProvider, err := models.SelectMemoryModel(ctx, c.modelsService, c.queries)
	if err != nil {
		return nil, err
	}
	clientType := strings.ToLower(strings.TrimSpace(memoryProvider.ClientType))
	if clientType != "openai" && clientType != "openai-compat" {
		return nil, fmt.Errorf("memory provider client type not supported: %s", memoryProvider.ClientType)
	}
	return memory.NewLLMClient(c.logger, memoryProvider.BaseUrl, memoryProvider.ApiKey, memoryModel.ModelID, c.timeout)
}

// skillLoaderAdapter bridges handlers.ContainerdHandler to chat.SkillLoader.
type skillLoaderAdapter struct {
	handler *handlers.ContainerdHandler
}

func (a *skillLoaderAdapter) LoadSkills(ctx context.Context, botID string) ([]chat.SkillEntry, error) {
	items, err := a.handler.LoadSkills(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries := make([]chat.SkillEntry, len(items))
	for i, item := range items {
		entries[i] = chat.SkillEntry{
			Name:        item.Name,
			Description: item.Description,
			Content:     item.Content,
			Metadata:    item.Metadata,
		}
	}
	return entries, nil
}
