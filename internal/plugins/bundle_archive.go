package plugins

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	MaxBundleCompressedBytes   = 25 * 1024 * 1024
	MaxBundleStreamBytes       = 16 * 1024 * 1024
	MaxBundleFileBytes         = 2 * 1024 * 1024
	MaxBundleFiles             = 1_000
	maxBundleUncompressedBytes = 10 * 1024 * 1024
	maxBundleEntries           = 2_000
	bundleCleanupTimeout       = 30 * time.Second
)

const (
	bundleArchiveKindManifest = "manifest"
	bundleArchiveKindHooks    = "hooks"
	bundleArchiveKindScripts  = "scripts"
)

type BundleAssetInstallResult struct {
	OK           bool   `json:"ok"`
	FilesWritten int    `json:"files_written"`
	Error        string `json:"error,omitempty"`
}

type BundleInstallResult struct {
	Hooks   BundleAssetInstallResult
	Scripts BundleAssetInstallResult
}

type bundleArchiveEntry struct {
	kind         string
	relativePath string
}

type bundleArchiveFile struct {
	entry      bundleArchiveEntry
	content    []byte
	executable bool
}

// BundleArchive is a validated Plugin bundle ready for workspace publication.
type BundleArchive struct {
	files []bundleArchiveFile
}

type bundleWriter interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	ExecWithEnv(ctx context.Context, command, workDir string, timeout int32, env []string) (*bridge.ExecResult, error)
	Mkdir(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	WriteFile(ctx context.Context, path string, content []byte) error
}

type bundlePublicationClient interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	Rename(ctx context.Context, oldPath, newPath string) error
}

// BundlePublication keeps the previous Plugin directory until Commit so a
// larger installation can roll back ordinary failures.
type BundlePublication struct {
	client       bundlePublicationClient
	pluginRoot   string
	backupDir    string
	targetExists bool
	closed       bool
	result       BundleInstallResult
}

// ReadBundleArchive validates the archive layout, manifest identity, Package
// references, and extraction budgets without writing to the workspace.
func ReadBundleArchive(
	archivePluginID, targetPluginID string,
	r io.Reader,
	expectedPackages []PackageReference,
) (BundleArchive, error) {
	if !skillset.IsValidPluginID(archivePluginID) {
		return BundleArchive{}, errors.New("plugin bundle archive id is invalid")
	}
	if _, err := skillset.PluginDirForID(targetPluginID); err != nil {
		return BundleArchive{}, errors.New("plugin bundle target id is invalid")
	}

	archive := BundleArchive{files: make([]bundleArchiveFile, 0)}
	seen := make(map[string]bool)
	var totalSize int64
	totalEntries := 0
	regularFiles := 0
	hasManifest := false
	var packageReferences []PackageReference
	stream := &io.LimitedReader{R: r, N: MaxBundleStreamBytes + 1}
	tr := tar.NewReader(stream)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return BundleArchive{}, fmt.Errorf("invalid plugin bundle tar: %w", err)
		}
		totalEntries++
		if totalEntries > maxBundleEntries {
			return BundleArchive{}, errors.New("plugin bundle contains too many entries")
		}

		name, err := normalizeBundleArchivePath(archivePluginID, hdr.Name)
		if err != nil {
			return BundleArchive{}, err
		}
		if name == "" {
			if hdr.Typeflag == tar.TypeDir {
				continue
			}
			return BundleArchive{}, errors.New("plugin bundle archive root must be a directory")
		}
		isFile := hdr.Typeflag == tar.TypeReg || hdr.Typeflag == 0
		if err := recordBundleArchivePath(seen, name, isFile); err != nil {
			return BundleArchive{}, err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if !isAllowedBundleDirectory(name) {
				return BundleArchive{}, fmt.Errorf("plugin bundle contains unsupported directory %q", hdr.Name)
			}
			continue
		case tar.TypeReg, 0:
		case tar.TypeSymlink, tar.TypeLink:
			return BundleArchive{}, fmt.Errorf("plugin bundle contains a link at %q", hdr.Name)
		default:
			return BundleArchive{}, fmt.Errorf("plugin bundle contains unsupported entry %q", hdr.Name)
		}
		regularFiles++
		if regularFiles > MaxBundleFiles {
			return BundleArchive{}, errors.New("plugin bundle exceeds the file limit")
		}
		if hdr.Size < 0 || hdr.Size > MaxBundleFileBytes {
			return BundleArchive{}, fmt.Errorf("plugin bundle file %q exceeds the file size limit", hdr.Name)
		}
		if hdr.Size > maxBundleUncompressedBytes-totalSize {
			return BundleArchive{}, errors.New("plugin bundle exceeds the uncompressed size limit")
		}
		content, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil || int64(len(content)) != hdr.Size {
			return BundleArchive{}, fmt.Errorf("plugin bundle file %q is truncated", hdr.Name)
		}
		totalSize += int64(len(content))

		entry, ok, err := bundleArchiveEntryForPath(archivePluginID, targetPluginID, hdr.Name)
		if err != nil {
			return BundleArchive{}, err
		}
		if !ok {
			return BundleArchive{}, fmt.Errorf("plugin bundle contains unsupported file %q", hdr.Name)
		}
		if entry.kind == bundleArchiveKindManifest {
			packageReferences, err = validateBundleManifest(content, targetPluginID)
			if err != nil {
				return BundleArchive{}, err
			}
			hasManifest = true
		}
		archive.files = append(archive.files, bundleArchiveFile{
			entry: entry, content: content, executable: hdr.FileInfo().Mode().Perm()&0o111 != 0,
		})
	}
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return BundleArchive{}, fmt.Errorf("plugin bundle decompression failed: %w", err)
	}
	if stream.N <= 0 {
		return BundleArchive{}, errors.New("plugin bundle exceeds the decompressed stream limit")
	}
	if !hasManifest {
		return BundleArchive{}, errors.New("plugin bundle does not contain a root plugin.yaml")
	}
	if !SamePackageReferences(packageReferences, expectedPackages) {
		return BundleArchive{}, errors.New("plugin bundle Package references do not match the catalog manifest")
	}
	return archive, nil
}

