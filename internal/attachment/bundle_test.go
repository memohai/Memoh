package attachment

import (
	"testing"

	"github.com/memohai/memoh/internal/media"
)

func TestBundleNormalizeTransportRefs(t *testing.T) {
	cases := []struct {
		name       string
		input      Bundle
		wantBase64 string
		wantPath   string
		wantURL    string
		wantMime   string
		wantName   string
		wantType   string
	}{
		{
			name:       "data url becomes inline base64",
			input:      Bundle{URL: "data:image/png;base64,AAAA"},
			wantBase64: "data:image/png;base64,AAAA",
			wantMime:   "image/png",
			wantType:   "image",
		},
		{
			name:       "raw base64 gets data url wrapper",
			input:      Bundle{Base64: "AAAA", Mime: "IMAGE/PNG; charset=utf-8"},
			wantBase64: "data:image/png;base64,AAAA",
			wantMime:   "image/png",
			wantType:   "image",
		},
		{
			name:     "local url becomes path",
			input:    Bundle{URL: "/data/screenshots/demo.png"},
			wantPath: "/data/screenshots/demo.png",
			wantName: "demo.png",
			wantType: "image",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := tc.input.Normalize()
			if bundle.Base64 != tc.wantBase64 {
				t.Fatalf("Base64 = %q, want %q", bundle.Base64, tc.wantBase64)
			}
			if bundle.Path != tc.wantPath {
				t.Fatalf("Path = %q, want %q", bundle.Path, tc.wantPath)
			}
			if bundle.URL != tc.wantURL {
				t.Fatalf("URL = %q, want %q", bundle.URL, tc.wantURL)
			}
			if bundle.Mime != tc.wantMime {
				t.Fatalf("Mime = %q, want %q", bundle.Mime, tc.wantMime)
			}
			if bundle.Name != tc.wantName {
				t.Fatalf("Name = %q, want %q", bundle.Name, tc.wantName)
			}
			if bundle.Type != tc.wantType {
				t.Fatalf("Type = %q, want %q", bundle.Type, tc.wantType)
			}
		})
	}
}

func TestParseToolInputBundles(t *testing.T) {
	bundles, ok := ParseToolInputBundles([]any{
		"screenshot.png",
		"https://example.com/demo.jpg",
		"data:image/png;base64,AAAA",
		map[string]any{
			"content_hash": "asset-1",
			"type":         "image",
		},
	})
	if !ok {
		t.Fatal("expected tool input parsing to succeed")
	}
	if len(bundles) != 4 {
		t.Fatalf("expected 4 bundles, got %d", len(bundles))
	}

	if bundles[0].Path != "/data/screenshot.png" {
		t.Fatalf("expected bare path to resolve under /data, got %q", bundles[0].Path)
	}
	if bundles[0].Type != "image" {
		t.Fatalf("expected inferred image type for png path, got %q", bundles[0].Type)
	}

	if bundles[1].URL != "https://example.com/demo.jpg" {
		t.Fatalf("unexpected remote URL bundle: %q", bundles[1].URL)
	}
	if bundles[1].Name != "demo.jpg" {
		t.Fatalf("expected inferred URL name demo.jpg, got %q", bundles[1].Name)
	}

	if bundles[2].Base64 != "data:image/png;base64,AAAA" {
		t.Fatalf("unexpected inline data bundle: %q", bundles[2].Base64)
	}
	if bundles[2].Type != "image" {
		t.Fatalf("expected inferred image type for inline data, got %q", bundles[2].Type)
	}

	if bundles[3].ContentHash != "asset-1" {
		t.Fatalf("expected content hash asset-1, got %q", bundles[3].ContentHash)
	}
	if bundles[3].Type != "image" {
		t.Fatalf("expected explicit image type preserved, got %q", bundles[3].Type)
	}
}

func TestParseToolInputBundles_InvalidTopLevelType(t *testing.T) {
	if _, ok := ParseToolInputBundles(42); ok {
		t.Fatal("expected invalid top-level input to be rejected")
	}
}

