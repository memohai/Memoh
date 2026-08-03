package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/config"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	supermarketclient "github.com/memohai/memoh/internal/supermarket"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestInstallPluginDownloadsReferencedRegistrySkills(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	reference := pluginspkg.PackageReference{RegistryID: "memoh", PackageID: "notion"}
	artifact := validSkillArtifact(t)
	digest := sha256.Sum256(artifact)
	digestText := hex.EncodeToString(digest[:])
	descriptor := validRegistrySkillDescriptor()
	descriptor.RegistryID = reference.RegistryID
	descriptor.PackageID = reference.PackageID
	descriptor.SkillID = "meeting"
	descriptor.InstallID = "memoh+notion+meeting"
	descriptor.Artifact.Digest = digestText
	descriptor.Artifact.Size = int64(len(artifact))
	descriptor.Artifact.DownloadURL = "/artifacts/meeting"
	manifest := pluginspkg.Manifest{ID: "notion", Name: "Notion", Packages: []pluginspkg.PackageReference{reference}}
	bundle := gzipTarArchive(t, map[string]string{
		"notion/plugin.yaml": "id: notion\npackages:\n  - registry_id: memoh\n    package_id: notion\n",
	})
	entry := validSupermarketPluginEntry(manifest, bundle, descriptor)
	packageJSON := supermarketPackageReleaseJSON(t, &entry.Release.Packages[0], descriptor)
	releaseJSON := sealSupermarketPluginEntry(t, &entry)
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal Plugin release: %v", err)
	}
	installer := &recordingPluginInstaller{}
	requestedPaths := make([]string, 0, 3)
	artifactRequestedDuringMutation := false
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPaths = append(requestedPaths, req.URL.Path)
			status := http.StatusOK
			var content []byte
			switch req.URL.Path {
			case "/api/plugins/notion":
				content = entryJSON
			case "/api/plugins/notion/releases/" + entry.Release.Revision:
				content = releaseJSON
			case registryPackageReleaseUpstreamPath(reference.RegistryID, reference.PackageID, entry.Release.Packages[0].Revision):
				content = packageJSON
			case "/api/artifacts/skill/" + digestText:
				artifactRequestedDuringMutation = installer.mutationCalls > 0
				content = artifact
			case "/api/artifacts/plugin/" + entry.Release.Artifact.Digest:
				artifactRequestedDuringMutation = artifactRequestedDuringMutation || installer.mutationCalls > 0
				content = bundle
			default:
				status = http.StatusNotFound
				content = nil
			}
			return testHTTPResponse(req, status, content), nil
		})}),
		botService:     env.handler.botService,
		accountService: env.handler.accountService,
		logger:         slog.New(slog.DiscardHandler),
	}
	handler.installer = supermarketclient.NewInstaller(handler.upstream, installer, manager, manager, handler.logger)

	recorder, err := env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: entry.Release.Revision,
	}, handler.InstallPlugin)
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v, cause = %v", err, apperror.CauseOf(err))
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("InstallPlugin() status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	installedSkill := pluginspkg.InstalledSkill{RegistryID: "memoh", PackageID: "notion", SkillID: "meeting"}
	metadata, ok := installer.request.SkillArtifacts[pluginspkg.InstalledSkillIdentity(installedSkill)]
	if !ok || metadata.ArtifactDigest != digestText || metadata.FilesWritten != 1 {
		t.Fatalf("installed Skill metadata = %+v, %v", metadata, ok)
	}
	if metadata.PackageRevision != entry.Release.Packages[0].Revision {
		t.Fatalf("Package revision = %q, want %q", metadata.PackageRevision, entry.Release.Packages[0].Revision)
	}
	if installer.request.Release.Revision != entry.Release.Revision ||
		installer.request.Release.ArtifactDigest != entry.Release.Artifact.Digest {
		t.Fatalf("installed Plugin release metadata = %+v, want revision and Artifact digest", installer.request.Release)
	}
	if installer.request.WorkspaceTargetID != workspace.WorkspaceTargetNative {
		t.Fatalf("workspace target = %q, want native", installer.request.WorkspaceTargetID)
	}
	if artifactRequestedDuringMutation {
		t.Fatal("Plugin installation downloaded an Artifact while holding the bot mutation lock")
	}
	if slices.Contains(requestedPaths, "/api/registries/memoh/packages/notion/skills/meeting") {
		t.Fatalf("Plugin installation queried the mutable Skill endpoint: %+v", requestedPaths)
	}
	packageReleasePath := registryPackageReleaseUpstreamPath(
		reference.RegistryID, reference.PackageID, entry.Release.Packages[0].Revision,
	)
	if !slices.Contains(requestedPaths, packageReleasePath) {
		t.Fatalf("Plugin installation did not fetch its immutable Package release: %+v", requestedPaths)
	}
	if slices.Contains(requestedPaths, registryPackageUpstreamPath(reference.RegistryID, reference.PackageID)) {
		t.Fatalf("Plugin installation queried the mutable Package endpoint: %+v", requestedPaths)
	}
	sourcePath := path.Join(skillset.ManagedDir(), "memoh", "notion", "meeting", "SKILL.md")
	if _, err := os.ReadFile(env.localPath(sourcePath)); err != nil {
		t.Fatalf("read installed Plugin Skill: %v", err)
	}
	if installer.mutationCalls != 1 {
		t.Fatalf("bot mutation scopes = %d, want 1", installer.mutationCalls)
	}
	if !installer.installInMutation {
		t.Fatal("Plugin ownership was recorded outside the bot mutation scope")
	}
	var installation pluginspkg.Installation
	if err := json.Unmarshal(recorder.Body.Bytes(), &installation); err != nil {
		t.Fatalf("decode installation: %v", err)
	}
	if _, ok := installation.Metadata["skills_install"]; !ok {
		t.Fatalf("installation metadata omitted skills_install: %+v", installation.Metadata)
	}
}

