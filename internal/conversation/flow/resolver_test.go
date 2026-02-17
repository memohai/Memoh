package flow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
)

func TestPostTriggerSchedule_Endpoint(t *testing.T) {
	var capturedPath string
	var capturedBody []byte
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)
		resp := gatewayResponse{
			Messages: []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("ok")}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	resolver := &Resolver{
		gatewayBaseURL: srv.URL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		logger:         slog.Default(),
	}

	maxCalls := 5
	req := triggerScheduleRequest{
		gatewayRequest: gatewayRequest{
			Model: gatewayModelConfig{
				ModelID:    "gpt-4",
				ClientType: "openai",
				APIKey:     "sk-test",
				BaseURL:    "https://api.openai.com",
			},
			ActiveContextTime: 1440,
			Channels:          []string{},
			Messages:          []conversation.ModelMessage{},
			Skills:            []string{},
			Identity: gatewayIdentity{
				BotID:             "bot-123",
				ContainerID:       "mcp-bot-123",
				ChannelIdentityID: "owner-user-1",
				DisplayName:       "Scheduler",
			},
			Attachments: []any{},
		},
		Schedule: gatewaySchedule{
			ID:          "sched-1",
			Name:        "daily report",
			Description: "generate daily report",
			Pattern:     "0 9 * * *",
			MaxCalls:    &maxCalls,
			Command:     "generate the daily report",
		},
	}

	resp, err := resolver.postTriggerSchedule(context.Background(), req, "Bearer test-token")
	if err != nil {
		t.Fatalf("postTriggerSchedule returned error: %v", err)
	}

	if capturedPath != "/chat/trigger-schedule" {
		t.Errorf("expected path /chat/trigger-schedule, got %s", capturedPath)
	}
	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Authorization header 'Bearer test-token', got %s", capturedAuth)
	}
	if len(resp.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(resp.Messages))
	}

	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}
	schedule, ok := body["schedule"].(map[string]any)
	if !ok {
		t.Fatal("expected 'schedule' field in request body")
	}
	if schedule["id"] != "sched-1" {
		t.Errorf("expected schedule.id=sched-1, got %v", schedule["id"])
	}
	if schedule["command"] != "generate the daily report" {
		t.Errorf("expected schedule.command, got %v", schedule["command"])
	}
	if _, hasQuery := body["query"]; hasQuery {
		t.Error("trigger-schedule request should not contain 'query' field")
	}
}

func TestPostTriggerSchedule_NoAuth(t *testing.T) {
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		resp := gatewayResponse{Messages: []conversation.ModelMessage{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	resolver := &Resolver{
		gatewayBaseURL: srv.URL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		logger:         slog.Default(),
	}

	req := triggerScheduleRequest{
		gatewayRequest: gatewayRequest{
			Channels:    []string{},
			Messages:    []conversation.ModelMessage{},
			Skills:      []string{},
			Attachments: []any{},
		},
		Schedule: gatewaySchedule{ID: "s1", Command: "test"},
	}

	_, err := resolver.postTriggerSchedule(context.Background(), req, "")
	if err != nil {
		t.Fatalf("postTriggerSchedule returned error: %v", err)
	}
	if capturedAuth != "" {
		t.Errorf("expected no Authorization header, got %s", capturedAuth)
	}
}

func TestPostTriggerSchedule_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	resolver := &Resolver{
		gatewayBaseURL: srv.URL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		logger:         slog.Default(),
	}

	req := triggerScheduleRequest{
		gatewayRequest: gatewayRequest{
			Channels:    []string{},
			Messages:    []conversation.ModelMessage{},
			Skills:      []string{},
			Attachments: []any{},
		},
		Schedule: gatewaySchedule{ID: "s1", Command: "test"},
	}

	_, err := resolver.postTriggerSchedule(context.Background(), req, "Bearer tok")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

type fakeGatewayAssetLoader struct {
	openFn func(ctx context.Context, assetID string) (io.ReadCloser, string, string, error)
}

func (f *fakeGatewayAssetLoader) OpenForGateway(ctx context.Context, assetID string) (io.ReadCloser, string, string, error) {
	if f == nil || f.openFn == nil {
		return nil, "", "", io.EOF
	}
	return f.openFn(ctx, assetID)
}

func TestPrepareGatewayAttachments_InlineAssetToBase64(t *testing.T) {
	resolver := &Resolver{
		logger: slog.Default(),
		assetLoader: &fakeGatewayAssetLoader{
			openFn: func(ctx context.Context, assetID string) (io.ReadCloser, string, string, error) {
				if assetID != "asset-1" {
					t.Fatalf("unexpected asset id: %s", assetID)
				}
				return io.NopCloser(strings.NewReader("image-binary")), "bot-1", "image/png", nil
			},
		},
	}
	req := conversation.ChatRequest{
		BotID: "bot-1",
		Attachments: []conversation.ChatAttachment{
			{
				Type:    "image",
				AssetID: "asset-1",
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
			InputModalities: []string{models.ModelInputText, models.ModelInputImage},
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
			openFn: func(ctx context.Context, assetID string) (io.ReadCloser, string, string, error) {
				return io.NopCloser(bytes.NewReader(jpegBytes)), "bot-1", "application/octet-stream", nil
			},
		},
	}
	req := conversation.ChatRequest{
		BotID: "bot-1",
		Attachments: []conversation.ChatAttachment{
			{
				Type:    "image",
				AssetID: "asset-2",
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
			InputModalities: []string{models.ModelInputText, models.ModelInputVideo},
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
