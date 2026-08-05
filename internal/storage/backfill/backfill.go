// Package backfill copies immutable, content-addressed media from legacy
// storage into the currently configured canonical provider.
package backfill

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

// Object describes one legacy object. Open is called synchronously, before
// Source.Walk advances to the next object, so sources may keep temporary
// mounts or workspace runtimes alive for the duration of the callback.
type Object struct {
	Key       string
	SizeBytes int64
	Open      func(context.Context) (io.ReadCloser, error)
}

type Visitor func(Object) error

// Source enumerates a legacy storage implementation.
type Source interface {
	Name() string
	Walk(context.Context, Visitor) error
}

type Result struct {
	Scanned     int64
	Copied      int64
	Existing    int64
	Ignored     int64
	Failed      int64
	BytesCopied int64
}

type Runner struct {
	target storage.Provider
	logger *slog.Logger
}

func New(target storage.Provider, logger *slog.Logger) (*Runner, error) {
	if target == nil {
		return nil, errors.New("backfill target provider is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{target: target, logger: logger}, nil
}

// Run is idempotent and resumable. An already-present target is skipped only
// after its size and SHA-256 digest have been verified against the key.
func (r *Runner) Run(ctx context.Context, sources ...Source) (Result, error) {
	var (
		result   Result
		failures []error
	)
	for _, source := range sources {
		if source == nil {
			continue
		}
		sourceName := strings.TrimSpace(source.Name())
		if sourceName == "" {
			sourceName = "legacy"
		}
		r.logger.Info("media storage backfill source started", slog.String("storage_source", sourceName))
		err := source.Walk(ctx, func(object Object) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			result.Scanned++
			outcome, size, migrateErr := r.copyObject(ctx, object)
			switch outcome {
			case outcomeCopied:
				result.Copied++
				result.BytesCopied += size
			case outcomeExisting:
				result.Existing++
			case outcomeIgnored:
				result.Ignored++
			}
			if migrateErr != nil {
				result.Failed++
				wrapped := fmt.Errorf("source %s object %q: %w", sourceName, object.Key, migrateErr)
				if len(failures) < 20 {
					failures = append(failures, wrapped)
				}
				r.logger.Warn("media storage backfill object failed",
					slog.String("storage_source", sourceName),
					slog.String("key", object.Key),
					slog.Any("error", migrateErr),
				)
			}
			if result.Scanned%100 == 0 {
				logProgress(r.logger, result)
			}
			return nil
		})
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Failed++
			wrapped := fmt.Errorf("walk source %s: %w", sourceName, err)
			if len(failures) < 20 {
				failures = append(failures, wrapped)
			}
		}
	}
	logProgress(r.logger, result)
	if result.Failed == 0 {
		return result, nil
	}
	return result, fmt.Errorf("media storage backfill failed for %d item(s): %w", result.Failed, errors.Join(failures...))
}

func logProgress(logger *slog.Logger, result Result) {
	logger.Info("media storage backfill progress",
		slog.Int64("scanned", result.Scanned),
		slog.Int64("copied", result.Copied),
		slog.Int64("existing", result.Existing),
		slog.Int64("ignored", result.Ignored),
		slog.Int64("failed", result.Failed),
		slog.Int64("bytes_copied", result.BytesCopied),
	)
}

type copyOutcome uint8

const (
	outcomeFailed copyOutcome = iota
	outcomeCopied
	outcomeExisting
	outcomeIgnored
)

func (r *Runner) copyObject(ctx context.Context, object Object) (copyOutcome, int64, error) {
	expectedHash, ok := contentHashFromKey(object.Key)
	if !ok {
		return outcomeIgnored, 0, nil
	}

	targetSize, targetHash, exists, err := digestStoredObject(ctx, r.target, object.Key)
	if err != nil {
		return outcomeFailed, 0, fmt.Errorf("inspect target: %w", err)
	}
	if exists && targetHash == expectedHash && (object.SizeBytes < 0 || targetSize == object.SizeBytes) {
		return outcomeExisting, 0, nil
	}
	if object.Open == nil {
		return outcomeFailed, 0, errors.New("legacy object has no opener")
	}

	tmp, sourceSize, sourceHash, err := spoolObject(ctx, object.Open)
	if err != nil {
		return outcomeFailed, 0, fmt.Errorf("read legacy object: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name()) //nolint:gosec // path is created by os.CreateTemp
	}()
	if sourceHash != expectedHash {
		return outcomeFailed, 0, fmt.Errorf("legacy SHA-256 %s does not match key hash %s", sourceHash, expectedHash)
	}
	if object.SizeBytes >= 0 && sourceSize != object.SizeBytes {
		return outcomeFailed, 0, fmt.Errorf("legacy size changed during backfill: listed %d, read %d", object.SizeBytes, sourceSize)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return outcomeFailed, 0, fmt.Errorf("rewind legacy object: %w", err)
	}
	if err := r.target.Put(ctx, object.Key, tmp); err != nil {
		return outcomeFailed, 0, fmt.Errorf("write target: %w", err)
	}

	verifiedSize, verifiedHash, exists, err := digestStoredObject(ctx, r.target, object.Key)
	if err != nil {
		return outcomeFailed, 0, fmt.Errorf("verify target: %w", err)
	}
	if !exists || verifiedSize != sourceSize || verifiedHash != sourceHash {
		return outcomeFailed, 0, fmt.Errorf(
			"target verification mismatch: size=%d hash=%s, want size=%d hash=%s",
			verifiedSize, verifiedHash, sourceSize, sourceHash,
		)
	}
	return outcomeCopied, sourceSize, nil
}

func spoolObject(ctx context.Context, open func(context.Context) (io.ReadCloser, error)) (*os.File, int64, string, error) {
	rc, err := open(ctx)
	if err != nil {
		return nil, 0, "", err
	}
	if rc == nil {
		return nil, 0, "", errors.New("legacy opener returned a nil reader")
	}
	defer func() { _ = rc.Close() }()

	tmp, err := os.CreateTemp("", "memoh-media-backfill-*")
	if err != nil {
		return nil, 0, "", err
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hasher), rc)
	if copyErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name()) //nolint:gosec // path is created by os.CreateTemp
		return nil, 0, "", copyErr
	}
	return tmp, size, hex.EncodeToString(hasher.Sum(nil)), nil
}

func digestStoredObject(ctx context.Context, provider storage.Provider, key string) (int64, string, bool, error) {
	rc, err := provider.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	if rc == nil {
		return 0, "", false, errors.New("provider returned a nil reader")
	}
	defer func() { _ = rc.Close() }()
	hasher := sha256.New()
	size, err := io.Copy(hasher, rc)
	if err != nil {
		return 0, "", false, err
	}
	return size, hex.EncodeToString(hasher.Sum(nil)), true, nil
}

func contentHashFromKey(key string) (string, bool) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	clean := path.Clean(key)
	if key == "" || clean != key || path.IsAbs(clean) || strings.HasPrefix(clean, "../") {
		return "", false
	}
	parts := strings.Split(clean, "/")
	if len(parts) < 3 {
		return "", false
	}
	base := path.Base(clean)
	ext := path.Ext(base)
	if ext == "" {
		return "", false
	}
	hash := strings.TrimSuffix(base, ext)
	if len(hash) != sha256.Size*2 || path.Base(path.Dir(clean)) != hash[:2] {
		return "", false
	}
	if _, err := hex.DecodeString(hash); err != nil || strings.ToLower(hash) != hash {
		return "", false
	}
	return hash, true
}
