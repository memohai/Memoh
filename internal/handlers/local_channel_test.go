package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/media"
	sessionpkg "github.com/memohai/memoh/internal/session"
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

func TestWSWriterIgnoresLateSendsAfterClose(t *testing.T) {
	t.Parallel()

	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.Close() }()
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("panic: %v", recovered)
			}
		}()

		writer := newWSWriter(conn)
		writer.Close()
		writer.Send([]byte(`{"type":"late"}`))
		writer.SendJSON(map[string]string{"type": "late"})
		writer.Close()
		done <- nil
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	_ = client.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket writer")
	}
}

func TestWSStreamRegistry_AbortsOnlyTargetStream(t *testing.T) {
	t.Parallel()

	registry := newWSStreamRegistry()
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	abortA := make(chan struct{}, 1)
	abortB := make(chan struct{}, 1)

	if err := registry.register(&activeWSStream{streamID: "stream-a", cancel: cancelA, abortCh: abortA}); err != nil {
		t.Fatalf("register stream-a: %v", err)
	}
	if err := registry.register(&activeWSStream{streamID: "stream-b", cancel: cancelB, abortCh: abortB}); err != nil {
		t.Fatalf("register stream-b: %v", err)
	}

	if !registry.abort("stream-a") {
		t.Fatal("expected stream-a abort to succeed")
	}

	select {
	case <-abortA:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream-a abort signal")
	}
	select {
	case <-ctxA.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream-a context cancellation")
	}
	select {
	case <-abortB:
		t.Fatal("stream-b received abort signal")
	default:
	}
	select {
	case <-ctxB.Done():
		t.Fatal("stream-b context was cancelled")
	default:
	}
}

func TestWSStreamRegistry_RejectsDuplicateStreamID(t *testing.T) {
	t.Parallel()

	registry := newWSStreamRegistry()
	_, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	_, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	if err := registry.register(&activeWSStream{streamID: "stream-a", cancel: cancelA, abortCh: make(chan struct{}, 1)}); err != nil {
		t.Fatalf("register stream-a: %v", err)
	}
	if err := registry.register(&activeWSStream{streamID: "stream-a", cancel: cancelB, abortCh: make(chan struct{}, 1)}); err == nil {
		t.Fatal("expected duplicate stream id registration to fail")
	}
}

func TestLocalChannelHandlerIssueRuntimeOwnerBearerToken(t *testing.T) {
	t.Parallel()

	const secret = "local-ws-runtime-secret" //nolint:gosec // G101 false positive: test fixture, not a real credential.
	handler := &LocalChannelHandler{logger: slog.Default()}
	handler.SetAuthTokenConfig(secret, time.Hour)

	got := handler.issueRuntimeOwnerBearerToken("runtime-owner", "Bearer caller-token")
	raw := strings.TrimSpace(strings.TrimPrefix(got, "Bearer "))
	parsed, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			t.Fatalf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("parse runtime token: token valid=%v err=%v", parsed != nil && parsed.Valid, err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims type = %T, want jwt.MapClaims", parsed.Claims)
	}
	if gotSub := claims["sub"]; gotSub != "runtime-owner" {
		t.Fatalf("runtime token subject = %v, want runtime-owner", gotSub)
	}

	handler.SetAuthTokenConfig("", time.Hour)
	if fallback := handler.issueRuntimeOwnerBearerToken("runtime-owner", "Bearer caller-token"); fallback != "Bearer caller-token" {
		t.Fatalf("fallback token = %q, want original bearer", fallback)
	}
}

func TestWSStreamRegistry_HasSessionTracksActiveStreams(t *testing.T) {
	t.Parallel()

	registry := newWSStreamRegistry()
	_, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	_, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	if registry.hasSession("session-1") {
		t.Fatal("empty registry reported active session")
	}
	if err := registry.register(&activeWSStream{streamID: "stream-a", sessionID: " session-1 ", cancel: cancelA, abortCh: make(chan struct{}, 1)}); err != nil {
		t.Fatalf("register stream-a: %v", err)
	}
	if err := registry.register(&activeWSStream{streamID: "stream-b", sessionID: "session-2", cancel: cancelB, abortCh: make(chan struct{}, 1)}); err != nil {
		t.Fatalf("register stream-b: %v", err)
	}

	if !registry.hasSession("session-1") {
		t.Fatal("expected session-1 to be active")
	}
	if !registry.hasSession(" session-2 ") {
		t.Fatal("expected trimmed session-2 to be active")
	}
	if registry.hasSession("session-3") {
		t.Fatal("unexpected session-3 activity")
	}

	registry.finish("stream-a")
	if registry.hasSession("session-1") {
		t.Fatal("session-1 should be inactive after finish")
	}
	if !registry.hasSession("session-2") {
		t.Fatal("session-2 should remain active")
	}
}

