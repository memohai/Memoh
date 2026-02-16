package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service provides media asset persistence operations.
type Service struct {
	queries  *sqlc.Queries
	provider StorageProvider
	logger   *slog.Logger
}

// NewService creates a media service with the given storage provider.
func NewService(log *slog.Logger, queries *sqlc.Queries, provider StorageProvider) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries:  queries,
		provider: provider,
		logger:   log.With(slog.String("service", "media")),
	}
}

// Ingest persists a new media asset. It hashes the content, deduplicates by
// (bot_id, content_hash), stores the bytes via the provider, and writes the
// DB record. Returns the asset (existing or newly created).
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

	pgBotID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return Asset{}, fmt.Errorf("invalid bot id: %w", err)
	}

	// Dedup: only create when hash truly not found; propagate other DB errors.
	existing, err := s.queries.GetMediaAssetByHash(ctx, sqlc.GetMediaAssetByHashParams{
		BotID:       pgBotID,
		ContentHash: contentHash,
	})
	if err == nil {
		return convertAsset(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, fmt.Errorf("check existing asset: %w", err)
	}

	ext := extensionFromMime(input.Mime)
	storageKey := path.Join(
		input.BotID,
		string(input.MediaType),
		contentHash[:4],
		contentHash+ext,
	)

	tempFile, err := os.Open(tempPath)
	if err != nil {
		return Asset{}, fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
	}()
	if err := s.provider.Put(ctx, storageKey, tempFile); err != nil {
		return Asset{}, fmt.Errorf("store media: %w", err)
	}

	metaBytes, err := json.Marshal(nonNilMap(input.Metadata))
	if err != nil {
		metaBytes = []byte("{}")
	}

	row, err := s.queries.CreateMediaAsset(ctx, sqlc.CreateMediaAssetParams{
		BotID:       pgBotID,
		ContentHash: contentHash,
		MediaType:   string(input.MediaType),
		Mime:        coalesce(input.Mime, "application/octet-stream"),
		SizeBytes:   sizeBytes,
		StorageKey:  storageKey,
		OriginalName: pgtype.Text{
			String: input.OriginalName,
			Valid:  strings.TrimSpace(input.OriginalName) != "",
		},
		Width:      toPgInt4(input.Width),
		Height:     toPgInt4(input.Height),
		DurationMs: toPgInt8(input.DurationMs),
		Metadata:   metaBytes,
	})
	if err != nil {
		return Asset{}, fmt.Errorf("create asset record: %w", err)
	}
	return convertAsset(row), nil
}

// Open returns a reader for the media asset identified by ID.
func (s *Service) Open(ctx context.Context, assetID string) (io.ReadCloser, Asset, error) {
	if s.provider == nil {
		return nil, Asset{}, ErrProviderUnavailable
	}
	pgID, err := dbpkg.ParseUUID(assetID)
	if err != nil {
		return nil, Asset{}, fmt.Errorf("invalid asset id: %w", err)
	}
	row, err := s.queries.GetMediaAssetByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, Asset{}, ErrAssetNotFound
		}
		return nil, Asset{}, fmt.Errorf("get asset: %w", err)
	}
	asset := convertAsset(row)
	reader, err := s.provider.Open(ctx, asset.StorageKey)
	if err != nil {
		return nil, Asset{}, fmt.Errorf("open storage: %w", err)
	}
	return reader, asset, nil
}

// GetByID returns an asset by its ID.
func (s *Service) GetByID(ctx context.Context, assetID string) (Asset, error) {
	pgID, err := dbpkg.ParseUUID(assetID)
	if err != nil {
		return Asset{}, fmt.Errorf("invalid asset id: %w", err)
	}
	row, err := s.queries.GetMediaAssetByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Asset{}, ErrAssetNotFound
		}
		return Asset{}, fmt.Errorf("get asset: %w", err)
	}
	return convertAsset(row), nil
}

// LinkToMessage creates a message-asset relationship.
func (s *Service) LinkToMessage(ctx context.Context, messageID, assetID, role string, ordinal int) error {
	pgMsgID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		return fmt.Errorf("invalid message id: %w", err)
	}
	pgAssetID, err := dbpkg.ParseUUID(assetID)
	if err != nil {
		return fmt.Errorf("invalid asset id: %w", err)
	}
	if strings.TrimSpace(role) == "" {
		role = "attachment"
	}
	_, err = s.queries.CreateMessageAsset(ctx, sqlc.CreateMessageAssetParams{
		MessageID: pgMsgID,
		AssetID:   pgAssetID,
		Role:      role,
		Ordinal:   int32(ordinal),
	})
	return err
}

// ListMessageAssets returns all assets linked to a message.
func (s *Service) ListMessageAssets(ctx context.Context, messageID string) ([]Asset, error) {
	pgMsgID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		return nil, fmt.Errorf("invalid message id: %w", err)
	}
	rows, err := s.queries.ListMessageAssets(ctx, pgMsgID)
	if err != nil {
		return nil, err
	}
	assets := make([]Asset, 0, len(rows))
	for _, row := range rows {
		assets = append(assets, Asset{
			ID:           row.AssetID.String(),
			MediaType:    MediaType(row.MediaType),
			Mime:         row.Mime,
			SizeBytes:    row.SizeBytes,
			StorageKey:   row.StorageKey,
			OriginalName: dbpkg.TextToString(row.OriginalName),
			Width:        int(row.Width.Int32),
			Height:       int(row.Height.Int32),
			DurationMs:   row.DurationMs.Int64,
		})
	}
	return assets, nil
}

// AccessPath returns a consumer-accessible reference for a persisted asset.
// Delegates to the storage provider to compute the format-appropriate path.
func (s *Service) AccessPath(asset Asset) string {
	if s.provider == nil {
		return ""
	}
	return s.provider.AccessPath(asset.StorageKey)
}

// --- helpers ---

func convertAsset(row sqlc.MediaAsset) Asset {
	a := Asset{
		ID:          row.ID.String(),
		BotID:       row.BotID.String(),
		ContentHash: row.ContentHash,
		MediaType:   MediaType(row.MediaType),
		Mime:        row.Mime,
		SizeBytes:   row.SizeBytes,
		StorageKey:  row.StorageKey,
		CreatedAt:   row.CreatedAt.Time,
	}
	if row.StorageProviderID.Valid {
		a.StorageProviderID = row.StorageProviderID.String()
	}
	if row.OriginalName.Valid {
		a.OriginalName = row.OriginalName.String
	}
	if row.Width.Valid {
		a.Width = int(row.Width.Int32)
	}
	if row.Height.Valid {
		a.Height = int(row.Height.Int32)
	}
	if row.DurationMs.Valid {
		a.DurationMs = row.DurationMs.Int64
	}
	var meta map[string]any
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &meta)
	}
	a.Metadata = meta
	return a
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

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func toPgInt4(v int) pgtype.Int4 {
	if v == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(v), Valid: true}
}

func toPgInt8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
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
