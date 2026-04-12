package nowledgemem

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unicode"

	"github.com/memohai/memoh/internal/mcp"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
)

const (
	NowledgeMemType = "nowledgemem"

	nmemSource           = "memoh"
	nmemToolSearchMemory = "search_memory"
	nmemDefaultLimit     = 10
	nmemMaxLimit         = 50
	nmemContextMaxItems  = 8
	nmemContextMaxChars  = 360
)

// NowledgeMemProvider implements adapters.Provider by delegating to a local Nowledge Mem instance.
type NowledgeMemProvider struct {
	client   *nmemClient
	logger   *slog.Logger
	spaceIDs sync.Map // botID → spaceID cache
}

func NewNowledgeMemProvider(log *slog.Logger, config map[string]any) (*NowledgeMemProvider, error) {
	if log == nil {
		log = slog.Default()
	}
	c, err := newNmemClient(config)
	if err != nil {
		return nil, err
	}
	return &NowledgeMemProvider{
		client: c,
		logger: log.With(slog.String("provider", NowledgeMemType)),
	}, nil
}

func (*NowledgeMemProvider) Type() string { return NowledgeMemType }

// --- Conversation Hooks ---

func (p *NowledgeMemProvider) OnBeforeChat(ctx context.Context, req adapters.BeforeChatRequest) (*adapters.BeforeChatResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, nil
	}
	spaceID, err := p.resolveSpaceID(ctx, req.BotID)
	if err != nil {
		p.logger.Warn("nowledgemem resolve space failed", slog.Any("error", err))
		return nil, nil
	}
	results, err := p.client.searchMemories(ctx, query, nmemContextMaxItems, spaceID)
	if err != nil {
		p.logger.Warn("nowledgemem search for context failed", slog.Any("error", err))
		return nil, nil
	}
	if len(results) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("<memory-context>\nRelevant memory context (use when helpful):\n")
	count := 0
	for _, sr := range results {
		if count >= nmemContextMaxItems {
			break
		}
		text := strings.TrimSpace(sr.Memory.Content)
		if text == "" {
			continue
		}
		sb.WriteString("- ")
		if t := strings.TrimSpace(sr.Memory.Time); t != "" {
			if len(t) > 10 {
				t = t[:10]
			}
			sb.WriteString("[")
			sb.WriteString(t)
			sb.WriteString("] ")
		}
		sb.WriteString(adapters.TruncateSnippet(text, nmemContextMaxChars))
		sb.WriteString("\n")
		count++
	}
	sb.WriteString("</memory-context>")
	return &adapters.BeforeChatResult{ContextText: sb.String()}, nil
}

func (p *NowledgeMemProvider) OnAfterChat(ctx context.Context, req adapters.AfterChatRequest) error {
	if len(req.Messages) == 0 {
		return nil
	}
	content := formatConversation(req.Messages, req.DisplayName, req.BotName, req.Platform, req.ConversationType, req.ConversationName)
	if content == "" {
		return nil
	}
	spaceID, err := p.resolveSpaceID(ctx, req.BotID)
	if err != nil {
		p.logger.Warn("nowledgemem resolve space failed", slog.Any("error", err))
		return nil
	}
	if _, err := p.client.addMemory(ctx, content, nmemSource, spaceID); err != nil {
		p.logger.Warn("nowledgemem store memory failed", slog.Any("error", err))
	}
	return nil
}

// --- MCP Tools ---

func (*NowledgeMemProvider) ListTools(_ context.Context, _ mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	return []mcp.ToolDescriptor{
		{
			Name:        nmemToolSearchMemory,
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

func (p *NowledgeMemProvider) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != nmemToolSearchMemory {
		return nil, mcp.ErrToolNotFound
	}
	query := mcp.StringArg(arguments, "query")
	if query == "" {
		return mcp.BuildToolErrorResult("query is required"), nil
	}
	limit := nmemDefaultLimit
	if value, ok, err := mcp.IntArg(arguments, "limit"); err != nil {
		return mcp.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = nmemDefaultLimit
	}
	if limit > nmemMaxLimit {
		limit = nmemMaxLimit
	}

	spaceID, err := p.resolveSpaceID(ctx, session.BotID)
	if err != nil {
		return mcp.BuildToolErrorResult(fmt.Sprintf("resolve space failed: %v", err)), nil
	}

	results, err := p.client.searchMemories(ctx, query, limit, spaceID)
	if err != nil {
		return mcp.BuildToolErrorResult("memory search failed"), nil
	}
	items := make([]map[string]any, 0, len(results))
	for _, sr := range results {
		items = append(items, map[string]any{
			"id":     sr.Memory.ID,
			"memory": sr.Memory.Content,
			"score":  sr.SimilarityScore,
		})
	}
	return mcp.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(items),
		"results": items,
	}), nil
}

// --- CRUD ---

func (p *NowledgeMemProvider) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	text := strings.TrimSpace(req.Message)
	if text == "" && len(req.Messages) > 0 {
		text = formatConversation(req.Messages, "", "", "", "", "")
	}
	if text == "" {
		return adapters.SearchResponse{}, errors.New("message is required")
	}
	spaceID, err := p.resolveSpaceID(ctx, req.BotID)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("resolve space: %w", err)
	}
	mem, err := p.client.addMemory(ctx, text, nmemSource, spaceID)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: []adapters.MemoryItem{nmemToItem(*mem)}}, nil
}

func (p *NowledgeMemProvider) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = nmemDefaultLimit
	} else if limit > nmemMaxLimit {
		limit = nmemMaxLimit
	}
	spaceID, err := p.resolveSpaceID(ctx, req.BotID)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("resolve space: %w", err)
	}
	results, err := p.client.searchMemories(ctx, req.Query, limit, spaceID)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := make([]adapters.MemoryItem, 0, len(results))
	for _, sr := range results {
		item := nmemToItem(sr.Memory)
		item.Score = sr.SimilarityScore
		items = append(items, item)
	}
	return adapters.SearchResponse{Results: items}, nil
}

