package mem0

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
	Mem0Type = "mem0"

	mem0ToolSearchMemory = "search_memory"
	mem0DefaultLimit     = 10
	mem0MaxLimit         = 50
	mem0ContextMaxItems  = 8
	mem0ContextMaxChars  = 220
)

// Mem0Provider implements adapters.Provider by delegating to a Mem0 API (self-hosted or SaaS).
type Mem0Provider struct {
	client *mem0Client
	logger *slog.Logger
}

func NewMem0Provider(log *slog.Logger, config map[string]any) (*Mem0Provider, error) {
	if log == nil {
		log = slog.Default()
	}
	c, err := newMem0Client(config)
	if err != nil {
		return nil, err
	}
	return &Mem0Provider{
		client: c,
		logger: log.With(slog.String("provider", Mem0Type)),
	}, nil
}

func (*Mem0Provider) Type() string { return Mem0Type }

// --- Conversation Hooks ---

func (p *Mem0Provider) OnBeforeChat(ctx context.Context, req adapters.BeforeChatRequest) (*adapters.BeforeChatResult, error) {
	query := strings.TrimSpace(req.Query)
	botID := strings.TrimSpace(req.BotID)
	if query == "" || botID == "" {
		return nil, nil
	}
	memories, err := p.client.Search(ctx, mem0SearchRequest{
		Query:   query,
		AgentID: botID,
		Limit:   mem0ContextMaxItems,
	})
	if err != nil {
		p.logger.Warn("mem0 search for context failed", slog.Any("error", err))
		return nil, nil
	}
	if len(memories) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("<memory-context>\nRelevant memory context (use when helpful):\n")
	for i, mem := range memories {
		if i >= mem0ContextMaxItems {
			break
		}
		text := strings.TrimSpace(mem.Memory)
		if text == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(adapters.TruncateSnippet(text, mem0ContextMaxChars))
		sb.WriteString("\n")
	}
	sb.WriteString("</memory-context>")
	return &adapters.BeforeChatResult{ContextText: sb.String()}, nil
}

func (p *Mem0Provider) OnAfterChat(ctx context.Context, req adapters.AfterChatRequest) error {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" || len(req.Messages) == 0 {
		return nil
	}
	_, err := p.client.Add(ctx, mem0AddRequest{
		Messages: req.Messages,
		AgentID:  botID,
	})
	if err != nil {
		p.logger.Warn("mem0 store memory failed", slog.String("bot_id", botID), slog.Any("error", err))
	}
	return nil
}

// --- MCP Tools ---

func (p *Mem0Provider) ListTools(_ context.Context, _ mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	return []mcp.ToolDescriptor{
		{
			Name:        mem0ToolSearchMemory,
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

func (p *Mem0Provider) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != mem0ToolSearchMemory {
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
	limit := mem0DefaultLimit
	if value, ok, err := mcp.IntArg(arguments, "limit"); err != nil {
		return mcp.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = mem0DefaultLimit
	}
	if limit > mem0MaxLimit {
		limit = mem0MaxLimit
	}

	memories, err := p.client.Search(ctx, mem0SearchRequest{
		Query:   query,
		AgentID: botID,
		Limit:   limit,
	})
	if err != nil {
		return mcp.BuildToolErrorResult("memory search failed"), nil
	}

	results := make([]map[string]any, 0, len(memories))
	for _, mem := range memories {
		results = append(results, map[string]any{
			"id":     mem.ID,
			"memory": mem.Memory,
		})
	}
	return mcp.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}

// --- CRUD ---

func (p *Mem0Provider) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id is required")
	}
	addReq := mem0AddRequest{
		AgentID: botID,
		RunID:   req.RunID,
	}
	if req.Message != "" {
		addReq.Messages = []adapters.Message{{Role: "user", Content: req.Message}}
	} else {
		addReq.Messages = req.Messages
	}
	if req.Metadata != nil {
		addReq.Metadata = req.Metadata
	}
	memories, err := p.client.Add(ctx, addReq)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: mem0ToItems(memories)}, nil
}

func (p *Mem0Provider) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = mem0DefaultLimit
	}
	memories, err := p.client.Search(ctx, mem0SearchRequest{
		Query:   req.Query,
		AgentID: botID,
		RunID:   req.RunID,
		Limit:   limit,
	})
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: mem0ToItems(memories)}, nil
}

func (p *Mem0Provider) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id is required")
	}
	memories, err := p.client.GetAll(ctx, botID, req.Limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := mem0ToItems(memories)
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return adapters.SearchResponse{Results: items}, nil
}

func (p *Mem0Provider) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("memory_id is required")
	}
	mem, err := p.client.Update(ctx, memoryID, req.Memory)
	if err != nil {
		return adapters.MemoryItem{}, err
	}
	return mem0ToItem(*mem), nil
}

func (p *Mem0Provider) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	if err := p.client.Delete(ctx, strings.TrimSpace(memoryID)); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memory deleted successfully"}, nil
}

func (p *Mem0Provider) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	for _, id := range memoryIDs {
		if err := p.client.Delete(ctx, strings.TrimSpace(id)); err != nil {
			return adapters.DeleteResponse{}, err
		}
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully"}, nil
}

func (p *Mem0Provider) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
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

func (p *Mem0Provider) Compact(_ context.Context, _ map[string]any, _ float64, _ int) (adapters.CompactResult, error) {
	return adapters.CompactResult{}, errors.New("compact is not supported by mem0 provider")
}

func (p *Mem0Provider) Usage(ctx context.Context, _ map[string]any) (adapters.UsageResponse, error) {
	return adapters.UsageResponse{}, errors.New("usage is not supported by mem0 provider")
}

// --- helpers ---

func mem0ToItems(memories []mem0Memory) []adapters.MemoryItem {
	items := make([]adapters.MemoryItem, 0, len(memories))
	for _, m := range memories {
		items = append(items, mem0ToItem(m))
	}
	return items
}

func mem0ToItem(m mem0Memory) adapters.MemoryItem {
	return adapters.MemoryItem{
		ID:        m.ID,
		Memory:    m.Memory,
		Hash:      m.Hash,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		Metadata:  m.Metadata,
	}
}
