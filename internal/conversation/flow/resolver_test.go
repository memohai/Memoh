package flow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
)

type fakeGatewayAssetLoader struct {
	openFn func(ctx context.Context, botID, contentHash string) (io.ReadCloser, string, error)
}

func (f *fakeGatewayAssetLoader) OpenForGateway(ctx context.Context, botID, contentHash string) (io.ReadCloser, string, error) {
	if f == nil || f.openFn == nil {
		return nil, "", io.EOF
	}
	return f.openFn(ctx, botID, contentHash)
}

func TestPrepareGatewayAttachments_InlineAssetToBase64(t *testing.T) {
	resolver := &Resolver{
		logger: slog.Default(),
		assetLoader: &fakeGatewayAssetLoader{
			openFn: func(_ context.Context, _, contentHash string) (io.ReadCloser, string, error) {
				if contentHash != "asset-1" {
					t.Fatalf("unexpected content hash: %s", contentHash)
				}
				return io.NopCloser(strings.NewReader("image-binary")), "image/png", nil
			},
		},
	}
	req := conversation.ChatRequest{
		BotID: "bot-1",
		Attachments: []conversation.ChatAttachment{
			{
				Type:        "image",
				ContentHash: "asset-1",
			},
		},
	}

	prepared := resolver.prepareGatewayAttachments(context.Background(), req)
	if len(prepared) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(prepared))
	}
	if prepared[0].Transport != gatewayTransportInlineDataURL {
		t.Fatalf("expected inline transport, got %q", prepared[0].Transport)
	}
	if !strings.HasPrefix(prepared[0].Payload, "data:image/png;base64,") {
		t.Fatalf("expected data url image attachment, got %q", prepared[0].Payload)
	}
	if prepared[0].Mime != "image/png" {
		t.Fatalf("expected mime image/png, got %q", prepared[0].Mime)
	}
}

func TestPrepareGatewayAttachments_DataURLFromURLFieldIsNativeInline(t *testing.T) {
	resolver := &Resolver{logger: slog.Default()}
	req := conversation.ChatRequest{
		Attachments: []conversation.ChatAttachment{
			{
				Type: "image",
				URL:  "data:image/png;base64,AAAA",
			},
		},
	}

	prepared := resolver.prepareGatewayAttachments(context.Background(), req)
	if len(prepared) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(prepared))
	}
	if prepared[0].Transport != gatewayTransportInlineDataURL {
		t.Fatalf("expected inline transport, got %q", prepared[0].Transport)
	}
	if prepared[0].Payload != "data:image/png;base64,AAAA" {
		t.Fatalf("unexpected payload: %q", prepared[0].Payload)
	}
	if prepared[0].FallbackPath != "" {
		t.Fatalf("expected empty fallback path, got %q", prepared[0].FallbackPath)
	}
}

func TestPrepareGatewayAttachments_PublicURLFromURLFieldIsNativePublic(t *testing.T) {
	resolver := &Resolver{logger: slog.Default()}
	req := conversation.ChatRequest{
		Attachments: []conversation.ChatAttachment{
			{
				Type: "image",
				URL:  "https://example.com/demo.png",
			},
		},
	}

	prepared := resolver.prepareGatewayAttachments(context.Background(), req)
	if len(prepared) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(prepared))
	}
	if prepared[0].Transport != gatewayTransportPublicURL {
		t.Fatalf("expected public transport, got %q", prepared[0].Transport)
	}
	if prepared[0].Payload != "https://example.com/demo.png" {
		t.Fatalf("unexpected payload: %q", prepared[0].Payload)
	}
	if prepared[0].FallbackPath != "" {
		t.Fatalf("expected empty fallback path, got %q", prepared[0].FallbackPath)
	}
}

func TestRouteAndMergeAttachments_ImagePathOnlyFallsBackToFile(t *testing.T) {
	resolver := &Resolver{logger: slog.Default()}
	model := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				Compatibilities: []string{models.CompatVision},
			},
		},
	}
	req := conversation.ChatRequest{
		Attachments: []conversation.ChatAttachment{
			{
				Type: "image",
				Path: "/data/media/image/demo.png",
			},
		},
	}

	merged := resolver.routeAndMergeAttachments(context.Background(), model, req)
	if len(merged) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(merged))
	}
	item, ok := merged[0].(gatewayAttachment)
	if !ok {
		t.Fatalf("expected gatewayAttachment type")
	}
	if item.Type != "file" {
		t.Fatalf("expected fallback type file, got %q", item.Type)
	}
	if item.Transport != gatewayTransportToolFileRef {
		t.Fatalf("expected tool_file_ref transport, got %q", item.Transport)
	}
	if item.Payload != "/data/media/image/demo.png" {
		t.Fatalf("unexpected fallback payload: %q", item.Payload)
	}
}