func TestInstallPluginRejectsStaleReleaseBeforeWorkspaceMutation(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	bundle := gzipTarArchive(t, map[string]string{"notion/plugin.yaml": "id: notion\n"})
	entry := validSupermarketPluginEntry(pluginspkg.Manifest{ID: "notion", Name: "Notion"}, bundle)
	releaseJSON := sealSupermarketPluginEntry(t, &entry)
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal Plugin entry: %v", err)
	}
	artifactRequested := false
	installer := &recordingPluginInstaller{}
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/plugins/notion":
				return testHTTPResponse(req, http.StatusOK, entryJSON), nil
			case "/api/plugins/notion/releases/" + entry.Release.Revision:
				return testHTTPResponse(req, http.StatusOK, releaseJSON), nil
			case "/api/artifacts/plugin/" + entry.Release.Artifact.Digest:
				artifactRequested = true
				return testHTTPResponse(req, http.StatusOK, bundle), nil
			default:
				return testHTTPResponse(req, http.StatusNotFound, nil), nil
			}
		})}),
		botService: env.handler.botService, accountService: env.handler.accountService,
		logger: slog.New(slog.DiscardHandler),
	}
	handler.installer = supermarketclient.NewInstaller(handler.upstream, installer, manager, manager, handler.logger)

	_, err = env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: strings.Repeat("0", 64),
	}, handler.InstallPlugin)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("InstallPlugin() error = %v, want HTTP 409", err)
	}
	if artifactRequested || installer.mutationCalls != 0 {
		t.Fatalf("stale release touched installation state: artifact=%v mutations=%d", artifactRequested, installer.mutationCalls)
	}
}

func TestInstallPluginRequiresExpectedInstalledRevision(t *testing.T) {
	env := newSkillsTestEnv(t)
	handler := &SupermarketHandler{
		botService:     env.handler.botService,
		accountService: env.handler.accountService,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, err := env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", map[string]any{
		"plugin_id":        "notion",
		"release_revision": strings.Repeat("a", 64),
	}, handler.InstallPlugin)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("InstallPlugin() error = %v, want HTTP 400", err)
	}
}

func TestInstallPluginRejectsInstallationChangedWithinSameReleaseWhilePreparing(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	bundle := gzipTarArchive(t, map[string]string{"notion/plugin.yaml": "id: notion\n"})
	entry := validSupermarketPluginEntry(pluginspkg.Manifest{ID: "notion", Name: "Notion"}, bundle)
	releaseJSON := sealSupermarketPluginEntry(t, &entry)
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal Plugin entry: %v", err)
	}
	oldRevision := strings.Repeat("a", 64)
	installedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	installer := &recordingPluginInstaller{
		installed: true, installedRevision: oldRevision, installedUpdatedAt: installedAt,
	}
	installer.beforeMutation = func() {
		installer.installedUpdatedAt = installedAt.Add(time.Second)
	}
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/plugins/notion":
				return testHTTPResponse(req, http.StatusOK, entryJSON), nil
			case "/api/plugins/notion/releases/" + entry.Release.Revision:
				return testHTTPResponse(req, http.StatusOK, releaseJSON), nil
			case "/api/artifacts/plugin/" + entry.Release.Artifact.Digest:
				return testHTTPResponse(req, http.StatusOK, bundle), nil
			default:
				return testHTTPResponse(req, http.StatusNotFound, nil), nil
			}
		})}),
		botService: env.handler.botService, accountService: env.handler.accountService,
		logger: slog.New(slog.DiscardHandler),
	}
	handler.installer = supermarketclient.NewInstaller(handler.upstream, installer, manager, manager, handler.logger)

	_, err = env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: entry.Release.Revision,
		ExpectedInstalledRevision: &oldRevision, ExpectedInstallationUpdatedAt: &installedAt,
	}, handler.InstallPlugin)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("InstallPlugin() error = %v, want HTTP 409", err)
	}
	if installer.installCalls != 0 || !installer.releaseReadInMutation {
		t.Fatalf("stale install state: calls=%d release_read_in_mutation=%v", installer.installCalls, installer.releaseReadInMutation)
	}
	pluginPath := path.Join(skillset.PluginDirPath, "notion", "plugin.yaml")
	if _, statErr := os.Stat(env.localPath(pluginPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stale install changed the Plugin workspace: %v", statErr)
	}
}

