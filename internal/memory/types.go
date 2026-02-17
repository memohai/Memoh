package memory

import "context"

// LLM is the interface for LLM operations needed by memory service.
type LLM interface {
	Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error)
	Decide(ctx context.Context, req DecideRequest) (DecideResponse, error)
	Compact(ctx context.Context, req CompactRequest) (CompactResponse, error)
	DetectLanguage(ctx context.Context, text string) (string, error)
}

// Message is a single role/content pair for memory LLM input.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AddRequest is the input for adding memories (message(s), scope filters, optional infer/embedding).
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

// SearchRequest is the input for memory search (query, scope, limit, sources, embedding flag).
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

// UpdateRequest is the input for updating a single memory by ID.
type UpdateRequest struct {
	MemoryID         string `json:"memory_id"`
	Memory           string `json:"memory"`
	EmbeddingEnabled *bool  `json:"embedding_enabled,omitempty"`
}

// GetAllRequest is the input for listing memories by scope (bot/agent/run, limit, filters).
type GetAllRequest struct {
	BotID   string         `json:"bot_id,omitempty"`
	AgentID string         `json:"agent_id,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
	Limit   int            `json:"limit,omitempty"`
	Filters map[string]any `json:"filters,omitempty"`
	NoStats bool           `json:"no_stats,omitempty"`
}

// DeleteAllRequest is the input for deleting memories by scope and optional filters.
type DeleteAllRequest struct {
	BotID   string         `json:"bot_id,omitempty"`
	AgentID string         `json:"agent_id,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
	Filters map[string]any `json:"filters,omitempty"`
}

// EmbedInput holds text and optional image/video URL for embedding upsert.
type EmbedInput struct {
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
}

// EmbedUpsertRequest is the input for embedding and upserting a single item into the store.
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

// EmbedUpsertResponse returns the upserted item and embedding metadata.
type EmbedUpsertResponse struct {
	Item       Item   `json:"item"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

// Item is a single memory record (id, text, hash, scope, score, optional stats).
type Item struct {
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

// SearchResponse holds search results and optional relations.
type SearchResponse struct {
	Results   []Item `json:"results"`
	Relations []any  `json:"relations,omitempty"`
}

// DeleteResponse holds a message after delete operations.
type DeleteResponse struct {
	Message string `json:"message"`
}

// ExtractRequest is the input for LLM fact extraction (messages, filters, metadata).
type ExtractRequest struct {
	Messages []Message      `json:"messages"`
	Filters  map[string]any `json:"filters,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ExtractResponse holds the extracted facts from the LLM.
type ExtractResponse struct {
	Facts []string `json:"facts"`
}

// CandidateMemory is a memory candidate passed to the decide step (id, memory, metadata).
type CandidateMemory struct {
	ID        string         `json:"id"`
	Memory    string         `json:"memory"`
	CreatedAt string         `json:"created_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// DecideRequest is the input for LLM decide step (facts, candidates, filters, metadata).
type DecideRequest struct {
	Facts      []string          `json:"facts"`
	Candidates []CandidateMemory `json:"candidates"`
	Filters    map[string]any    `json:"filters,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// DecisionAction is a single add/update/delete action from the decide step.
type DecisionAction struct {
	Event     string `json:"event"`
	ID        string `json:"id,omitempty"`
	Text      string `json:"text"`
	OldMemory string `json:"old_memory,omitempty"`
}

// DecideResponse holds the list of actions from the decide step.
type DecideResponse struct {
	Actions []DecisionAction `json:"actions"`
}

// CompactRequest is the input for LLM compact step (memories, target count, decay days).
type CompactRequest struct {
	Memories    []CandidateMemory `json:"memories"`
	TargetCount int               `json:"target_count"`
	DecayDays   int               `json:"decay_days,omitempty"`
}

// CompactResponse holds the compacted facts from the LLM.
type CompactResponse struct {
	Facts []string `json:"facts"`
}

// CompactResult holds before/after counts, ratio, and resulting memory items.
type CompactResult struct {
	BeforeCount int     `json:"before_count"`
	AfterCount  int     `json:"after_count"`
	Ratio       float64 `json:"ratio"`
	Results     []Item  `json:"results"`
}

// UsageResponse holds memory usage stats (count, bytes, estimated storage).
type UsageResponse struct {
	Count                 int   `json:"count"`
	TotalTextBytes        int64 `json:"total_text_bytes"`
	AvgTextBytes          int64 `json:"avg_text_bytes"`
	EstimatedStorageBytes int64 `json:"estimated_storage_bytes"`
}

// RebuildResult holds counts after a rebuild (fs, qdrant, missing, restored).
type RebuildResult struct {
	FsCount       int `json:"fs_count"`
	QdrantCount   int `json:"qdrant_count"`
	MissingCount  int `json:"missing_count"`
	RestoredCount int `json:"restored_count"`
}
