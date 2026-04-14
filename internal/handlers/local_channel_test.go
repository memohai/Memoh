package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/storage"
)

func TestFormatLocalStreamEvent_UsesChannelEventShape(t *testing.T) {
	t.Parallel()

	data, err := formatLocalStreamEvent(channel.StreamEvent{
		Type:  channel.StreamEventDelta,
		Delta: "hello",
		Phase: channel.StreamPhaseText,
	})
	if err != nil {
		t.Fatalf("format local stream event failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if got := payload["type"]; got != "delta" {
		t.Fatalf("expected type delta, got %#v", got)
	}
	if got := payload["delta"]; got != "hello" {
		t.Fatalf("expected delta hello, got %#v", got)
	}
	if got := payload["phase"]; got != "text" {
		t.Fatalf("expected phase text, got %#v", got)
	}
	if _, ok := payload["target"]; ok {
		t.Fatalf("unexpected wrapper field target in payload")
	}
	if _, ok := payload["event"]; ok {
		t.Fatalf("unexpected wrapper field event in payload")
	}
}

func TestFormatLocalStreamEvent_EncodesToolCallAsToolCallObject(t *testing.T) {
	t.Parallel()

	data, err := formatLocalStreamEvent(channel.StreamEvent{
		Type: channel.StreamEventToolCallStart,
		ToolCall: &channel.StreamToolCall{
			Name:   "exec",
			CallID: "call-1",
			Input: map[string]any{
				"command": "pwd",
			},
		},
	})
	if err != nil {
		t.Fatalf("format local stream event failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	toolCall, ok := payload["tool_call"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_call object, got %#v", payload["tool_call"])
	}
	if got := toolCall["name"]; got != "exec" {
		t.Fatalf("expected tool_call.name exec, got %#v", got)
	}
	if got := toolCall["call_id"]; got != "call-1" {
		t.Fatalf("expected tool_call.call_id call-1, got %#v", got)
	}
	if _, ok := payload["toolName"]; ok {
		t.Fatalf("unexpected camelCase toolName in payload")
	}
}

func TestWSIngestAttachments_RewritesContainerPathToAssetRef(t *testing.T) {
	t.Parallel()

	handler, provider := newLocalChannelHandlerWithMedia()
	provider.SeedContainerFile("bot-1", "/data/images/demo.png", []byte("image-bytes"))

	original, err := json.Marshal(map[string]any{
		"type": "attachment_delta",
		"attachments": []any{
			map[string]any{
				"type": "image",
				"path": "/data/images/demo.png",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal original event: %v", err)
	}

	processed := handler.wsIngestAttachments(context.Background(), "bot-1", original)
	if len(processed) != 1 {
		t.Fatalf("expected 1 processed event, got %d", len(processed))
	}

	var envelope struct {
		Type        string           `json:"type"`
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal(processed[0], &envelope); err != nil {
		t.Fatalf("unmarshal processed event: %v", err)
	}
	if envelope.Type != "attachment_delta" {
		t.Fatalf("unexpected event type: %q", envelope.Type)
	}
	if len(envelope.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(envelope.Attachments))
	}

	att := envelope.Attachments[0]
	if got, _ := att["content_hash"].(string); got == "" {
		t.Fatalf("expected content_hash after ingestion, got %#v", att["content_hash"])
	}
	if got, _ := att["name"].(string); got != "demo.png" {
		t.Fatalf("expected inferred name demo.png, got %#v", att["name"])
	}
	meta, ok := att["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", att["metadata"])
	}
	if meta["source_path"] != "/data/images/demo.png" {
		t.Fatalf("expected source_path preserved, got %#v", meta["source_path"])
	}
	if meta["storage_key"] == nil || meta["storage_key"] == "" {
		t.Fatalf("expected storage_key metadata, got %#v", meta["storage_key"])
	}
	if got, _ := att["url"].(string); got != "" {
		t.Fatalf("expected empty url after asset rewrite, got %#v", att["url"])
	}
}

func TestBuildTtsAttachment_FallbackKeepsDataURLInBase64Field(t *testing.T) {
	t.Parallel()

	handler := &LocalChannelHandler{logger: slog.Default()}
	att := handler.buildTtsAttachment(context.Background(), "bot-1", "audio/mpeg", []byte("audio"))

	if got, _ := att["type"].(string); got != "voice" {
		t.Fatalf("expected voice attachment type, got %#v", att["type"])
	}
	if got, _ := att["base64"].(string); got == "" {
		t.Fatalf("expected fallback data URL in base64 field, got %#v", att["base64"])
	}
	if got, _ := att["url"].(string); got != "" {
		t.Fatalf("expected empty url field in fallback attachment, got %#v", att["url"])
	}
}

func TestExtractAssetRefsFromProcessedEvent_UsesBundleParsing(t *testing.T) {
	t.Parallel()

	event, err := json.Marshal(map[string]any{
		"type": "attachment_delta",
		"attachments": []any{
			map[string]any{
				"type":         "image",
				"content_hash": "asset-1",
				"mime":         "image/png",
				"size":         42,
				"metadata": map[string]any{
					"name":        "demo.png",
					"storage_key": "aa/asset-1.png",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	refs := extractAssetRefsFromProcessedEvent(event)
	if len(refs) != 1 {
		t.Fatalf("expected 1 asset ref, got %d", len(refs))
	}
	if refs[0].ContentHash != "asset-1" {
		t.Fatalf("unexpected content hash: %q", refs[0].ContentHash)
	}
	if refs[0].Name != "demo.png" {
		t.Fatalf("expected metadata name fallback, got %q", refs[0].Name)
	}
	if refs[0].StorageKey != "aa/asset-1.png" {
		t.Fatalf("unexpected storage key: %q", refs[0].StorageKey)
	}
	if refs[0].SizeBytes != 42 {
		t.Fatalf("unexpected size bytes: %d", refs[0].SizeBytes)
	}
}

func TestParseWSClientAttachments_NormalizesToolStyleInputs(t *testing.T) {
	t.Parallel()

	rawAttachments := []json.RawMessage{
		json.RawMessage(`"screenshot.png"`),
		json.RawMessage(`{"url":"data:image/png;base64,AAAA","type":"image"}`),
	}
	got := parseWSClientAttachments(rawAttachments)
	if len(got) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(got))
	}
	if got[0].Path != "/data/screenshot.png" {
		t.Fatalf("expected bare path normalized to /data prefix, got %q", got[0].Path)
	}
	if got[1].Base64 != "data:image/png;base64,AAAA" {
		t.Fatalf("expected inline data preserved in base64 field, got %q", got[1].Base64)
	}
	if got[1].Mime != "image/png" {
		t.Fatalf("expected inferred image/png mime, got %q", got[1].Mime)
	}
}

func TestApplyBundleToItemMap_UsesMergedBundleFields(t *testing.T) {
	t.Parallel()

	item := map[string]any{
		"legacy": "keep",
		"url":    "stale",
	}
	got := applyBundleToItemMap(item, attachmentpkg.Bundle{
		Type:        "image",
		ContentHash: "asset-1",
		Mime:        "image/png",
		Metadata: map[string]any{
			attachmentpkg.MetadataKeyStorageKey: "aa/asset-1.png",
		},
	})
	if got["legacy"] != "keep" {
		t.Fatalf("expected unknown fields preserved, got %#v", got["legacy"])
	}
	if got["content_hash"] != "asset-1" {
		t.Fatalf("expected content_hash updated, got %#v", got["content_hash"])
	}
	if url, ok := got["url"]; ok && url != "" {
		t.Fatalf("expected url absent or empty after bundle merge, got %#v", url)
	}
}

type localChannelMemoryProvider struct {
	mu             sync.Mutex
	objects        map[string][]byte
	containerFiles map[string][]byte
}

func (p *localChannelMemoryProvider) Put(ctx context.Context, key string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.objects[key] = append([]byte(nil), data...)
	_ = ctx
	return nil
}

func (p *localChannelMemoryProvider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, ok := p.objects[key]
	if !ok {
		return nil, io.EOF
	}
	_ = ctx
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (*localChannelMemoryProvider) Delete(context.Context, string) error { return nil }

func (*localChannelMemoryProvider) AccessPath(key string) string {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return "/data/media/" + key
	}
	return "/data/media/" + parts[1]
}

func (p *localChannelMemoryProvider) OpenContainerFile(ctx context.Context, botID, containerPath string) (io.ReadCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, ok := p.containerFiles[botID+":"+containerPath]
	if !ok {
		return nil, io.EOF
	}
	_ = ctx
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (p *localChannelMemoryProvider) SeedContainerFile(botID, containerPath string, data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.containerFiles[botID+":"+containerPath] = append([]byte(nil), data...)
}

var (
	_ storage.Provider            = (*localChannelMemoryProvider)(nil)
	_ storage.ContainerFileOpener = (*localChannelMemoryProvider)(nil)
)

func newLocalChannelHandlerWithMedia() (*LocalChannelHandler, *localChannelMemoryProvider) {
	provider := &localChannelMemoryProvider{
		objects:        make(map[string][]byte),
		containerFiles: make(map[string][]byte),
	}
	handler := &LocalChannelHandler{logger: slog.Default()}
	handler.SetMediaService(media.NewService(slog.Default(), provider))
	return handler, provider
}
