package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/memohai/memoh/internal/storage"
)

// Service provides content-addressed media asset persistence.
// All metadata is derived from object keys — no database or sidecar files.
type Service struct {
	provider       storage.Provider
	containerFiles storage.ContainerFileOpener
	logger         *slog.Logger
}

// NewService creates a media service with the given storage provider.
func NewService(log *slog.Logger, provider storage.Provider) *Service {
	if log == nil {
		log = slog.Default()
	}
	service := &Service{
		provider: provider,
		logger:   log.With(slog.String("service", "media")),
	}
	if opener, ok := provider.(storage.ContainerFileOpener); ok {
		service.containerFiles = opener
	}
	return service
}

// SetContainerFileOpener keeps workspace source-file access independent from
// the object storage backend. S3 persists the resulting asset while the
// container bridge only reads the original workspace file.
func (s *Service) SetContainerFileOpener(opener storage.ContainerFileOpener) {
	if s != nil {
		s.containerFiles = opener
	}
}

// Ingest persists a new media asset. It hashes the content, deduplicates by
// checking object storage, and stores the bytes. Returns a derived Asset.
func (s *Service) Ingest(ctx context.Context, input IngestInput) (Asset, error) {
	botID := strings.TrimSpace(input.BotID)
	if botID == "" {
		return Asset{}, errors.New("bot id is required")
	}
	return s.ingest(ctx, botID, botID, "", ScopedIngestInput{
		Mime:        input.Mime,
		Reader:      input.Reader,
		MaxBytes:    input.MaxBytes,
		OriginalExt: input.OriginalExt,
	})
}

// IngestScoped persists media in a validated non-bot storage namespace.
func (s *Service) IngestScoped(ctx context.Context, namespace string, input ScopedIngestInput) (Asset, error) {
	namespace, err := normalizeStorageNamespace(namespace)
	if err != nil {
		return Asset{}, err
	}
	return s.ingest(ctx, namespace, "", namespace, input)
}

func (s *Service) ingest(ctx context.Context, routingNamespace, botID, assetNamespace string, input ScopedIngestInput) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	if input.Reader == nil {
		return Asset{}, errors.New("reader is required")
	}

	maxBytes := input.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxAssetBytes
	}
	contentHash, sizeBytes, tempFile, err := spoolAndHashWithLimit(input.Reader, maxBytes)
	if err != nil {
		return Asset{}, fmt.Errorf("read input: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name()) //nolint:gosec // G703: path is from os.CreateTemp, not from user input
	}()

	mime := coalesce(input.Mime, "application/octet-stream")
	ext := extensionFromMime(mime)
	if ext == ".bin" && input.OriginalExt != "" {
		ext = input.OriginalExt
	}
	storageKey := path.Join(contentHash[:2], contentHash+ext)
	routingKey := path.Join(routingNamespace, storageKey)

	// Content-addressed dedup: if the object already exists, skip the write.
	if existing, openErr := s.provider.Open(ctx, routingKey); openErr == nil {
		if existing != nil {
			_ = existing.Close()
		}
		return Asset{
			ContentHash: contentHash,
			BotID:       botID,
			Mime:        mime,
			SizeBytes:   sizeBytes,
			StorageKey:  storageKey,
			Namespace:   assetNamespace,
		}, nil
	}

	if err := s.provider.Put(ctx, routingKey, tempFile); err != nil {
		return Asset{}, fmt.Errorf("store media: %w", err)
	}

	return Asset{
		ContentHash: contentHash,
		BotID:       botID,
		Mime:        mime,
		SizeBytes:   sizeBytes,
		StorageKey:  storageKey,
		Namespace:   assetNamespace,
	}, nil
}

// Resolve finds an asset by content hash (no stream open). Used to fill mime/storage_key when DB has none.
func (s *Service) Resolve(ctx context.Context, botID, contentHash string) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	return s.resolveByContentHash(ctx, botID, botID, "", contentHash)
}

// ResolveScoped finds an asset in a non-bot storage namespace.
func (s *Service) ResolveScoped(ctx context.Context, namespace, contentHash string) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	namespace, err := normalizeStorageNamespace(namespace)
	if err != nil {
		return Asset{}, err
	}
	return s.resolveByContentHash(ctx, namespace, "", namespace, contentHash)
}

