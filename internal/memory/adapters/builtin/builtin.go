package builtin

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/mcp"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
)

const (
	BuiltinType = "builtin"

	sharedMemoryNamespace = "bot"

	defaultMemoryToolLimit = 8
	maxMemoryToolLimit     = 50
	toolSearchMemory       = "search_memory"
)

// BuiltinProvider wraps the existing Service as a Provider.
type BuiltinProvider struct {
	service      memoryRuntime
	llm          adapters.LLM
	chatAccessor conversation.Accessor
	adminChecker AdminChecker
	logger       *slog.Logger
	packer       contextPackerConfig
}

// memoryRuntime is the runtime memory backend required by the builtin provider.
// It is intentionally defined as an interface to decouple provider wiring from
// concrete service structs in the memory package.
type memoryRuntime interface {
	Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error)
	Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error)
	GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error)
	Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error)
	Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error)
	DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error)
	DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error)
	Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (adapters.CompactResult, error)
	Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error)
	Mode() string
	Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error)
	Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error)
}

// AdminChecker checks whether a channel identity has admin privileges.
type AdminChecker interface {
	IsAdmin(ctx context.Context, channelIdentityID string) (bool, error)
}

func NewBuiltinProvider(log *slog.Logger, service any, chatAccessor conversation.Accessor, adminChecker AdminChecker) *BuiltinProvider {
	if log == nil {
		log = slog.Default()
	}
	logger := log.With(slog.String("provider", BuiltinType))
	runtimeService, ok := service.(memoryRuntime)
	if service != nil && !ok {
		logger.Warn("service does not implement memoryRuntime; provider will operate without a backend")
	}
	return &BuiltinProvider{
		service:      runtimeService,
		chatAccessor: chatAccessor,
		adminChecker: adminChecker,
		logger:       logger,
		packer:       defaultPackerConfig,
	}
}

// SetLLM injects the LLM client used for Extract/Decide in memory formation.
func (p *BuiltinProvider) SetLLM(llm adapters.LLM) {
	p.llm = llm
}

// SetPackerConfig overrides the default context packing configuration.
// Zero-valued fields fall back to defaults.
func (p *BuiltinProvider) SetPackerConfig(cfg contextPackerConfig) {
	if cfg.TargetItems > 0 {
		p.packer.TargetItems = cfg.TargetItems
	}
	if cfg.MaxTotalChars > 0 {
		p.packer.MaxTotalChars = cfg.MaxTotalChars
	}
	if cfg.MinItemChars > 0 {
		p.packer.MinItemChars = cfg.MinItemChars
	}
	if cfg.MaxItemChars > 0 {
		p.packer.MaxItemChars = cfg.MaxItemChars
	}
	if cfg.OverfetchRatio > 0 {
		p.packer.OverfetchRatio = cfg.OverfetchRatio
	}
}

// ApplyProviderConfig reads context packing knobs from a provider config map
// and applies any non-zero values to the provider's packer configuration.
func (p *BuiltinProvider) ApplyProviderConfig(providerConfig map[string]any) {
	p.SetPackerConfig(contextPackerConfig{
		TargetItems:   intFromConfig(providerConfig, "context_target_items"),
		MaxTotalChars: intFromConfig(providerConfig, "context_max_total_chars"),
	})
}

