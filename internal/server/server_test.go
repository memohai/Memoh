package server

import "testing"

func TestShouldSkipJWT_FeishuWebhookPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/channels/feishu/webhook/cfg-1", want: true},
		{path: "/channels/feishu/webhook", want: false},
		{path: "/api/channels/feishu/webhook", want: false},
	}

	for _, tc := range cases {
		got := shouldSkipJWT(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}