// PublishBundleArchive stages a validated bundle before replacing its target.
func PublishBundleArchive(
	ctx context.Context,
	client bundleWriter,
	workspaceOS, targetPluginID string,
	archive BundleArchive,
) (*BundlePublication, error) {
	result := newBundleInstallResult()
	if client == nil {
		return nil, errors.New("workspace is not reachable")
	}
	pluginRoot, err := skillset.PluginDirForID(targetPluginID)
	if err != nil {
		return nil, err
	}
	stagingRoot := path.Join(skillset.PluginDirPath, ".staging")
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	tempDir := path.Join(stagingRoot, "install-"+targetPluginID+"-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-"+targetPluginID+"-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() {
		cleanupCtx, cancel := bundleCleanupContext(ctx)
		defer cancel()
		_ = client.DeleteFile(cleanupCtx, tempDir, true)
	}()
	if err := client.Mkdir(ctx, stagingRoot); err != nil {
		return nil, fmt.Errorf("create plugin staging root: %w", err)
	}
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return nil, fmt.Errorf("create temporary plugin directory: %w", err)
	}

	executablePaths := make([]string, 0)
	for _, file := range archive.files {
		relativePath := file.entry.relativePath
		if file.entry.kind == bundleArchiveKindScripts {
			relativePath = path.Join("scripts", relativePath)
		}
		filePath := path.Clean(path.Join(tempDir, relativePath))
		if filePath == tempDir || !strings.HasPrefix(filePath, tempDir+"/") {
			return nil, fmt.Errorf("plugin bundle path escapes staging root: %s", relativePath)
		}
		if dir := path.Dir(filePath); dir != tempDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return nil, fmt.Errorf("create plugin bundle directory %s: %w", dir, err)
			}
		}
		if err := client.WriteFile(ctx, filePath, file.content); err != nil {
			return nil, fmt.Errorf("write plugin bundle file %s: %w", relativePath, err)
		}
		if file.entry.kind == bundleArchiveKindScripts && file.executable {
			executablePaths = append(executablePaths, filePath)
		}
		switch file.entry.kind {
		case bundleArchiveKindHooks:
			result.Hooks.FilesWritten++
		case bundleArchiveKindScripts:
			result.Scripts.FilesWritten++
		}
	}
	if err := applyBundleExecutableModes(ctx, client, workspaceOS, executablePaths); err != nil {
		return nil, err
	}

	targetExists := true
	if err := client.Rename(ctx, pluginRoot, backupDir); err != nil {
		if errors.Is(err, bridge.ErrNotFound) {
			targetExists = false
		} else {
			return nil, fmt.Errorf("prepare existing plugin bundle for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, pluginRoot); err != nil {
		if targetExists {
			rollbackCtx, cancel := bundleCleanupContext(ctx)
			defer cancel()
			if rollbackErr := client.Rename(rollbackCtx, backupDir, pluginRoot); rollbackErr != nil {
				return nil, fmt.Errorf(
					"publish plugin bundle: %w; restore previous bundle from %q: %w",
					err, backupDir, rollbackErr,
				)
			}
		}
		return nil, fmt.Errorf("publish plugin bundle: %w", err)
	}
	return &BundlePublication{
		client: client, pluginRoot: pluginRoot, backupDir: backupDir, targetExists: targetExists, result: result,
	}, nil
}

func (p *BundlePublication) Result() BundleInstallResult {
	if p == nil {
		return BundleInstallResult{}
	}
	return p.result
}

func (p *BundlePublication) Commit(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	cleanupCtx, cancel := bundleCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(cleanupCtx, p.backupDir, true); err != nil {
		return err
	}
	p.closed = true
	return nil
}

func (p *BundlePublication) Rollback(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	rollbackCtx, cancel := bundleCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(rollbackCtx, p.pluginRoot, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("remove replacement Plugin bundle: %w", err)
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	if err := p.client.Rename(rollbackCtx, p.backupDir, p.pluginRoot); err != nil {
		return fmt.Errorf("restore previous Plugin bundle from %q: %w", p.backupDir, err)
	}
	p.closed = true
	return nil
}

func bundleCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), bundleCleanupTimeout)
}