func TestInstallPluginRollsBackSkillsWhenBundleInstallFails(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	reference := pluginspkg.PackageReference{RegistryID: "memoh", PackageID: "notion"}
	artifact := validSkillArtifact(t)
	digest := sha256.Sum256(artifact)
	descriptor := validRegistrySkillDescriptor()
	descriptor.RegistryID = reference.RegistryID
	descriptor.PackageID = reference.PackageID
	descriptor.SkillID = "meeting"
	descriptor.InstallID = "memoh+notion+meeting"
	descriptor.Artifact.Digest = hex.EncodeToString(digest[:])
	descriptor.Artifact.Size = int64(len(artifact))
	descriptor.Artifact.DownloadURL = "/artifacts/meeting"
	manifest := pluginspkg.Manifest{ID: "notion", Name: "Notion", Packages: []pluginspkg.PackageReference{reference}}
	bundle := gzipTarArchive(t, map[string]string{
		"notion/plugin.yaml": "id: notion\npackages:\n  - registry_id: memoh\n    package_id: notion\n",
	})
	entry := validSupermarketPluginEntry(manifest, bundle, descriptor)
	packageJSON := supermarketPackageReleaseJSON(t, &entry.Release.Packages[0], descriptor)
	releaseJSON := sealSupermarketPluginEntry(t, &entry)
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal Plugin release: %v", err)
	}
	env.writeSkillFile(t, path.Join(skillset.PluginDirPath, ".staging"), "not a directory")
	installer := &recordingPluginInstaller{}
	requestedMutableSkill := false
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/plugins/notion":
				return testHTTPResponse(req, http.StatusOK, entryJSON), nil
			case "/api/plugins/notion/releases/" + entry.Release.Revision:
				return testHTTPResponse(req, http.StatusOK, releaseJSON), nil
			case registryPackageReleaseUpstreamPath(reference.RegistryID, reference.PackageID, entry.Release.Packages[0].Revision):
				return testHTTPResponse(req, http.StatusOK, packageJSON), nil
			case "/api/registries/memoh/packages/notion/skills/meeting":
				requestedMutableSkill = true
				return testHTTPResponse(req, http.StatusInternalServerError, nil), nil
			case "/api/artifacts/skill/" + descriptor.Artifact.Digest:
				return testHTTPResponse(req, http.StatusOK, artifact), nil
			case "/api/artifacts/plugin/" + entry.Release.Artifact.Digest:
				return testHTTPResponse(req, http.StatusOK, bundle), nil
			default:
				return testHTTPResponse(req, http.StatusNotFound, nil), nil
			}
		})}),
		botService:     env.handler.botService,
		accountService: env.handler.accountService,
		logger:         slog.New(slog.DiscardHandler),
	}
	handler.installer = supermarketclient.NewInstaller(handler.upstream, installer, manager, manager, handler.logger)

	_, err = env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: entry.Release.Revision,
	}, handler.InstallPlugin)
	if err == nil {
		t.Fatal("InstallPlugin() succeeded after bundle download failure")
	}
	if installer.installCalls != 0 {
		t.Fatalf("Plugin ownership was recorded after bundle failure: %d calls", installer.installCalls)
	}
	if requestedMutableSkill {
		t.Fatal("Plugin installation queried the mutable Skill endpoint")
	}
	sourcePath := path.Join(skillset.ManagedDir(), "memoh", "notion", "meeting", "SKILL.md")
	if _, statErr := os.Stat(env.localPath(sourcePath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed Plugin installation retained a Skill: %v", statErr)
	}
}

func TestInstallPluginRestoresPreviousWorkspaceWhenDatabaseInstallFails(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	reference := pluginspkg.PackageReference{RegistryID: "memoh", PackageID: "notion"}
	skillArtifact := validSkillArtifact(t)
	skillDigest := sha256.Sum256(skillArtifact)
	descriptor := validRegistrySkillDescriptor()
	descriptor.RegistryID = reference.RegistryID
	descriptor.PackageID = reference.PackageID
	descriptor.SkillID = "meeting"
	descriptor.InstallID = "memoh+notion+meeting"
	descriptor.Artifact.Digest = hex.EncodeToString(skillDigest[:])
	descriptor.Artifact.Size = int64(len(skillArtifact))
	manifest := pluginspkg.Manifest{ID: "notion", Name: "Notion", Packages: []pluginspkg.PackageReference{reference}}
	bundle := gzipTarArchive(t, map[string]string{
		"notion/plugin.yaml": "id: notion\npackages:\n  - registry_id: memoh\n    package_id: notion\n",
	})
	entry := validSupermarketPluginEntry(manifest, bundle, descriptor)
	packageJSON := supermarketPackageReleaseJSON(t, &entry.Release.Packages[0], descriptor)
	releaseJSON := sealSupermarketPluginEntry(t, &entry)
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal Plugin entry: %v", err)
	}
	skillPath := path.Join(skillset.ManagedDir(), "memoh", "notion", "meeting", "SKILL.md")
	pluginPath := path.Join(skillset.PluginDirPath, "notion", "plugin.yaml")
	oldSkill := managedSkillRaw("meeting", "Previous meeting Skill")
	oldPlugin := "id: notion\nversion: old\n"
	env.writeSkillFile(t, skillPath, oldSkill)
	env.writeSkillFile(t, pluginPath, oldPlugin)
	installer := &recordingPluginInstaller{installErr: errors.New("database write failed")}
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/plugins/notion":
				return testHTTPResponse(req, http.StatusOK, entryJSON), nil
			case "/api/plugins/notion/releases/" + entry.Release.Revision:
				return testHTTPResponse(req, http.StatusOK, releaseJSON), nil
			case registryPackageReleaseUpstreamPath(reference.RegistryID, reference.PackageID, entry.Release.Packages[0].Revision):
				return testHTTPResponse(req, http.StatusOK, packageJSON), nil
			case "/api/artifacts/plugin/" + entry.Release.Artifact.Digest:
				return testHTTPResponse(req, http.StatusOK, bundle), nil
			case "/api/artifacts/skill/" + descriptor.Artifact.Digest:
				return testHTTPResponse(req, http.StatusOK, skillArtifact), nil
			default:
				return testHTTPResponse(req, http.StatusNotFound, nil), nil
			}
		})}),
		botService:     env.handler.botService,
		accountService: env.handler.accountService,
		logger:         slog.New(slog.DiscardHandler),
	}
	handler.installer = supermarketclient.NewInstaller(handler.upstream, installer, manager, manager, handler.logger)

	_, installErr := env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: entry.Release.Revision,
	}, handler.InstallPlugin)
	if installErr == nil {
		t.Fatal("InstallPlugin() succeeded after database failure")
	}
	var httpErr *echo.HTTPError
	if !errors.As(installErr, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("InstallPlugin() error = %v, want original HTTP 400 after successful rollback", installErr)
	}
	if got, err := os.ReadFile(env.localPath(skillPath)); err != nil || string(got) != oldSkill {
		t.Fatalf("previous Skill was not restored: content=%q error=%v", got, err)
	}
	if got, err := os.ReadFile(env.localPath(pluginPath)); err != nil || string(got) != oldPlugin {
		t.Fatalf("previous Plugin bundle was not restored: content=%q error=%v", got, err)
	}
}