func TestBundleFromMapAndToMap(t *testing.T) {
	raw := map[string]any{
		"type": "image",
		"url":  "/data/images/demo.png",
		"mime": "IMAGE/PNG; charset=utf-8",
		"size": float64(12),
		"metadata": map[string]any{
			"source": "tool",
		},
	}

	bundle := BundleFromMap(raw)
	if bundle.Path != "/data/images/demo.png" {
		t.Fatalf("expected path normalized from map, got %q", bundle.Path)
	}
	if bundle.Mime != "image/png" {
		t.Fatalf("expected normalized mime image/png, got %q", bundle.Mime)
	}
	if bundle.Size != 12 {
		t.Fatalf("expected size 12, got %d", bundle.Size)
	}

	roundTrip := bundle.ToMap()
	if roundTrip["path"] != "/data/images/demo.png" {
		t.Fatalf("expected serialized path preserved, got %#v", roundTrip["path"])
	}
	if roundTrip["mime"] != "image/png" {
		t.Fatalf("expected serialized mime image/png, got %#v", roundTrip["mime"])
	}
}

func TestBundleWithAsset(t *testing.T) {
	bundle := Bundle{
		Type: "image",
		URL:  "https://example.com/demo.png",
	}.WithAsset("bot-1", media.Asset{
		ContentHash: "asset-1",
		Mime:        "image/png",
		SizeBytes:   42,
		StorageKey:  "aa/asset-1.png",
	})

	if bundle.ContentHash != "asset-1" {
		t.Fatalf("expected content hash asset-1, got %q", bundle.ContentHash)
	}
	if bundle.URL != "" || bundle.Path != "" || bundle.Base64 != "" {
		t.Fatalf("expected transport refs cleared after asset rewrite: %#v", bundle)
	}
	if bundle.Metadata[MetadataKeyBotID] != "bot-1" {
		t.Fatalf("expected bot_id metadata, got %#v", bundle.Metadata[MetadataKeyBotID])
	}
	if bundle.Metadata[MetadataKeyStorageKey] != "aa/asset-1.png" {
		t.Fatalf("expected storage key metadata, got %#v", bundle.Metadata[MetadataKeyStorageKey])
	}
	if bundle.Metadata[MetadataKeySourceURL] != "https://example.com/demo.png" {
		t.Fatalf("expected source_url metadata, got %#v", bundle.Metadata[MetadataKeySourceURL])
	}
	if bundle.Name != "demo.png" {
		t.Fatalf("expected inferred name demo.png, got %q", bundle.Name)
	}
}

func TestBundleWithAssetAccess(t *testing.T) {
	t.Parallel()

	bundle := Bundle{
		Type: "file",
		Path: "/data/work/demo.txt",
	}.WithAssetAccess("bot-1", media.Asset{
		ContentHash: "asset-2",
		Mime:        "text/plain",
		SizeBytes:   7,
		StorageKey:  "bb/asset-2.txt",
	}, "/data/media/bb/asset-2.txt")

	if bundle.Path != "/data/media/bb/asset-2.txt" {
		t.Fatalf("expected access path preserved, got %q", bundle.Path)
	}
	if MetadataString(bundle.Metadata, MetadataKeySourcePath) != "/data/work/demo.txt" {
		t.Fatalf("expected source_path metadata preserved, got %#v", bundle.Metadata[MetadataKeySourcePath])
	}
}

func TestExtractStorageKey(t *testing.T) {
	t.Parallel()

	if got := ExtractStorageKey("/data/media/aa/demo.png"); got != "aa/demo.png" {
		t.Fatalf("unexpected storage key: %q", got)
	}
	if got := ExtractStorageKey("/tmp/demo.png"); got != "" {
		t.Fatalf("expected empty storage key for non-media path, got %q", got)
	}
}

func TestDataSubpath(t *testing.T) {
	t.Parallel()

	if got, ok := DataSubpath("/data/work/demo.txt"); !ok || got != "work/demo.txt" {
		t.Fatalf("expected work/demo.txt, got %q ok=%v", got, ok)
	}
	if _, ok := DataSubpath("/tmp/work/demo.txt"); ok {
		t.Fatal("expected non-data path to be rejected")
	}
	if _, ok := DataSubpath("/data"); ok {
		t.Fatal("expected bare data mount to be rejected")
	}
}
