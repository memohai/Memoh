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
	"path"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/config"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
)

const validSkillArtifactContent = "---\nname: skill\ndescription: Demo\n---\n\n# Demo\n"

const validSkillArtifactUncompressedSize = int64(len(validSkillArtifactContent))

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

func TestValidateRegistrySkillRequiresBoundedUncompressedSize(t *testing.T) {
	for _, size := range []int64{0, maxRegistrySkillArtifactUncompressedBytes + 1} {
		skill := validRegistrySkillDescriptor()
		skill.Artifact.UncompressedSize = size
		if err := validateRegistrySkill(skill, "registry", "package", "skill"); err == nil {
			t.Fatalf("validateRegistrySkill() accepted uncompressed_size %d", size)
		}
	}
}

func TestValidateRegistrySkillRequiresArchiveAndFileBudgets(t *testing.T) {
	for name, mutate := range map[string]func(*SupermarketSkillArtifact){
		"archive size": func(artifact *SupermarketSkillArtifact) {
			artifact.ArchiveSize = maxRegistrySkillArtifactArchiveBytes + 1
		},
		"file count": func(artifact *SupermarketSkillArtifact) {
			artifact.FileCount = maxRegistrySkillArtifactFiles + 1
		},
	} {
		t.Run(name, func(t *testing.T) {
			skill := validRegistrySkillDescriptor()
			mutate(&skill.Artifact)
			if err := validateRegistrySkill(skill, "registry", "package", "skill"); err == nil {
				t.Fatalf("validateRegistrySkill() accepted invalid %s", name)
			}
		})
	}
}

func TestRegistrySkillRuntimeRequirementsEnforceWorkspaceOS(t *testing.T) {
	skill := validRegistrySkillDescriptor()
	skill.RuntimeRequirements.OS = []string{"linux", "windows"}
	if err := validateRegistrySkill(skill, "registry", "package", "skill"); err != nil {
		t.Fatalf("validateRegistrySkill(valid OS requirements) error = %v", err)
	}
	if !registrySkillSupportsOS(skill.RuntimeRequirements, "linux") {
		t.Fatal("linux workspace should be supported")
	}
	if !registrySkillSupportsOS(skill.RuntimeRequirements, "win32") {
		t.Fatal("windows alias should normalize to win32")
	}
	if registrySkillSupportsOS(skill.RuntimeRequirements, "darwin") {
		t.Fatal("darwin workspace should be rejected")
	}

	skill.RuntimeRequirements.OS = []string{"freebsd"}
	if err := validateRegistrySkill(skill, "registry", "package", "skill"); err == nil {
		t.Fatal("validateRegistrySkill accepted an unknown runtime OS")
	}
	if !registrySkillSupportsOS(SupermarketSkillRuntimeRequirements{}, "unknown") {
		t.Fatal("an undeclared OS requirement should remain installable")
	}
}

func TestInstallRegistrySkillArtifactRejectsIncompatibleOSBeforeDownload(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	client, err := manager.MCPClient(context.Background(), env.botID)
	if err != nil {
		t.Fatalf("get workspace client: %v", err)
	}
	skill := validRegistrySkillDescriptor()
	skill.RuntimeRequirements.OS = []string{"linux"}
	descriptor, err := json.Marshal(skill)
	if err != nil {
		t.Fatalf("marshal descriptor: %v", err)
	}
	artifactRequested := false
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == skill.Artifact.DownloadURL {
				artifactRequested = true
			}
			return testHTTPResponse(req, http.StatusOK, descriptor), nil
		})},
	}

	_, err = handler.installRegistrySkillArtifact(
		context.Background(), client, "darwin", true, skill.RegistryID, skill.PackageID, skill.SkillID,
	)
	if got := apperror.CodeOf(err); got != apperror.CodeRegistrySkillIncompatible {
		t.Fatalf("incompatible OS code = %q, want %q", got, apperror.CodeRegistrySkillIncompatible)
	}
	if artifactRequested {
		t.Fatal("incompatible Skill Artifact was downloaded")
	}
	args := apperror.ArgsOf(err)
	if args["os"] != "darwin" || args["supported_os"] != "linux" {
		t.Fatalf("incompatible OS args = %#v", args)
	}
}

