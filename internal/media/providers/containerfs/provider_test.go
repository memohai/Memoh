package containerfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestProvider_HostPath(t *testing.T) {
	t.Parallel()
	p := &Provider{dataRoot: "/srv/data"}

	tests := []struct {
		key     string
		want    string
		wantErr bool
	}{
		{key: "bot-1/image/ab12/ab12cd.png", want: "/srv/data/bots/bot-1/media/image/ab12/ab12cd.png"},
		{key: "/absolute/path", wantErr: true},
		{key: "../escape", wantErr: true},
		{key: "nosubpath", wantErr: true},
		{key: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := p.hostPath(tt.key)
		if tt.wantErr {
			if err == nil {
				t.Errorf("hostPath(%q) expected error", tt.key)
			}
			continue
		}
		if err != nil {
			t.Errorf("hostPath(%q) unexpected error: %v", tt.key, err)
			continue
		}
		if got != tt.want {
			t.Errorf("hostPath(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestProvider_AccessPath(t *testing.T) {
	t.Parallel()
	p := &Provider{dataRoot: "/srv/data"}

	tests := []struct {
		key  string
		want string
	}{
		{key: "bot-1/image/ab12/ab12cd.png", want: "/data/media/image/ab12/ab12cd.png"},
		{key: "bot-1/file/xx/doc.pdf", want: "/data/media/file/xx/doc.pdf"},
	}
	for _, tt := range tests {
		got := p.AccessPath(tt.key)
		if got != tt.want {
			t.Errorf("AccessPath(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestProvider_PutOpenDelete(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	p, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	key := "bot-1/image/ab/test.png"
	data := []byte("hello media content")

	if err := p.Put(context.Background(), key, bytes.NewReader(data)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	hostFile := filepath.Join(tmpDir, "bots", "bot-1", "media", "image", "ab", "test.png")
	if _, err := os.Stat(hostFile); err != nil {
		t.Fatalf("file not found on host: %v", err)
	}

	reader, err := p.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if !bytes.Equal(got, data) {
		t.Errorf("Open returned %q, want %q", got, data)
	}

	if err := p.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := os.Stat(hostFile); !os.IsNotExist(err) {
		t.Fatalf("file should be deleted: %v", err)
	}
}

func TestProvider_PathTraversal(t *testing.T) {
	t.Parallel()
	p := &Provider{dataRoot: "/srv/data"}

	bad := []string{
		"../etc/passwd",
		"/absolute/key",
		"bot-1/../../escape",
	}
	for _, key := range bad {
		if _, err := p.hostPath(key); err == nil {
			t.Errorf("hostPath(%q) should reject traversal", key)
		}
	}
}