func TestInstallPluginRejectsInvalidPluginArtifactBeforeWorkspaceMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*SupermarketPluginArtifact)
	}{
		{
			name: "size",
			mutate: func(artifact *SupermarketPluginArtifact) {
				artifact.Size++
			},
		},
		{
			name: "digest",
			mutate: func(artifact *SupermarketPluginArtifact) {
				artifact.Digest = strings.Repeat("0", 64)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newSkillsTestEnv(t)
			manager := workspace.NewManager(
				slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
			)
			reference := pluginspkg.PackageReference{RegistryID: "memoh", PackageID: "notion"}
			skillArtifact := validSkillArtifact(t)
			skillDigest := sha256.Sum256(skillArtifact)
			descriptor := validRegistrySkillDescriptor()
			descriptor.RegistryID = reference.RegistryID
			descriptor.PackageID = reference.PackageID
			descriptor.SkillID = "meeting"
			descriptor.InstallID = "memoh+notion+meeting"
			descriptor.Artifact.Digest = hex.EncodeToString(skillDigest[:])
			descriptor.Artifact.Size = int64(len(skillArtifact))
			descriptor.Artifact.DownloadURL = "/artifacts/meeting"
			manifest := pluginspkg.Manifest{ID: "notion", Name: "Notion", Packages: []pluginspkg.PackageReference{reference}}
			bundle := gzipTarArchive(t, map[string]string{
				"notion/plugin.yaml": "id: notion\npackages:\n  - registry_id: memoh\n    package_id: notion\n",
			})
			entry := validSupermarketPluginEntry(manifest, bundle, descriptor)
			packageJSON := supermarketPackageReleaseJSON(t, &entry.Release.Packages[0], descriptor)
			test.mutate(&entry.Release.Artifact)
			releaseJSON := sealSupermarketPluginEntry(t, &entry)
			entryJSON, err := json.Marshal(entry)
			if err != nil {
				t.Fatalf("marshal Plugin release: %v", err)
			}
			skillArtifactRequested := false
			pluginInstaller := &recordingPluginInstaller{}
			handler := &SupermarketHandler{
				upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/plugins/notion":
						return testHTTPResponse(req, http.StatusOK, entryJSON), nil
					case "/api/plugins/notion/releases/" + entry.Release.Revision:
						return testHTTPResponse(req, http.StatusOK, releaseJSON), nil
					case registryPackageReleaseUpstreamPath(reference.RegistryID, reference.PackageID, entry.Release.Packages[0].Revision):
						return testHTTPResponse(req, http.StatusOK, packageJSON), nil
					case "/api/artifacts/plugin/" + entry.Release.Artifact.Digest:
						return testHTTPResponse(req, http.StatusOK, bundle), nil
					case "/api/artifacts/skill/" + descriptor.Artifact.Digest:
						skillArtifactRequested = true
						return testHTTPResponse(req, http.StatusOK, skillArtifact), nil
					default:
						return testHTTPResponse(req, http.StatusNotFound, nil), nil
					}
				})}),
				botService:     env.handler.botService,
				accountService: env.handler.accountService,
				logger:         slog.New(slog.DiscardHandler),
			}
			handler.installer = supermarketclient.NewInstaller(handler.upstream, pluginInstaller, manager, manager, handler.logger)

			if _, err := env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
				PluginID: "notion", ReleaseRevision: entry.Release.Revision,
			}, handler.InstallPlugin); err == nil {
				t.Fatal("InstallPlugin() accepted an invalid Plugin Artifact")
			}
			if skillArtifactRequested {
				t.Fatal("Skill Artifact was downloaded before Plugin Artifact preflight completed")
			}
			sourcePath := path.Join(skillset.ManagedDir(), "memoh", "notion", "meeting", "SKILL.md")
			if _, err := os.Stat(env.localPath(sourcePath)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("workspace changed before Plugin Artifact validation: %v", err)
			}
		})
	}
}

func TestInstallPluginRejectsDeclaredSkillBudgetBeforeArtifactDownload(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	reference := pluginspkg.PackageReference{RegistryID: "memoh", PackageID: "notion"}
	manifest := pluginspkg.Manifest{ID: "notion", Name: "Notion", Packages: []pluginspkg.PackageReference{reference}}
	skills := make([]SupermarketCatalogSkill, 0, 26)
	for index := 0; index < 26; index++ {
		skillID := fmt.Sprintf("skill-%02d", index)
		skill := validRegistrySkillDescriptor()
		skill.RegistryID = reference.RegistryID
		skill.PackageID = reference.PackageID
		skill.SkillID = skillID
		skill.InstallID = strings.Join([]string{reference.RegistryID, reference.PackageID, skillID}, "+")
		skill.Artifact.UncompressedSize = 5 * 1024 * 1024
		skills = append(skills, skill)
	}
	bundle := gzipTarArchive(t, map[string]string{
		"notion/plugin.yaml": "id: notion\npackages:\n  - registry_id: memoh\n    package_id: notion\n",
	})
	entry := validSupermarketPluginEntry(manifest, bundle, skills...)
	packageJSON := supermarketPackageReleaseJSON(t, &entry.Release.Packages[0], skills...)
	releaseJSON := sealSupermarketPluginEntry(t, &entry)
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal Plugin entry: %v", err)
	}
	artifactRequested := false
	pluginInstaller := &recordingPluginInstaller{}
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/plugins/notion":
				return testHTTPResponse(req, http.StatusOK, entryJSON), nil
			case "/api/plugins/notion/releases/" + entry.Release.Revision:
				return testHTTPResponse(req, http.StatusOK, releaseJSON), nil
			case registryPackageReleaseUpstreamPath(reference.RegistryID, reference.PackageID, entry.Release.Packages[0].Revision):
				return testHTTPResponse(req, http.StatusOK, packageJSON), nil
			default:
				artifactRequested = true
				return testHTTPResponse(req, http.StatusOK, bundle), nil
			}
		})}),
		botService:     env.handler.botService,
		accountService: env.handler.accountService,
		logger:         slog.New(slog.DiscardHandler),
	}
	handler.installer = supermarketclient.NewInstaller(handler.upstream, pluginInstaller, manager, manager, handler.logger)

	_, err = env.callJSON(t, http.MethodPost, "/bots/:bot_id/supermarket/install-plugin", InstallPluginRequest{
		PluginID: "notion", ReleaseRevision: entry.Release.Revision,
	}, handler.InstallPlugin)
	if err == nil || !strings.Contains(err.Error(), "uncompressed limit") {
		t.Fatalf("InstallPlugin() error = %v, want declared Skill budget rejection", err)
	}
	if artifactRequested {
		t.Fatal("an Artifact was downloaded before the declared Skill budget was rejected")
	}
}

