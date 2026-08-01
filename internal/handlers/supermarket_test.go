package handlers

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestPluginBundleArchiveEntryAllowsTrustedBundleFiles(t *testing.T) {
	tests := []struct {
		name         string
		wantKind     string
		wantRelative string
	}{
		{name: "github/hooks.json", wantKind: pluginArchiveKindHooks, wantRelative: "hooks.json"},
		{name: "github/scripts/hook.py", wantKind: pluginArchiveKindScripts, wantRelative: "hook.py"},
	}

	for _, tt := range tests {
		got, ok, err := pluginBundleArchiveEntry("github", "github", tt.name)
		if err != nil {
			t.Fatalf("pluginBundleArchiveEntry(%q) err = %v", tt.name, err)
		}
		if !ok || got.kind != tt.wantKind || got.relativePath != tt.wantRelative {
			t.Fatalf("pluginBundleArchiveEntry(%q) = %+v, %v; want kind %q relative %q", tt.name, got, ok, tt.wantKind, tt.wantRelative)
		}
	}
}

func TestPluginBundleArchiveEntryRejectsEmbeddedSkillContent(t *testing.T) {
	if _, ok, err := pluginBundleArchiveEntry("github", "github", "github/skills/review/SKILL.md"); err == nil || ok {
		t.Fatalf("embedded Skill entry = ok:%v err:%v, want explicit rejection", ok, err)
	}
}

func TestPluginBundleArchiveEntryRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{
		"",
		"github/plugin.yaml",
		"github/../escape",
		"github/skills/../escape",
		"github/scripts/../escape",
		"../escape",
		"/data/escape",
		"github/skills\\escape",
	} {
		if got, ok, err := pluginBundleArchiveEntry("github", "github", name); err != nil || ok {
			t.Fatalf("pluginBundleArchiveEntry(%q) = %+v, %v, %v; want rejected", name, got, ok, err)
		}
	}
}

func TestExtractPluginBundleArchiveWritesOnlySafeBundleFiles(t *testing.T) {
	pluginRoot, err := skillset.PluginDirForID("github")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	archive := tarArchive(t, map[string]string{
		"github/plugin.yaml":       "id: github",
		"github/hooks.json":        `{"version":1,"hooks":[]}`,
		"github/scripts/hook.py":   "print('ok')",
		"github/scripts/../escape": "escape",
		"github/../outside":        "outside",
		"/data/outside":            "absolute",
	})
	writer := &pluginBundleTestWriter{files: map[string]string{
		pluginRoot + "/hooks.json":          `{"version":1,"hooks":[{"name":"stale"}]}`,
		pluginRoot + "/scripts/stale.py":    "print('stale')",
		"/data/.memoh/plugins/other/keep":   "keep",
		"/data/.memoh/plugins/github2/keep": "keep",
	}}

	result, err := extractPluginBundleArchive(context.Background(), writer, "github", "github", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("extractPluginBundleArchive returned error: %v", err)
	}
	if result.Hooks.FilesWritten != 1 || result.Scripts.FilesWritten != 1 {
		t.Fatalf("install result = %+v, want 1 hook and 1 script", result)
	}
	if len(writer.deletes) != 1 {
		t.Fatalf("deletes = %+v, want one plugin root delete", writer.deletes)
	}
	if writer.deletes[0].path != pluginRoot || !writer.deletes[0].recursive {
		t.Fatalf("delete = %+v, want recursive delete of %s", writer.deletes[0], pluginRoot)
	}
	wantFiles := map[string]string{
		pluginRoot + "/hooks.json":      `{"version":1,"hooks":[]}`,
		pluginRoot + "/scripts/hook.py": "print('ok')",
	}
	for path, want := range wantFiles {
		if got := writer.files[path]; got != want {
			t.Fatalf("file %s = %q, want %q", path, got, want)
		}
	}
	for _, stalePath := range []string{
		pluginRoot + "/scripts/stale.py",
	} {
		if _, ok := writer.files[stalePath]; ok {
			t.Fatalf("stale file was not cleared before extraction: %s", stalePath)
		}
	}
	for _, preservedPath := range []string{
		"/data/.memoh/plugins/other/keep",
		"/data/.memoh/plugins/github2/keep",
	} {
		if got := writer.files[preservedPath]; got != "keep" {
			t.Fatalf("unrelated plugin file %s = %q, want keep", preservedPath, got)
		}
	}
	for path := range writer.files {
		if strings.Contains(path, "plugin.yaml") || strings.Contains(path, "outside") || strings.Contains(path, "escape") {
			t.Fatalf("unsafe file was written: %s", path)
		}
	}
}

