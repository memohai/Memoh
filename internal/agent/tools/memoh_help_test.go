package tools

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestMemohHelpToolSchema(t *testing.T) {
	provider := NewMemohHelpProvider(nil)
	tool := mustMemohHelpTool(t, provider)
	if tool.Name != "memoh_help" {
		t.Fatalf("tool name = %q, want memoh_help", tool.Name)
	}

	params, ok := tool.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]any", tool.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map[string]any", params["properties"])
	}
	if _, ok := props["query"]; !ok {
		t.Fatal("schema missing query property")
	}
	if _, ok := props["limit"]; !ok {
		t.Fatal("schema missing limit property")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("required type = %T, want []string", params["required"])
	}
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("required = %#v, want [query]", required)
	}
}

func TestMemohHelpRequiresQuery(t *testing.T) {
	provider := NewMemohHelpProvider(nil)
	tool := mustMemohHelpTool(t, provider)

	_, err := tool.Execute(nil, map[string]any{"query": "  "})
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("error = %v, want query required", err)
	}
}

func TestMemohHelpLimitDefaultAndMax(t *testing.T) {
	provider := NewMemohHelpProvider(nil)
	tool := mustMemohHelpTool(t, provider)

	result, err := tool.Execute(nil, map[string]any{"query": "Memoh"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(memohHelpResults(t, result)); got > memohHelpDefaultLimit {
		t.Fatalf("default result count = %d, want <= %d", got, memohHelpDefaultLimit)
	}

	result, err = tool.Execute(nil, map[string]any{"query": "Memoh", "limit": 50})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(memohHelpResults(t, result)); got > memohHelpMaxLimit {
		t.Fatalf("max result count = %d, want <= %d", got, memohHelpMaxLimit)
	}
}

func mustMemohHelpTool(t *testing.T, provider *MemohHelpProvider) sdk.Tool {
	t.Helper()
	toolset, err := provider.Tools(context.Background(), SessionContext{})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}
	if len(toolset) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolset))
	}
	return toolset[0]
}

func memohHelpResults(t *testing.T, result any) []map[string]any {
	t.Helper()
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	rawResults, ok := payload["results"].([]map[string]any)
	if !ok {
		t.Fatalf("results type = %T, want []map[string]any", payload["results"])
	}
	return rawResults
}

func TestMemohHelpLimitBoundaries(t *testing.T) {
	provider := NewMemohHelpProvider(nil)
	tool := mustMemohHelpTool(t, provider)

	for _, lim := range []int{-5, 0, 1} {
		result, err := tool.Execute(nil, map[string]any{"query": "Memoh", "limit": lim})
		if err != nil {
			t.Fatalf("limit=%d Execute error: %v", lim, err)
		}
		got := len(memohHelpResults(t, result))
		if got > 1 {
			t.Fatalf("limit=%d returned %d results, want <= 1", lim, got)
		}
	}
}

