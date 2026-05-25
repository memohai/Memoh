package bridgesvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

func TestLocalPathResolverMapsDataMountToWorkspaceRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srv := New(Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         "/data",
		AllowHostAbsolute: true,
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    "/data/notes/demo.txt",
		Content: []byte("demo"),
	}); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "notes", "demo.txt")) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read mapped file failed: %v", err)
	}
	if string(got) != "demo" {
		t.Fatalf("mapped file = %q, want demo", string(got))
	}
}

func TestLocalPathResolverMapsTeamSlugToHostRoot(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	allowed := t.TempDir()
	srv := New(Options{
		DefaultWorkDir: workspaceRoot,
		WorkspaceRoot:  workspaceRoot,
		DataMount:      "/data",
		TeamMount:      "/team",
		TeamRoots:      map[string]string{"engineering": allowed},
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    "/team/engineering/note.md",
		Content: []byte("hello team"),
	}); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(allowed, "note.md")) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read mapped team file failed: %v", err)
	}
	if string(got) != "hello team" {
		t.Fatalf("mapped team file = %q, want \"hello team\"", string(got))
	}
}

func TestLocalPathResolverDeniesUnlistedTeamSlug(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	allowed := t.TempDir()
	srv := New(Options{
		DefaultWorkDir: workspaceRoot,
		WorkspaceRoot:  workspaceRoot,
		DataMount:      "/data",
		TeamMount:      "/team",
		TeamRoots:      map[string]string{"engineering": allowed},
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    "/team/marketing/secret.md",
		Content: []byte("nope"),
	}); err == nil {
		t.Fatalf("expected unlisted team path to fail, got nil error")
	}
	// The file must not have been written under the allowed dir either.
	if _, err := os.Stat(filepath.Join(allowed, "secret.md")); err == nil {
		t.Fatalf("unlisted team write leaked into allowed dir")
	}
}

func TestLocalPathResolverAllowsHostAbsolutePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	srv := New(Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         "/data",
		AllowHostAbsolute: true,
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    outside,
		Content: []byte("outside"),
	}); err != nil {
		t.Fatalf("WriteFile absolute path failed: %v", err)
	}
	got, err := os.ReadFile(outside) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read absolute file failed: %v", err)
	}
	if string(got) != "outside" {
		t.Fatalf("absolute file = %q, want outside", string(got))
	}
}
