package media

import (
	"fmt"
	"io"
)

const (
	// MaxAssetBytes is the global max accepted payload size.
	MaxAssetBytes int64 = 200 * 1024 * 1024
)

// ReadAllWithLimit reads from reader and rejects payloads larger than maxBytes.
func ReadAllWithLimit(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("reader is required")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("max bytes must be greater than 0")
	}
	limited := &io.LimitedReader{
		R: reader,
		N: maxBytes + 1,
	}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: max %d bytes", ErrAssetTooLarge, maxBytes)
	}
	return data, nil
}
