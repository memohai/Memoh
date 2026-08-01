package skills

import (
	"errors"
	"path"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	// UserSkillNamespace is reserved for Skills authored from the Memoh UI.
	UserSkillNamespace = "user"
	// UserSkillPackage groups a workspace user's personal Skills.
	UserSkillPackage = "personal"
)

// SkillDirForIDs returns the canonical workspace directory for one Skill:
// /data/skills/<namespace_id>/<package_id>/<skill_id>.
func SkillDirForIDs(namespaceID, packageID, skillID string) (string, error) {
	namespaceID = strings.TrimSpace(namespaceID)
	packageID = strings.TrimSpace(packageID)
	skillID = strings.TrimSpace(skillID)
	if !IsValidName(namespaceID) || !IsValidName(packageID) || !IsValidName(skillID) {
		return "", bridge.ErrBadRequest
	}
	dirPath := path.Clean(path.Join(ManagedDirPath, namespaceID, packageID, skillID))
	if !strings.HasPrefix(dirPath, ManagedDirPath+"/") {
		return "", bridge.ErrBadRequest
	}
	return dirPath, nil
}

// SkillPackageDirForIDs is the discovery root for a Skill package.
func SkillPackageDirForIDs(namespaceID, packageID string) (string, error) {
	namespaceID = strings.TrimSpace(namespaceID)
	packageID = strings.TrimSpace(packageID)
	if !IsValidName(namespaceID) || !IsValidName(packageID) {
		return "", bridge.ErrBadRequest
	}
	dirPath := path.Clean(path.Join(ManagedDirPath, namespaceID, packageID))
	if !strings.HasPrefix(dirPath, ManagedDirPath+"/") {
		return "", bridge.ErrBadRequest
	}
	return dirPath, nil
}

// RegistrySkillIDs returns the Registry identity encoded by one canonical
// /data/skills/<registry>/<package>/<skill>/SKILL.md path.
func RegistrySkillIDs(skillMDPath string) (registryID, packageID, skillID string, ok bool) {
	registryID, packageID, skillID, ok = skillPathIDs(skillMDPath)
	if !ok || registryID == UserSkillNamespace {
		return "", "", "", false
	}
	return registryID, packageID, skillID, true
}

// RegistrySkillDirIDs returns the Registry identity encoded by one canonical
// /data/skills/<registry>/<package>/<skill> directory.
func RegistrySkillDirIDs(skillDir string) (registryID, packageID, skillID string, ok bool) {
	skillDir = path.Clean(strings.TrimSpace(skillDir))
	return RegistrySkillIDs(path.Join(skillDir, "SKILL.md"))
}

func skillNamespaceDirForID(namespaceID string) (string, error) {
	namespaceID = strings.TrimSpace(namespaceID)
	if !IsValidName(namespaceID) {
		return "", bridge.ErrBadRequest
	}
	dirPath := path.Clean(path.Join(ManagedDirPath, namespaceID))
	if dirPath == ManagedDirPath || !strings.HasPrefix(dirPath, ManagedDirPath+"/") {
		return "", bridge.ErrBadRequest
	}
	return dirPath, nil
}

func userSkillDirForName(name string) (string, error) {
	return SkillDirForIDs(UserSkillNamespace, UserSkillPackage, name)
}

// UpsertPlan is the filesystem plan for creating or updating one Skill.
type UpsertPlan struct {
	WritePath     string
	RenameFromDir string
}

// PlanUpsert resolves the target for a user edit. User Skills retain their
// canonical namespace and rename by moving their containing directory. A
// locally installed Registry Skill is edited in place.
func PlanUpsert(raw, sourcePath string) (UpsertPlan, error) {
	parsed := ParseFile(raw, "")
	if !IsValidName(parsed.Name) {
		return UpsertPlan{}, bridge.ErrBadRequest
	}
	userDir, err := userSkillDirForName(parsed.Name)
	if err != nil {
		return UpsertPlan{}, err
	}
	userWrite := path.Join(userDir, "SKILL.md")

	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return UpsertPlan{WritePath: userWrite}, nil
	}
	sourcePath = path.Clean(sourcePath)
	if path.Base(sourcePath) != "SKILL.md" || !path.IsAbs(sourcePath) {
		return UpsertPlan{}, bridge.ErrBadRequest
	}
	if oldName, ok := UserSkillName(sourcePath); ok {
		plan := UpsertPlan{WritePath: userWrite}
		if oldName != parsed.Name {
			oldDir, dirErr := userSkillDirForName(oldName)
			if dirErr != nil {
				return UpsertPlan{}, dirErr
			}
			plan.RenameFromDir = oldDir
		}
		return plan, nil
	}
	if _, ok := BuiltinSkillName(sourcePath); ok {
		return UpsertPlan{}, ErrBuiltinSkillReadOnly
	}
	if IsNamespacedSkillPath(sourcePath) {
		return UpsertPlan{WritePath: sourcePath}, nil
	}
	return UpsertPlan{WritePath: userWrite}, nil
}

