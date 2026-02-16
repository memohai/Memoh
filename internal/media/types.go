package media

import (
	"context"
	"io"
	"time"
)

// MediaType classifies the kind of media asset.
type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeAudio MediaType = "audio"
	MediaTypeVideo MediaType = "video"
	MediaTypeFile  MediaType = "file"
)

// Asset is the domain representation of a persisted media object.
type Asset struct {
	ID                string         `json:"id"`
	BotID             string         `json:"bot_id"`
	StorageProviderID string         `json:"storage_provider_id,omitempty"`
	ContentHash       string         `json:"content_hash"`
	MediaType         MediaType      `json:"media_type"`
	Mime              string         `json:"mime"`
	SizeBytes         int64          `json:"size_bytes"`
	StorageKey        string         `json:"storage_key"`
	OriginalName      string         `json:"original_name,omitempty"`
	Width             int            `json:"width,omitempty"`
	Height            int            `json:"height,omitempty"`
	DurationMs        int64          `json:"duration_ms,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

// IngestInput carries the data needed to persist a new media asset.
type IngestInput struct {
	BotID        string
	MediaType    MediaType
	Mime         string
	OriginalName string
	Width        int
	Height       int
	DurationMs   int64
	Metadata     map[string]any
	// Reader provides the raw bytes; caller is responsible for closing.
	Reader io.Reader
	// MaxBytes optionally overrides the media-type default size limit.
	MaxBytes int64
}

// MessageAssetLink represents the relationship between a message and an asset.
type MessageAssetLink struct {
	AssetID string `json:"asset_id"`
	Role    string `json:"role"`
	Ordinal int    `json:"ordinal"`
}

// StorageProvider abstracts object storage operations.
type StorageProvider interface {
	// Put writes data to storage under the given key.
	Put(ctx context.Context, key string, reader io.Reader) error
	// Open returns a reader for the given storage key.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object at key.
	Delete(ctx context.Context, key string) error
	// AccessPath returns a consumer-accessible reference for a storage key.
	// The format depends on the backend (e.g. container path, signed URL).
	AccessPath(key string) string
}