// Stat returns asset metadata for the given content hash without opening the file.
// It satisfies the channel.OutboundAttachmentStore interface.
func (s *Service) Stat(ctx context.Context, botID, contentHash string) (Asset, error) {
	return s.Resolve(ctx, botID, contentHash)
}

// Open returns a reader for the media asset identified by content hash.
// It locates the file by scanning extensions under the hash prefix and derives MIME from the extension.
func (s *Service) Open(ctx context.Context, botID, contentHash string) (io.ReadCloser, Asset, error) {
	if s.provider == nil {
		return nil, Asset{}, ErrProviderUnavailable
	}
	asset, err := s.resolveByContentHash(ctx, botID, botID, "", contentHash)
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

// OpenScoped opens an asset from a non-bot storage namespace.
func (s *Service) OpenScoped(ctx context.Context, namespace, contentHash string) (io.ReadCloser, Asset, error) {
	if s.provider == nil {
		return nil, Asset{}, ErrProviderUnavailable
	}
	namespace, err := normalizeStorageNamespace(namespace)
	if err != nil {
		return nil, Asset{}, err
	}
	asset, err := s.resolveByContentHash(ctx, namespace, "", namespace, contentHash)
	if err != nil {
		return nil, Asset{}, err
	}
	reader, err := s.provider.Open(ctx, path.Join(namespace, asset.StorageKey))
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
		if !errors.Is(err, storage.ErrNotFound) {
			return Asset{}, fmt.Errorf("open storage: %w", err)
		}
		return Asset{}, ErrAssetNotFound
	}
	_ = rc.Close()
	return deriveAssetFromKey(botID, "", storageKey), nil
}

// AccessPath returns a reachable consumer reference for a persisted asset.
// Providers that support materialization may promote spill storage into their
// consumer-addressable primary store. Errors are intentionally collapsed for
// legacy callers; new routing code should use EnsureAccessPath.
func (s *Service) AccessPath(ctx context.Context, asset Asset) string {
	accessPath, _ := s.EnsureAccessPath(ctx, asset)
	return accessPath
}

// EnsureAccessPath returns a consumer-visible path, materializing the asset
// into addressable storage when the provider supports it.
func (s *Service) EnsureAccessPath(ctx context.Context, asset Asset) (string, error) {
	if s.provider == nil {
		return "", ErrProviderUnavailable
	}
	routingNamespace := strings.TrimSpace(asset.Namespace)
	if routingNamespace == "" {
		routingNamespace = strings.TrimSpace(asset.BotID)
	}
	if routingNamespace == "" {
		return "", errors.New("media storage namespace is required")
	}
	routingKey := path.Join(routingNamespace, asset.StorageKey)
	if ensurer, ok := s.provider.(storage.AccessPathEnsurer); ok {
		accessPath, err := ensurer.EnsureAccessPath(ctx, routingKey)
		if err != nil {
			return "", err
		}
		accessPath = strings.TrimSpace(accessPath)
		if accessPath == "" {
			return "", storage.ErrAccessPathUnavailable
		}
		return accessPath, nil
	}
	accessPath := strings.TrimSpace(s.provider.AccessPath(ctx, routingKey))
	if accessPath == "" {
		return "", storage.ErrAccessPathUnavailable
	}
	return accessPath, nil
}

// IngestContainerFile reads an arbitrary file from a bot's /data/ directory
// and ingests it into the media store. The provider must implement ContainerFileOpener.
func (s *Service) IngestContainerFile(ctx context.Context, botID, containerPath string) (Asset, error) {
	if s.provider == nil {
		return Asset{}, ErrProviderUnavailable
	}
	opener := s.containerFiles
	if opener == nil {
		return Asset{}, storage.ErrContainerFileNotSupported
	}
	f, err := opener.OpenContainerFile(ctx, botID, containerPath)
	if err != nil {
		return Asset{}, fmt.Errorf("open workspace file: %w", err)
	}
	defer func() { _ = f.Close() }()
	ext := path.Ext(containerPath)
	mime := mimeFromExtension(ext)
	return s.Ingest(ctx, IngestInput{BotID: botID, Mime: mime, Reader: f, OriginalExt: ext})
}

