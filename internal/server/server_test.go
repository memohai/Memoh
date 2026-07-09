package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/teams"
)

func TestShouldSkipJWT_ChannelWebhookPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/channels/feishu/webhook/cfg-1", want: true},
		{path: "/channels/wechatoa/webhook/cfg-1", want: true},
		{path: "/channels/line/webhook/cfg-1", want: true},
		{path: "/channels/line/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/preview.jpg", want: true},
		{path: "/channels/line/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/metadata", want: false},
		{path: "/channels/telegram/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/preview.jpg", want: true},
		{path: "/channels/feishu/webhook", want: false},
		{path: "/api/channels/feishu/webhook", want: false},
		{path: "/webhook-tunnel/status", want: false},
	}

	for _, tc := range cases {
		got := shouldSkipJWT(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}

func TestNewServerInjectsDefaultTeamScope(t *testing.T) {
	t.Parallel()

	handler := &teamProbeHandler{}
	srv := NewServer(slog.New(slog.DiscardHandler), ":0", "", nil, handler)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if handler.seen.TeamID != teams.DefaultTeamID {
		t.Fatalf("team id = %q, want %q", handler.seen.TeamID, teams.DefaultTeamID)
	}
}

type teamProbeHandler struct {
	seen teams.Scope
}

func (h *teamProbeHandler) Register(e *echo.Echo) {
	e.GET("/ping", func(c echo.Context) error {
		scope, err := teams.ScopeFromContext(c.Request().Context())
		if err != nil {
			return err
		}
		h.seen = scope
		return c.NoContent(http.StatusNoContent)
	})
}

func TestShouldLimitPublicRequestBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/channels/line/webhook/cfg-1", want: true},
		{path: "/channels/telegram/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/preview.jpg", want: false},
		{path: "/api/fs/upload", want: false},
		{path: "/api/bots/backup/import", want: false},
	}

	for _, tc := range cases {
		got := shouldLimitPublicRequestBody(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}

func TestSafeRequestLogURIStripsPublicMediaQuery(t *testing.T) {
	t.Parallel()

	u, err := neturl.Parse("/channels/line/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/preview.jpg?exp=123&sig=secret")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	got := safeRequestLogURI(u, u.RequestURI())
	want := "/channels/line/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/preview.jpg"
	if got != want {
		t.Fatalf("safeRequestLogURI = %q, want %q", got, want)
	}
}

func TestShouldSkipJWT_MCPOAuthCallbackPaths(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/oauth/mcp/callback", "/api/oauth/mcp/callback"} {
		if !shouldSkipJWT(path) {
			t.Fatalf("path=%q should skip jwt", path)
		}
	}
}
