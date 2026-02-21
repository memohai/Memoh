package handlers

import (
	"testing"

	ctr "github.com/memohai/memoh/internal/containerd"
)

func TestSnapshotLineage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		root      string
		input     []ctr.SnapshotInfo
		wantFound bool
		wantNames []string
	}{
		{
			name: "walk full snapshot ancestry",
			root: "active-3",
			input: []ctr.SnapshotInfo{
				{Name: "active-3", Parent: "version-2"},
				{Name: "version-2", Parent: "version-1"},
				{Name: "version-1", Parent: "sha256:base-layer"},
				{Name: "sha256:base-layer", Parent: ""},
				{Name: "unrelated", Parent: ""},
			},
			wantFound: true,
			wantNames: []string{"active-3", "version-2", "version-1", "sha256:base-layer"},
		},
		{
			name: "root snapshot not found",
			root: "missing",
			input: []ctr.SnapshotInfo{
				{Name: "active-1", Parent: "sha256:base-layer"},
				{Name: "sha256:base-layer", Parent: ""},
			},
			wantFound: false,
			wantNames: nil,
		},
		{
			name: "missing parent keeps known chain",
			root: "active-1",
			input: []ctr.SnapshotInfo{
				{Name: "active-1", Parent: "version-1"},
			},
			wantFound: true,
			wantNames: []string{"active-1"},
		},
		{
			name: "cycle is bounded by visited set",
			root: "a",
			input: []ctr.SnapshotInfo{
				{Name: "a", Parent: "b"},
				{Name: "b", Parent: "a"},
			},
			wantFound: true,
			wantNames: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := snapshotLineage(tt.root, tt.input)
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if !found {
				return
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("len(got) = %d, want %d", len(got), len(tt.wantNames))
			}
			for i := range tt.wantNames {
				if got[i].Name != tt.wantNames[i] {
					t.Fatalf("got[%d].Name = %q, want %q", i, got[i].Name, tt.wantNames[i])
				}
			}
		})
	}
}