func applyBundleExecutableModes(ctx context.Context, client bundleWriter, workspaceOS string, paths []string) error {
	if len(paths) == 0 || strings.EqualFold(workspaceOS, "windows") || strings.EqualFold(workspaceOS, "win32") {
		return nil
	}
	for start := 0; start < len(paths); start += 100 {
		end := min(start+100, len(paths))
		quoted := make([]string, 0, end-start)
		for _, filePath := range paths[start:end] {
			quoted = append(quoted, bundleShellQuote(filePath))
		}
		result, err := client.ExecWithEnv(ctx, "chmod 755 -- "+strings.Join(quoted, " "), "/", 30, nil)
		if err != nil {
			return fmt.Errorf("preserve executable Plugin scripts: %w", err)
		}
		if result != nil && result.ExitCode != 0 {
			return fmt.Errorf("preserve executable Plugin scripts: chmod exited with code %d", result.ExitCode)
		}
	}
	return nil
}

func bundleShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func bundleArchiveEntryForPath(archivePluginID, targetPluginID, rawName string) (bundleArchiveEntry, bool, error) {
	name, err := normalizeBundleArchivePath(archivePluginID, rawName)
	if err != nil {
		return bundleArchiveEntry{}, false, err
	}
	if name == "" {
		return bundleArchiveEntry{}, false, nil
	}
	segments := strings.Split(name, "/")

	switch segments[0] {
	case "plugin.yaml":
		if len(segments) == 1 {
			return bundleArchiveEntry{kind: bundleArchiveKindManifest, relativePath: "plugin.yaml"}, true, nil
		}
		return bundleArchiveEntry{}, false, errors.New("plugin bundle contains a path below plugin.yaml")
	case "hooks.json":
		if len(segments) != 1 {
			return bundleArchiveEntry{}, false, errors.New("plugin bundle contains a path below hooks.json")
		}
		if _, err := skillset.PluginDirForID(targetPluginID); err != nil {
			return bundleArchiveEntry{}, false, err
		}
		return bundleArchiveEntry{kind: bundleArchiveKindHooks, relativePath: "hooks.json"}, true, nil
	case "skills":
		return bundleArchiveEntry{}, false, errors.New("plugin bundle must reference Registry Skills instead of embedding skills/**")
	case "scripts":
		if len(segments) < 2 {
			return bundleArchiveEntry{}, false, errors.New("plugin bundle scripts path must name a file")
		}
		if _, err := skillset.PluginScriptsDirForID(targetPluginID); err != nil {
			return bundleArchiveEntry{}, false, err
		}
		return bundleArchiveEntry{kind: bundleArchiveKindScripts, relativePath: strings.Join(segments[1:], "/")}, true, nil
	}
	return bundleArchiveEntry{}, false, fmt.Errorf("plugin bundle contains unsupported file %q", rawName)
}

