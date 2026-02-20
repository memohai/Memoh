package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/memohai/memoh/internal/storage"
)

// Service provides content-addressed media asset persistence.
// All metadata is derived from the filesystem — no database, no sidecar files.
type Service struct {
	provider storage.Provider
	logger   *slog.Logger
}

// NewService creates a media service with the given storage provider.
func NewService(log *slog.Logger, provider storage.Provider) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		provider: provider,
		logger:   log.With(slog.String("service", "media")),
	}
}

// Ingest persists a new media asset. It hashes the content, deduplicates by
// checking the filesystem, and stores the bytes. Returns a derived Asset.
func (s *Service) Ingest(ctx context.Context, input IngestInput) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	if strings.TrimSpace(input.BotID) == "" {
		return Asset{}, fmt.Errorf("bot id is required")
	}
	if input.Reader == nil {
		return Asset{}, fmt.Errorf("reader is required")
	}

	maxBytes := input.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxAssetBytes
	}
	contentHash, sizeBytes, tempPath, err := spoolAndHashWithLimit(input.Reader, maxBytes)
	if err != nil {
		return Asset{}, fmt.Errorf("read input: %w", err)
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	mime := coalesce(input.Mime, "application/octet-stream")
	ext := extensionFromMime(mime)
	storageKey := path.Join(contentHash[:2], contentHash+ext)
	routingKey := path.Join(input.BotID, storageKey)

	// Filesystem dedup: if the file already exists, skip write.
	if _, openErr := s.provider.Open(ctx, routingKey); openErr == nil {
		return Asset{
			ContentHash: contentHash,
			BotID:       input.BotID,
			Mime:        mime,
			SizeBytes:   sizeBytes,
			StorageKey:  storageKey,
		}, nil
	}

	tempFile, err := os.Open(tempPath)
	if err != nil {
		return Asset{}, fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
	}()
	if err := s.provider.Put(ctx, routingKey, tempFile); err != nil {
		return Asset{}, fmt.Errorf("store media: %w", err)
	}

	return Asset{
		ContentHash: contentHash,
		BotID:       input.BotID,
		Mime:        mime,
		SizeBytes:   sizeBytes,
		StorageKey:  storageKey,
	}, nil
}

// Resolve finds an asset by content hash (no stream open). Used to fill mime/storage_key when DB has none.
func (s *Service) Resolve(ctx context.Context, botID, contentHash string) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	return s.resolveByContentHash(ctx, botID, contentHash)
}

// Open returns a reader for the media asset identified by content hash.
// It locates the file by scanning extensions under the hash prefix and derives MIME from the extension.
func (s *Service) Open(ctx context.Context, botID, contentHash string) (io.ReadCloser, Asset, error) {
	if s.provider == nil {
		return nil, Asset{}, ErrProviderUnavailable
	}
	asset, err := s.resolveByContentHash(ctx, botID, contentHash)
	if err != nil {
		return nil, Asset{}, err
	}
	routingKey := path.Join(botID, asset.StorageKey)
	reader, err := s.provider.Open(ctx, routingKey)
	if err != nil {
		return nil, Asset{}, fmt.Errorf("open storage: %w", err)
	}
	return reader, asset, nil
}

// GetByStorageKey returns an asset derived from a known storage key.
func (s *Service) GetByStorageKey(ctx context.Context, botID, storageKey string) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	routingKey := path.Join(botID, storageKey)
	rc, err := s.provider.Open(ctx, routingKey)
	if err != nil {
		return Asset{}, ErrAssetNotFound
	}
	_ = rc.Close()
	return deriveAssetFromKey(botID, storageKey), nil
}

// AccessPath returns a consumer-accessible reference for a persisted asset.
func (s *Service) AccessPath(asset Asset) string {
	if s.provider == nil {
		return ""
	}
	routingKey := path.Join(asset.BotID, asset.StorageKey)
	return s.provider.AccessPath(routingKey)
}

// IngestContainerFile reads an arbitrary file from a bot's /data/ directory
// and ingests it into the media store. The provider must implement ContainerFileOpener.
func (s *Service) IngestContainerFile(ctx context.Context, botID, containerPath string) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	opener, ok := s.provider.(storage.ContainerFileOpener)
	if !ok {
		return Asset{}, fmt.Errorf("provider does not support container file reading")
	}
	f, err := opener.OpenContainerFile(botID, containerPath)
	if err != nil {
		return Asset{}, fmt.Errorf("open container file: %w", err)
	}
	defer f.Close()
	mime := mimeFromExtension(path.Ext(containerPath))
	return s.Ingest(ctx, IngestInput{BotID: botID, Mime: mime, Reader: f})
}

// resolveByContentHash scans hash-prefix directory by extension to find the file.
func (s *Service) resolveByContentHash(ctx context.Context, botID, contentHash string) (Asset, error) {
	if strings.TrimSpace(contentHash) == "" || len(contentHash) < 2 {
		return Asset{}, ErrAssetNotFound
	}
	prefix := contentHash[:2]
	for _, ext := range knownExtensions {
		storageKey := path.Join(prefix, contentHash+ext)
		routingKey := path.Join(botID, storageKey)
		rc, err := s.provider.Open(ctx, routingKey)
		if err != nil {
			continue
		}
		_ = rc.Close()
		return deriveAssetFromKey(botID, storageKey), nil
	}
	return Asset{}, ErrAssetNotFound
}

// deriveAssetFromKey builds an Asset from the storage key (hash_2char_prefix/hash.ext).
func deriveAssetFromKey(botID, storageKey string) Asset {
	base := path.Base(storageKey)
	ext := path.Ext(base)
	hash := strings.TrimSuffix(base, ext)
	return Asset{
		ContentHash: hash,
		BotID:       botID,
		Mime:        mimeFromExtension(ext),
		StorageKey:  storageKey,
	}
}

var knownExtensions = []string{".jpg", ".png", ".gif", ".webp", ".mp3", ".wav", ".ogg", ".mp4", ".webm", ".pdf", ".bin"}

func mimeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func extensionFromMime(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func spoolAndHashWithLimit(reader io.Reader, maxBytes int64) (string, int64, string, error) {
	if reader == nil {
		return "", 0, "", fmt.Errorf("reader is required")
	}
	if maxBytes <= 0 {
		return "", 0, "", fmt.Errorf("max bytes must be greater than 0")
	}
	tempFile, err := os.CreateTemp("", "memoh-media-*")
	if err != nil {
		return "", 0, "", fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	keepFile := false
	defer func() {
		_ = tempFile.Close()
		if !keepFile {
			_ = os.Remove(tempPath)
		}
	}()

	hasher := sha256.New()
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	written, err := io.Copy(io.MultiWriter(tempFile, hasher), limited)
	if err != nil {
		return "", 0, "", fmt.Errorf("copy to temp file: %w", err)
	}
	if written > maxBytes {
		return "", 0, "", fmt.Errorf("%w: max %d bytes", ErrAssetTooLarge, maxBytes)
	}
	if written == 0 {
		return "", 0, "", fmt.Errorf("asset payload is empty")
	}
	keepFile = true
	return hex.EncodeToString(hasher.Sum(nil)), written, tempPath, nil
}
