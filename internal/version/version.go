// Package version provides application version and build info.
//
//nolint:revive
package version

import (
	"fmt"
	"runtime/debug"
)

var (
	// Version is the current version of the application.
	// It can be overridden by ldflags at build time.
	Version = "dev"
	// CommitHash is the git commit hash at build time.
	// It can be overridden by ldflags at build time.
	CommitHash = ""
	// BuildTime is the time when the application was built.
	// It can be overridden by ldflags at build time.
	BuildTime = ""
)

// GetInfo returns a formatted version string including the version and commit hash.
func GetInfo() string {
	if CommitHash == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					CommitHash = setting.Value
				}
				if setting.Key == "vcs.time" {
					BuildTime = setting.Value
				}
			}
		}
	}

	res := Version
	if CommitHash != "" {
		// Only use the first 7 characters of the commit hash if it's long
		shortHash := CommitHash
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}
		res += fmt.Sprintf(" (%s)", shortHash)
	}
	return res
}