func validSupermarketPluginEntry(
	manifest pluginspkg.Manifest,
	bundle []byte,
	_ ...SupermarketCatalogSkill,
) SupermarketPluginEntry {
	digest := sha256.Sum256(bundle)
	resolved := make([]SupermarketPluginResolvedPackage, 0, len(manifest.Packages))
	for _, pkg := range manifest.Packages {
		resolved = append(resolved, SupermarketPluginResolvedPackage{
			RegistryID: pkg.RegistryID, PackageID: pkg.PackageID, Revision: strings.Repeat("b", 64),
		})
	}
	return SupermarketPluginEntry{
		Manifest: manifest,
		Release: SupermarketPluginRelease{
			Revision:    strings.Repeat("c", 64),
			PublishedAt: "2026-08-01T00:00:00Z",
			Artifact: SupermarketPluginArtifact{
				Format:      "memoh_plugin_v1",
				Digest:      hex.EncodeToString(digest[:]),
				Size:        int64(len(bundle)),
				ContentType: "application/gzip",
				DownloadURL: "/artifacts/plugin",
			},
			Packages: resolved,
		},
	}
}

func sealSupermarketPluginEntry(t *testing.T, entry *SupermarketPluginEntry) []byte {
	t.Helper()
	release := SupermarketImmutablePluginRelease{
		SchemaVersion: "1",
		Plugin:        entry.Manifest,
		Artifact:      entry.Release.Artifact,
		Packages:      append([]SupermarketPluginResolvedPackage(nil), entry.Release.Packages...),
	}
	release.Artifact.DownloadURL = ""
	payload, err := json.Marshal(release)
	if err != nil {
		t.Fatalf("marshal immutable Plugin release: %v", err)
	}
	digest := sha256.Sum256(payload)
	entry.Release.Revision = hex.EncodeToString(digest[:])
	return payload
}

func supermarketPackageReleaseJSON(
	t *testing.T,
	resolved *SupermarketPluginResolvedPackage,
	skills ...SupermarketCatalogSkill,
) []byte {
	t.Helper()
	members := make([]supermarketSkillPackageReleaseSkill, 0, len(skills))
	for _, skill := range skills {
		members = append(members, supermarketSkillPackageReleaseSkill{
			SchemaVersion: skill.SchemaVersion, RegistryID: skill.RegistryID, PackageID: skill.PackageID,
			SkillID: skill.SkillID, InstallID: skill.InstallID, Name: skill.Name,
			Description: skill.Description, Author: skill.Author, Homepage: skill.Homepage,
			Tags: skill.Tags, Category: skill.Category, CategoryName: skill.CategoryName,
			SourceCategory: skill.SourceCategory, Files: skill.Files, Icon: skill.Icon, Artifact: skill.Artifact,
		})
	}
	release := SupermarketSkillPackageRelease{
		SchemaVersion: "1", RegistryID: resolved.RegistryID, PackageID: resolved.PackageID,
		Name: resolved.PackageID, Description: resolved.PackageID, Tags: []string{},
		Skills: members,
	}
	payload, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		t.Fatalf("marshal immutable Package release: %v", err)
	}
	payload = append(payload, '\n')
	digest := sha256.Sum256(payload)
	resolved.Revision = hex.EncodeToString(digest[:])
	return payload
}

func TestExtractPluginBundleArchivePublishesSafeBundle(t *testing.T) {
	pluginRoot, err := skillset.PluginDirForID("github_plugin")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	archive := tarArchiveEntries(t, []pluginBundleTarEntry{
		{name: "github-source/plugin.yaml", content: "id: github_plugin"},
		{name: "github-source/hooks.json", content: `{"version":1,"hooks":[]}`},
		{name: "github-source/scripts/it's.sh", content: "#!/bin/sh\n", mode: 0o755},
	})
	writer := &pluginBundleTestWriter{files: map[string]string{
		pluginRoot + "/hooks.json":                "stale",
		pluginRoot + "/scripts/stale.py":          "print('stale')",
		"/data/.memoh/plugins/github-source/keep": "keep",
		"/data/.memoh/plugins/other/keep":         "keep",
		"/data/.memoh/plugins/github2/keep":       "keep",
	}}

	result, err := extractPluginBundleArchiveForTest(
		context.Background(), writer, "linux", "github-source", "github_plugin", bytes.NewReader(archive),
	)
	if err != nil {
		t.Fatalf("extractPluginBundleArchive returned error: %v", err)
	}
	if result.Hooks.FilesWritten != 1 || result.Scripts.FilesWritten != 1 {
		t.Fatalf("install result = %+v, want 1 hook and 1 script", result)
	}
	if len(writer.renames) != 2 {
		t.Fatalf("renames = %+v, want backup and publish", writer.renames)
	}
	if writer.renames[0].oldPath != pluginRoot || !strings.Contains(writer.renames[0].newPath, "/.staging/backup-github_plugin-") {
		t.Fatalf("first rename = %+v, want plugin root to backup", writer.renames[0])
	}
	if !strings.Contains(writer.renames[1].oldPath, "/.staging/install-github_plugin-") || writer.renames[1].newPath != pluginRoot {
		t.Fatalf("second rename = %+v, want staged bundle publication", writer.renames[1])
	}
	wantFiles := map[string]string{
		pluginRoot + "/plugin.yaml":     "id: github_plugin",
		pluginRoot + "/hooks.json":      `{"version":1,"hooks":[]}`,
		pluginRoot + "/scripts/it's.sh": "#!/bin/sh\n",
	}
	for path, want := range wantFiles {
		if got := writer.files[path]; got != want {
			t.Fatalf("file %s = %q, want %q", path, got, want)
		}
	}
	if _, ok := writer.files[pluginRoot+"/scripts/stale.py"]; ok {
		t.Fatal("stale file was not cleared before extraction")
	}
	for _, preservedPath := range []string{
		"/data/.memoh/plugins/github-source/keep",
		"/data/.memoh/plugins/other/keep",
		"/data/.memoh/plugins/github2/keep",
	} {
		if got := writer.files[preservedPath]; got != "keep" {
			t.Fatalf("unrelated plugin file %s = %q, want keep", preservedPath, got)
		}
	}
	for filePath := range writer.files {
		if strings.Contains(filePath, "outside") || strings.Contains(filePath, "escape") {
			t.Fatalf("non-runtime bundle file was written: %s", filePath)
		}
	}
	if len(writer.execCommands) != 1 {
		t.Fatalf("chmod commands = %v, want one", writer.execCommands)
	}
	if want := "chmod 755 -- '/data/.memoh/plugins/.staging/"; !strings.HasPrefix(writer.execCommands[0], want) {
		t.Fatalf("chmod command = %q, want quoted staging path", writer.execCommands[0])
	}
	if !strings.Contains(writer.execCommands[0], `it'"'"'s.sh'`) {
		t.Fatalf("chmod command does not safely quote apostrophe: %q", writer.execCommands[0])
	}
}