func intFromConfig(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

func (*BuiltinProvider) Type() string { return BuiltinType }

func memorySourceLabel(item adapters.MemoryItem) string {
	var parts []string
	if item.Metadata != nil {
		if name, ok := item.Metadata["profile_display_name"].(string); ok {
			name = strings.TrimSpace(name)
			if name != "" {
				parts = append(parts, name)
			}
		}
	}
	if ts := strings.TrimSpace(item.CreatedAt); ts != "" {
		if len(ts) > 10 {
			ts = ts[:10]
		}
		parts = append(parts, ts)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// --- Conversation Hooks ---

func (p *BuiltinProvider) OnBeforeChat(ctx context.Context, req adapters.BeforeChatRequest) (*adapters.BeforeChatResult, error) {
	if p.service == nil {
		return nil, nil
	}
	if strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.BotID) == "" {
		return nil, nil
	}

	fetchLimit := overfetchLimit(p.packer)
	resp, err := p.service.Search(ctx, adapters.SearchRequest{
		Query: req.Query,
		BotID: req.BotID,
		Limit: fetchLimit,
		Filters: map[string]any{
			"namespace": sharedMemoryNamespace,
			"scopeId":   req.BotID,
			"bot_id":    req.BotID,
		},
		NoStats: true,
	})
	if err != nil {
		p.logger.Warn("memory search for context failed", slog.Any("error", err))
		return nil, nil
	}

	candidates := deduplicateAndSort(resp.Results)
	if len(candidates) == 0 {
		return nil, nil
	}

	packed := packContext(candidates, p.packer)
	if len(packed.Items) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("<memory-context>\nRelevant memory context (use when helpful):\n")
	for _, entry := range packed.Items {
		sb.WriteString("- ")
		if label := memorySourceLabel(entry.Item); label != "" {
			sb.WriteString("[")
			sb.WriteString(label)
			sb.WriteString("] ")
		}
		sb.WriteString(entry.Snippet)
		sb.WriteString("\n")
	}
	sb.WriteString("</memory-context>")
	payload := strings.TrimSpace(sb.String())
	if payload == "" {
		return nil, nil
	}
	return &adapters.BeforeChatResult{ContextText: payload}, nil
}

func (p *BuiltinProvider) OnAfterChat(ctx context.Context, req adapters.AfterChatRequest) error {
	if p.service == nil {
		return nil
	}
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return nil
	}
	if len(req.Messages) == 0 {
		return nil
	}

	if p.llm != nil {
		result := runFormation(ctx, p.logger, p.llm, p.service, req)
		p.logger.Debug("memory formation completed",
			slog.String("bot_id", botID),
			slog.Int("extracted", result.ExtractedFacts),
			slog.Int("added", result.Added),
			slog.Int("updated", result.Updated),
			slog.Int("deleted", result.Deleted),
			slog.Int("skipped", result.Skipped),
		)
		return nil
	}

	// Fallback: no LLM configured, store raw transcript (legacy path).
	filters := map[string]any{
		"namespace": sharedMemoryNamespace,
		"scopeId":   botID,
		"bot_id":    botID,
	}
	metadata := adapters.BuildProfileMetadata(req.UserID, req.ChannelIdentityID, req.DisplayName)
	if _, err := p.service.Add(ctx, adapters.AddRequest{
		Messages: req.Messages,
		BotID:    botID,
		Metadata: metadata,
		Filters:  filters,
	}); err != nil {
		p.logger.Warn("store memory failed", slog.String("bot_id", botID), slog.Any("error", err))
	}
	return nil
}

// --- MCP Tools ---

func (p *BuiltinProvider) ListTools(_ context.Context, _ mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	if p.service == nil {
		return []mcp.ToolDescriptor{}, nil
	}
	return []mcp.ToolDescriptor{
		{
			Name:        toolSearchMemory,
			Description: "Search for memories relevant to the current chat",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The query to search memories",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of memory results",
					},
				},
				"required": []string{"query"},
			},
		},
	}, nil
}

