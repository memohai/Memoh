package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestReadRegistrySkillArchiveRewritesNamespacedManifest(t *testing.T) {
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
	if !strings.Contains(string(manifest.content), "name: "+installID) {
		t.Fatalf("manifest did not receive namespaced name:\n%s", manifest.content)
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

func TestSupportsWorkspaceOSNormalizesWindows(t *testing.T) {
	if !supportsWorkspaceOS([]string{"win32"}, "windows") {
		t.Fatal("windows workspace should satisfy win32 requirement")
	}
	if supportsWorkspaceOS([]string{"darwin"}, "linux") {
		t.Fatal("linux workspace should not satisfy darwin requirement")
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
	if _, err := handler.downloadRegistrySkillArtifact(context.Background(), artifact); err == nil {
		t.Fatal("cross-origin artifact URL was accepted")
	}
	artifact.DownloadURL = "/api/artifacts/digest/download"
	artifact.Digest = strings.Repeat("0", 64)
	if _, err := handler.downloadRegistrySkillArtifact(context.Background(), artifact); err == nil {
		t.Fatal("invalid Artifact digest was accepted")
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
