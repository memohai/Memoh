package tools

import "testing"

func TestMemohHelpEmbeddedCorpusHits(t *testing.T) {
	provider := NewMemohHelpProvider(nil)
	if len(provider.chunks) == 0 {
		t.Fatal("expected embedded help corpus chunks")
	}

	cases := []struct {
		name     string
		query    string
		wantPath string
	}{
		{
			name:     "telegram channel setup",
			query:    "Memoh 配置 Telegram channel Telegram bot setup",
			wantPath: "en/channels/telegram.md",
		},
		{
			name:     "desktop install",
			query:    "Memoh Desktop 安装 install desktop app",
			wantPath: "zh/installation/desktop.md",
		},
		{
			name:     "mcp connection",
			query:    "MCP connection 是什么 Memoh MCP",
			wantPath: "en/getting-started/mcp.md",
		},
		{
			name:     "skills management",
			query:    "Memoh skills manage create edit install skills",
			wantPath: "en/getting-started/skills.md",
		},
		{
			name:     "compaction",
			query:    "Memoh compaction memory compaction 是什么",
			wantPath: "en/getting-started/compaction.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results := searchMemohHelpIndex(provider.index, tc.query, 5)
			if len(results) == 0 {
				t.Fatalf("expected results for query %q", tc.query)
			}
			if got := results[0]["path"]; got != tc.wantPath {
				t.Fatalf("top result path = %v, want %q; results=%v", got, tc.wantPath, results)
			}
		})
	}
}