func TestExtractPluginBundleArchiveSeparatesArchiveAndTargetPluginIDs(t *testing.T) {
	archive := tarArchive(t, map[string]string{
		"GitHub.Plugin/hooks.json":      `{"version":1,"hooks":[]}`,
		"GitHub.Plugin/scripts/hook.py": "print('ok')",
	})
	writer := &pluginBundleTestWriter{files: map[string]string{}}

	result, err := extractPluginBundleArchive(context.Background(), writer, "GitHub.Plugin", "github_plugin", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("extractPluginBundleArchive returned error: %v", err)
	}
	if result.Hooks.FilesWritten != 1 || result.Scripts.FilesWritten != 1 {
		t.Fatalf("install result = %+v, want one hook and one script", result)
	}

	pluginRoot, err := skillset.PluginDirForID("github_plugin")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	if got := writer.files[pluginRoot+"/hooks.json"]; got != `{"version":1,"hooks":[]}` {
		t.Fatalf("target hooks file = %q, want hooks config", got)
	}
	if got := writer.files[pluginRoot+"/scripts/hook.py"]; got != "print('ok')" {
		t.Fatalf("target script file = %q, want script", got)
	}
}

func TestRunPluginInstallCommandsUsesPluginRootAndMemohEnv(t *testing.T) {
	pluginRoot, err := skillset.PluginDirForID("github")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	longOutput := strings.Repeat("x", pluginInstallScriptOutputLimit+8)
	executor := &pluginInstallScriptTestExecutor{
		results: []*bridge.ExecResult{
			{Stdout: longOutput, ExitCode: 0},
			{Stderr: "setup ok\n", ExitCode: 0},
		},
	}

	result, err := runPluginInstallCommands(context.Background(), executor, "bot-1", "github", []string{
		" sh scripts/install.sh ",
		"",
		"python3 scripts/setup.py",
	})
	if err != nil {
		t.Fatalf("runPluginInstallCommands returned error: %v", err)
	}
	if !result.OK || result.CommandsRun != 2 || len(result.Results) != 2 {
		t.Fatalf("result = %+v, want two successful commands", result)
	}
	if len(result.Results[0].Stdout) != pluginInstallScriptOutputLimit {
		t.Fatalf("stdout was not truncated to limit: %d", len(result.Results[0].Stdout))
	}

	wantCommands := []string{"sh scripts/install.sh", "python3 scripts/setup.py"}
	if len(executor.calls) != len(wantCommands) {
		t.Fatalf("calls = %+v, want %d calls", executor.calls, len(wantCommands))
	}
	for i, call := range executor.calls {
		if call.command != wantCommands[i] {
			t.Fatalf("call %d command = %q, want %q", i, call.command, wantCommands[i])
		}
		if call.workDir != pluginRoot {
			t.Fatalf("call %d work dir = %q, want %q", i, call.workDir, pluginRoot)
		}
		if call.timeout != pluginInstallScriptTimeoutSeconds {
			t.Fatalf("call %d timeout = %d, want %d", i, call.timeout, pluginInstallScriptTimeoutSeconds)
		}
		wantEnv := []string{
			"MEMOH_PLUGIN_ID=github",
			"MEMOH_PLUGIN_DIR=" + pluginRoot,
			"MEMOH_BOT_ID=bot-1",
		}
		if strings.Join(call.env, "\n") != strings.Join(wantEnv, "\n") {
			t.Fatalf("call %d env = %#v, want %#v", i, call.env, wantEnv)
		}
	}
}