func TestPluginBundlePublicationCommitCanRetryCleanup(t *testing.T) {
	failures := 0
	pluginRoot, err := skillset.PluginDirForID("github")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	var writer *pluginBundleTestWriter
	writer = &pluginBundleTestWriter{
		files: map[string]string{pluginRoot + "/plugin.yaml": "id: github"},
		deleteError: func(filePath string) error {
			if strings.Contains(filePath, "/.staging/backup-github-") && len(writer.renames) >= 2 && failures == 0 {
				failures++
				return errors.New("temporary cleanup failure")
			}
			return nil
		},
	}
	archive, err := pluginspkg.ReadBundleArchive(
		"github", "github",
		bytes.NewReader(tarArchive(t, map[string]string{"github/plugin.yaml": "id: github"})),
		nil,
	)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	publication, err := pluginspkg.PublishBundleArchive(context.Background(), writer, "linux", "github", archive)
	if err != nil {
		t.Fatalf("publish bundle: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := publication.Commit(canceled); err == nil {
		t.Fatal("first commit error = nil")
	}
	if err := publication.Commit(canceled); err != nil {
		t.Fatalf("second commit error = %v", err)
	}
	if len(writer.deleteHasDeadline) < 2 || !writer.deleteHasDeadline[len(writer.deleteHasDeadline)-2] || !writer.deleteHasDeadline[len(writer.deleteHasDeadline)-1] {
		t.Fatalf("cleanup deadlines=%v", writer.deleteHasDeadline)
	}
}

func TestExtractPluginBundleArchiveRejectsInvalidArchiveBeforeWorkspaceMutation(t *testing.T) {
	pluginRoot, err := skillset.PluginDirForID("github")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	tooManyFiles := map[string]string{"github/plugin.yaml": "id: github"}
	for i := 0; i < pluginspkg.MaxBundleFiles; i++ {
		tooManyFiles[fmt.Sprintf("github/scripts/ignored/%04d.txt", i)] = "x"
	}
	totalSizeExceeded := map[string]string{"github/plugin.yaml": "id: github"}
	for i := 0; i < 6; i++ {
		totalSizeExceeded[fmt.Sprintf("github/scripts/ignored/%d.bin", i)] = strings.Repeat("x", pluginspkg.MaxBundleFileBytes)
	}
	tests := []struct {
		name    string
		archive []byte
	}{
		{name: "missing manifest", archive: tarArchive(t, map[string]string{
			"github/hooks.json": `{"version":1,"hooks":[]}`,
		})},
		{name: "truncated file", archive: truncatedTarArchive(t, "github/plugin.yaml", 100, "id: github")},
		{name: "oversized file", archive: tarArchive(t, map[string]string{
			"github/plugin.yaml":         "id: github",
			"github/scripts/ignored.bin": strings.Repeat("x", pluginspkg.MaxBundleFileBytes+1),
		})},
		{name: "too many files", archive: tarArchive(t, tooManyFiles)},
		{name: "total size exceeded", archive: tarArchive(t, totalSizeExceeded)},
		{name: "embedded skill", archive: tarArchive(t, map[string]string{
			"github/plugin.yaml":          "id: github",
			"github/skills/demo/SKILL.md": "---\nname: demo\n---",
		})},
		{name: "wrong root", archive: tarArchive(t, map[string]string{
			"other/plugin.yaml": "id: github",
		})},
		{name: "unknown file", archive: tarArchive(t, map[string]string{
			"github/plugin.yaml": "id: github",
			"github/README.md":   "unsupported",
		})},
		{name: "windows device path", archive: tarArchive(t, map[string]string{
			"github/plugin.yaml":     "id: github",
			"github/scripts/CON.txt": "unsupported",
		})},
		{name: "unknown directory", archive: tarArchiveEntries(t, []pluginBundleTarEntry{
			{name: "github/unknown/", typeflag: tar.TypeDir},
			{name: "github/plugin.yaml", content: "id: github"},
		})},
		{name: "symlink", archive: tarArchiveEntries(t, []pluginBundleTarEntry{
			{name: "github/plugin.yaml", content: "id: github"},
			{name: "github/scripts/current", typeflag: tar.TypeSymlink, linkname: "../outside"},
		})},
		{name: "duplicate path", archive: tarArchiveEntries(t, []pluginBundleTarEntry{
			{name: "github/plugin.yaml", content: "id: github"},
			{name: "github/hooks.json", content: "first"},
			{name: "github/hooks.json", content: "second"},
		})},
		{name: "case-insensitive duplicate path", archive: tarArchiveEntries(t, []pluginBundleTarEntry{
			{name: "github/plugin.yaml", content: "id: github"},
			{name: "github/hooks.json", content: "first"},
			{name: "github/HOOKS.JSON", content: "second"},
		})},
		{name: "file child conflict", archive: tarArchiveEntries(t, []pluginBundleTarEntry{
			{name: "github/plugin.yaml", content: "id: github"},
			{name: "github/scripts/tool", content: "file"},
			{name: "github/scripts/tool/config", content: "child"},
		})},
		{name: "decompressed stream limit", archive: append(
			tarArchive(t, map[string]string{"github/plugin.yaml": "id: github"}),
			bytes.Repeat([]byte{0}, pluginspkg.MaxBundleStreamBytes+1)...,
		)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &pluginBundleTestWriter{files: map[string]string{
				pluginRoot + "/hooks.json": "stale",
			}}
			if _, err := extractPluginBundleArchiveForTest(context.Background(), writer, "linux", "github", "github", bytes.NewReader(test.archive)); err == nil {
				t.Fatal("extractPluginBundleArchive() accepted invalid archive")
			}
			if got := writer.files[pluginRoot+"/hooks.json"]; got != "stale" {
				t.Fatalf("existing plugin bundle changed to %q", got)
			}
			if len(writer.renames) != 0 || len(writer.deletes) != 0 || len(writer.dirs) != 0 {
				t.Fatalf("workspace mutated before archive validation: %+v", writer)
			}
		})
	}
}