func (*NowledgeMemProvider) GetAll(_ context.Context, _ adapters.GetAllRequest) (adapters.SearchResponse, error) {
	return adapters.SearchResponse{}, errors.New("getall is not supported by nowledgemem provider")
}

func (p *NowledgeMemProvider) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("memory_id is required")
	}
	mem, err := p.client.updateMemory(ctx, memoryID, req.Memory)
	if err != nil {
		return adapters.MemoryItem{}, err
	}
	return nmemToItem(*mem), nil
}

func (p *NowledgeMemProvider) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	if err := p.client.deleteMemory(ctx, strings.TrimSpace(memoryID)); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memory deleted successfully"}, nil
}

func (p *NowledgeMemProvider) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	for _, id := range memoryIDs {
		if err := p.client.deleteMemory(ctx, strings.TrimSpace(id)); err != nil {
			return adapters.DeleteResponse{}, err
		}
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully"}, nil
}

func (*NowledgeMemProvider) DeleteAll(_ context.Context, _ adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	return adapters.DeleteResponse{}, errors.New("deleteall is not supported by nowledgemem provider")
}

// --- Lifecycle ---

func (*NowledgeMemProvider) Compact(_ context.Context, _ map[string]any, _ float64, _ int) (adapters.CompactResult, error) {
	return adapters.CompactResult{}, errors.New("compact is not supported by nowledgemem provider")
}

func (*NowledgeMemProvider) Usage(_ context.Context, _ map[string]any) (adapters.UsageResponse, error) {
	return adapters.UsageResponse{}, errors.New("usage is not supported by nowledgemem provider")
}

// --- Helpers ---

const nmemSpacePrefix = "memoh:"

// resolveSpaceID returns the Nowledge Mem space ID for a given bot.
// It caches the mapping in a sync.Map to avoid repeated API calls.
func (p *NowledgeMemProvider) resolveSpaceID(ctx context.Context, botID string) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", errors.New("botID is required for space resolution")
	}
	if cached, ok := p.spaceIDs.Load(botID); ok {
		return cached.(string), nil
	}
	spaceName := nmemSpacePrefix + botID
	spaceID, err := p.client.ensureSpace(ctx, spaceName)
	if err != nil {
		return "", fmt.Errorf("ensure space %q: %w", spaceName, err)
	}
	p.spaceIDs.Store(botID, spaceID)
	return spaceID, nil
}

// formatConversation formats a round of messages into attributed text for storage.
// User messages are tagged with the sender's display name parsed from the YAML
// front-matter header; bot messages are tagged with [botName].
// A header line annotates platform and conversation context.
func formatConversation(messages []adapters.Message, fallbackDisplayName, botName, platform, convType, convName string) string {
	var sb strings.Builder

	// Header annotation: (Platform convType「convName」)
	header := buildContextHeader(platform, convType, convName)
	if header != "" {
		sb.WriteString(header)
	}

	for _, msg := range messages {
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}
		role := strings.TrimSpace(msg.Role)

		switch role {
		case "user":
			displayName, body := parseDisplayNameFromYAML(text)
			if displayName == "" {
				displayName = strings.TrimSpace(fallbackDisplayName)
			}
			if displayName == "" {
				displayName = "用户"
			}
			body = strings.TrimSpace(body)
			if body == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString("[")
			sb.WriteString(displayName)
			sb.WriteString("] ")
			sb.WriteString(body)

		case "assistant":
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString("[")
			sb.WriteString(botName)
			sb.WriteString("] ")
			sb.WriteString(text)
		}
	}
	return sb.String()
}

// buildContextHeader produces the platform/conversation annotation line.
// Format: (Platform convType「convName」)
func buildContextHeader(platform, convType, convName string) string {
	platform = strings.TrimSpace(platform)
	convType = strings.TrimSpace(convType)
	convName = strings.TrimSpace(convName)

	platformDisplay := capitalizeFirst(platform)
	convTypeDisplay := mapConversationType(convType)

	var sb strings.Builder
	sb.WriteString("(")
	sb.WriteString(platformDisplay)
	sb.WriteString(" ")
	sb.WriteString(convTypeDisplay)
	if convName != "" {
		sb.WriteString("「")
		sb.WriteString(convName)
		sb.WriteString("」")
	}
	sb.WriteString(")")
	return sb.String()
}

func mapConversationType(t string) string {
	switch t {
	case "group":
		return "群组"
	case "private":
		return "私聊"
	case "thread":
		return "话题"
	default:
		return t
	}
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// parseDisplayNameFromYAML extracts the display-name field from a YAML
// front-matter header (---\n...\n---\n) and returns the remaining body.
// If no valid header is found, displayName is empty and body is the original text.
func parseDisplayNameFromYAML(content string) (displayName, body string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[4:] // skip opening "---\n"
	endIdx := strings.Index(rest, "\n---\n")
	if endIdx < 0 {
		return "", content
	}
	header := rest[:endIdx]
	body = strings.TrimSpace(rest[endIdx+5:]) // skip "\n---\n"

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "display-name:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "display-name:"))
			// Strip surrounding quotes if present.
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			displayName = strings.TrimSpace(val)
			break
		}
	}
	return displayName, body
}

func nmemToItem(m nmemMemory) adapters.MemoryItem {
	item := adapters.MemoryItem{
		ID:       m.ID,
		Memory:   m.Content,
		Metadata: m.Metadata,
	}
	if m.Time != "" {
		item.CreatedAt = m.Time
		item.UpdatedAt = m.Time
	}
	return item
}
