package supermarket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	pluginspkg "github.com/memohai/memoh/internal/plugins"
)

func TestFetchPluginEntryResolvesImmutableRelease(t *testing.T) {
	release := ImmutablePluginRelease{
		SchemaVersion: "1",
		Plugin: pluginspkg.Manifest{
			SchemaVersion: "1", ID: "example", Name: "Example", Version: "1.0.0",
		},
		Artifact: PluginArtifact{
			Format: "memoh_plugin_v1", Digest: strings.Repeat("a", 64),
			Size: 10, ContentType: "application/gzip",
		},
		Packages: []PluginResolvedPackage{},
	}
	releaseBytes := mustJSONBytes(t, release)
	revision := digestText(releaseBytes)
	currentBytes := mustJSONBytes(t, PluginEntry{
		Manifest: release.Plugin,
		Release: PluginRelease{
			Revision: revision, PublishedAt: "2026-08-01T00:00:00Z",
			Artifact: release.Artifact, Packages: release.Packages,
		},
	})
	client := NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/plugins/example":
			return protocolTestResponse(req, http.StatusOK, currentBytes), nil
		case "/api/plugins/example/releases/" + revision:
			return protocolTestResponse(req, http.StatusOK, releaseBytes), nil
		default:
			return protocolTestResponse(req, http.StatusNotFound, nil), nil
		}
	})})

	entry, err := client.FetchPluginEntry(context.Background(), "example")
	if err != nil {
		t.Fatalf("FetchPluginEntry() error = %v", err)
	}
	if entry.ID != "example" || entry.Release.Revision != revision {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Release.Artifact.DownloadURL != "/api/artifacts/plugin/"+release.Artifact.Digest {
		t.Fatalf("download URL = %q", entry.Release.Artifact.DownloadURL)
	}
}

func TestFetchPluginEntryRejectsTamperedRelease(t *testing.T) {
	releaseBytes := mustJSONBytes(t, ImmutablePluginRelease{
		SchemaVersion: "1",
		Plugin:        pluginspkg.Manifest{SchemaVersion: "1", ID: "example", Name: "Example"},
		Artifact:      PluginArtifact{},
		Packages:      []PluginResolvedPackage{},
	})
	revision := digestText(releaseBytes)
	currentBytes := mustJSONBytes(t, PluginEntry{Release: PluginRelease{
		Revision: revision, PublishedAt: "2026-08-01T00:00:00Z",
	}})
	client := NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/plugins/example" {
			return protocolTestResponse(req, http.StatusOK, currentBytes), nil
		}
		return protocolTestResponse(req, http.StatusOK, append(releaseBytes, ' ')), nil
	})})

	_, err := client.FetchPluginEntry(context.Background(), "example")
	if ErrorKindOf(err) != ErrorInvalidResponse || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("error = %v, kind = %q", err, ErrorKindOf(err))
	}
}

func TestFetchPackageReleaseHydratesArtifactURLs(t *testing.T) {
	release := SkillPackageRelease{
		SchemaVersion: "1", RegistryID: "openai", PackageID: "docs",
		Name: "Docs", Description: "Docs", Tags: []string{},
		Skills: []SkillPackageReleaseSkill{{
			SchemaVersion: "1", RegistryID: "openai", PackageID: "docs", SkillID: "write-docs",
			InstallID: "openai+docs+write-docs", Name: "Write docs", Description: "Write docs",
			Author: Author{Name: "OpenAI", Email: "support@example.com"}, Tags: []string{}, Files: []string{"SKILL.md"},
			Artifact: SkillArtifact{
				Format: "memoh_skill_v1", Digest: strings.Repeat("b", 64), Size: 10,
				UncompressedSize: 10, ArchiveSize: 512, FileCount: 1, ContentType: "application/gzip",
			},
		}},
	}
	payload := mustJSONBytes(t, release)
	revision := digestText(payload)
	client := NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return protocolTestResponse(req, http.StatusOK, payload), nil
	})})

	pkg, err := client.FetchPackageRelease(context.Background(), "openai", "docs", revision)
	if err != nil {
		t.Fatalf("FetchPackageRelease() error = %v", err)
	}
	if pkg.Revision != revision || len(pkg.Skills) != 1 || pkg.SkillCount != 1 {
		t.Fatalf("Package = %+v", pkg)
	}
	wantURL := "/api/artifacts/skill/" + release.Skills[0].Artifact.Digest
	if pkg.Skills[0].Artifact.DownloadURL != wantURL {
		t.Fatalf("download URL = %q, want %q", pkg.Skills[0].Artifact.DownloadURL, wantURL)
	}
}

func TestDownloadArtifactVerifiesDescriptor(t *testing.T) {
	content := []byte("artifact")
	digest := digestText(content)
	client := NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return protocolTestResponse(req, http.StatusOK, content), nil
	})})

	got, err := client.DownloadArtifact(context.Background(), ArtifactDownloadDescriptor{
		Digest: digest, Size: int64(len(content)), DownloadURL: "/api/artifacts/skill/" + digest,
	})
	if err != nil || string(got) != string(content) {
		t.Fatalf("DownloadArtifact() = %q, %v", got, err)
	}
	_, err = client.DownloadArtifact(context.Background(), ArtifactDownloadDescriptor{
		Digest: strings.Repeat("0", 64), Size: int64(len(content)), DownloadURL: "/api/artifacts/skill/invalid",
	})
	if ErrorKindOf(err) != ErrorInvalidResponse {
		t.Fatalf("digest mismatch kind = %q, error = %v", ErrorKindOf(err), err)
	}
}

func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return payload
}

func digestText(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func protocolTestResponse(req *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Body:          io.NopCloser(strings.NewReader(string(body))),
		Request:       req,
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
	}
}
