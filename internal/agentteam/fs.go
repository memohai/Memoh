package agentteam

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// TeamDirName returns the on-disk directory name for a team. Preference:
//  1. explicit shared_dir_name (already validated when set);
//  2. a filesystem-safe slug derived from the team's display name;
//  3. fallback to the team's UUID when neither yields anything usable.
//
// The slug keeps ASCII letters, digits, dashes, dots, and underscores;
// runs of other characters collapse to a single dash; and the result is
// lower-cased, trimmed of separator runs, and capped at 64 chars.
func TeamDirName(team Team) string {
	if name := strings.TrimSpace(team.SharedDirName); name != "" {
		return name
	}
	if slug := slugifyName(team.Name); slug != "" {
		return slug
	}
	return team.ID
}

func slugifyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	lastSep := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
			lastSep = false
		case unicode.IsSpace(r), unicode.IsPunct(r), unicode.IsSymbol(r):
			if !lastSep {
				b.WriteByte('-')
				lastSep = true
			}
		default:
			// Non-ASCII (e.g. CJK) — keep as-is to preserve user intent.
			// Filesystems handle UTF-8 transparently.
			b.WriteRune(r)
			lastSep = false
		}
	}
	s := strings.Trim(b.String(), "-._")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-._")
	}
	return s
}

// TeamFSPath returns the host path that backs the team's `/team/<dir>`
// mount. Empty team yields "".
func TeamFSPath(root string, team Team) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	dir := TeamDirName(team)
	if dir == "" {
		return ""
	}
	return filepath.Join(root, dir)
}

// teamLegacyFSPath returns the host path that the team used to live at
// before we switched from UUIDs to slugs. Used only to migrate existing
// directories transparently on the next provision pass.
func teamLegacyFSPath(root string, team Team) string {
	root = strings.TrimSpace(root)
	if root == "" || strings.TrimSpace(team.ID) == "" {
		return ""
	}
	return filepath.Join(root, team.ID)
}

// ProvisionTeamFS makes sure the host directory that backs `/team/<dir>`
// exists, contains at least a placeholder `README.md`, and — if the team
// previously lived under a UUID directory — gets transparently renamed to
// the new slug location.
//
// The function is idempotent: running it on an already-populated
// directory is a no-op aside from the rename check.
func ProvisionTeamFS(root string, team Team) error {
	dir := TeamFSPath(root, team)
	if dir == "" {
		return nil
	}

	// One-shot migration: rename `<root>/<uuid>` to `<root>/<dir>` when
	// the legacy dir exists and the target doesn't. Old code seeded
	// directories at the UUID path; renaming here keeps existing files
	// (and the user's edits) intact.
	if legacy := teamLegacyFSPath(root, team); legacy != "" && legacy != dir {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if _, err := os.Stat(legacy); err == nil {
				if err := os.Rename(legacy, dir); err != nil {
					return fmt.Errorf("rename team dir %s -> %s: %w", legacy, dir, err)
				}
			}
		}
	}

	// 0o750 — group-readable shared dir. Bot containers bind-mount this
	// path so any container user that can reach /team gets in.
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // group-readable shared team dir
		return fmt.Errorf("create team dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read team dir: %w", err)
	}
	if len(entries) > 0 {
		return nil
	}
	readme := buildReadme(team)
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readme), 0o640); err != nil { //nolint:gosec // group-readable shared team dir
		return fmt.Errorf("seed README: %w", err)
	}
	return nil
}

func buildReadme(team Team) string {
	name := strings.TrimSpace(team.Name)
	if name == "" {
		name = team.ID
	}
	dir := TeamDirName(team)
	var sb strings.Builder
	sb.WriteString("# " + name + "\n\n")
	if d := strings.TrimSpace(team.Description); d != "" {
		sb.WriteString(d + "\n\n")
	}
	sb.WriteString("This directory is the shared workspace for the **" + name + "** team. ")
	sb.WriteString("Inside a bot container it lives at `/team/" + dir + "/`; on the host it lives under `<data_root>/teams/" + dir + "/`.\n\n")
	sb.WriteString("Files written here are visible to every bot and human member of this team. Bot private data still belongs in `/data`.\n\n")
	sb.WriteString("Edit or delete this README at any time — it is only here so the directory is not empty.\n")
	return sb.String()
}
