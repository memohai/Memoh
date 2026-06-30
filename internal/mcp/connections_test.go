package mcp

import (
	"reflect"
	"testing"
)

func TestInferTypeAndConfig(t *testing.T) {
	tests := []struct {
		name       string
		req        UpsertRequest
		wantType   string
		wantConfig map[string]any
		wantErr    bool
	}{
		{
			name: "stdio",
			req: UpsertRequest{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/path"},
				Env:     map[string]string{"TOKEN": "abc"},
				Cwd:     "/workspace",
			},
			wantType: "stdio",
			wantConfig: map[string]any{
				"command": "npx",
				"args":    []string{"-y", "@modelcontextprotocol/server-filesystem", "/path"},
				"env":     map[string]string{"TOKEN": "abc"},
				"cwd":     "/workspace",
			},
		},
		{
			name: "http",
			req: UpsertRequest{
				URL:     "https://example.com/mcp",
				Headers: map[string]string{"Authorization": "Bearer sk-xxx"},
			},
			wantType: "http",
			wantConfig: map[string]any{
				"url":     "https://example.com/mcp",
				"headers": map[string]string{"Authorization": "Bearer sk-xxx"},
			},
		},
		{
			name:       "sse",
			req:        UpsertRequest{URL: "https://example.com/sse", Transport: "sse"},
			wantType:   "sse",
			wantConfig: map[string]any{"url": "https://example.com/sse"},
		},
		{
			name:    "missing endpoint",
			req:     UpsertRequest{Name: "empty"},
			wantErr: true,
		},
		{
			name:    "conflicting endpoints",
			req:     UpsertRequest{Command: "npx", URL: "https://example.com"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotConfig, err := inferTypeAndConfig(tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tc.wantType {
				t.Fatalf("type = %q, want %q", gotType, tc.wantType)
			}
			if !reflect.DeepEqual(gotConfig, tc.wantConfig) {
				t.Fatalf("config = %#v, want %#v", gotConfig, tc.wantConfig)
			}
		})
	}
}

func TestConnectionToExportEntry(t *testing.T) {
	tests := []struct {
		name string
		conn Connection
		want MCPServerEntry
	}{
		{
			name: "stdio",
			conn: Connection{
				Type: "stdio",
				Config: map[string]any{
					"command": "npx",
					"args":    []any{"-y", "server"},
					"env":     map[string]any{"KEY": "val"},
					"cwd":     "/work",
				},
			},
			want: MCPServerEntry{
				Command: "npx",
				Args:    []string{"-y", "server"},
				Env:     map[string]string{"KEY": "val"},
				Cwd:     "/work",
			},
		},
		{
			name: "http",
			conn: Connection{
				Type: "http",
				Config: map[string]any{
					"url":     "https://example.com/mcp",
					"headers": map[string]any{"Authorization": "Bearer xxx"},
				},
			},
			want: MCPServerEntry{
				URL:     "https://example.com/mcp",
				Headers: map[string]string{"Authorization": "Bearer xxx"},
			},
		},
		{
			name: "sse",
			conn: Connection{
				Type:   "sse",
				Config: map[string]any{"url": "https://example.com/sse"},
			},
			want: MCPServerEntry{
				URL:       "https://example.com/sse",
				Transport: "sse",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectionToExportEntry(tc.conn); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("entry = %#v, want %#v", got, tc.want)
			}
		})
	}
}
