package directory

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

func TestExecutor_ListTools(t *testing.T) {
	reg := channel.NewRegistry()
	reg.MustRegister(&dirMockAdapter{channelType: "dir-test"})
	svc := &fakeConfigResolver{}
	exec := NewExecutor(nil, reg, svc, reg)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != toolLookupChannelUser {
		t.Errorf("tool name = %q", tools[0].Name)
	}
}

func TestExecutor_ListTools_NilDeps(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, nil)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools when deps nil, got %d", len(tools))
	}
}

func TestExecutor_CallTool_NotFound(t *testing.T) {
	exec := NewExecutor(nil, channel.NewRegistry(), &fakeConfigResolver{}, channel.NewRegistry())
	_, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{}, "other_tool", nil)
	if !errors.Is(err, mcpgw.ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}

type dirMockAdapter struct {
	channelType channel.Type
}

func (d *dirMockAdapter) Type() channel.Type { return d.channelType }
func (d *dirMockAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{Type: d.channelType, DisplayName: "DirTest"}
}

func (d *dirMockAdapter) ListPeers(context.Context, channel.Config, channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (d *dirMockAdapter) ListGroups(context.Context, channel.Config, channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (d *dirMockAdapter) ListGroupMembers(context.Context, channel.Config, string, channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (d *dirMockAdapter) ResolveEntry(context.Context, channel.Config, string, channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
	return channel.DirectoryEntry{Kind: channel.DirectoryEntryUser, ID: "id1", Name: "Test User"}, nil
}

type fakeConfigResolver struct{}

func (f *fakeConfigResolver) ResolveEffectiveConfig(context.Context, string, channel.Type) (channel.Config, error) {
	return channel.Config{}, nil
}
