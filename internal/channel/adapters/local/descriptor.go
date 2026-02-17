// Package local implements the CLI and Web channel adapters for local development.
package local

import "github.com/memohai/memoh/internal/channel"

const (
	// CLIType is the registered ChannelType for the CLI adapter.
	CLIType channel.Type = "cli"
	// WebType is the registered ChannelType for the Web adapter.
	WebType channel.Type = "web"
)
