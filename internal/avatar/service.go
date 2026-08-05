// Package avatar stores user and bot avatars as globally content-addressed
// media while keeping the underlying storage provider private.
package avatar

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/media"
)

const (
	StorageNamespace = "avatars"
	MaxBytes         = int64(5 * 1024 * 1024)
)

var (
	ErrInvalidImage = errors.New("invalid avatar image")
	ErrTooLarge     = errors.New("avatar image too large")
)

type Service struct {
	media *media.Service
}

type Stored struct {
	ContentHash string `json:"content_hash"`
	URL         string `json:"url"`
	Mime        string `json:"mime"`
	SizeBytes   int64  `json:"size_bytes"`
}

func NewService(mediaService *media.Service) *Service {
	return &Service{media: mediaService}
}

// StoreIfInline converts an inline data URL into a private-storage-backed
// avatar URL. Ordinary external URLs and an empty value pass through unchanged.
func (s *Service) StoreIfInline(ctx context.Context, value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if !attachment.IsDataURL(value) {
		return value, false, nil
	}
	stored, err := s.StoreDataURL(ctx, value)
	if err != nil {
		return "", true, err
	}
	return stored.URL, true, nil
}

func (s *Service) StoreDataURL(ctx context.Context, dataURL string) (Stored, error) {
	if s == nil || s.media == nil {
		return Stored{}, errors.New("avatar media service is unavailable")
	}
	dataURL = strings.TrimSpace(dataURL)
	comma := strings.IndexByte(dataURL, ',')
	if comma <= len("data:") || !strings.Contains(strings.ToLower(dataURL[:comma]), ";base64") {
		return Stored{}, ErrInvalidImage
	}

	encoded := strings.TrimSpace(dataURL[comma+1:])
	if encoded == "" {
		return Stored{}, ErrInvalidImage
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	data, err := media.ReadAllWithLimit(decoder, MaxBytes)
	if err != nil {
		if errors.Is(err, media.ErrAssetTooLarge) {
			return Stored{}, ErrTooLarge
		}
		return Stored{}, ErrInvalidImage
	}
	if len(data) == 0 {
		return Stored{}, ErrInvalidImage
	}

	mimeType := attachment.NormalizeMime(http.DetectContentType(data))
	if !IsSupportedMime(mimeType) {
		return Stored{}, ErrInvalidImage
	}
	asset, err := s.media.IngestScoped(ctx, StorageNamespace, media.ScopedIngestInput{
		Mime:     mimeType,
		Reader:   bytes.NewReader(data),
		MaxBytes: MaxBytes,
	})
	if err != nil {
		if errors.Is(err, media.ErrAssetTooLarge) {
			return Stored{}, ErrTooLarge
		}
		return Stored{}, fmt.Errorf("store avatar media: %w", err)
	}
	return Stored{
		ContentHash: asset.ContentHash,
		URL:         URLPath(asset.ContentHash),
		Mime:        asset.Mime,
		SizeBytes:   asset.SizeBytes,
	}, nil
}

func (s *Service) Open(ctx context.Context, contentHash string) (io.ReadCloser, media.Asset, error) {
	if s == nil || s.media == nil {
		return nil, media.Asset{}, errors.New("avatar media service is unavailable")
	}
	reader, asset, err := s.media.OpenScoped(ctx, StorageNamespace, contentHash)
	if err != nil {
		return nil, media.Asset{}, err
	}
	if !IsSupportedMime(asset.Mime) {
		_ = reader.Close()
		return nil, media.Asset{}, ErrInvalidImage
	}
	return reader, asset, nil
}

// URLPath is the stable, authenticated Memoh API path for an avatar.
func URLPath(contentHash string) string {
	contentHash = media.NormalizeContentHash(contentHash)
	if !media.IsContentHash(contentHash) {
		return ""
	}
	return path.Join("/avatars", contentHash)
}

func IsSupportedMime(mimeType string) bool {
	switch attachment.NormalizeMime(mimeType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}
