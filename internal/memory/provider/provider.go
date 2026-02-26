package provider

import (
	"context"

	"github.com/memohai/memoh/internal/mcp"
)

// Provider is the unified interface for memory systems. Each provider type
// (builtin, mem0, openmemory, etc.) implements this independently with its
// own storage, retrieval, and tool logic.
type Provider interface {
	// Type returns the provider type identifier (e.g. "builtin", "mem0").
	Type() string

	// --- Conversation Hooks ---

	// OnBeforeChat is called before sending to the agent gateway.
	// It returns memory context to inject into the conversation, or nil if none.
	OnBeforeChat(ctx context.Context, req BeforeChatRequest) (*BeforeChatResult, error)

	// OnAfterChat is called after receiving the gateway response.
	// It extracts facts from the conversation and stores them.
	OnAfterChat(ctx context.Context, req AfterChatRequest) error

	// --- MCP Tools ---

	// ListTools returns MCP tool descriptors provided by this memory provider.
	ListTools(ctx context.Context, session mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error)

	// CallTool executes an MCP tool owned by this memory provider.
	CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error)

	// --- CRUD ---

	Add(ctx context.Context, req AddRequest) (SearchResponse, error)
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
	GetAll(ctx context.Context, req GetAllRequest) (SearchResponse, error)
	Update(ctx context.Context, req UpdateRequest) (MemoryItem, error)
	Delete(ctx context.Context, memoryID string) (DeleteResponse, error)
	DeleteBatch(ctx context.Context, memoryIDs []string) (DeleteResponse, error)
	DeleteAll(ctx context.Context, req DeleteAllRequest) (DeleteResponse, error)

	// --- Lifecycle ---

	Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (CompactResult, error)
	Usage(ctx context.Context, filters map[string]any) (UsageResponse, error)
}