func TestCanOpenLocalWebSocketAllowsWorkspaceOrManage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		perms []string
		want  bool
	}{
		{name: "workspace exec", perms: []string{bots.PermissionWorkspaceExec}, want: true},
		{name: "manage", perms: []string{bots.PermissionManage}, want: true},
		{name: "chat only", perms: []string{bots.PermissionChat}, want: false},
		{name: "none", perms: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := canOpenLocalWebSocket(tt.perms); got != tt.want {
				t.Fatalf("canOpenLocalWebSocket(%v) = %v, want %v", tt.perms, got, tt.want)
			}
		})
	}
}

type localChannelSessionAuthQueries struct {
	dbstore.Queries
	bot     sqlc.GetBotByIDRow
	session sqlc.BotSession
	grants  []sqlc.ListBotUserGrantsForUserRow
}

func (q localChannelSessionAuthQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q localChannelSessionAuthQueries) GetSessionByID(_ context.Context, _ pgtype.UUID) (sqlc.BotSession, error) {
	return q.session, nil
}

func (q localChannelSessionAuthQueries) ListBotUserGrantsForUser(_ context.Context, _ sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
	return q.grants, nil
}

func TestLocalChannelAuthorizeWSSessionScopesChatToCreator(t *testing.T) {
	t.Parallel()

	const (
		botID       = "11111111-1111-1111-1111-111111111111"
		sessionID   = "22222222-2222-2222-2222-222222222222"
		currentUser = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		otherUser   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	queries := localChannelSessionAuthQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:              testUUID(sessionID),
			BotID:           testUUID(botID),
			Type:            sessionpkg.TypeChat,
			CreatedByUserID: testUUID(otherUser),
			Metadata:        []byte(`{}`),
		},
		grants: []sqlc.ListBotUserGrantsForUserRow{
			{
				ID:          testUUID("dddddddd-dddd-dddd-dddd-dddddddddddd"),
				BotID:       testUUID(botID),
				SubjectType: bots.GrantSubjectUser,
				UserID:      testUUID(currentUser),
				Permissions: []byte(`["chat"]`),
			},
		},
	}
	handler := &LocalChannelHandler{
		botService:     bots.NewService(nil, queries),
		accountService: accounts.NewService(nil, testAdminAccountStore{role: "user"}),
		sessionService: sessionpkg.NewService(nil, queries, nil),
	}

	err := handler.authorizeWSSession(testEchoContext(currentUser), currentUser, botID, sessionID)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
		t.Fatalf("authorizeWSSession() error = %v, want HTTP 404", err)
	}
}

func TestLocalChannelAuthorizeWSSessionAllowsManageAccess(t *testing.T) {
	t.Parallel()

	const (
		botID       = "11111111-1111-1111-1111-111111111111"
		sessionID   = "22222222-2222-2222-2222-222222222222"
		currentUser = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		otherUser   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	queries := localChannelSessionAuthQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:              testUUID(sessionID),
			BotID:           testUUID(botID),
			Type:            sessionpkg.TypeChat,
			CreatedByUserID: testUUID(otherUser),
			Metadata:        []byte(`{}`),
		},
		grants: []sqlc.ListBotUserGrantsForUserRow{
			{
				ID:          testUUID("dddddddd-dddd-dddd-dddd-dddddddddddd"),
				BotID:       testUUID(botID),
				SubjectType: bots.GrantSubjectUser,
				UserID:      testUUID(currentUser),
				Permissions: []byte(`["manage"]`),
			},
		},
	}
	handler := &LocalChannelHandler{
		botService:     bots.NewService(nil, queries),
		accountService: accounts.NewService(nil, testAdminAccountStore{role: "user"}),
		sessionService: sessionpkg.NewService(nil, queries, nil),
	}

	if err := handler.authorizeWSSession(testEchoContext(currentUser), currentUser, botID, sessionID); err != nil {
		t.Fatalf("authorizeWSSession() error = %v, want nil", err)
	}
}

func testEchoContext(userID string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()
	return testAuthContext(e, req, rec, userID)
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