func TestPrepareGatewayAttachments_DetectsImageMimeWhenOctetStream(t *testing.T) {
	jpegBytes := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
		0x49, 0x46, 0x00, 0x01, 0xFF, 0xD9,
	}
	resolver := &Resolver{
		logger: slog.Default(),
		assetLoader: &fakeGatewayAssetLoader{
			openFn: func(_ context.Context, _, _ string) (io.ReadCloser, string, error) {
				return io.NopCloser(bytes.NewReader(jpegBytes)), "application/octet-stream", nil
			},
		},
	}
	req := conversation.ChatRequest{
		BotID: "bot-1",
		Attachments: []conversation.ChatAttachment{
			{
				Type:        "image",
				ContentHash: "asset-2",
			},
		},
	}

	prepared := resolver.prepareGatewayAttachments(context.Background(), req)
	if len(prepared) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(prepared))
	}
	if prepared[0].Transport != gatewayTransportInlineDataURL {
		t.Fatalf("expected inline transport, got %q", prepared[0].Transport)
	}
	if !strings.HasPrefix(prepared[0].Payload, "data:image/jpeg;base64,") {
		t.Fatalf("expected detected image/jpeg data url, got %q", prepared[0].Payload)
	}
	if prepared[0].Mime != "image/jpeg" {
		t.Fatalf("expected mime image/jpeg, got %q", prepared[0].Mime)
	}
}

func TestRouteAndMergeAttachments_DropsUnsupportedInlineWithoutFallbackPath(t *testing.T) {
	resolver := &Resolver{logger: slog.Default()}
	model := models.GetResponse{
		Model: models.Model{
			Config: models.ModelConfig{
				Compatibilities: []string{},
			},
		},
	}
	req := conversation.ChatRequest{
		Attachments: []conversation.ChatAttachment{
			{
				Type:   "video",
				Base64: "AAAA",
			},
		},
	}

	merged := resolver.routeAndMergeAttachments(context.Background(), model, req)
	if len(merged) != 0 {
		t.Fatalf("expected unsupported inline attachment to be dropped, got %d", len(merged))
	}
}

func TestEncodeReaderAsDataURL_DetectsImageMime(t *testing.T) {
	jpegBytes := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
		0x49, 0x46, 0x00, 0x01, 0xFF, 0xD9,
	}

	dataURL, mime, err := encodeReaderAsDataURL(
		bytes.NewReader(jpegBytes),
		int64(len(jpegBytes)),
		"image",
		"application/octet-stream",
	)
	if err != nil {
		t.Fatalf("encodeReaderAsDataURL returned error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("expected image/jpeg mime, got %q", mime)
	}
	expected := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegBytes)
	if dataURL != expected {
		t.Fatalf("unexpected data URL")
	}
}

func TestEncodeReaderAsDataURL_RejectsOversizedPayload(t *testing.T) {
	_, _, err := encodeReaderAsDataURL(strings.NewReader("12345"), 4, "image", "image/png")
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "asset too large to inline") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutboundAssetRefsToMessageRefs(t *testing.T) {
	t.Parallel()
	refs := []conversation.OutboundAssetRef{
		{ContentHash: "a1", Role: "attachment", Ordinal: 0},
		{ContentHash: "", Role: "attachment", Ordinal: 1},
		{ContentHash: "a2", Ordinal: 2},
	}
	result := outboundAssetRefsToMessageRefs(refs)
	if len(result) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(result))
	}
	if result[0].ContentHash != "a1" || result[0].Role != "attachment" {
		t.Fatalf("unexpected ref[0]: %+v", result[0])
	}
	if result[1].ContentHash != "a2" || result[1].Role != "attachment" {
		t.Fatalf("unexpected ref[1]: %+v", result[1])
	}
}

func TestOutboundAssetRefsToMessageRefs_Empty(t *testing.T) {
	t.Parallel()
	result := outboundAssetRefsToMessageRefs(nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestNormalizeImagePartsToDataURL_ConvertsIndexedObject(t *testing.T) {
	msg := conversation.ModelMessage{
		Role: "user",
		Content: json.RawMessage(`[
			{"type":"text","text":"hello"},
			{"type":"image","image":{"0":82,"1":73,"2":70,"3":70},"mediaType":"image/webp"}
		]`),
	}

	normalized, changed := normalizeImagePartsToDataURL(msg)
	if !changed {
		t.Fatal("expected message to be normalized")
	}

	var parts []map[string]any
	if err := json.Unmarshal(normalized.Content, &parts); err != nil {
		t.Fatalf("failed to unmarshal normalized content: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	image, ok := parts[1]["image"].(string)
	if !ok {
		t.Fatalf("expected image to be string data url, got %T", parts[1]["image"])
	}
	expected := "data:image/webp;base64," + base64.StdEncoding.EncodeToString([]byte{82, 73, 70, 70})
	if image != expected {
		t.Fatalf("unexpected data url, got %q", image)
	}
}

func TestNormalizeImagePartsToDataURL_LeavesStringImageUntouched(t *testing.T) {
	original := `[
		{"type":"image","image":"data:image/png;base64,AAAA","mediaType":"image/png"}
	]`
	msg := conversation.ModelMessage{
		Role:    "user",
		Content: json.RawMessage(original),
	}

	normalized, changed := normalizeImagePartsToDataURL(msg)
	if changed {
		t.Fatal("expected no normalization for string image")
	}
	if string(normalized.Content) != original {
		t.Fatalf("expected content unchanged, got %s", string(normalized.Content))
	}
}