func TestMemohHelpTokens(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "empty", text: "", want: nil},
		{name: "ascii words", text: "Hello World", want: []string{"hello", "world"}},
		{name: "single han", text: "桌", want: []string{"桌"}},
		{name: "han pair", text: "桌面", want: []string{"桌面"}},
		{name: "han run", text: "怎么配置", want: []string{"怎么", "么配", "配置"}},
		{name: "han with punct", text: "怎么，配置", want: []string{"怎么", "配置"}},
		{name: "mixed inline", text: "ab怎么cd", want: []string{"ab", "cd", "怎么"}},
		{name: "mixed spaced", text: "Telegram 怎么配置", want: []string{"telegram", "怎么", "么配", "配置"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := memohHelpTokens(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("memohHelpTokens(%q) = %#v, want %#v", tc.text, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("memohHelpTokens(%q)[%d] = %q, want %q (full %#v)", tc.text, i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestMemohHelpTermsDoesNotEmitCommonSingleChars(t *testing.T) {
	got := memohHelpTokens("怎么配置桌面数据库")
	for _, term := range got {
		if utf8.RuneCountInString(term) == 1 {
			t.Fatalf("multi-rune Han query produced single-char term %q (terms=%v); expected only bigrams", term, got)
		}
	}
}

func TestSplitMemohHelpDoc(t *testing.T) {
	cases := []struct {
		name           string
		path           string
		body           string
		wantLen        int
		wantFirstTitle string
		wantRelPath    string
	}{
		{name: "empty", path: "docs/getting-started/x.md", body: "", wantLen: 0, wantRelPath: "en/getting-started/x.md"},
		{name: "no heading", path: "docs/getting-started/x.md", body: "just a body line\nanother line", wantLen: 1, wantFirstTitle: "en/getting-started/x.md", wantRelPath: "en/getting-started/x.md"},
		{name: "single heading no body", path: "docs/getting-started/x.md", body: "# Heading Only", wantLen: 0, wantRelPath: "en/getting-started/x.md"},
		{name: "heading then body", path: "docs/getting-started/x.md", body: "# Heading\nbody line", wantLen: 1, wantFirstTitle: "Heading", wantRelPath: "en/getting-started/x.md"},
		{name: "multiple headings", path: "docs/getting-started/x.md", body: "# H1\nb1\n# H2\nb2", wantLen: 2, wantFirstTitle: "H1", wantRelPath: "en/getting-started/x.md"},
		{name: "crlf line endings", path: "docs/getting-started/x.md", body: "# H1\r\nbody\r\n", wantLen: 1, wantFirstTitle: "H1", wantRelPath: "en/getting-started/x.md"},
		{name: "english path", path: "docs/channels/telegram.md", body: "# Telegram\nuse a bot token", wantLen: 1, wantFirstTitle: "Telegram", wantRelPath: "en/channels/telegram.md"},
		{name: "chinese path", path: "docs/zh/channels/telegram.md", body: "# Telegram\n使用 bot token", wantLen: 1, wantFirstTitle: "Telegram", wantRelPath: "zh/channels/telegram.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := splitMemohHelpDoc(tc.path, tc.body)
			if len(chunks) != tc.wantLen {
				t.Fatalf("len=%d want %d; chunks=%+v", len(chunks), tc.wantLen, chunks)
			}
			if tc.wantLen > 0 && chunks[0].Title != tc.wantFirstTitle {
				t.Fatalf("first title=%q want %q", chunks[0].Title, tc.wantFirstTitle)
			}
			for _, c := range chunks {
				if c.Path != tc.wantRelPath {
					t.Fatalf("chunk path=%q want %q", c.Path, tc.wantRelPath)
				}
			}
		})
	}
}

func TestSplitMemohHelpDocIgnoresHeadingsInsideFences(t *testing.T) {
	body := strings.Join([]string{
		"# Skills",
		"before",
		"```yaml",
		"---",
		"name: coder-skill",
		"---",
		"# Coder Skill",
		"content in sample",
		"```",
		"after",
		"## Next",
		"next body",
		"~~~toml",
		"# comment in toml",
		"host = \"\"",
		"~~~",
	}, "\n")

	chunks := splitMemohHelpDoc("docs/getting-started/skills.md", body)
	if len(chunks) != 2 {
		t.Fatalf("len=%d want 2; chunks=%+v", len(chunks), chunks)
	}
	if chunks[0].Title != "Skills" {
		t.Fatalf("first title=%q want Skills", chunks[0].Title)
	}
	if !strings.Contains(chunks[0].Text, "# Coder Skill") {
		t.Fatalf("first chunk text lost fenced heading: %q", chunks[0].Text)
	}
	if chunks[1].Title != "Next" {
		t.Fatalf("second title=%q want Next", chunks[1].Title)
	}
	if !strings.Contains(chunks[1].Text, "# comment in toml") {
		t.Fatalf("second chunk text lost fenced comment: %q", chunks[1].Text)
	}
}

func TestStripVitePressFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "none", in: "# Title\nbody", want: "# Title\nbody"},
		{name: "lf block", in: "---\ntitle: Foo\nlayout: home\n---\n# Title\nbody", want: "# Title\nbody"},
		{name: "crlf block", in: "---\r\ntitle: Foo\r\n---\r\n# Title", want: "# Title"},
		{name: "empty frontmatter", in: "---\n---\nbody", want: "body"},
		{name: "horizontal rule not frontmatter", in: "# Title\n\n---\n\nsection", want: "# Title\n\n---\n\nsection"},
		{name: "looks like dashes but not closed", in: "---\ntitle: Foo\nbody never closes", want: "---\ntitle: Foo\nbody never closes"},
		{name: "trailing whitespace closer", in: "---\nkey: value\n---  \nbody", want: "body"},
		{name: "frontmatter then horizontal rule body", in: "---\nkey: value\n---\n# H\n---\nmore", want: "# H\n---\nmore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripVitePressFrontmatter(tc.in)
			if got != tc.want {
				t.Fatalf("stripVitePressFrontmatter(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSearchMemohHelpChunksChineseQuery(t *testing.T) {
	chunks := []memohHelpChunk{
		{Path: "a.md", Title: "桌面模式", Text: "桌面客户端使用 SQLite 本地数据库。"},
		{Path: "b.md", Title: "频道", Text: "支持 Telegram 等聊天频道，可以绑定到 bot。"},
		{Path: "c.md", Title: "记忆", Text: "通过 Qdrant 做语义检索，支持长期记忆。"},
	}

	results := searchMemohHelpChunks(chunks, "桌面数据库", 3)
	if len(results) == 0 {
		t.Fatalf("expected at least one result for 桌面数据库, got none")
	}
	if results[0]["path"] != "a.md" {
		t.Fatalf("expected a.md ranked first for 桌面数据库, got %v", results)
	}

	results = searchMemohHelpChunks(chunks, "记忆检索", 3)
	if len(results) == 0 || results[0]["path"] != "c.md" {
		t.Fatalf("expected c.md ranked first for 记忆检索, got %v", results)
	}

	results = searchMemohHelpChunks(chunks, "聊天频道", 3)
	if len(results) == 0 || results[0]["path"] != "b.md" {
		t.Fatalf("expected b.md ranked first for 聊天频道, got %v", results)
	}
}

func TestSearchMemohHelpBM25RanksRareTermsAboveCommonPhrases(t *testing.T) {
	chunks := []memohHelpChunk{
		{
			Path:  "zh/getting-started/memory.md",
			Title: "在做什么",
			Text:  "memory 是什么 记忆 memory memory bot 长期记忆 工作区 对话 历史",
		},
		{
			Path:  "en/getting-started/compaction.md",
			Title: "Compaction",
			Text:  "compaction compaction session context memory compact history messages",
		},
	}

	results := searchMemohHelpChunks(chunks, "Memoh compaction memory compaction 是什么", 2)
	if len(results) == 0 {
		t.Fatalf("expected results for compaction query, got none")
	}
	if results[0]["path"] != "en/getting-started/compaction.md" {
		t.Fatalf("expected compaction.md ranked first, got %v", results)
	}
}
