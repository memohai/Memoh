package agentteam

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSlugifyName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Team Alpha", "team-alpha"},
		{"punct", "Frontend / Backend!", "frontend-backend"},
		{"under-dash", "design_team-v2", "design_team-v2"},
		{"collapse", "Foo   Bar    Baz", "foo-bar-baz"},
		{"trim", "  -- hello --  ", "hello"},
		{"empty", "   ", ""},
		{"chinese", "测试团队", "测试团队"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := slugifyName(tc.in); got != tc.want {
				t.Fatalf("slugifyName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTeamDirNamePrefersExplicitSharedDir(t *testing.T) {
	t.Parallel()
	team := Team{ID: "uuid", Name: "Display", SharedDirName: "engineering"}
	if got := TeamDirName(team); got != "engineering" {
		t.Fatalf("TeamDirName explicit shared_dir = %q, want engineering", got)
	}
}

func TestTeamDirNameFallsBackToSlug(t *testing.T) {
	t.Parallel()
	team := Team{ID: "uuid", Name: "Hello World"}
	if got := TeamDirName(team); got != "hello-world" {
		t.Fatalf("TeamDirName slug = %q, want hello-world", got)
	}
}

func TestProvisionTeamFSMigratesLegacyDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	team := Team{ID: "team-uuid", Name: "Engineering"}

	legacy := filepath.Join(root, team.ID)
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatalf("seed legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.md"), []byte("old"), 0o600); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	if err := ProvisionTeamFS(root, team); err != nil {
		t.Fatalf("ProvisionTeamFS: %v", err)
	}

	newDir := filepath.Join(root, "engineering")
	if _, err := os.Stat(filepath.Join(newDir, "old.md")); err != nil {
		t.Fatalf("expected legacy file at new dir: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy dir should be gone, stat err = %v", err)
	}
}

func TestProvisionTeamFSSeedsReadmeOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	team := Team{ID: "team-uuid", Name: "Ops"}

	if err := ProvisionTeamFS(root, team); err != nil {
		t.Fatalf("first ProvisionTeamFS: %v", err)
	}
	readmePath := filepath.Join(root, "ops", "README.md")
	stat1, err := os.Stat(readmePath)
	if err != nil {
		t.Fatalf("expected README, got %v", err)
	}

	if err := ProvisionTeamFS(root, team); err != nil {
		t.Fatalf("second ProvisionTeamFS: %v", err)
	}
	stat2, err := os.Stat(readmePath)
	if err != nil {
		t.Fatalf("README disappeared on rerun: %v", err)
	}
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Fatalf("README was rewritten on idempotent rerun (mtime changed)")
	}
}
