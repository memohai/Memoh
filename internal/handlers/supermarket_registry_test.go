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
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/config"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
)

func TestReadRegistrySkillArchivePreservesManifestName(t *testing.T) {
	installID := "registry+package+skill"
	archive, err := readRegistrySkillArchive(registrySkillTestArchive(t, []registrySkillTestEntry{
		{name: installID + "/SKILL.md", content: "---\nname: skill\ndescription: Demo\n---\n\n# Demo\n"},
		{name: installID + "/scripts/run.sh", content: "#!/bin/sh\n", mode: 0o755},
	}), installID)
	if err != nil {
		t.Fatalf("readRegistrySkillArchive() error = %v", err)
	}
	if len(archive.files) != 2 {
		t.Fatalf("files = %d, want 2", len(archive.files))
	}
	manifest := registrySkillArchiveFileByPath(t, archive, "SKILL.md")
	if !strings.Contains(string(manifest.content), "name: skill") {
		t.Fatalf("manifest name should stay original:\n%s", manifest.content)
	}
	if strings.Contains(string(manifest.content), "name: "+installID) {
		t.Fatalf("manifest should not be rewritten to install_id:\n%s", manifest.content)
	}
	if !strings.Contains(string(manifest.content), "# Demo") {
		t.Fatalf("manifest body was not preserved:\n%s", manifest.content)
	}
	if !registrySkillArchiveFileByPath(t, archive, "scripts/run.sh").executable {
		t.Fatal("executable mode was not retained")
	}
}

func TestReadRegistrySkillArchiveRejectsUnsafeEntries(t *testing.T) {
	installID := "registry+package+skill"
	tests := []struct {
		name    string
		entries []registrySkillTestEntry
	}{
		{
			name: "path traversal",
			entries: []registrySkillTestEntry{
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
				{name: installID + "/../escape", content: "bad"},
			},
		},
		{
			name: "backslash",
			entries: []registrySkillTestEntry{
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
				{name: installID + `\escape`, content: "bad"},
			},
		},
		{
			name: "symlink",
			entries: []registrySkillTestEntry{
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
				{name: installID + "/link", entryType: tar.TypeSymlink, linkName: "../../escape"},
			},
		},
		{
			name: "duplicate",
			entries: []registrySkillTestEntry{
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
			},
		},
		{
			name: "directory then file conflict",
			entries: []registrySkillTestEntry{
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
				{name: installID + "/scripts/", entryType: tar.TypeDir},
				{name: installID + "/scripts", content: "file"},
			},
		},
		{
			name: "file directory conflict",
			entries: []registrySkillTestEntry{
				{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
				{name: installID + "/scripts", content: "file"},
				{name: installID + "/scripts/run.sh", content: "bad"},
			},
		},
		{
			name: "wrong root",
			entries: []registrySkillTestEntry{
				{name: "other/SKILL.md", content: validRegistrySkillManifest},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readRegistrySkillArchive(registrySkillTestArchive(t, test.entries), installID); err == nil {
				t.Fatal("readRegistrySkillArchive() error = nil, want rejection")
			}
		})
	}
}

func TestValidateRegistrySkillRequiresNamespacedIdentity(t *testing.T) {
	skill := validRegistrySkillDescriptor()
	if err := validateRegistrySkill(skill, "registry", "package", "skill"); err != nil {
		t.Fatalf("validateRegistrySkill(valid) error = %v", err)
	}
	skill.InstallID = "skill"
	if err := validateRegistrySkill(skill, "registry", "package", "skill"); err == nil {
		t.Fatal("validateRegistrySkill(unnamespaced) error = nil")
	}
}

func TestDownloadRegistrySkillArtifactVerifiesOriginAndDigest(t *testing.T) {
	content := []byte("artifact")
	digest := sha256.Sum256(content)
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(bytes.NewReader(content)),
				ContentLength: int64(len(content)),
				Request:       req,
				Header:        make(http.Header),
			}, nil
		})},
	}
	artifact := SupermarketSkillArtifact{
		Digest: hex.EncodeToString(digest[:]), Size: int64(len(content)), DownloadURL: "/api/artifacts/digest/download",
	}
	got, err := handler.downloadRegistrySkillArtifact(context.Background(), artifact)
	if err != nil {
		t.Fatalf("downloadRegistrySkillArtifact() error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content = %q, want %q", got, content)
	}

	artifact.DownloadURL = "https://attacker.example/artifact"
	if _, err := handler.downloadRegistrySkillArtifact(context.Background(), artifact); apperror.CodeOf(err) != apperror.CodeRegistrySkillInvalid {
		t.Fatalf("cross-origin artifact code = %q, want %q", apperror.CodeOf(err), apperror.CodeRegistrySkillInvalid)
	}
	artifact.DownloadURL = "/api/artifacts/digest/download"
	artifact.Digest = strings.Repeat("0", 64)
	if _, err := handler.downloadRegistrySkillArtifact(context.Background(), artifact); apperror.CodeOf(err) != apperror.CodeRegistrySkillInvalid {
		t.Fatalf("invalid Artifact digest code = %q, want %q", apperror.CodeOf(err), apperror.CodeRegistrySkillInvalid)
	}
}

