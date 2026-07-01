package server

import (
	neturl "net/url"
	"testing"
)

func TestShouldSkipJWT_PublicUnauthenticatedPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/channels/line/webhook/cfg-1", want: true},
		{path: "/channels/line/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/preview.jpg", want: true},
		{path: "/channels/line/public/media/bot-1/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/metadata", want: false},
		{path: "/channels/line/webhook", want: false},
		{path: "/api/channels/line/webhook", want: false},
		{path: "/oauth/mcp/callback", want: true},
		{path: "/api/oauth/mcp/callback", want: true},
	}

	for _, tc := range cases {
		got := shouldSkipJWT(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
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