func isAllowedBundleDirectory(name string) bool {
	return name == "scripts" || strings.HasPrefix(name, "scripts/")
}

func normalizeBundleArchivePath(archivePluginID, rawName string) (string, error) {
	if rawName == "" || rawName != strings.TrimSpace(rawName) || path.IsAbs(rawName) || strings.Contains(rawName, "\\") {
		return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
	}
	name := strings.TrimSuffix(rawName, "/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("plugin bundle contains non-canonical path %q", rawName)
	}
	if !skillset.IsValidPluginID(archivePluginID) {
		return "", errors.New("plugin bundle archive id is invalid")
	}
	if name == archivePluginID {
		return "", nil
	}
	if !strings.HasPrefix(name, archivePluginID+"/") {
		return "", fmt.Errorf("plugin bundle path %q is outside the %q root", rawName, archivePluginID)
	}
	name = strings.TrimPrefix(name, archivePluginID+"/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("plugin bundle contains non-canonical path %q", rawName)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || segment != strings.TrimSpace(segment) ||
			strings.HasSuffix(segment, ".") || strings.ContainsAny(segment, `<>:"|?*`) || isWindowsReservedPathName(segment) {
			return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
		}
		for _, character := range segment {
			if character < 0x20 || character == 0x7f {
				return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
			}
		}
	}
	return name, nil
}

func isWindowsReservedPathName(value string) bool {
	base := strings.ToLower(strings.SplitN(value, ".", 2)[0])
	if base == "con" || base == "prn" || base == "aux" || base == "nul" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) &&
		base[3] >= '1' && base[3] <= '9'
}

func recordBundleArchivePath(seen map[string]bool, name string, isFile bool) error {
	canonicalName := strings.ToLower(name)
	if _, exists := seen[canonicalName]; exists {
		return fmt.Errorf("plugin bundle contains duplicate path %q", name)
	}
	for parent := path.Dir(canonicalName); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if seen[parent] {
			return fmt.Errorf("plugin bundle path %q is nested below file %q", name, parent)
		}
	}
	if isFile {
		for candidate := range seen {
			if strings.HasPrefix(candidate, canonicalName+"/") {
				return fmt.Errorf("plugin bundle file %q conflicts with child path %q", name, candidate)
			}
		}
	}
	seen[canonicalName] = isFile
	return nil
}

func validateBundleManifest(content []byte, targetPluginID string) ([]PackageReference, error) {
	var manifest struct {
		ID       string `yaml:"id"`
		Packages []struct {
			RegistryID string `yaml:"registry_id"`
			PackageID  string `yaml:"package_id"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("plugin bundle contains an invalid plugin.yaml: %w", err)
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	if manifest.ID != targetPluginID {
		return nil, fmt.Errorf("plugin bundle manifest id %q does not match %q", manifest.ID, targetPluginID)
	}
	references := make([]PackageReference, 0, len(manifest.Packages))
	for _, reference := range manifest.Packages {
		references = append(references, PackageReference{
			RegistryID: strings.TrimSpace(reference.RegistryID),
			PackageID:  strings.TrimSpace(reference.PackageID),
		})
	}
	if err := ValidatePackageReferences(references); err != nil {
		return nil, fmt.Errorf("plugin bundle manifest contains invalid Package references: %w", err)
	}
	return references, nil
}

func SamePackageReferences(left, right []PackageReference) bool {
	if len(left) != len(right) {
		return false
	}
	identities := make(map[string]struct{}, len(left))
	for _, reference := range left {
		identities[PackageReferenceIdentity(reference)] = struct{}{}
	}
	for _, reference := range right {
		if _, ok := identities[PackageReferenceIdentity(reference)]; !ok {
			return false
		}
	}
	return true
}

func newBundleInstallResult() BundleInstallResult {
	return BundleInstallResult{
		Hooks:   BundleAssetInstallResult{OK: true},
		Scripts: BundleAssetInstallResult{OK: true},
	}
}