func TestFetchRegistrySkillUsesStablePrivateErrors(t *testing.T) {
	tests := []struct {
		name      string
		transport roundTripFunc
		wantCode  apperror.Code
	}{
		{
			name: "unavailable",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("SECRET upstream dial failure")
			},
			wantCode: apperror.CodeRegistryUnavailable,
		},
		{
			name: "not found",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("SECRET missing response")),
					Request:    req,
					Header:     make(http.Header),
				}, nil
			},
			wantCode: apperror.CodeRegistrySkillNotFound,
		},
		{
			name: "invalid response",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("SECRET malformed response")),
					Request:    req,
					Header:     make(http.Header),
				}, nil
			},
			wantCode: apperror.CodeRegistrySkillInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &SupermarketHandler{
				baseURL:    "https://supermarket.example",
				httpClient: &http.Client{Transport: test.transport},
			}
			_, err := handler.fetchRegistrySkill(context.Background(), "registry", "package", "skill")
			if got := apperror.CodeOf(err); got != test.wantCode {
				t.Fatalf("fetch code = %q, want %q", got, test.wantCode)
			}
			public, ok := apperror.PublicFrom(err, "request-id")
			if !ok {
				t.Fatal("fetch error is not a public app error")
			}
			if strings.Contains(public.Detail, "SECRET") || strings.Contains(public.Detail, "supermarket.example") {
				t.Fatalf("public fetch error leaked private detail: %q", public.Detail)
			}
		})
	}
}

func TestInstallRegistrySkillRejectsLayoutConflictBeforeNetwork(t *testing.T) {
	env := newSkillsTestEnv(t)
	registryID := "openai-api-curated"
	flatSkillPath := path.Join(skillset.ManagedDir(), registryID, "SKILL.md")
	env.writeSkillFile(t, flatSkillPath, managedSkillRaw(registryID, "Local skill"))

	upstreamCalls := 0
	manager := workspace.NewManager(
		slog.Default(),
		nil,
		nil,
		config.WorkspaceConfig{DataRoot: env.dataRoot},
		"",
		nil,
	)
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			upstreamCalls++
			return nil, errors.New("unexpected upstream request")
		})},
		workspaces: manager,
	}

	_, err := handler.installRegistrySkill(context.Background(), env.botID, InstallSkillRequest{
		RegistryID: registryID,
		PackageID:  "docs",
		SkillID:    "xlsx",
	})
	if got := apperror.CodeOf(err); got != apperror.CodeRegistrySkillLayoutConflict {
		t.Fatalf("install conflict code = %q, want %q", got, apperror.CodeRegistrySkillLayoutConflict)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstreamCalls)
	}
	if _, statErr := os.Stat(env.localPath(path.Join(skillset.ManagedDir(), registryID, "docs"))); !os.IsNotExist(statErr) {
		t.Fatalf("registry package directory should not be created, stat err = %v", statErr)
	}
}

