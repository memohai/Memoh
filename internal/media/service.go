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
	if ext == ".bin" && input.OriginalExt != "" {
		ext = input.OriginalExt
	}
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
	f, err := opener.OpenContainerFile(ctx, botID, containerPath)
	if err != nil {
		return Asset{}, fmt.Errorf("open container file: %w", err)
	}
	defer f.Close()
	ext := path.Ext(containerPath)
	mime := mimeFromExtension(ext)
	return s.Ingest(ctx, IngestInput{BotID: botID, Mime: mime, Reader: f, OriginalExt: ext})
}

// resolveByContentHash scans hash-prefix directory by extension to find the file.
// It first tries known extensions (fast path), then falls back to a directory
// listing if the provider supports it, so arbitrary file types are found.
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

	if lister, ok := s.provider.(storage.PrefixLister); ok {
		keyPrefix := path.Join(botID, prefix, contentHash)
		keys, err := lister.ListPrefix(ctx, keyPrefix)
		if err == nil {
			for _, k := range keys {
				_, storageKey := splitFirst(k, '/')
				if storageKey != "" {
					return deriveAssetFromKey(botID, storageKey), nil
				}
			}
		}
	}

	return Asset{}, ErrAssetNotFound
}

func splitFirst(s string, sep byte) (string, string) {
	i := strings.IndexByte(s, sep)
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
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

var extToMime = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".png": "image/png", ".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg", ".flac": "audio/flac", ".aac": "audio/aac",
	".mp4": "video/mp4", ".webm": "video/webm", ".avi": "video/x-msvideo", ".mov": "video/quicktime",
	".pdf": "application/pdf", ".zip": "application/zip", ".gz": "application/gzip",
	".json": "application/json", ".xml": "application/xml", ".csv": "text/csv",
	".txt": "text/plain", ".md": "text/markdown", ".log": "text/plain",
	".html": "text/html", ".css": "text/css",
	".js": "text/javascript", ".ts": "text/typescript",
	".py": "text/x-python", ".go": "text/x-go", ".rs": "text/x-rust",
	".c": "text/x-c", ".cpp": "text/x-c++", ".h": "text/x-c",
	".java": "text/x-java", ".rb": "text/x-ruby", ".sh": "text/x-shellscript",
	".yaml": "text/yaml", ".yml": "text/yaml", ".toml": "text/toml",
	".sql": "text/x-sql", ".ini": "text/plain", ".conf": "text/plain",
}

var mimeToExt = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif",
	"image/webp": ".webp", "image/svg+xml": ".svg",
	"audio/mpeg": ".mp3", "audio/wav": ".wav", "audio/ogg": ".ogg",
	"audio/flac": ".flac", "audio/aac": ".aac",
	"video/mp4": ".mp4", "video/webm": ".webm", "video/x-msvideo": ".avi", "video/quicktime": ".mov",
	"application/pdf": ".pdf", "application/zip": ".zip", "application/gzip": ".gz",
	"application/json": ".json", "application/xml": ".xml",
	"text/plain": ".txt", "text/markdown": ".md", "text/csv": ".csv",
	"text/html": ".html", "text/css": ".css",
	"text/javascript": ".js", "text/typescript": ".ts",
	"text/x-python": ".py", "text/x-go": ".go", "text/x-rust": ".rs",
	"text/x-c": ".c", "text/x-c++": ".cpp",
	"text/x-java": ".java", "text/x-ruby": ".rb", "text/x-shellscript": ".sh",
	"text/yaml": ".yaml", "text/toml": ".toml", "text/x-sql": ".sql",
}

var knownExtensions []string

func init() {
	seen := make(map[string]bool)
	for ext := range extToMime {
		if !seen[ext] {
			knownExtensions = append(knownExtensions, ext)
			seen[ext] = true
		}
	}
	if !seen[".bin"] {
		knownExtensions = append(knownExtensions, ".bin")
	}
}

func mimeFromExtension(ext string) string {
	if mime, ok := extToMime[strings.ToLower(ext)]; ok {
		return mime
	}
	return "application/octet-stream"
}

func extensionFromMime(mime string) string {
	if ext, ok := mimeToExt[strings.ToLower(strings.TrimSpace(mime))]; ok {
		return ext
	}
	return ".bin"
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
