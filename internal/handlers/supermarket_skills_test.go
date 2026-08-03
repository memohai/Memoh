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
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/config"
	supermarketclient "github.com/memohai/memoh/internal/supermarket"
	"github.com/memohai/memoh/internal/workspace"
)

const validSkillArtifactContent = "---\nname: skill\ndescription: Demo\n---\n\n# Demo\n"

const validSkillArtifactUncompressedSize = int64(len(validSkillArtifactContent))

func TestSupermarketSkillRoutesUseRegistryCatalogOnly(t *testing.T) {
	var upstreamRequestURI string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestURI = r.URL.RequestURI()
		w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		_, _ = w.Write([]byte(`{"data":[],"total":0,"page":1,"limit":50}`))
	}))
	t.Cleanup(upstream.Close)

	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient(upstream.URL, upstream.Client()),
		logger:   slog.New(slog.DiscardHandler),
	}
	e := echo.New()
	handler.Register(e)

	req := httptest.NewRequest(http.MethodGet, "/supermarket/skills?registry=memoh&limit=50", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /supermarket/skills status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if upstreamRequestURI != "/api/skills?registry=memoh&limit=50" {
		t.Fatalf("upstream request URI = %q, want canonical Skill collection", upstreamRequestURI)
	}
}

func TestSupermarketPackageRoutesUsePackageCatalog(t *testing.T) {
	var upstreamRequestURI string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestURI = r.URL.RequestURI()
		w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient(upstream.URL, upstream.Client()), logger: slog.New(slog.DiscardHandler),
	}
	e := echo.New()
	handler.Register(e)

	tests := []struct {
		path string
		want string
	}{
		{"/supermarket/packages?registry=memoh&limit=50", "/api/packages?registry=memoh&limit=50"},
		{"/supermarket/registries/memoh/packages?q=web", "/api/registries/memoh/packages?q=web"},
		{"/supermarket/registries/memoh/packages/web-tools", "/api/registries/memoh/packages/web-tools"},
	}
	for _, test := range tests {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200: %s", test.path, rec.Code, rec.Body.String())
		}
		if upstreamRequestURI != test.want {
			t.Fatalf("GET %s upstream = %q, want %q", test.path, upstreamRequestURI, test.want)
		}
	}
}

func TestInstallRegistryPackagePublishesMembersInOneMutation(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	artifact := validSkillArtifact(t)
	digest := sha256.Sum256(artifact)
	pkg := validRegistryPackageDescriptor()
	for index := range pkg.Skills {
		pkg.Skills[index].Artifact.Digest = hex.EncodeToString(digest[:])
		pkg.Skills[index].Artifact.Size = int64(len(artifact))
	}
	release := registryPackageReleaseBytes(t, pkg)
	revision := sha256.Sum256(release)
	pkg.Revision = hex.EncodeToString(revision[:])
	installer := &recordingPluginInstaller{}
	artifactRequestedDuringMutation := false
	obsoletePath := "/data/skills/registry/package/obsolete/SKILL.md"
	env.writeSkillFile(t, obsoletePath, managedSkillRaw("obsolete", "Obsolete"))
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.HasPrefix(req.URL.Path, "/api/artifacts/skill/") {
				artifactRequestedDuringMutation = artifactRequestedDuringMutation || installer.mutationCalls > 0
				return testHTTPResponse(req, http.StatusOK, artifact), nil
			}
			wantReleasePath := "/api/registries/registry/packages/package/releases/" + pkg.Revision
			if req.URL.Path != wantReleasePath {
				t.Fatalf("unexpected upstream request path %q, want %q", req.URL.Path, wantReleasePath)
			}
			return testHTTPResponse(req, http.StatusOK, release), nil
		})}),
	}

	service := supermarketclient.NewInstaller(handler.upstream, installer, nil, manager, slog.New(slog.DiscardHandler))
	result, err := service.InstallPackage(context.Background(), env.botID, supermarketclient.InstallPackageRequest{
		RegistryID: pkg.RegistryID, PackageID: pkg.PackageID, Revision: pkg.Revision,
	})
	if err != nil || !result.OK || len(result.Skills) != len(pkg.Skills) {
		t.Fatalf("installRegistryPackage() result=%+v error=%v", result, err)
	}
	if artifactRequestedDuringMutation || installer.mutationCalls != 1 {
		t.Fatalf(
			"Package preflight state: artifact_in_lock=%v mutations=%d",
			artifactRequestedDuringMutation, installer.mutationCalls,
		)
	}
	if _, err := os.Stat(env.localPath(obsoletePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete Package member survived replacement: %v", err)
	}
	for _, skillID := range []string{"skill", "second"} {
		installedPath := "/data/skills/registry/package/" + skillID + "/SKILL.md"
		content, err := os.ReadFile(env.localPath(installedPath))
		if err != nil || string(content) != validSkillArtifactContent {
			t.Fatalf("installed Package member %q content=%q error=%v", skillID, content, err)
		}
	}
	stagingEntries, err := os.ReadDir(env.localPath("/data/skills/.staging"))
	if err != nil || len(stagingEntries) != 0 {
		t.Fatalf("Package staging was not cleaned: entries=%v error=%v", stagingEntries, err)
	}
}

