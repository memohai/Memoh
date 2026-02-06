package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

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

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	fmt.Printf("Starting Memoh Agent %s\n", version.GetInfo())
	ctx := context.Background()
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format)

	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		logger.Error("jwt secret is required")
		os.Exit(1)
	}
	jwtExpiresIn, err := time.ParseDuration(cfg.Auth.JWTExpiresIn)
	if err != nil {
		logger.Error("invalid jwt expires in", slog.Any("error", err))
		os.Exit(1)
	}

	addr := cfg.Server.Addr
	if value := os.Getenv("HTTP_ADDR"); value != "" {
		addr = value
	}

	socketPath := cfg.Containerd.SocketPath
	if value := os.Getenv("CONTAINERD_SOCKET"); value != "" {
		socketPath = value
	}
	factory := ctr.DefaultClientFactory{SocketPath: socketPath}
	client, err := factory.New(ctx)
	if err != nil {
		logger.Error("connect containerd", slog.Any("error", err))
		os.Exit(1)
	}
	defer client.Close()

	service := ctr.NewDefaultService(logger.L, client, cfg.Containerd.Namespace)
	manager := mcp.NewManager(logger.L, service, cfg.MCP)

	pingHandler := handlers.NewPingHandler(logger.L)
	// containerdHandler is created later after DB services are initialized

	conn, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Error("db connect", slog.Any("error", err))
		os.Exit(1)
	}
	defer conn.Close()
	manager.WithDB(conn)
	queries := dbsqlc.New(conn)
	modelsService := models.NewService(logger.L, queries)
	botService := bots.NewService(logger.L, queries)
	usersService := users.NewService(logger.L, queries)

	containerdHandler := handlers.NewContainerdHandler(logger.L, service, cfg.MCP, cfg.Containerd.Namespace, botService, usersService, queries)

	if err := ensureAdminUser(ctx, logger.L, queries, cfg); err != nil {
		logger.Error("ensure admin user", slog.Any("error", err))
		os.Exit(1)
	}

	authHandler := handlers.NewAuthHandler(logger.L, usersService, cfg.Auth.JWTSecret, jwtExpiresIn)

	// Initialize chat resolver after memory service is configured.
	var chatResolver *chat.Resolver

	// Create LLM client for memory operations (deferred model/provider selection).
	var llmClient memory.LLM = &lazyLLMClient{
		modelsService: modelsService,
		queries:       queries,
		timeout:       30 * time.Second,
		logger:        logger.L,
	}

	resolver := embeddings.NewResolver(logger.L, modelsService, queries, 10*time.Second)
	vectors, textModel, multimodalModel, hasEmbeddingModels, err := embeddings.CollectEmbeddingVectors(ctx, modelsService)
	if err != nil {
		logger.Error("embedding models", slog.Any("error", err))
		os.Exit(1)
	}

	textEmbedder := buildTextEmbedder(resolver, textModel, hasEmbeddingModels, logger.L)
	if hasEmbeddingModels && multimodalModel.ModelID == "" {
		logger.Warn("No multimodal embedding model configured. Multimodal embedding features will be limited.")
	}
	store := buildQdrantStore(logger.L, cfg.Qdrant, vectors, hasEmbeddingModels, textModel.Dimensions)

	bm25Indexer := memory.NewBM25Indexer(logger.L)
	memoryService := memory.NewService(logger.L, llmClient, textEmbedder, store, resolver, bm25Indexer, textModel.ModelID, multimodalModel.ModelID)
	memoryHandler := handlers.NewMemoryHandler(logger.L, memoryService, botService, usersService)
	go func() {
		if err := memoryService.WarmupBM25(ctx, 200); err != nil {
			logger.Warn("bm25 warmup failed", slog.Any("error", err))
		}
	}()

	// Initialize providers and models handlers
	providersService := providers.NewService(logger.L, queries)
	providersHandler := handlers.NewProvidersHandler(logger.L, providersService, modelsService)
	settingsService := settings.NewService(logger.L, queries)
	settingsHandler := handlers.NewSettingsHandler(logger.L, settingsService, botService, usersService)
	modelsHandler := handlers.NewModelsHandler(logger.L, modelsService, settingsService)
	policyService := policy.NewService(logger.L, botService, settingsService)
	historyService := history.NewService(logger.L, queries)
	historyHandler := handlers.NewHistoryHandler(logger.L, historyService, botService, usersService)
	contactsService := contacts.NewService(queries)
	contactsHandler := handlers.NewContactsHandler(contactsService, botService, usersService)
	preauthService := preauth.NewService(queries)
	preauthHandler := handlers.NewPreauthHandler(preauthService, botService, usersService)

	chatResolver = chat.NewResolver(logger.L, modelsService, queries, memoryService, historyService, settingsService, cfg.AgentGateway.BaseURL(), 120*time.Second)
	embeddingsHandler := handlers.NewEmbeddingsHandler(logger.L, modelsService, queries)
	swaggerHandler := handlers.NewSwaggerHandler(logger.L)
	chatHandler := handlers.NewChatHandler(logger.L, chatResolver, botService, usersService)
	channelService := channel.NewService(queries)
	channelRouter := router.NewChannelInboundProcessor(logger.L, channelService, chatResolver, contactsService, policyService, preauthService, cfg.Auth.JWTSecret, 5*time.Minute)
	channelManager := channel.NewManager(logger.L, channelService, channelRouter)
	if mw := channelRouter.IdentityMiddleware(); mw != nil {
		channelManager.Use(mw)
	}
	sessionHub := channel.NewSessionHub()
	channelManager.RegisterAdapter(telegram.NewTelegramAdapter(logger.L))
	channelManager.RegisterAdapter(feishu.NewFeishuAdapter(logger.L))
	channelManager.RegisterAdapter(local.NewCLIAdapter(sessionHub))
	channelManager.RegisterAdapter(local.NewWebAdapter(sessionHub))
	channelManager.Start(ctx)
	channelHandler := handlers.NewChannelHandler(channelService)
	usersHandler := handlers.NewUsersHandler(logger.L, usersService, botService, channelService, channelManager)
	cliHandler := handlers.NewLocalChannelHandler(local.CLIType, channelManager, channelService, sessionHub, botService, usersService)
	webHandler := handlers.NewLocalChannelHandler(local.WebType, channelManager, channelService, sessionHub, botService, usersService)
	scheduleGateway := chat.NewScheduleGateway(chatResolver)
	scheduleService := schedule.NewService(logger.L, queries, scheduleGateway, cfg.Auth.JWTSecret)
	if err := scheduleService.Bootstrap(ctx); err != nil {
		logger.Error("schedule bootstrap", slog.Any("error", err))
		os.Exit(1)
	}
	scheduleHandler := handlers.NewScheduleHandler(logger.L, scheduleService, botService, usersService)
	subagentService := subagent.NewService(logger.L, queries)
	subagentHandler := handlers.NewSubagentHandler(logger.L, subagentService, botService, usersService)
	srv := server.NewServer(logger.L, addr, cfg.Auth.JWTSecret, pingHandler, authHandler, memoryHandler, embeddingsHandler, chatHandler, swaggerHandler, providersHandler, modelsHandler, settingsHandler, historyHandler, contactsHandler, preauthHandler, scheduleHandler, subagentHandler, containerdHandler, channelHandler, usersHandler, cliHandler, webHandler)

	if err := srv.Start(); err != nil {
		logger.Error("server failed", slog.Any("error", err))
		os.Exit(1)
	}
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

func buildQdrantStore(log *slog.Logger, cfg config.QdrantConfig, vectors map[string]int, hasModels bool, textDims int) *memory.QdrantStore {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if hasModels && len(vectors) > 0 {
		store, err := memory.NewQdrantStoreWithVectors(
			log,
			cfg.BaseURL,
			cfg.APIKey,
			cfg.Collection,
			vectors,
			"sparse_hash",
			timeout,
		)
		if err != nil {
			log.Error("qdrant named vectors init", slog.Any("error", err))
			os.Exit(1)
		}
		return store
	}
	store, err := memory.NewQdrantStore(
		log,
		cfg.BaseURL,
		cfg.APIKey,
		cfg.Collection,
		textDims,
		"sparse_hash",
		timeout,
	)
	if err != nil {
		log.Error("qdrant init", slog.Any("error", err))
		os.Exit(1)
	}
	return store
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
	return memory.NewLLMClient(c.logger, memoryProvider.BaseUrl, memoryProvider.ApiKey, memoryModel.ModelID, c.timeout), nil
}