func TestPrepareRegistrySkillArtifactRejectsDeclaredBudgetBeforeDownload(t *testing.T) {
	skill := validRegistrySkillDescriptor()
	artifactRequested := false
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			artifactRequested = true
			return testHTTPResponse(req, http.StatusOK, nil), nil
		})},
	}

	_, err := handler.prepareResolvedRegistrySkillArtifactWithLimit(
		context.Background(), "linux", skill, skill.Artifact.UncompressedSize-1,
	)
	if got := apperror.CodeOf(err); got != apperror.CodeRegistrySkillInvalid {
		t.Fatalf("prepare code = %q, want %q", got, apperror.CodeRegistrySkillInvalid)
	}
	if artifactRequested {
		t.Fatal("over-budget Skill Artifact was downloaded")
	}
}

func TestPrepareRegistrySkillArtifactVerifiesDeclaredUncompressedSize(t *testing.T) {
	artifact := validSkillArtifact(t)
	digest := sha256.Sum256(artifact)
	skill := validRegistrySkillDescriptor()
	skill.Artifact.Digest = hex.EncodeToString(digest[:])
	skill.Artifact.Size = int64(len(artifact))
	skill.Artifact.UncompressedSize = validSkillArtifactUncompressedSize + 1
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return testHTTPResponse(req, http.StatusOK, artifact), nil
		})},
	}

	_, err := handler.prepareResolvedRegistrySkillArtifact(context.Background(), "linux", skill)
	if got := apperror.CodeOf(err); got != apperror.CodeRegistrySkillInvalid {
		t.Fatalf("prepare code = %q, want %q", got, apperror.CodeRegistrySkillInvalid)
	}
	cause := apperror.CauseOf(err)
	if cause == nil || !strings.Contains(cause.Error(), "does not match its descriptor") {
		t.Fatalf("prepare cause = %v, want declared size mismatch", cause)
	}
}

func TestDirectRegistrySkillOwnerIsPublishedAtomically(t *testing.T) {
	env := newSkillsTestEnv(t)
	artifact := validSkillArtifact(t)
	digest := sha256.Sum256(artifact)
	skill := validRegistrySkillDescriptor()
	skill.Artifact.Digest = hex.EncodeToString(digest[:])
	skill.Artifact.Size = int64(len(artifact))
	descriptor, err := json.Marshal(skill)
	if err != nil {
		t.Fatalf("marshal descriptor: %v", err)
	}
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	installer := &recordingPluginInstaller{}
	artifactRequestedDuringMutation := false
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == skill.Artifact.DownloadURL {
				artifactRequestedDuringMutation = installer.mutationCalls > 0
				return testHTTPResponse(req, http.StatusOK, artifact), nil
			}
			return testHTTPResponse(req, http.StatusOK, descriptor), nil
		})},
		pluginService: installer,
		workspaces:    manager,
	}

	result, err := handler.installRegistrySkill(context.Background(), env.botID, InstallSkillRequest{
		RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID,
		ArtifactDigest: skill.Artifact.Digest,
	})
	if err != nil || !result.OK {
		t.Fatalf("installRegistrySkill() result=%+v error=%v", result, err)
	}
	if artifactRequestedDuringMutation || installer.mutationCalls != 1 {
		t.Fatalf("direct Skill preflight lock state: artifact_in_lock=%v mutations=%d", artifactRequestedDuringMutation, installer.mutationCalls)
	}
	client, err := manager.MCPClient(context.Background(), env.botID)
	if err != nil {
		t.Fatalf("get workspace client: %v", err)
	}
	if !skillset.HasDirectOwner(context.Background(), client, skill.RegistryID, skill.PackageID, skill.SkillID) {
		t.Fatal("published Registry Skill has no direct owner")
	}

	secondEnv := newSkillsTestEnv(t)
	secondManager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: secondEnv.dataRoot}, "", nil,
	)
	secondEnv.bridge.writeBaseErrors[skillset.DirectOwnerFileName] = errors.New("injected marker failure")
	handler.workspaces = secondManager
	_, err = handler.installRegistrySkill(context.Background(), secondEnv.botID, InstallSkillRequest{
		RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID,
		ArtifactDigest: skill.Artifact.Digest,
	})
	if got := apperror.CodeOf(err); got != apperror.CodeRegistrySkillInstallFailed {
		t.Fatalf("marker failure code = %q, want %q", got, apperror.CodeRegistrySkillInstallFailed)
	}
	targetDir, pathErr := skillset.SkillDirForIDs(skill.RegistryID, skill.PackageID, skill.SkillID)
	if pathErr != nil {
		t.Fatalf("SkillDirForIDs() error = %v", pathErr)
	}
	if _, statErr := os.Stat(secondEnv.localPath(targetDir)); !os.IsNotExist(statErr) {
		t.Fatalf("ownerless Registry Skill should not be published, stat err=%v", statErr)
	}
}

