package skills

import (
	"context"
	"path"
	"slices"
	"strings"

	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

func appendDiscoveryRoots(roots []Root, extra ...Root) []Root {
	if len(extra) == 0 {
		return roots
	}
	seen := make(map[string]struct{}, len(roots)+len(extra))
	for _, root := range roots {
		seen[root.Path] = struct{}{}
	}
	for _, root := range extra {
		if root.Path == "" {
			continue
		}
		if _, ok := seen[root.Path]; ok {
			continue
		}
		seen[root.Path] = struct{}{}
		roots = append(roots, root)
	}
	return roots
}

// orderedDiscoveryRoots defines the source precedence consumed by resolve():
// user > built-in > legacy > compat > Registry.
func orderedDiscoveryRoots(ctx context.Context, client fileClient, rawCompatRoots, rawRegistrySkillRoots []string) []Root {
	userRoots, directRegistryRoots := discoverOwnedSkillRoots(ctx, client)
	roots := appendDiscoveryRoots(userRoots, DiscoveryRoots(rawCompatRoots)...)
	roots = appendDiscoveryRoots(roots, directRegistryRoots...)
	for _, registryRoot := range normalizeRegistrySkillRoots(rawRegistrySkillRoots) {
		roots = appendDiscoveryRoots(roots, Root{
			Path:    registryRoot,
			Kind:    SourceKindRegistry,
			Managed: true,
		})
	}
	return roots
}

// discoverOwnedSkillRoots walks the canonical namespace/package/Skill layout.
// User packages are always owned; Registry Skills require a valid direct-owner
// marker unless an enabled Plugin supplies their exact path separately.
func discoverOwnedSkillRoots(ctx context.Context, client fileClient) (userRoots, directRegistryRoots []Root) {
	if client == nil {
		return nil, nil
	}
	namespaces, err := client.ListDirAll(ctx, ManagedDirPath, false)
	if err != nil {
		return nil, nil
	}
	slices.SortFunc(namespaces, func(a, b *pb.FileEntry) int {
		return strings.Compare(a.GetPath(), b.GetPath())
	})
	userRoots = make([]Root, 0)
	directRegistryRoots = make([]Root, 0)
	for _, namespaceEntry := range namespaces {
		if !namespaceEntry.GetIsDir() {
			continue
		}
		namespaceID := path.Base(namespaceEntry.GetPath())
		if !IsValidName(namespaceID) {
			continue
		}
		namespacePath, err := skillNamespaceDirForID(namespaceID)
		if err != nil {
			continue
		}
		packages, err := client.ListDirAll(ctx, namespacePath, false)
		if err != nil {
			continue
		}
		slices.SortFunc(packages, func(a, b *pb.FileEntry) int {
			return strings.Compare(a.GetPath(), b.GetPath())
		})
		for _, packageEntry := range packages {
			if !packageEntry.GetIsDir() {
				continue
			}
			packageID := path.Base(packageEntry.GetPath())
			if !IsValidName(packageID) {
				continue
			}
			packagePath, err := SkillPackageDirForIDs(namespaceID, packageID)
			if err != nil {
				continue
			}
			if namespaceID == UserSkillNamespace {
				userRoots = append(userRoots, Root{Path: packagePath, Kind: SourceKindManaged, Managed: true})
				continue
			}
			skills, err := client.ListDirAll(ctx, packagePath, false)
			if err != nil {
				continue
			}
			slices.SortFunc(skills, func(a, b *pb.FileEntry) int {
				return strings.Compare(a.GetPath(), b.GetPath())
			})
			for _, skillEntry := range skills {
				if !skillEntry.GetIsDir() {
					continue
				}
				skillID := path.Base(skillEntry.GetPath())
				if !IsValidName(skillID) || !HasDirectOwner(ctx, client, namespaceID, packageID, skillID) {
					continue
				}
				skillPath, err := SkillDirForIDs(namespaceID, packageID, skillID)
				if err != nil {
					continue
				}
				directRegistryRoots = append(directRegistryRoots, Root{
					Path:        skillPath,
					Kind:        SourceKindRegistry,
					Managed:     true,
					DirectOwned: true,
				})
			}
		}
	}
	return userRoots, directRegistryRoots
}
