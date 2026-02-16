package media

import "errors"

var (
	// ErrAssetNotFound indicates the requested media asset does not exist.
	ErrAssetNotFound = errors.New("media asset not found")
	// ErrProviderUnavailable indicates the storage provider is not configured or reachable.
	ErrProviderUnavailable = errors.New("storage provider unavailable")
	// ErrAssetTooLarge indicates the payload exceeds the configured max asset size.
	ErrAssetTooLarge = errors.New("media asset too large")
	// ErrPathTraversal indicates a storage key attempted directory traversal.
	ErrPathTraversal = errors.New("path traversal is forbidden")
)
