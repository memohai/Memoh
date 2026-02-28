package provider

import (
	"context"
	"time"
)

// BeforeChatRequest is passed to OnBeforeChat before sending to the agent gateway.
type BeforeChatRequest struct {
	Query  string
	BotID  string
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

// LLM is the interface for LLM operations needed by memory service
type LLM interface {
	Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error)
	Decide(ctx context.Context, req DecideRequest) (DecideResponse, error)
	Compact(ctx context.Context, req CompactRequest) (CompactResponse, error)
	DetectLanguage(ctx context.Context, text string) (string, error)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AddRequest struct {
	Message          string         `json:"message,omitempty"`
	Messages         []Message      `json:"messages,omitempty"`
	BotID            string         `json:"bot_id,omitempty"`
	AgentID          string         `json:"agent_id,omitempty"`
	RunID            string         `json:"run_id,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Filters          map[string]any `json:"filters,omitempty"`
	Infer            *bool          `json:"infer,omitempty"`
	EmbeddingEnabled *bool          `json:"embedding_enabled,omitempty"`
}

type SearchRequest struct {
	Query            string         `json:"query"`
	BotID            string         `json:"bot_id,omitempty"`
	AgentID          string         `json:"agent_id,omitempty"`
	RunID            string         `json:"run_id,omitempty"`
	Limit            int            `json:"limit,omitempty"`
	Filters          map[string]any `json:"filters,omitempty"`
	Sources          []string       `json:"sources,omitempty"`
	EmbeddingEnabled *bool          `json:"embedding_enabled,omitempty"`
	NoStats          bool           `json:"no_stats,omitempty"`
}

type UpdateRequest struct {
	MemoryID         string `json:"memory_id"`
	Memory           string `json:"memory"`
	EmbeddingEnabled *bool  `json:"embedding_enabled,omitempty"`
}

type GetAllRequest struct {
	BotID   string         `json:"bot_id,omitempty"`
	AgentID string         `json:"agent_id,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
	Limit   int            `json:"limit,omitempty"`
	Filters map[string]any `json:"filters,omitempty"`
	NoStats bool           `json:"no_stats,omitempty"`
}

type DeleteAllRequest struct {
	BotID   string         `json:"bot_id,omitempty"`
	AgentID string         `json:"agent_id,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
	Filters map[string]any `json:"filters,omitempty"`
}

type EmbedInput struct {
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
}

type EmbedUpsertRequest struct {
	Type     string         `json:"type"`
	Provider string         `json:"provider,omitempty"`
	Model    string         `json:"model,omitempty"`
	Input    EmbedInput     `json:"input"`
	Source   string         `json:"source,omitempty"`
	BotID    string         `json:"bot_id,omitempty"`
	AgentID  string         `json:"agent_id,omitempty"`
	RunID    string         `json:"run_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Filters  map[string]any `json:"filters,omitempty"`
}

type EmbedUpsertResponse struct {
	Item       MemoryItem `json:"item"`
	Provider   string     `json:"provider"`
	Model      string     `json:"model"`
	Dimensions int        `json:"dimensions"`
}

type MemoryItem struct {
	ID          string         `json:"id"`
	Memory      string         `json:"memory"`
	Hash        string         `json:"hash,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
	Score       float64        `json:"score,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	BotID       string         `json:"bot_id,omitempty"`
	AgentID     string         `json:"agent_id,omitempty"`
	RunID       string         `json:"run_id,omitempty"`
	TopKBuckets []TopKBucket   `json:"top_k_buckets,omitempty"`
	CDFCurve    []CDFPoint     `json:"cdf_curve,omitempty"`
}

// TopKBucket represents one bar in the Top-K sparse dimension bar chart.
type TopKBucket struct {
	Index uint32  `json:"index"` // sparse dimension index (term hash)
	Value float32 `json:"value"` // weight (term frequency)
}

// CDFPoint represents one point on the cumulative contribution curve.
type CDFPoint struct {
	K          int     `json:"k"`          // rank position (1-based, sorted by value desc)
	Cumulative float64 `json:"cumulative"` // cumulative weight fraction [0.0, 1.0]
}

type SearchResponse struct {
	Results   []MemoryItem `json:"results"`
	Relations []any        `json:"relations,omitempty"`
}

type DeleteResponse struct {
	Message string `json:"message"`
}

type ExtractRequest struct {
	Messages []Message      `json:"messages"`
	Filters  map[string]any `json:"filters,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ExtractResponse struct {
	Facts []string `json:"facts"`
}

type CandidateMemory struct {
	ID        string         `json:"id"`
	Memory    string         `json:"memory"`
	CreatedAt string         `json:"created_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type DecideRequest struct {
	Facts      []string          `json:"facts"`
	Candidates []CandidateMemory `json:"candidates"`
	Filters    map[string]any    `json:"filters,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

type DecisionAction struct {
	Event     string `json:"event"`
	ID        string `json:"id,omitempty"`
	Text      string `json:"text"`
	OldMemory string `json:"old_memory,omitempty"`
}

type DecideResponse struct {
	Actions []DecisionAction `json:"actions"`
}

type CompactRequest struct {
	Memories    []CandidateMemory `json:"memories"`
	TargetCount int               `json:"target_count"`
	DecayDays   int               `json:"decay_days,omitempty"`
}

type CompactResponse struct {
	Facts []string `json:"facts"`
}

type CompactResult struct {
	BeforeCount int          `json:"before_count"`
	AfterCount  int          `json:"after_count"`
	Ratio       float64      `json:"ratio"`
	Results     []MemoryItem `json:"results"`
}

type UsageResponse struct {
	Count                 int   `json:"count"`
	TotalTextBytes        int64 `json:"total_text_bytes"`
	AvgTextBytes          int64 `json:"avg_text_bytes"`
	EstimatedStorageBytes int64 `json:"estimated_storage_bytes"`
}

type RebuildResult struct {
	FsCount       int `json:"fs_count"`
	QdrantCount   int `json:"qdrant_count"`
	MissingCount  int `json:"missing_count"`
	RestoredCount int `json:"restored_count"`
}

// Memory provider admin types.
type ProviderType string

const (
	ProviderBuiltin ProviderType = "builtin"
)

type ProviderCreateRequest struct {
	Name     string         `json:"name"`
	Provider ProviderType   `json:"provider"`
	Config   map[string]any `json:"config,omitempty"`
}

type ProviderUpdateRequest struct {
	Name   *string        `json:"name,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type ProviderGetResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	Config    map[string]any `json:"config,omitempty"`
	IsDefault bool           `json:"is_default"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ProviderConfigSchema struct {
	Fields map[string]ProviderFieldSchema `json:"fields"`
}

type ProviderFieldSchema struct {
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Example     any    `json:"example,omitempty"`
}

type ProviderMeta struct {
	Provider     string               `json:"provider"`
	DisplayName  string               `json:"display_name"`
	ConfigSchema ProviderConfigSchema `json:"config_schema"`
}