func TestInstallRegistrySkillHidesWorkspaceFailureDetails(t *testing.T) {
	env := newSkillsTestEnv(t)
	env.writeSkillFile(t, path.Join(skillset.ManagedDir(), ".staging"), "not a directory")

	skill := validRegistrySkillDescriptor()
	artifact := registrySkillTestArchive(t, []registrySkillTestEntry{
		{name: skill.InstallID + "/SKILL.md", content: validRegistrySkillManifest},
	})
	digest := sha256.Sum256(artifact)
	skill.Artifact.Digest = hex.EncodeToString(digest[:])
	skill.Artifact.Size = int64(len(artifact))
	descriptor, err := json.Marshal(skill)
	if err != nil {
		t.Fatalf("marshal registry Skill descriptor: %v", err)
	}

	manager := workspace.NewManager(
		slog.Default(),
		nil,
		nil,
		config.WorkspaceConfig{DataRoot: env.dataRoot},
		"",
		nil,
	)
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := descriptor
			contentType := "application/json"
			if req.URL.Path == skill.Artifact.DownloadURL {
				body = artifact
				contentType = "application/gzip"
			}
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(bytes.NewReader(body)),
				ContentLength: int64(len(body)),
				Request:       req,
				Header:        http.Header{"Content-Type": []string{contentType}},
			}, nil
		})},
		workspaces: manager,
	}

	_, err = handler.installRegistrySkill(context.Background(), env.botID, InstallSkillRequest{
		RegistryID: skill.RegistryID,
		PackageID:  skill.PackageID,
		SkillID:    skill.SkillID,
	})
	if got := apperror.CodeOf(err); got != apperror.CodeRegistrySkillInstallFailed {
		t.Fatalf("install failure code = %q, want %q", got, apperror.CodeRegistrySkillInstallFailed)
	}
	public, ok := apperror.PublicFrom(err, "request-id")
	if !ok {
		t.Fatal("install failure is not a public app error")
	}
	if strings.Contains(public.Detail, ".staging") || strings.Contains(public.Detail, env.dataRoot) {
		t.Fatalf("public install failure leaked workspace path: %q", public.Detail)
	}
	cause := apperror.CauseOf(err)
	if cause == nil || !strings.Contains(cause.Error(), ".staging") {
		t.Fatalf("private install failure cause = %v, want staging diagnostic", cause)
	}
}

func TestProxySkillImageVerifiesDigestAndHeaders(t *testing.T) {
	content := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	digest := sha256.Sum256(content)
	digestText := hex.EncodeToString(digest[:])
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "image/svg+xml")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			header.Set("ETag", `"`+digestText+`"`)
			return &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(content)),
				ContentLength: int64(len(content)), Request: req, Header: header,
			}, nil
		})},
	}
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/supermarket/skill-images/"+digestText, nil)
	recorder := httptest.NewRecorder()
	if err := handler.proxySkillImage(e.NewContext(request, recorder), digestText); err != nil {
		t.Fatalf("proxySkillImage() error = %v", err)
	}
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), content) {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.Bytes())
	}
	if recorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("immutable cache header was not forwarded")
	}

	handler.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("wrong")),
			ContentLength: 5, Request: req, Header: http.Header{"Content-Type": []string{"image/svg+xml"}},
		}, nil
	})}
	if err := handler.proxySkillImage(e.NewContext(request, httptest.NewRecorder()), digestText); err == nil {
		t.Fatal("proxySkillImage() accepted content with the wrong digest")
	}
}

