package supermarket

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	pluginspkg "github.com/memohai/memoh/internal/plugins"
	"github.com/memohai/memoh/internal/skillpackages"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestInstallerPreparationLimit(t *testing.T) {
	installer := NewInstaller(nil, nil, nil, nil, nil, nil)
	first, err := installer.acquirePreparation(context.Background())
	if err != nil {
		t.Fatalf("acquire first preparation: %v", err)
	}
	second, err := installer.acquirePreparation(context.Background())
	if err != nil {
		first()
		t.Fatalf("acquire second preparation: %v", err)
	}
	defer first()
	defer second()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release, err := installer.acquirePreparation(ctx)
	if release != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("saturated acquire = (%v, %v), want canceled", release != nil, err)
	}
}

func TestValidatePluginEntryRejectsPackageLockMismatch(t *testing.T) {
	manifest := pluginspkg.Manifest{
		ID:       "notion",
		Packages: []pluginspkg.PackageReference{{RegistryID: "memoh", PackageID: "notion"}},
	}
	entry := PluginEntry{
		Manifest: manifest,
		Release: PluginRelease{
			Revision:    "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PublishedAt: "2026-08-01T00:00:00Z",
			Artifact: PluginArtifact{
				Format: "memoh_plugin_v1", ContentType: "application/gzip", Size: 1,
				Digest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				DownloadURL: "/api/artifacts/plugin/digest",
			},
			Packages: []PluginResolvedPackage{{
				RegistryID: "memoh", PackageID: "other",
				Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			}},
		},
	}
	if err := validatePluginEntry(entry, "notion", manifest); err == nil {
		t.Fatal("validatePluginEntry accepted a Package lock that differs from plugin.yaml")
	}
}

func TestValidatePackageRejectsUnboundedArtifact(t *testing.T) {
	skill := CatalogSkill{
		RegistryID: "registry", PackageID: "package", SkillID: "skill",
		InstallID: "registry+package+skill",
		Artifact: SkillArtifact{
			Format: "memoh_skill_v1", ContentType: "application/gzip",
			Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Size:   1, UncompressedSize: 1, ArchiveSize: 1, FileCount: 0,
			DownloadURL: "/api/artifacts/skill/digest",
		},
	}
	pkg := SkillPackageDescriptor{
		SkillPackageSummary: SkillPackageSummary{
			SchemaVersion: "1", RegistryID: "registry", PackageID: "package", SkillCount: 1,
		},
		Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Skills:   []CatalogSkill{skill},
	}
	if err := validatePackage(pkg, "registry", "package"); err == nil {
		t.Fatal("validatePackage accepted an Artifact without a positive file count")
	}
}

func TestValidatePackageRejectsIdentityCountAndDuplicates(t *testing.T) {
	validSkill := func(id string) CatalogSkill {
		return CatalogSkill{
			RegistryID: "registry", PackageID: "package", SkillID: id,
			InstallID: "registry+package+" + id,
			Artifact: SkillArtifact{
				Format: "memoh_skill_v1", ContentType: "application/gzip",
				Digest: strings.Repeat("a", 64), Size: 1, UncompressedSize: 1,
				ArchiveSize: 1, FileCount: 1, DownloadURL: "/api/artifacts/skill/digest",
			},
		}
	}
	validPackage := func() SkillPackageDescriptor {
		return SkillPackageDescriptor{
			SkillPackageSummary: SkillPackageSummary{
				SchemaVersion: "1", RegistryID: "registry", PackageID: "package", SkillCount: 1,
			},
			Revision: strings.Repeat("b", 64), Skills: []CatalogSkill{validSkill("skill")},
		}
	}
	tests := map[string]func(*SkillPackageDescriptor){
		"registry identity": func(pkg *SkillPackageDescriptor) { pkg.RegistryID = "other" },
		"member count":      func(pkg *SkillPackageDescriptor) { pkg.SkillCount = 2 },
		"member package":    func(pkg *SkillPackageDescriptor) { pkg.Skills[0].PackageID = "other" },
		"duplicate member": func(pkg *SkillPackageDescriptor) {
			pkg.Skills = append(pkg.Skills, pkg.Skills[0])
			pkg.SkillCount = 2
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			pkg := validPackage()
			mutate(&pkg)
			if err := validatePackage(pkg, "registry", "package"); err == nil {
				t.Fatal("validatePackage accepted invalid Package")
			}
		})
	}
	pkg := validPackage()
	pkg.Skills[0].Artifact.UncompressedSize = maxPackageArtifactsUncompressed + 1
	if err := validatePackageBudget(pkg.Skills); err == nil {
		t.Fatal("validatePackageBudget accepted an oversized Package")
	}
}