func TestExtractPluginBundleArchiveRollsBackStagingAndPublishFailures(t *testing.T) {
	pluginRoot, err := skillset.PluginDirForID("github")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	archive := tarArchive(t, map[string]string{
		"github/plugin.yaml":      "id: github",
		"github/hooks.json":       "new hooks",
		"github/scripts/setup.sh": "new setup",
	})

	t.Run("staging write", func(t *testing.T) {
		writer := &pluginBundleTestWriter{
			files: map[string]string{pluginRoot + "/hooks.json": "stale"},
			writeError: func(filePath string) error {
				if strings.HasSuffix(filePath, "/hooks.json") {
					return errors.New("injected staging write failure")
				}
				return nil
			},
		}
		if _, err := extractPluginBundleArchiveForTest(context.Background(), writer, "linux", "github", "github", bytes.NewReader(archive)); err == nil {
			t.Fatal("extractPluginBundleArchive() succeeded after staging write failure")
		}
		if got := writer.files[pluginRoot+"/hooks.json"]; got != "stale" {
			t.Fatalf("existing bundle changed after staging failure: %q", got)
		}
		if len(writer.renames) != 0 {
			t.Fatalf("staging failure reached publication: %+v", writer.renames)
		}
	})

	t.Run("publish rename", func(t *testing.T) {
		writer := &pluginBundleTestWriter{
			files: map[string]string{pluginRoot + "/hooks.json": "stale"},
			renameError: func(oldPath, newPath string) error {
				if strings.Contains(oldPath, "/.staging/install-github-") && newPath == pluginRoot {
					return errors.New("injected publish failure")
				}
				return nil
			},
		}
		if _, err := extractPluginBundleArchiveForTest(context.Background(), writer, "linux", "github", "github", bytes.NewReader(archive)); err == nil {
			t.Fatal("extractPluginBundleArchive() succeeded after publish failure")
		}
		if got := writer.files[pluginRoot+"/hooks.json"]; got != "stale" {
			t.Fatalf("previous bundle was not restored: %q", got)
		}
		if len(writer.renames) != 2 || !strings.Contains(writer.renames[1].oldPath, "/.staging/backup-github-") {
			t.Fatalf("publish rollback renames = %+v", writer.renames)
		}
	})
}

func extractPluginBundleArchiveForTest(
	ctx context.Context,
	writer *pluginBundleTestWriter,
	workspaceOS, archivePluginID, targetPluginID string,
	r io.Reader,
) (pluginspkg.BundleInstallResult, error) {
	archive, err := pluginspkg.ReadBundleArchive(archivePluginID, targetPluginID, r, nil)
	if err != nil {
		return pluginspkg.BundleInstallResult{}, err
	}
	publication, err := pluginspkg.PublishBundleArchive(ctx, writer, workspaceOS, targetPluginID, archive)
	if err != nil {
		return pluginspkg.BundleInstallResult{}, err
	}
	_ = publication.Commit(ctx)
	return publication.Result(), nil
}

type recordingPluginInstaller struct {
	request               pluginspkg.InstallRequest
	installCalls          int
	installInMutation     bool
	mutationCalls         int
	conflictExpected      map[string]string
	conflictTarget        string
	conflictErr           error
	installErr            error
	installed             bool
	installedRevision     string
	installedUpdatedAt    time.Time
	releaseReadInMutation bool
	beforeMutation        func()
}

type recordingPluginMutationKey struct{}

func (i *recordingPluginInstaller) WithBotMutation(
	ctx context.Context,
	_ string,
	fn func(context.Context) error,
) error {
	i.mutationCalls++
	if i.beforeMutation != nil {
		i.beforeMutation()
	}
	return fn(context.WithValue(ctx, recordingPluginMutationKey{}, true))
}

func (i *recordingPluginInstaller) InstalledPluginState(
	ctx context.Context,
	_, _ string,
) (pluginspkg.InstalledPluginState, bool, error) {
	i.releaseReadInMutation, _ = ctx.Value(recordingPluginMutationKey{}).(bool)
	return pluginspkg.InstalledPluginState{
		ReleaseRevision: i.installedRevision,
		UpdatedAt:       i.installedUpdatedAt,
	}, i.installed, nil
}

func (i *recordingPluginInstaller) Install(ctx context.Context, botID string, req pluginspkg.InstallRequest) (pluginspkg.Installation, error) {
	i.installCalls++
	i.installInMutation, _ = ctx.Value(recordingPluginMutationKey{}).(bool)
	i.request = req
	if i.installErr != nil {
		return pluginspkg.Installation{}, i.installErr
	}
	return pluginspkg.Installation{
		BotID:      botID,
		PluginID:   req.Manifest.ID,
		PluginName: req.Manifest.Name,
		Manifest:   req.Manifest,
		Metadata:   map[string]any{},
		Status:     pluginspkg.StatusReady,
		Enabled:    true,
	}, nil
}

