// Package templates holds the built-in files seeded into every agent
// workspace on first boot: the AGENTS.md persona scaffold, memory and
// heartbeat notes, and the default .memoh directory (hooks, built-in
// skills).
//
// The canonical copy lives in the workspace/ subdirectory. It is consumed
// in two ways:
//   - docker/Dockerfile.server copies workspace/ into the runtime toolkit
//     assembly, from which the bridge seeds /data on first boot
//     (cmd/bridge/main.go); devenv compose files bind-mount it the same way.
//   - Downstream distributions import this package and seed WorkspaceFS()
//     into freshly provisioned workspaces themselves.
package templates

import (
	"embed"
	"io/fs"
)

// The all: prefix is load-bearing: without it the hidden .memoh directory
// (hooks.json and built-in skills) is silently excluded from the embed.
//
//go:embed all:workspace
var workspaceFS embed.FS

// WorkspaceFS returns the workspace bootstrap template tree, rooted at the
// template contents (AGENTS.md, .memoh/, ...). Callers seeding a workspace
// should skip .gitkeep placeholder files and never overwrite files that
// already exist in the destination.
func WorkspaceFS() fs.FS {
	sub, err := fs.Sub(workspaceFS, "workspace")
	if err != nil {
		panic("templates: workspace subtree missing from embed: " + err.Error())
	}
	return sub
}
