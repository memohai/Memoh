// Package arch guards the runtime-boundary import rules from the channel
// boundary spec (docs/superpowers/specs/2026-07-17-channel-boundary-design.md §8).
// Rules check direct imports; go list resolves the real build graph so the
// test fails on any regression, not just textual matches.
package arch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/memohai/memoh/"

type pkgInfo struct {
	ImportPath string
	Imports    []string
}

func loadImports(t *testing.T, patterns ...string) []pkgInfo {
	t.Helper()
	args := append([]string{"list", "-json=ImportPath,Imports"}, patterns...)
	cmd := exec.CommandContext(t.Context(), "go", args...) //nolint:gosec // fixed binary, test-controlled args
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list %v: %v", patterns, err)
	}
	var pkgs []pkgInfo
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkgInfo
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// boundaryRule denies direct imports. deny entries match exactly OR as a
// path prefix followed by "/"; allowPrefixes carve out exceptions on the
// denied side; allowPackages carve out whole importing packages.
type boundaryRule struct {
	name          string
	scopes        []string
	deny          []string
	allowPrefixes []string
	allowPackages []string
}

func TestBoundaryImports(t *testing.T) {
	rules := []boundaryRule{
		{
			// Channel and the discuss pipeline reach the agent only through
			// the turn contract and the event vocabulary.
			name:   "channel and pipeline must not import agent internals",
			scopes: []string{modulePrefix + "internal/channel/...", modulePrefix + "internal/pipeline"},
			deny:   []string{modulePrefix + "internal/agent"},
			allowPrefixes: []string{
				modulePrefix + "internal/agent/turn",
				modulePrefix + "internal/agent/event",
			},
		},
		{
			name:   "channel and pipeline must not import the flow resolver",
			scopes: []string{modulePrefix + "internal/channel/...", modulePrefix + "internal/pipeline"},
			deny:   []string{modulePrefix + "internal/conversation/flow"},
		},
		{
			// The conversation package (ChatRequest et al.) is agent-side.
			// internal/channel/route keeps a documented domain-service
			// exception: it creates conversation records for thread routing.
			name:          "channel and pipeline must not import conversation",
			scopes:        []string{modulePrefix + "internal/channel/...", modulePrefix + "internal/pipeline"},
			deny:          []string{modulePrefix + "internal/conversation"},
			allowPackages: []string{modulePrefix + "internal/channel/route"},
		},
		{
			// The contract package stays lean: no HTTP framework, no DI
			// container, no generated SQL, no channel back-edge.
			name:   "turn contract must not import echo/fx/sqlc/channel",
			scopes: []string{modulePrefix + "internal/agent/turn"},
			deny: []string{
				"github.com/labstack/echo/v4",
				"go.uber.org/fx",
				modulePrefix + "internal/db/postgres/sqlc",
				modulePrefix + "internal/channel",
			},
		},
		{
			// Composition roots depend on business packages, never the
			// reverse.
			name:   "business packages must not import composition roots",
			scopes: []string{modulePrefix + "internal/..."},
			deny:   []string{modulePrefix + "internal/app"},
			allowPackages: []string{
				modulePrefix + "internal/app/core",
				modulePrefix + "internal/app/channel",
			},
		},
	}

	for _, rule := range rules {
		t.Run(rule.name, func(t *testing.T) {
			for _, p := range loadImports(t, rule.scopes...) {
				if allowed(p.ImportPath, rule.allowPackages) {
					continue
				}
				for _, imp := range p.Imports {
					if !denied(imp, rule.deny) || allowed(imp, rule.allowPrefixes) {
						continue
					}
					t.Errorf("%s imports forbidden %s", p.ImportPath, imp)
				}
			}
		})
	}
}

func denied(imp string, deny []string) bool {
	for _, d := range deny {
		if imp == d || strings.HasPrefix(imp, d+"/") {
			return true
		}
	}
	return false
}

func allowed(path string, prefixes []string) bool {
	for _, a := range prefixes {
		if path == a || strings.HasPrefix(path, a+"/") {
			return true
		}
	}
	return false
}

// TestDefaultTeamIDReferences keeps the single-team constant confined to
// the DB layer, composition roots, commands, and the memory provider
// registry's documented resolver defaults. Business packages must resolve
// the team from context instead of hardcoding the singleton (spec §8).
func TestDefaultTeamIDReferences(t *testing.T) {
	allowedPrefixes := []string{
		"internal/db/",
		"internal/app/",
		"internal/team/",
		"internal/memory/adapters/",
		"cmd/",
	}
	root := repoRoot(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".claude" || name == "apps" || name == "packages" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // repo-local walk in a test
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "team.DefaultTeamID") {
			t.Errorf("%s references team.DefaultTeamID outside the allowed layers", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