func TestInstallPluginRejectsPartialExpectedState(t *testing.T) {
	revision := strings.Repeat("a", 64)
	installer := NewInstaller(nil, &installerPluginStub{}, nil, nil, nil, nil)
	_, err := installer.InstallPlugin(context.Background(), "bot", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: revision, ExpectedInstalledRevision: &revision,
	})
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Status != 400 {
		t.Fatalf("InstallPlugin partial state error = %v", err)
	}
}

func TestPackageRevisionConflictUsesHTTPConflict(t *testing.T) {
	err := packageLifecycleError(skillpackages.ErrRevisionConflict)
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Status != http.StatusConflict {
		t.Fatalf("packageLifecycleError() = %v, want HTTP 409", err)
	}
}

func TestRunPluginScriptsUsesPluginRootAndStopsOnFailure(t *testing.T) {
	executor := &scriptExecutorStub{results: []*bridge.ExecResult{
		{Stdout: strings.Repeat("x", pluginScriptOutputLimit+1)},
		{Stderr: "boom", ExitCode: 7},
		{Stdout: "not run"},
	}}
	result, err := runPluginScripts(context.Background(), executor, "bot-1", "notion", pluginspkg.InstallCommands{
		" sh scripts/one.sh ", "sh scripts/two.sh", "sh scripts/three.sh",
	})
	if err == nil || result.OK || result.CommandsRun != 2 || len(executor.calls) != 2 {
		t.Fatalf("runPluginScripts result=%+v calls=%d error=%v", result, len(executor.calls), err)
	}
	if len(result.Results[0].Stdout) != pluginScriptOutputLimit {
		t.Fatalf("stdout length = %d", len(result.Results[0].Stdout))
	}
	if executor.calls[0].workDir != "/data/.memoh/plugins/notion" || executor.calls[0].timeout != pluginScriptTimeoutSeconds {
		t.Fatalf("first call = %+v", executor.calls[0])
	}
	wantEnv := "MEMOH_PLUGIN_ID=notion\nMEMOH_PLUGIN_DIR=/data/.memoh/plugins/notion\nMEMOH_BOT_ID=bot-1"
	if strings.Join(executor.calls[0].env, "\n") != wantEnv {
		t.Fatalf("environment = %#v", executor.calls[0].env)
	}
}

func TestRunPluginScriptsReportsExecutionError(t *testing.T) {
	executor := &scriptExecutorStub{errs: []error{errors.New("bridge unavailable")}}
	result, err := runPluginScripts(
		context.Background(), executor, "bot-1", "notion", pluginspkg.InstallCommands{"sh scripts/install.sh"},
	)
	if err == nil || result.OK || result.CommandsRun != 1 || result.Results[0].Error != "bridge unavailable" {
		t.Fatalf("runPluginScripts result=%+v error=%v", result, err)
	}
}

type installerPluginStub struct{}

func (*installerPluginStub) WithBotMutation(context.Context, string, func(context.Context) error) error {
	return nil
}

func (*installerPluginStub) Install(context.Context, string, pluginspkg.InstallRequest) (pluginspkg.Installation, error) {
	return pluginspkg.Installation{}, nil
}

func (*installerPluginStub) InstalledPluginState(context.Context, string, string) (pluginspkg.InstalledPluginState, bool, error) {
	return pluginspkg.InstalledPluginState{}, false, nil
}

type scriptCall struct {
	command string
	workDir string
	timeout int32
	env     []string
}

type scriptExecutorStub struct {
	results []*bridge.ExecResult
	errs    []error
	calls   []scriptCall
}

func (s *scriptExecutorStub) ExecWithEnv(_ context.Context, command, workDir string, timeout int32, env []string) (*bridge.ExecResult, error) {
	index := len(s.calls)
	s.calls = append(s.calls, scriptCall{command: command, workDir: workDir, timeout: timeout, env: append([]string(nil), env...)})
	var result *bridge.ExecResult
	if index < len(s.results) {
		result = s.results[index]
	}
	var err error
	if index < len(s.errs) {
		err = s.errs[index]
	}
	return result, err
}
