package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxArchiveUncompressedBytes = 100 * 1024 * 1024
	maxArchiveStreamBytes       = 128 * 1024 * 1024
	maxArchiveFiles             = 10_000
	maxArchiveEntries           = 20_000
)

type archiveFile struct {
	path       string
	content    []byte
	executable bool
}

// Archive is a validated Skill archive, ready to be installed.
type Archive struct {
	files             []archiveFile
	uncompressedBytes int64
}

// FileCount returns the number of regular files in the archive.
func (a Archive) FileCount() int {
	return len(a.files)
}

// UncompressedSize returns the aggregate size of regular files in the archive.
func (a Archive) UncompressedSize() int64 {
	return a.uncompressedBytes
}

// ReadArchive validates and reads a Skill archive rooted at SKILL.md.
func ReadArchive(content []byte) (Archive, error) {
	return ReadArchiveWithUncompressedLimit(content, maxArchiveUncompressedBytes)
}

// ReadArchiveWithUncompressedLimit applies a caller-specific aggregate budget
// without weakening the package-wide archive limit.
func ReadArchiveWithUncompressedLimit(content []byte, maximum int64) (Archive, error) {
	if maximum <= 0 {
		return Archive{}, errors.New("skill artifact exceeds the uncompressed size limit")
	}
	maximum = min(maximum, int64(maxArchiveUncompressedBytes))
	gz, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return Archive{}, errors.New("skill artifact is not valid gzip")
	}
	defer func() { _ = gz.Close() }()

	archive := Archive{}
	seen := make(map[string]bool)
	var totalSize int64
	totalEntries := 0
	hasManifest := false
	stream := &io.LimitedReader{R: gz, N: maxArchiveStreamBytes + 1}
	tr := tar.NewReader(stream)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Archive{}, fmt.Errorf("skill artifact contains invalid tar data: %w", err)
		}
		totalEntries++
		if totalEntries > maxArchiveEntries {
			return Archive{}, errors.New("skill artifact contains too many entries")
		}
		if header.Name == "" || path.IsAbs(header.Name) || strings.Contains(header.Name, `\`) {
			return Archive{}, fmt.Errorf("skill artifact contains unsafe path %q", header.Name)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || path.Clean(name) != name {
			return Archive{}, fmt.Errorf("skill artifact contains non-canonical path %q", header.Name)
		}
		canonicalName := strings.ToLower(name)
		canonicalOwnerPath := strings.ToLower(DirectOwnerFileName)
		if canonicalName == canonicalOwnerPath || strings.HasPrefix(canonicalName, canonicalOwnerPath+"/") {
			return Archive{}, fmt.Errorf("skill artifact contains reserved path %q", header.Name)
		}
		if containsUnsafePathPart(strings.Split(name, "/")) {
			return Archive{}, fmt.Errorf("skill artifact contains unsafe path %q", header.Name)
		}
		if _, exists := seen[canonicalName]; exists {
			return Archive{}, fmt.Errorf("skill artifact contains duplicate path %q", header.Name)
		}
		seen[canonicalName] = header.Typeflag == tar.TypeReg
		if err := rejectArchivePathConflict(seen, canonicalName); err != nil {
			return Archive{}, err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		case tar.TypeSymlink, tar.TypeLink:
			return Archive{}, fmt.Errorf("skill artifact contains a link at %q", header.Name)
		default:
			return Archive{}, fmt.Errorf("skill artifact contains unsupported entry %q", header.Name)
		}
		if len(archive.files) >= maxArchiveFiles {
			return Archive{}, errors.New("skill artifact exceeds the file limit")
		}
		if header.Size < 0 || header.Size > maximum-totalSize {
			return Archive{}, errors.New("skill artifact exceeds the uncompressed size limit")
		}
		fileContent, err := io.ReadAll(io.LimitReader(tr, header.Size+1))
		if err != nil || int64(len(fileContent)) != header.Size {
			return Archive{}, fmt.Errorf("skill artifact file %q is truncated", header.Name)
		}
		totalSize += int64(len(fileContent))
		if name == "SKILL.md" {
			if err := validateArchiveManifest(fileContent); err != nil {
				return Archive{}, err
			}
			hasManifest = true
		}
		archive.files = append(archive.files, archiveFile{
			path: name, content: fileContent, executable: header.FileInfo().Mode().Perm()&0o111 != 0,
		})
	}
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return Archive{}, fmt.Errorf("skill artifact decompression failed: %w", err)
	}
	if stream.N <= 0 {
		return Archive{}, errors.New("skill artifact exceeds the decompressed stream limit")
	}
	if len(archive.files) == 0 {
		return Archive{}, errors.New("skill artifact is empty")
	}
	if !hasManifest {
		return Archive{}, errors.New("skill artifact does not contain a root SKILL.md")
	}
	sort.Slice(archive.files, func(i, j int) bool { return archive.files[i].path < archive.files[j].path })
	archive.uncompressedBytes = totalSize
	return archive, nil
}

func containsUnsafePathPart(parts []string) bool {
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || part != strings.TrimSpace(part) ||
			strings.HasSuffix(part, ".") || strings.ContainsAny(part, `<>:"|?*`) {
			return true
		}
		for _, character := range part {
			if character < 0x20 || character == 0x7f {
				return true
			}
		}
	}
	return false
}

func rejectArchivePathConflict(seen map[string]bool, name string) error {
	for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if seen[parent] {
			return fmt.Errorf("skill artifact path %q is nested below file %q", name, parent)
		}
	}
	if !seen[name] {
		return nil
	}
	for candidate := range seen {
		if strings.HasPrefix(candidate, name+"/") {
			return fmt.Errorf("skill artifact file %q conflicts with child path %q", name, candidate)
		}
	}
	return nil
}

func validateArchiveManifest(content []byte) error {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return errors.New("skill SKILL.md is missing YAML frontmatter")
	}
	rest := normalized[4:]
	closing := strings.Index(rest, "\n---\n")
	if closing < 0 && strings.HasSuffix(rest, "\n---") {
		closing = len(rest) - len("\n---")
	}
	if closing < 0 {
		return errors.New("skill SKILL.md has malformed YAML frontmatter")
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:closing]), &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("skill SKILL.md has invalid YAML frontmatter")
	}
	return nil
}