func (*recordingPluginInstaller) List(context.Context, string) ([]pluginspkg.Installation, error) {
	return nil, nil
}

func (i *recordingPluginInstaller) CheckSkillArtifactConflicts(
	_ context.Context,
	_ string,
	_ string,
	workspaceTargetID string,
	expected map[string]string,
) error {
	i.conflictTarget = workspaceTargetID
	i.conflictExpected = make(map[string]string, len(expected))
	for identity, digest := range expected {
		i.conflictExpected[identity] = digest
	}
	return i.conflictErr
}

func testHTTPResponse(req *http.Request, status int, content []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Body:          io.NopCloser(bytes.NewReader(content)),
		ContentLength: int64(len(content)),
		Request:       req,
		Header:        make(http.Header),
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

type pluginBundleTarEntry struct {
	name     string
	content  string
	typeflag byte
	linkname string
	mode     int64
}

func tarArchiveEntries(t *testing.T, entries []pluginBundleTarEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	tw := tar.NewWriter(&output)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		size := int64(0)
		if typeflag == tar.TypeReg {
			size = int64(len(entry.content))
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o600
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: entry.name, Mode: mode, Size: size, Typeflag: typeflag, Linkname: entry.linkname,
		}); err != nil {
			t.Fatalf("write tar entry %q: %v", entry.name, err)
		}
		if size > 0 {
			if _, err := tw.Write([]byte(entry.content)); err != nil {
				t.Fatalf("write tar entry content %q: %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar entries: %v", err)
	}
	return output.Bytes()
}

func gzipTarArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	zw := gzip.NewWriter(&output)
	if _, err := zw.Write(tarArchive(t, files)); err != nil {
		t.Fatalf("write gzip archive: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip archive: %v", err)
	}
	return output.Bytes()
}

func truncatedTarArchive(t *testing.T, name string, declaredSize int64, content string) []byte {
	t.Helper()
	var output bytes.Buffer
	tw := tar.NewWriter(&output)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: declaredSize}); err != nil {
		t.Fatalf("write truncated tar header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("write truncated tar content: %v", err)
	}
	return output.Bytes()
}

type pluginBundleTestWriter struct {
	dirs              []string
	deletes           []pluginBundleTestDelete
	renames           []pluginBundleTestRename
	files             map[string]string
	execCommands      []string
	deleteHasDeadline []bool
	deleteError       func(path string) error
	writeError        func(path string) error
	renameError       func(oldPath, newPath string) error
}

type pluginBundleTestDelete struct {
	path      string
	recursive bool
}

type pluginBundleTestRename struct {
	oldPath string
	newPath string
}

func (w *pluginBundleTestWriter) DeleteFile(ctx context.Context, path string, recursive bool) error {
	w.deletes = append(w.deletes, pluginBundleTestDelete{path: path, recursive: recursive})
	_, hasDeadline := ctx.Deadline()
	w.deleteHasDeadline = append(w.deleteHasDeadline, hasDeadline)
	if w.deleteError != nil {
		if err := w.deleteError(path); err != nil {
			return err
		}
	}
	if !recursive {
		delete(w.files, path)
		return nil
	}
	for filePath := range w.files {
		if filePath == path || strings.HasPrefix(filePath, path+"/") {
			delete(w.files, filePath)
		}
	}
	remainingDirs := w.dirs[:0]
	for _, dir := range w.dirs {
		if dir != path && !strings.HasPrefix(dir, path+"/") {
			remainingDirs = append(remainingDirs, dir)
		}
	}
	w.dirs = remainingDirs
	return nil
}

func (w *pluginBundleTestWriter) ExecWithEnv(
	_ context.Context,
	command, _ string,
	_ int32,
	_ []string,
) (*bridge.ExecResult, error) {
	w.execCommands = append(w.execCommands, command)
	return &bridge.ExecResult{}, nil
}

func (w *pluginBundleTestWriter) Mkdir(_ context.Context, path string) error {
	if !slices.Contains(w.dirs, path) {
		w.dirs = append(w.dirs, path)
	}
	return nil
}

func (w *pluginBundleTestWriter) Rename(_ context.Context, oldPath, newPath string) error {
	if w.renameError != nil {
		if err := w.renameError(oldPath, newPath); err != nil {
			return err
		}
	}
	if !w.hasPath(oldPath) {
		return bridge.ErrNotFound
	}
	w.renames = append(w.renames, pluginBundleTestRename{oldPath: oldPath, newPath: newPath})
	movedFiles := make(map[string]string)
	for filePath, content := range w.files {
		if filePath == oldPath || strings.HasPrefix(filePath, oldPath+"/") {
			movedFiles[newPath+strings.TrimPrefix(filePath, oldPath)] = content
			delete(w.files, filePath)
		}
	}
	for filePath, content := range movedFiles {
		w.files[filePath] = content
	}
	for i, dir := range w.dirs {
		if dir == oldPath || strings.HasPrefix(dir, oldPath+"/") {
			w.dirs[i] = newPath + strings.TrimPrefix(dir, oldPath)
		}
	}
	return nil
}

func (w *pluginBundleTestWriter) WriteFile(_ context.Context, path string, content []byte) error {
	if w.writeError != nil {
		if err := w.writeError(path); err != nil {
			return err
		}
	}
	w.files[path] = string(content)
	return nil
}

func (w *pluginBundleTestWriter) hasPath(target string) bool {
	for filePath := range w.files {
		if filePath == target || strings.HasPrefix(filePath, target+"/") {
			return true
		}
	}
	for _, dir := range w.dirs {
		if dir == target || strings.HasPrefix(dir, target+"/") {
			return true
		}
	}
	return false
}
