// Package arch mechanically enforces the channel-boundary dependency rules
// from docs/superpowers/specs/2026-07-17-channel-boundary-design.md §8.
// Every exemption below is deliberate and carries its rationale; removing
// code from an exemption list is always safe, adding to one is a design
// decision that belongs in the spec.
package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const modulePrefix = "github.com/memohai/memoh/"

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate arch test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// goFiles yields non-test .go files under dir (relative to root), with
// forward-slash paths relative to the repo root.
func goFiles(t *testing.T, root, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func imports(t *testing.T, root, relPath string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, relPath), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}
	var result []string
	for _, imp := range f.Imports {
		result = append(result, strings.Trim(imp.Path.Value, `"`))
	}
	return result
}

// TestChannelAndPipelineOnlyDependOnTurnPort: after the split, the only
// agent-side surface the channel boundary may touch is the turn port and
// the pure event vocabulary its payloads carry.
func TestChannelAndPipelineOnlyDependOnTurnPort(t *testing.T) {
	exempt := map[string]string{
		// Spec §4 defers the route/ reorganization: conversation-routing
		// storage stays put until the follow-up move-only PR.
		"internal/channel/route/service.go": "route service still owns conversation-record creation (spec §4 deferral)",
	}
	allowedAgentImports := map[string]bool{
		modulePrefix + "internal/agent/turn": true,
		// Pure data vocabulary (StreamEvent kinds); turn event payloads
		// are serialized agent events, so consuming the vocabulary is
		// part of the port, not a boundary leak.
		modulePrefix + "internal/agent/event": true,
	}
	root := repoRoot(t)
	for _, dir := range []string{"internal/channel", "internal/pipeline"} {
		for _, file := range goFiles(t, root, dir) {
			if _, ok := exempt[file]; ok {
				continue
			}
			for _, imp := range imports(t, root, file) {
				if allowedAgentImports[imp] || strings.HasPrefix(imp, modulePrefix+"internal/agent/turn/") {
					continue
				}
				if imp == modulePrefix+"internal/conversation" ||
					strings.HasPrefix(imp, modulePrefix+"internal/conversation/") ||
					imp == modulePrefix+"internal/agent" ||
					strings.HasPrefix(imp, modulePrefix+"internal/agent/") {
					t.Errorf("%s imports %s: the channel boundary may only depend on internal/agent/turn (and internal/agent/event)", file, imp)
				}
			}
		}
	}
}

// TestChannelDoesNotImportEcho: webhook endpoints are the only HTTP surface
// the channel package owns (spec §8 exemption).
func TestChannelDoesNotImportEcho(t *testing.T) {
	exempt := map[string]string{
		"internal/channel/webhook_handler.go":                 "channel-owned webhook HTTP endpoint",
		"internal/channel/adapters/feishu/webhook_handler.go": "platform webhook endpoint",
		"internal/channel/adapters/line/adapter.go":           "platform webhook endpoint",
		"internal/channel/adapters/wechatoa/inbound.go":       "platform webhook endpoint",
		"internal/channel/adapters/weixin/qr_handler.go":      "weixin QR login HTTP endpoint",
	}
	root := repoRoot(t)
	for _, file := range goFiles(t, root, "internal/channel") {
		if _, ok := exempt[file]; ok {
			continue
		}
		for _, imp := range imports(t, root, file) {
			if strings.HasPrefix(imp, "github.com/labstack/echo") {
				t.Errorf("%s imports Echo outside the webhook-endpoint exemptions", file)
			}
		}
	}
}

// TestChannelAndPipelineDoNotImportFx: assembly lives in cmd/** only.
func TestChannelAndPipelineDoNotImportFx(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{"internal/channel", "internal/pipeline"} {
		for _, file := range goFiles(t, root, dir) {
			for _, imp := range imports(t, root, file) {
				if strings.HasPrefix(imp, "go.uber.org/fx") {
					t.Errorf("%s imports fx: assembly belongs to cmd/**", file)
				}
			}
		}
	}
}

// TestTurnPortStaysPure: the contract package (and its transports) must not
// depend on HTTP frameworks, assembly, generated SQL, or the channel side.
func TestTurnPortStaysPure(t *testing.T) {
	root := repoRoot(t)
	for _, file := range goFiles(t, root, "internal/agent/turn") {
		for _, imp := range imports(t, root, file) {
			switch {
			case strings.HasPrefix(imp, "github.com/labstack/echo"):
				t.Errorf("%s imports Echo", file)
			case strings.HasPrefix(imp, "go.uber.org/fx"):
				t.Errorf("%s imports fx", file)
			case imp == modulePrefix+"internal/db/postgres/sqlc":
				t.Errorf("%s imports generated sqlc", file)
			case imp == modulePrefix+"internal/channel" || strings.HasPrefix(imp, modulePrefix+"internal/channel/"):
				t.Errorf("%s imports the channel side", file)
			}
		}
	}
}

// TestDefaultTeamIDReferences: business packages must not hardcode the
// single-team assumption. Each exemption is a deliberate singleton-team
// touchpoint that a hosted multi-team runtime replaces wholesale.
func TestDefaultTeamIDReferences(t *testing.T) {
	allowedPrefixes := []string{
		"cmd/",
		"internal/db/",
		"internal/team/",
	}
	exempt := map[string]string{
		"internal/memory/adapters/registry.go":               "memory provider registry resolves the singleton team when no scope is bound",
		"internal/memory/adapters/builtin/pgvector_index.go": "builtin pgvector index binds the singleton team resolver",
		"internal/channel/service.go":                        "configless channels (web/cli) synthesize a config; turn.Service fails closed on empty TeamID",
	}
	ref := regexp.MustCompile(`\bteam\.DefaultTeamID\b`)
	root := repoRoot(t)
	for _, dir := range []string{"internal", "cmd"} {
		for _, file := range goFiles(t, root, dir) {
			data, err := os.ReadFile(filepath.Join(root, file))
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if !ref.Match(data) {
				continue
			}
			if _, ok := exempt[file]; ok {
				continue
			}
			allowed := false
			for _, prefix := range allowedPrefixes {
				if strings.HasPrefix(file, prefix) {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("%s references team.DefaultTeamID outside the allowed layers (internal/db, cmd/**, tests) without a documented exemption", file)
			}
		}
	}
}