// resolveByContentHash scans the hash prefix to find the stored object. Providers
// with prefix listing use one lookup; providers without an authoritative list
// fall back to probing known extensions.
func (s *Service) resolveByContentHash(ctx context.Context, routingNamespace, botID, assetNamespace, contentHash string) (Asset, error) {
	contentHash = NormalizeContentHash(contentHash)
	if !IsContentHash(contentHash) {
		return Asset{}, ErrAssetNotFound
	}
	prefix := contentHash[:2]

	if lister, ok := s.provider.(storage.PrefixLister); ok {
		keyPrefix := path.Join(routingNamespace, prefix, contentHash)
		keys, err := lister.ListPrefix(ctx, keyPrefix)
		if err == nil {
			namespacePrefix := strings.TrimSuffix(routingNamespace, "/") + "/"
			for _, key := range keys {
				key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
				if !strings.HasPrefix(key, namespacePrefix) {
					continue
				}
				storageKey := strings.TrimPrefix(key, namespacePrefix)
				base := path.Base(storageKey)
				if strings.TrimSuffix(base, path.Ext(base)) != contentHash {
					continue
				}
				return deriveAssetFromKey(botID, assetNamespace, storageKey), nil
			}
			if _, authoritative := s.provider.(storage.AuthoritativePrefixLister); authoritative {
				return Asset{}, ErrAssetNotFound
			}
		} else if !errors.Is(err, storage.ErrNotFound) {
			return Asset{}, fmt.Errorf("list storage: %w", err)
		}
	}

	for _, ext := range knownExtensions {
		storageKey := path.Join(prefix, contentHash+ext)
		routingKey := path.Join(routingNamespace, storageKey)
		rc, err := s.provider.Open(ctx, routingKey)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return Asset{}, fmt.Errorf("probe storage: %w", err)
		}
		_ = rc.Close()
		return deriveAssetFromKey(botID, assetNamespace, storageKey), nil
	}

	return Asset{}, ErrAssetNotFound
}

// deriveAssetFromKey builds an Asset from the storage key (hash_2char_prefix/hash.ext).
func deriveAssetFromKey(botID, namespace, storageKey string) Asset {
	base := path.Base(storageKey)
	ext := path.Ext(base)
	hash := strings.TrimSuffix(base, ext)
	return Asset{
		ContentHash: hash,
		BotID:       botID,
		Mime:        mimeFromExtension(ext),
		StorageKey:  storageKey,
		Namespace:   namespace,
	}
}

func normalizeStorageNamespace(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || path.IsAbs(value) {
		return "", errors.New("media storage namespace is required")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrPathTraversal
	}
	return cleaned, nil
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

// MimeFromPath derives MIME from an already-persisted filename or storage key.
// It is deliberately metadata-only: history reads can use it without opening
// workspace storage or waking a runtime provider.
func MimeFromPath(filePath string) string {
	ext := path.Ext(strings.TrimSpace(filePath))
	if ext == "" {
		return ""
	}
	mime := mimeFromExtension(ext)
	if mime == "application/octet-stream" {
		return ""
	}
	return mime
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

// spoolAndHashWithLimit streams reader into a temp file while computing its SHA-256.
// Returns the open file sought to the beginning; caller must close and remove it.
func spoolAndHashWithLimit(reader io.Reader, maxBytes int64) (contentHash string, size int64, f *os.File, err error) {
	if reader == nil {
		return "", 0, nil, errors.New("reader is required")
	}
	if maxBytes <= 0 {
		return "", 0, nil, errors.New("max bytes must be greater than 0")
	}
	tmp, createErr := os.CreateTemp("", "memoh-media-*")
	if createErr != nil {
		return "", 0, nil, fmt.Errorf("create temp file: %w", createErr)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703: path is from os.CreateTemp, not from user input
	}

	hasher := sha256.New()
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	written, copyErr := io.Copy(io.MultiWriter(tmp, hasher), limited)
	if copyErr != nil {
		cleanup()
		return "", 0, nil, fmt.Errorf("copy to temp file: %w", copyErr)
	}
	if written > maxBytes {
		cleanup()
		return "", 0, nil, fmt.Errorf("%w: max %d bytes", ErrAssetTooLarge, maxBytes)
	}
	if written == 0 {
		cleanup()
		return "", 0, nil, errors.New("asset payload is empty")
	}
	if _, seekErr := tmp.Seek(0, io.SeekStart); seekErr != nil {
		cleanup()
		return "", 0, nil, fmt.Errorf("seek temp file: %w", seekErr)
	}
	return hex.EncodeToString(hasher.Sum(nil)), written, tmp, nil
}
