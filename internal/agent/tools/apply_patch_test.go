package tools

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type applyPatchFakeFS struct {
	files map[string]string
	dirs  map[string]bool
}

func (fs applyPatchFakeFS) Stat(_ context.Context, path string) (applyPatchFileInfo, error) {
	if fs.dirs[path] {
		return applyPatchFileInfo{exists: true, isDir: true}, nil
	}
	if _, ok := fs.files[path]; ok {
		return applyPatchFileInfo{exists: true}, nil
	}
	return applyPatchFileInfo{exists: false}, nil
}

func (fs applyPatchFakeFS) ReadText(_ context.Context, path string) (string, error) {
	if content, ok := fs.files[path]; ok {
		return content, nil
	}
	return "", errors.New("not found")
}

func TestParseApplyPatchMultipleOperations(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: nested/new.txt
+hello
+world
*** Delete File: obsolete.txt
*** Update File: old/name.txt
*** Move to: renamed/name.txt
@@
-old
+new
*** End Patch`

	hunks, err := parseApplyPatch(patch)
	if err != nil {
		t.Fatalf("parseApplyPatch() error = %v", err)
	}
	if len(hunks) != 3 {
		t.Fatalf("expected 3 hunks, got %d", len(hunks))
	}
	if hunks[0].kind != applyPatchHunkAdd || hunks[0].path != "nested/new.txt" || hunks[0].contents != "hello\nworld\n" {
		t.Fatalf("unexpected add hunk: %+v", hunks[0])
	}
	if hunks[1].kind != applyPatchHunkDelete || hunks[1].path != "obsolete.txt" {
		t.Fatalf("unexpected delete hunk: %+v", hunks[1])
	}
	if hunks[2].kind != applyPatchHunkUpdate || hunks[2].movePath != "renamed/name.txt" {
		t.Fatalf("unexpected update hunk: %+v", hunks[2])
	}
	if got := hunks[2].chunks[0].oldLines; !reflect.DeepEqual(got, []string{"old"}) {
		t.Fatalf("unexpected old lines: %#v", got)
	}
	if got := hunks[2].chunks[0].newLines; !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("unexpected new lines: %#v", got)
	}
}

func TestBuildApplyPatchPlanMultipleOperations(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: nested/new.txt
+created
*** Update File: modify.txt
@@
 foo
-bar
+baz
*** Delete File: obsolete.txt
*** Update File: old/name.txt
*** Move to: renamed/name.txt
@@
-old
+new
*** End Patch`
	hunks, err := parseApplyPatch(patch)
	if err != nil {
		t.Fatalf("parseApplyPatch() error = %v", err)
	}

	plan, err := buildApplyPatchPlan(context.Background(), applyPatchFakeFS{
		files: map[string]string{
			"modify.txt":   "foo\nbar\n",
			"obsolete.txt": "gone\n",
			"old/name.txt": "old\n",
		},
	}, "/data", hunks)
	if err != nil {
		t.Fatalf("buildApplyPatchPlan() error = %v", err)
	}

	if got, want := plan.added, []string{"nested/new.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("added = %#v, want %#v", got, want)
	}
	if got, want := plan.modified, []string{"modify.txt", "old/name.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("modified = %#v, want %#v", got, want)
	}
	if got, want := plan.deleted, []string{"obsolete.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deleted = %#v, want %#v", got, want)
	}
	if len(plan.operations) != 5 {
		t.Fatalf("expected 5 operations, got %d: %#v", len(plan.operations), plan.operations)
	}
	if op := plan.operations[1]; op.kind != applyPatchOperationWrite || op.path != "modify.txt" || op.content != "foo\nbaz\n" {
		t.Fatalf("unexpected modify operation: %#v", op)
	}
	if op := plan.operations[3]; op.kind != applyPatchOperationWrite || op.path != "renamed/name.txt" || op.content != "new\n" {
		t.Fatalf("unexpected move write operation: %#v", op)
	}
	if op := plan.operations[4]; op.kind != applyPatchOperationDelete || op.path != "old/name.txt" {
		t.Fatalf("unexpected move delete operation: %#v", op)
	}
}

func TestBuildApplyPatchPlanRejectsMissingContextBeforeCommitPlan(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: created.txt
+created
*** Update File: modify.txt
@@
-missing
+new
*** End Patch`
	hunks, err := parseApplyPatch(patch)
	if err != nil {
		t.Fatalf("parseApplyPatch() error = %v", err)
	}

	_, err = buildApplyPatchPlan(context.Background(), applyPatchFakeFS{
		files: map[string]string{"modify.txt": "original\n"},
	}, "/data", hunks)
	if err == nil {
		t.Fatal("expected missing context error")
	}
	if !strings.Contains(err.Error(), "failed to find expected lines") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeApplyPatchPathRejectsRelativeTraversal(t *testing.T) {
	if _, err := normalizeApplyPatchPath("../outside.txt", "/data"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	path, err := normalizeApplyPatchPath("/data/src/main.go", "/data")
	if err != nil {
		t.Fatalf("normalizeApplyPatchPath() error = %v", err)
	}
	if path != "src/main.go" {
		t.Fatalf("normalized path = %q, want src/main.go", path)
	}
}
