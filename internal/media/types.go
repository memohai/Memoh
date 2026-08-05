package media

import (
	"encoding/hex"
	"io"
	"strings"
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
// ContentHash is the content-addressed identifier (SHA-256 hex).
type Asset struct {
	ContentHash string `json:"content_hash"`
	BotID       string `json:"bot_id"`
	Mime        string `json:"mime"`
	SizeBytes   int64  `json:"size_bytes"`
	StorageKey  string `json:"storage_key"`

	// Namespace is the storage routing scope for assets that are not owned by
	// a bot (for example globally content-addressed avatars). It is internal
	// routing metadata and intentionally omitted from JSON contracts.
	Namespace string `json:"-"`
}

// IngestInput carries the data needed to persist a new media asset.
type IngestInput struct {
	BotID string
	Mime  string
	// Reader provides the raw bytes; caller is responsible for closing.
	Reader io.Reader
	// MaxBytes optionally overrides the default size limit.
	MaxBytes int64
	// OriginalExt preserves the source file extension (e.g. ".md") so it
	// survives even when the MIME type is unknown / generic.
	OriginalExt string
}

// ScopedIngestInput carries media data for a non-bot storage namespace.
// Bot-owned callers should continue using IngestInput and Ingest.
type ScopedIngestInput struct {
	Mime        string
	Reader      io.Reader
	MaxBytes    int64
	OriginalExt string
}

// NormalizeContentHash returns the canonical lowercase SHA-256 hex form.
func NormalizeContentHash(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// IsContentHash reports whether value is a complete SHA-256 hex digest.
func IsContentHash(value string) bool {
	value = NormalizeContentHash(value)
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
