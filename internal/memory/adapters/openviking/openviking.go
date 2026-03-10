package openviking

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/mcp"
)

const (
	OpenVikingType = "openviking"

	ovToolSearchMemory = "search_memory"
	ovDefaultLimit     = 10
	ovMaxLimit         = 50
	ovContextMaxItems   = 8
	ovContextMaxChars   = 220
)

// OpenVikingProvider implements adapters.Provider by delegating to an OpenViking API (self-hosted or SaaS).
type OpenVikingProvider struct {
	client *openVikingClient
	logger *slog.Logger
}

func NewOpenVikingProvider(log *slog.Logger, config map[string]any) (*OpenVikingProvider, error) {
	if log == nil {
		log = slog.Default()
	}
	c, err := newOpenVikingClient(config)
	if err != nil {
		return nil, err
	}
	return &OpenVikingProvider{
		client: c,
		logger: log.With(slog.String("provider", OpenVikingType)),
	}, nil
}

func (*OpenVikingProvider) Type() string { return OpenVikingType }

// --- Conversation Hooks ---

func (p *OpenVikingProvider) OnBeforeChat(ctx context.Context, req adapters.BeforeChatRequest) (*adapters.BeforeChatResult, error) {
	query := strings.TrimSpace(req.Query)
	botID := strings.TrimSpace(req.BotID)
	if query == "" || botID == "" {
		return nil, nil
	}
	memories, err := p.client.Search(ctx, botID, query, ovContextMaxItems)
	if err != nil {
		p.logger.Warn("openviking search for context failed", slog.Any("error", err))
		return nil, nil
	}
	if len(memories) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("<memory-context>\nRelevant memory context (use when helpful):\n")
	for i, mem := range memories {
		if i >= ovContextMaxItems {
			break
		}
		text := strings.TrimSpace(mem.Content)
		if text == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(adapters.TruncateSnippet(text, ovContextMaxChars))
		sb.WriteString("\n")
	}
	sb.WriteString("</memory-context>")
	return &adapters.BeforeChatResult{ContextText: sb.String()}, nil
}

func (p *OpenVikingProvider) OnAfterChat(ctx context.Context, req adapters.AfterChatRequest) error {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" || len(req.Messages) == 0 {
		return nil
	}
	var parts []string
	for _, msg := range req.Messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "MESSAGE"
		}
		parts = append(parts, "["+role+"] "+content)
	}
	if len(parts) == 0 {
		return nil
	}
	_, err := p.client.Add(ctx, botID, strings.Join(parts, "\n"))
	if err != nil {
		p.logger.Warn("openviking store memory failed", slog.String("bot_id", botID), slog.Any("error", err))
	}
	return nil
}

// --- MCP Tools ---

func (p *OpenVikingProvider) ListTools(_ context.Context, _ mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	return []mcp.ToolDescriptor{
		{
			Name:        ovToolSearchMemory,
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

func (p *OpenVikingProvider) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != ovToolSearchMemory {
		return nil, mcp.ErrToolNotFound
	}
	query := mcp.StringArg(arguments, "query")
	if query == "" {
		return mcp.BuildToolErrorResult("query is required"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcp.BuildToolErrorResult("bot_id is required"), nil
	}
	limit := ovDefaultLimit
	if value, ok, err := mcp.IntArg(arguments, "limit"); err != nil {
		return mcp.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = ovDefaultLimit
	}
	if limit > ovMaxLimit {
		limit = ovMaxLimit
	}
	memories, err := p.client.Search(ctx, botID, query, limit)
	if err != nil {
		return mcp.BuildToolErrorResult("memory search failed"), nil
	}
	results := make([]map[string]any, 0, len(memories))
	for _, mem := range memories {
		results = append(results, map[string]any{
			"id":     mem.ID,
			"memory": mem.Content,
			"score":  mem.Score,
		})
	}
	return mcp.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}

// --- CRUD ---

func (p *OpenVikingProvider) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id is required")
	}
	text := strings.TrimSpace(req.Message)
	if text == "" && len(req.Messages) > 0 {
		parts := make([]string, 0, len(req.Messages))
		for _, m := range req.Messages {
			content := strings.TrimSpace(m.Content)
			if content == "" {
				continue
			}
			role := strings.ToUpper(strings.TrimSpace(m.Role))
			if role == "" {
				role = "MESSAGE"
			}
			parts = append(parts, "["+role+"] "+content)
		}
		text = strings.Join(parts, "\n")
	}
	if text == "" {
		return adapters.SearchResponse{}, errors.New("message is required")
	}
	mem, err := p.client.Add(ctx, botID, text)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: []adapters.MemoryItem{ovToItem(*mem)}}, nil
}

func (p *OpenVikingProvider) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = ovDefaultLimit
	}
	memories, err := p.client.Search(ctx, botID, req.Query, limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: ovToItems(memories)}, nil
}

func (p *OpenVikingProvider) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id is required")
	}
	memories, err := p.client.GetAll(ctx, botID, req.Limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := ovToItems(memories)
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return adapters.SearchResponse{Results: items}, nil
}

func (p *OpenVikingProvider) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("memory_id is required")
	}
	mem, err := p.client.Update(ctx, memoryID, req.Memory)
	if err != nil {
		return adapters.MemoryItem{}, err
	}
	return ovToItem(*mem), nil
}

func (p *OpenVikingProvider) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	if err := p.client.Delete(ctx, strings.TrimSpace(memoryID)); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memory deleted successfully"}, nil
}

func (p *OpenVikingProvider) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	for _, id := range memoryIDs {
		if err := p.client.Delete(ctx, strings.TrimSpace(id)); err != nil {
			return adapters.DeleteResponse{}, err
		}
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully"}, nil
}

func (p *OpenVikingProvider) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.DeleteResponse{}, errors.New("bot_id is required")
	}
	if err := p.client.DeleteAll(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "All memories deleted"}, nil
}

// --- Lifecycle ---

func (p *OpenVikingProvider) Compact(_ context.Context, _ map[string]any, _ float64, _ int) (adapters.CompactResult, error) {
	return adapters.CompactResult{}, errors.New("compact is not supported by openviking provider")
}

func (p *OpenVikingProvider) Usage(_ context.Context, _ map[string]any) (adapters.UsageResponse, error) {
	return adapters.UsageResponse{}, errors.New("usage is not supported by openviking provider")
}

// --- helpers ---

func ovToItems(memories []ovMemory) []adapters.MemoryItem {
	items := make([]adapters.MemoryItem, 0, len(memories))
	for _, m := range memories {
		items = append(items, ovToItem(m))
	}
	return items
}

func ovToItem(m ovMemory) adapters.MemoryItem {
	return adapters.MemoryItem{
		ID:        m.ID,
		Memory:    m.Content,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		Metadata:  m.Metadata,
		Score:     m.Score,
	}
}