// DeletableSkillDirForSourcePath resolves the directory to remove for a
// user-managed or locally installed Registry Skill.
func DeletableSkillDirForSourcePath(sourcePath string) (string, error) {
	sourcePath = path.Clean(strings.TrimSpace(sourcePath))
	if !path.IsAbs(sourcePath) || path.Base(sourcePath) != "SKILL.md" {
		return "", bridge.ErrBadRequest
	}
	if _, ok := UserSkillName(sourcePath); ok {
		return path.Dir(sourcePath), nil
	}
	if _, ok := BuiltinSkillName(sourcePath); ok {
		return "", ErrBuiltinSkillReadOnly
	}
	if IsNamespacedSkillPath(sourcePath) {
		return path.Dir(sourcePath), nil
	}
	return "", bridge.ErrBadRequest
}

// PrunableSkillNamespaceDirs returns empty package and namespace directories
// after one namespaced Skill is deleted. It never removes ManagedDirPath.
func PrunableSkillNamespaceDirs(skillDir string) []string {
	skillDir = path.Clean(strings.TrimSpace(skillDir))
	packageDir := path.Dir(skillDir)
	namespaceDir := path.Dir(packageDir)
	if path.Dir(namespaceDir) != ManagedDirPath {
		return nil
	}
	if !IsValidName(path.Base(skillDir)) || !IsValidName(path.Base(packageDir)) || !IsValidName(path.Base(namespaceDir)) {
		return nil
	}
	return []string{packageDir, namespaceDir}
}

// UserSkillName reports the name for
// /data/skills/user/personal/<skill>/SKILL.md.
func UserSkillName(skillMDPath string) (string, bool) {
	namespaceID, packageID, skillID, ok := skillPathIDs(skillMDPath)
	if !ok || namespaceID != UserSkillNamespace || packageID != UserSkillPackage {
		return "", false
	}
	return skillID, true
}

// BuiltinSkillName reports the directory name for
// /data/.memoh/skills/<name>/SKILL.md.
func BuiltinSkillName(skillMDPath string) (string, bool) {
	skillMDPath = path.Clean(strings.TrimSpace(skillMDPath))
	if path.Base(skillMDPath) != "SKILL.md" {
		return "", false
	}
	dir := path.Dir(skillMDPath)
	if path.Dir(dir) != IndexDirPath {
		return "", false
	}
	name := path.Base(dir)
	if !IsValidName(name) {
		return "", false
	}
	return name, true
}

// IsNamespacedSkillPath reports whether path follows the canonical managed
// namespace/package/Skill layout below ManagedDirPath.
func IsNamespacedSkillPath(skillMDPath string) bool {
	_, _, _, ok := skillPathIDs(skillMDPath)
	return ok
}

func skillPathIDs(skillMDPath string) (namespaceID, packageID, skillID string, ok bool) {
	skillMDPath = path.Clean(strings.TrimSpace(skillMDPath))
	if path.Base(skillMDPath) != "SKILL.md" {
		return "", "", "", false
	}
	skillDir := path.Dir(skillMDPath)
	packageDir := path.Dir(skillDir)
	namespaceDir := path.Dir(packageDir)
	if path.Dir(namespaceDir) != ManagedDirPath {
		return "", "", "", false
	}
	namespaceID, packageID, skillID = path.Base(namespaceDir), path.Base(packageDir), path.Base(skillDir)
	if !IsValidName(namespaceID) || !IsValidName(packageID) || !IsValidName(skillID) {
		return "", "", "", false
	}
	return namespaceID, packageID, skillID, true
}

var ErrBuiltinSkillReadOnly = errors.New("built-in skills are read-only")
