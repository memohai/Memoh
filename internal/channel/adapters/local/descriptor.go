// Package local implements the local channel adapter for WebUI and API access.
package local

import "github.com/memohai/memoh/internal/channel"

const (
	// WebType is the registered ChannelType for the local adapter (WebUI / API).
	WebType channel.ChannelType = "local"
)