func TestProxySkillIconVerifiesDigestAndHeaders(t *testing.T) {
	content := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	digest := sha256.Sum256(content)
	digestText := hex.EncodeToString(digest[:])
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "image/svg+xml")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			header.Set("ETag", `"`+digestText+`"`)
			return &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(content)),
				ContentLength: int64(len(content)), Request: req, Header: header,
			}, nil
		})}),
	}
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/supermarket/artifacts/icon/"+digestText, nil)
	recorder := httptest.NewRecorder()
	if err := handler.proxySkillIcon(e.NewContext(request, recorder), digestText); err != nil {
		t.Fatalf("proxySkillIcon() error = %v", err)
	}
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), content) {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.Bytes())
	}
	if recorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("immutable cache header was not forwarded")
	}

	handler.upstream = supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("wrong")),
			ContentLength: 5, Request: req, Header: http.Header{"Content-Type": []string{"image/svg+xml"}},
		}, nil
	})})
	if err := handler.proxySkillIcon(e.NewContext(request, httptest.NewRecorder()), digestText); err == nil {
		t.Fatal("proxySkillIcon() accepted content with the wrong digest")
	}
}

func TestProxySkillIconOverridesUpstreamSecurityHeaders(t *testing.T) {
	content := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	digest := sha256.Sum256(content)
	digestText := hex.EncodeToString(digest[:])
	handler := &SupermarketHandler{
		upstream: supermarketclient.NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "image/svg+xml")
			// A compromised/misconfigured upstream sends a permissive CSP; the
			// proxy must not forward it.
			header.Set("Content-Security-Policy", "default-src *")
			return &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(content)),
				ContentLength: int64(len(content)), Request: req, Header: header,
			}, nil
		})}),
	}
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/supermarket/artifacts/icon/"+digestText, nil)
	recorder := httptest.NewRecorder()
	if err := handler.proxySkillIcon(e.NewContext(request, recorder), digestText); err != nil {
		t.Fatalf("proxySkillIcon() error = %v", err)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") || strings.Contains(got, "*") {
		t.Fatalf("upstream CSP was not overridden: %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func validSkillArtifact(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	content := []byte(validSkillArtifactContent)
	if err := tw.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0o644, Typeflag: tar.TypeReg, Size: int64(len(content))}); err != nil {
		t.Fatalf("WriteHeader(SKILL.md): %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("Write(SKILL.md): %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return output.Bytes()
}

func validRegistrySkillDescriptor() SupermarketCatalogSkill {
	return SupermarketCatalogSkill{
		RegistryID: "registry", PackageID: "package", SkillID: "skill", InstallID: "registry+package+skill",
		Artifact: SupermarketSkillArtifact{
			Format: "memoh_skill_v1",
			Digest: strings.Repeat("a", 64), Size: 1,
			UncompressedSize: validSkillArtifactUncompressedSize,
			ArchiveSize:      2 * 1024,
			FileCount:        1,
			ContentType:      "application/gzip", DownloadURL: "/artifact",
		},
	}
}

func validRegistryPackageDescriptor() SupermarketSkillPackageDescriptor {
	first := validRegistrySkillDescriptor()
	second := validRegistrySkillDescriptor()
	second.SkillID = "second"
	second.InstallID = "registry+package+second"
	return SupermarketSkillPackageDescriptor{
		SkillPackageSummary: SupermarketSkillPackageSummary{
			SchemaVersion: "1", RegistryID: "registry", PackageID: "package", Name: "Package",
			Description: "Demo", Tags: []string{}, Categories: []SupermarketSkillPackageCategory{}, SkillCount: 2,
		},
		Revision: strings.Repeat("b", 64),
		Skills:   []SupermarketCatalogSkill{first, second},
	}
}

func registryPackageReleaseBytes(t *testing.T, pkg SupermarketSkillPackageDescriptor) []byte {
	t.Helper()
	members := make([]supermarketSkillPackageReleaseSkill, 0, len(pkg.Skills))
	for _, skill := range pkg.Skills {
		members = append(members, supermarketSkillPackageReleaseSkill{
			SchemaVersion: skill.SchemaVersion, RegistryID: skill.RegistryID, PackageID: skill.PackageID,
			SkillID: skill.SkillID, InstallID: skill.InstallID, Name: skill.Name,
			Description: skill.Description, Author: skill.Author, Homepage: skill.Homepage,
			Tags: skill.Tags, Category: skill.Category, CategoryName: skill.CategoryName,
			SourceCategory: skill.SourceCategory, Files: skill.Files, Icon: skill.Icon, Artifact: skill.Artifact,
		})
	}
	payload, err := json.Marshal(SupermarketSkillPackageRelease{
		SchemaVersion: pkg.SchemaVersion,
		RegistryID:    pkg.RegistryID,
		PackageID:     pkg.PackageID,
		Name:          pkg.Name,
		Description:   pkg.Description,
		Tags:          pkg.Tags,
		Icon:          pkg.Icon,
		Skills:        members,
	})
	if err != nil {
		t.Fatalf("marshal immutable Package release: %v", err)
	}
	return payload
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