func TestRunPluginInstallCommandsStopsOnNonZeroExit(t *testing.T) {
	executor := &pluginInstallScriptTestExecutor{
		results: []*bridge.ExecResult{
			{Stdout: "ok\n", ExitCode: 0},
			{Stderr: "boom\n", ExitCode: 7},
			{Stdout: "should not run\n", ExitCode: 0},
		},
	}

	result, err := runPluginInstallCommands(context.Background(), executor, "bot-1", "github", []string{
		"sh scripts/one.sh",
		"sh scripts/two.sh",
		"sh scripts/three.sh",
	})
	if err == nil {
		t.Fatal("expected non-zero exit to fail")
	}
	if result.OK || result.CommandsRun != 2 || len(result.Results) != 2 {
		t.Fatalf("result = %+v, want failure after second command", result)
	}
	if result.Results[1].ExitCode != 7 || result.Results[1].Stderr != "boom\n" || result.Results[1].Error == "" {
		t.Fatalf("failed command result = %+v, want exit code, stderr, and error", result.Results[1])
	}
	if len(executor.calls) != 2 {
		t.Fatalf("commands run = %d, want 2", len(executor.calls))
	}
}

func TestRunPluginInstallCommandsReportsExecError(t *testing.T) {
	executor := &pluginInstallScriptTestExecutor{
		errors: []error{errors.New("bridge unavailable")},
	}

	result, err := runPluginInstallCommands(context.Background(), executor, "bot-1", "github", []string{"sh scripts/install.sh"})
	if err == nil {
		t.Fatal("expected exec error")
	}
	if result.OK || result.CommandsRun != 1 || len(result.Results) != 1 || result.Results[0].Error != "bridge unavailable" {
		t.Fatalf("result = %+v, want exec error metadata", result)
	}
}

func tarArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

type pluginBundleTestWriter struct {
	dirs    []string
	deletes []pluginBundleTestDelete
	files   map[string]string
}

type pluginBundleTestDelete struct {
	path      string
	recursive bool
}

func (w *pluginBundleTestWriter) DeleteFile(_ context.Context, path string, recursive bool) error {
	w.deletes = append(w.deletes, pluginBundleTestDelete{path: path, recursive: recursive})
	if !recursive {
		delete(w.files, path)
		return nil
	}
	for filePath := range w.files {
		if filePath == path || strings.HasPrefix(filePath, path+"/") {
			delete(w.files, filePath)
		}
	}
	return nil
}

func (w *pluginBundleTestWriter) Mkdir(_ context.Context, path string) error {
	w.dirs = append(w.dirs, path)
	return nil
}

func (w *pluginBundleTestWriter) WriteFile(_ context.Context, path string, content []byte) error {
	w.files[path] = string(content)
	return nil
}

type pluginInstallScriptTestExecutor struct {
	calls   []pluginInstallScriptTestCall
	results []*bridge.ExecResult
	errors  []error
}

type pluginInstallScriptTestCall struct {
	command string
	workDir string
	timeout int32
	env     []string
}

func (e *pluginInstallScriptTestExecutor) ExecWithEnv(_ context.Context, command, workDir string, timeout int32, env []string) (*bridge.ExecResult, error) {
	callIndex := len(e.calls)
	e.calls = append(e.calls, pluginInstallScriptTestCall{
		command: command,
		workDir: workDir,
		timeout: timeout,
		env:     append([]string(nil), env...),
	})
	var result *bridge.ExecResult
	if callIndex < len(e.results) {
		result = e.results[callIndex]
	}
	if result == nil {
		result = &bridge.ExecResult{ExitCode: 0}
	}
	var err error
	if callIndex < len(e.errors) {
		err = e.errors[callIndex]
	}
	return result, err
}