func (p *BuiltinProvider) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolSearchMemory {
		return nil, mcp.ErrToolNotFound
	}
	if p.service == nil {
		return mcp.BuildToolErrorResult("memory service not available"), nil
	}

	query := mcp.StringArg(arguments, "query")
	if query == "" {
		return mcp.BuildToolErrorResult("query is required"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcp.BuildToolErrorResult("bot_id is required"), nil
	}
	chatID := strings.TrimSpace(session.ChatID)
	if chatID == "" {
		chatID = botID
	}

	limit := defaultMemoryToolLimit
	if value, ok, err := mcp.IntArg(arguments, "limit"); err != nil {
		return mcp.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = defaultMemoryToolLimit
	}
	if limit > maxMemoryToolLimit {
		limit = maxMemoryToolLimit
	}

	if chatID != botID {
		if p.chatAccessor == nil {
			return mcp.BuildToolErrorResult("chat service not available"), nil
		}
		chatObj, err := p.chatAccessor.Get(ctx, chatID)
		if err != nil {
			return mcp.BuildToolErrorResult("chat not found"), nil
		}
		if strings.TrimSpace(chatObj.BotID) != botID {
			return mcp.BuildToolErrorResult("bot mismatch"), nil
		}
		channelIdentityID := strings.TrimSpace(session.ChannelIdentityID)
		if channelIdentityID != "" {
			allowed, err := p.canAccessChat(ctx, chatID, channelIdentityID)
			if err != nil {
				return mcp.BuildToolErrorResult(err.Error()), nil
			}
			if !allowed {
				return mcp.BuildToolErrorResult("not a chat participant"), nil
			}
		}
	}

	resp, err := p.service.Search(ctx, adapters.SearchRequest{
		Query: query,
		BotID: botID,
		Limit: limit,
		Filters: map[string]any{
			"namespace": sharedMemoryNamespace,
			"scopeId":   botID,
			"bot_id":    botID,
		},
		NoStats: true,
	})
	if err != nil {
		return mcp.BuildToolErrorResult("memory search failed"), nil
	}

	allResults := adapters.DeduplicateItems(resp.Results)
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	results := make([]map[string]any, 0, len(allResults))
	for _, item := range allResults {
		results = append(results, map[string]any{
			"id":     item.ID,
			"memory": item.Memory,
			"score":  item.Score,
		})
	}

	return mcp.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}

func (p *BuiltinProvider) canAccessChat(ctx context.Context, chatID, channelIdentityID string) (bool, error) {
	if p.adminChecker != nil {
		isAdmin, err := p.adminChecker.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return false, err
		}
		if isAdmin {
			return true, nil
		}
	}
	if p.chatAccessor == nil {
		return false, errors.New("chat service not available")
	}
	return p.chatAccessor.IsParticipant(ctx, chatID, channelIdentityID)
}

// --- CRUD ---

func (p *BuiltinProvider) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	if p.service == nil {
		return adapters.SearchResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.Add(ctx, req)
}

func (p *BuiltinProvider) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	if p.service == nil {
		return adapters.SearchResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.Search(ctx, req)
}

func (p *BuiltinProvider) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	if p.service == nil {
		return adapters.SearchResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.GetAll(ctx, req)
}

func (p *BuiltinProvider) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	if p.service == nil {
		return adapters.MemoryItem{}, errors.New("memory runtime not configured")
	}
	return p.service.Update(ctx, req)
}

func (p *BuiltinProvider) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	if p.service == nil {
		return adapters.DeleteResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.Delete(ctx, memoryID)
}

func (p *BuiltinProvider) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	if p.service == nil {
		return adapters.DeleteResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.DeleteBatch(ctx, memoryIDs)
}

func (p *BuiltinProvider) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	if p.service == nil {
		return adapters.DeleteResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.DeleteAll(ctx, req)
}

func (p *BuiltinProvider) Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (adapters.CompactResult, error) {
	if p.service == nil {
		return adapters.CompactResult{}, errors.New("memory runtime not configured")
	}
	return p.service.Compact(ctx, filters, ratio, decayDays)
}

func (p *BuiltinProvider) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
	if p.service == nil {
		return adapters.UsageResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.Usage(ctx, filters)
}

func (p *BuiltinProvider) Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error) {
	if p.service == nil {
		return adapters.MemoryStatusResponse{}, errors.New("memory runtime not configured")
	}
	return p.service.Status(ctx, botID)
}

func (p *BuiltinProvider) Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error) {
	if p.service == nil {
		return adapters.RebuildResult{}, errors.New("memory runtime not configured")
	}
	return p.service.Rebuild(ctx, botID)
}