func TestReadRegistrySkillArchiveRejectsEntryFlood(t *testing.T) {
	installID := "registry+package+skill"
	entries := []registrySkillTestEntry{
		{name: installID + "/SKILL.md", content: validRegistrySkillManifest},
	}
	// Directory headers carry no body, so neither the file-count nor the
	// uncompressed-size cap bounds them. Without a total-entry cap a tiny gzip of
	// directory headers could expand into an unbounded seen map.
	for i := 0; i <= maxRegistrySkillArtifactEntries; i++ {
		entries = append(entries, registrySkillTestEntry{name: fmt.Sprintf("%s/dir%d/", installID, i), entryType: tar.TypeDir})
	}
	_, err := readRegistrySkillArchive(registrySkillTestArchive(t, entries), installID)
	if err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("want too-many-entries rejection, got %v", err)
	}
}

func TestProxySkillImageOverridesUpstreamSecurityHeaders(t *testing.T) {
	content := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	digest := sha256.Sum256(content)
	digestText := hex.EncodeToString(digest[:])
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "image/svg+xml")
			// A compromised/misconfigured upstream sends a permissive CSP; the
			// proxy must not forward it.
			header.Set("Content-Security-Policy", "default-src *")
			return &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(content)),
				ContentLength: int64(len(content)), Request: req, Header: header,
			}, nil
		})},
	}
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/supermarket/skill-images/"+digestText, nil)
	recorder := httptest.NewRecorder()
	if err := handler.proxySkillImage(e.NewContext(request, recorder), digestText); err != nil {
		t.Fatalf("proxySkillImage() error = %v", err)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") || strings.Contains(got, "*") {
		t.Fatalf("upstream CSP was not overridden: %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestReadRegistrySkillArchiveRejectsMalformedFrontmatter(t *testing.T) {
	installID := "registry+package+skill"
	cases := map[string]string{
		"missing closing fence": "---\nname: skill\n",
		"not a mapping":         "---\n- a\n- b\n---\n# Body\n",
		"missing frontmatter":   "# Body only\n",
	}
	for name, manifest := range cases {
		t.Run(name, func(t *testing.T) {
			entries := []registrySkillTestEntry{{name: installID + "/SKILL.md", content: manifest}}
			if _, err := readRegistrySkillArchive(registrySkillTestArchive(t, entries), installID); err == nil {
				t.Fatal("readRegistrySkillArchive() accepted malformed SKILL.md frontmatter")
			}
		})
	}
}

const validRegistrySkillManifest = "---\nname: skill\ndescription: Demo\n---\n\n# Demo\n"

type registrySkillTestEntry struct {
	name      string
	content   string
	mode      int64
	entryType byte
	linkName  string
}

func registrySkillTestArchive(t *testing.T, entries []registrySkillTestEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		entryType := entry.entryType
		if entryType == 0 {
			entryType = tar.TypeReg
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{
			Name: entry.name, Mode: mode, Typeflag: entryType, Linkname: entry.linkName,
		}
		if entryType == tar.TypeReg {
			header.Size = int64(len(entry.content))
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q): %v", entry.name, err)
		}
		if entryType == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.content)); err != nil {
				t.Fatalf("Write(%q): %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return output.Bytes()
}

func registrySkillArchiveFileByPath(t *testing.T, archive registrySkillArchive, name string) registrySkillArchiveFile {
	t.Helper()
	for _, file := range archive.files {
		if file.path == name {
			return file
		}
	}
	t.Fatalf("archive does not contain %q", name)
	return registrySkillArchiveFile{}
}

func validRegistrySkillDescriptor() SupermarketCatalogSkill {
	return SupermarketCatalogSkill{
		RegistryID: "registry", PackageID: "package", SkillID: "skill", InstallID: "registry+package+skill",
		Artifact: SupermarketSkillArtifact{
			RegistryID: "registry", PackageID: "package", SkillID: "skill", Format: "memoh_skill_v1",
			Digest: strings.Repeat("a", 64), Size: 1, ContentType: "application/gzip", DownloadURL: "/artifact",
		},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