func TestInstallRegistrySkillRejectsStaleArtifactBeforeDownload(t *testing.T) {
	env := newSkillsTestEnv(t)
	manager := workspace.NewManager(
		slog.Default(), nil, nil, config.WorkspaceConfig{DataRoot: env.dataRoot}, "", nil,
	)
	skill := validRegistrySkillDescriptor()
	artifact := validSkillArtifact(t)
	digest := sha256.Sum256(artifact)
	skill.Artifact.Digest = hex.EncodeToString(digest[:])
	skill.Artifact.Size = int64(len(artifact))
	descriptor, err := json.Marshal(skill)
	if err != nil {
		t.Fatalf("marshal Skill descriptor: %v", err)
	}
	artifactRequested := false
	installer := &recordingPluginInstaller{}
	handler := &SupermarketHandler{
		baseURL: "https://supermarket.example",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == skill.Artifact.DownloadURL {
				artifactRequested = true
				return testHTTPResponse(req, http.StatusOK, artifact), nil
			}
			return testHTTPResponse(req, http.StatusOK, descriptor), nil
		})},
		pluginService: installer,
		workspaces:    manager,
	}

	_, err = handler.installRegistrySkill(context.Background(), env.botID, InstallSkillRequest{
		RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID,
		ArtifactDigest: strings.Repeat("0", 64),
	})
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("installRegistrySkill() error = %v, want HTTP 409", err)
	}
	if artifactRequested || installer.mutationCalls != 0 {
		t.Fatalf("stale Skill touched installation state: artifact=%v mutations=%d", artifactRequested, installer.mutationCalls)
	}
}

func TestSupermarketSkillRoutesUseRegistryCatalogOnly(t *testing.T) {
	var upstreamRequestURI string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestURI = r.URL.RequestURI()
		w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		_, _ = w.Write([]byte(`{"data":[],"total":0,"page":1,"limit":50}`))
	}))
	t.Cleanup(upstream.Close)

	handler := &SupermarketHandler{
		baseURL:    upstream.URL,
		httpClient: upstream.Client(),
		logger:     slog.New(slog.DiscardHandler),
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

	for _, legacyPath := range []string{"/supermarket/catalog/skills", "/supermarket/skills/flat-id"} {
		req = httptest.NewRequest(http.MethodGet, legacyPath, nil)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", legacyPath, rec.Code)
		}
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

func TestInstallRegistrySkillHidesWorkspaceFailureDetails(t *testing.T) {
	env := newSkillsTestEnv(t)
	env.writeSkillFile(t, path.Join(skillset.ManagedDir(), ".staging"), "not a directory")

	skill := validRegistrySkillDescriptor()
	artifact := validSkillArtifact(t)
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
		RegistryID:     skill.RegistryID,
		PackageID:      skill.PackageID,
		SkillID:        skill.SkillID,
		ArtifactDigest: skill.Artifact.Digest,
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

func TestProxySkillIconVerifiesDigestAndHeaders(t *testing.T) {
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

	handler.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("wrong")),
			ContentLength: 5, Request: req, Header: http.Header{"Content-Type": []string{"image/svg+xml"}},
		}, nil
	})}
	if err := handler.proxySkillIcon(e.NewContext(request, httptest.NewRecorder()), digestText); err == nil {
		t.Fatal("proxySkillIcon() accepted content with the wrong digest")
	}
}

func TestProxySkillIconOverridesUpstreamSecurityHeaders(t *testing.T) {
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
