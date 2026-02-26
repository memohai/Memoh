package provider

import "github.com/memohai/memoh/internal/memory"

// Re-export core types from the memory package so providers and callers
// use a single set of request/response types.
type (
	Message         = memory.Message
	MemoryItem      = memory.MemoryItem
	AddRequest      = memory.AddRequest
	SearchRequest   = memory.SearchRequest
	SearchResponse  = memory.SearchResponse
	GetAllRequest   = memory.GetAllRequest
	UpdateRequest   = memory.UpdateRequest
	DeleteAllRequest = memory.DeleteAllRequest
	DeleteResponse  = memory.DeleteResponse
	CompactResult   = memory.CompactResult
	UsageResponse   = memory.UsageResponse
)

// BeforeChatRequest is passed to OnBeforeChat before sending to the agent gateway.
type BeforeChatRequest struct {
	Query string
	BotID string
	ChatID string
}

// BeforeChatResult contains memory context to inject into the conversation.
type BeforeChatResult struct {
	ContextText string // formatted text to inject as a user message
}

// AfterChatRequest is passed to OnAfterChat after receiving the gateway response.
type AfterChatRequest struct {
	BotID    string
	Messages []Message
}
