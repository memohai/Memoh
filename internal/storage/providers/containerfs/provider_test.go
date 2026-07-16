package containerfs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

type accessPathWorkspaceProvider struct {
	info bridge.WorkspaceInfo
	err  error
}

func (*accessPathWorkspaceProvider) MCPClient(context.Context, string) (*bridge.Client, error) {
	return nil, errors.New("not used")
}

func (p *accessPathWorkspaceProvider) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return p.info, p.err
}

func TestParseRoutingKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key     string
		wantErr bool
	}{
		{key: "bot-1/image/ab12/ab12cd.png", wantErr: false},
		{key: "/absolute/path", wantErr: true},
		{key: "../escape", wantErr: true},
		{key: "nosubpath", wantErr: true},
		{key: "", wantErr: true},
	}
	for _, tt := range tests {
		_, _, err := parseRoutingKey(tt.key)
		if tt.wantErr && err == nil {
			t.Errorf("parseRoutingKey(%q) expected error", tt.key)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("parseRoutingKey(%q) unexpected error: %v", tt.key, err)
		}
	}
}

func TestProvider_AccessPath(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	tests := []struct {
		key  string
		want string
	}{
		{key: "bot-1/image/ab12/ab12cd.png", want: "/data/media/image/ab12/ab12cd.png"},
		{key: "bot-1/file/xx/doc.pdf", want: "/data/media/file/xx/doc.pdf"},
	}
	for _, tt := range tests {
		got := p.AccessPath(context.Background(), tt.key)
		if got != tt.want {
			t.Errorf("AccessPath(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestProvider_AccessPathUsesHostPathForLocalWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	p := &Provider{clients: &accessPathWorkspaceProvider{info: bridge.WorkspaceInfo{
		Backend:        bridge.WorkspaceBackendLocal,
		DefaultWorkDir: root,
	}}}

	got := p.AccessPath(context.Background(), "bot-1/aa/doc.pdf")
	want := filepath.Join(root, "media", "aa", "doc.pdf")
	if got != want {
		t.Fatalf("AccessPath() = %q, want %q", got, want)
	}
}

func TestProvider_AccessPathReturnsEmptyWhenWorkspaceInfoFails(t *testing.T) {
	t.Parallel()

	p := &Provider{clients: &accessPathWorkspaceProvider{err: errors.New("workspace unavailable")}}
	if got := p.AccessPath(context.Background(), "bot-1/aa/doc.pdf"); got != "" {
		t.Fatalf("AccessPath() = %q, want empty", got)
	}
}

func TestParseRoutingKey_PathTraversal(t *testing.T) {
	t.Parallel()

	bad := []string{
		"../etc/passwd",
		"/absolute/key",
		"bot-1/../../escape",
	}
	for _, key := range bad {
		if _, _, err := parseRoutingKey(key); err == nil {
			t.Errorf("parseRoutingKey(%q) should reject traversal", key)
		}
	}
}

func TestSplitRoutingKey(t *testing.T) {
	t.Parallel()

	botID, sub := splitRoutingKey("bot-1/image/test.png")
	if botID != "bot-1" || sub != "image/test.png" {
		t.Errorf("splitRoutingKey: got (%q, %q)", botID, sub)
	}

	botID2, sub2 := splitRoutingKey("nosubpath")
	if botID2 != "" || sub2 != "nosubpath" {
		t.Errorf("splitRoutingKey single: got (%q, %q)", botID2, sub2)
	}
}
