package skills

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	DirectOwnerFileName      = ".memoh-direct-owner.json"
	maxDirectOwnerFileBytes  = 16 * 1024
	directOwnerSchemaVersion = 1
)

type DirectOwner struct {
	Version        int    `json:"version"`
	RegistryID     string `json:"registry_id"`
	PackageID      string `json:"package_id"`
	SkillID        string `json:"skill_id"`
	ArtifactDigest string `json:"artifact_digest"`
}

type directOwnerReader interface {
	ReadRaw(ctx context.Context, path string) (io.ReadCloser, error)
}

type directOwnerWriter interface {
	directOwnerReader
	WriteRaw(ctx context.Context, path string, r io.Reader) (int64, error)
	DeleteFile(ctx context.Context, path string, recursive bool) error
}

func DirectOwnerPathForIDs(registryID, packageID, skillID string) (string, error) {
	registryID, packageID, skillID, err := normalizeRegistrySkillIdentity(registryID, packageID, skillID)
	if err != nil {
		return "", err
	}
	skillDir, err := SkillDirForIDs(registryID, packageID, skillID)
	if err != nil {
		return "", bridge.ErrBadRequest
	}
	return path.Join(skillDir, DirectOwnerFileName), nil
}

func MarkDirectOwner(
	ctx context.Context,
	client directOwnerWriter,
	registryID, packageID, skillID, artifactDigest string,
) error {
	if client == nil {
		return errors.New("workspace is not reachable")
	}
	markerPath, err := DirectOwnerPathForIDs(registryID, packageID, skillID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	payload, err := directOwnerPayload(registryID, packageID, skillID, artifactDigest)
	if err != nil {
		return err
	}
	if _, err := client.WriteRaw(ctx, markerPath, bytes.NewReader(payload)); err != nil {
		return fmt.Errorf("write direct Skill owner: %w", err)
	}
	return nil
}

func HasDirectOwner(
	ctx context.Context,
	client directOwnerReader,
	registryID, packageID, skillID string,
) bool {
	_, ok, _ := ReadDirectOwner(ctx, client, registryID, packageID, skillID)
	return ok
}

func HasDirectOwnerForSourcePath(ctx context.Context, client directOwnerReader, sourcePath string) bool {
	_, ok, _ := DirectOwnerForSourcePath(ctx, client, sourcePath)
	return ok
}

func RemoveDirectOwner(
	ctx context.Context,
	client directOwnerWriter,
	registryID, packageID, skillID string,
) error {
	if client == nil {
		return errors.New("workspace is not reachable")
	}
	markerPath, err := DirectOwnerPathForIDs(registryID, packageID, skillID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	if err := client.DeleteFile(ctx, markerPath, false); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("remove direct Skill owner: %w", err)
	}
	return nil
}

// ReadDirectOwner returns a validated direct-install owner marker. Missing
// markers are not errors; unreadable or malformed markers fail closed.
func ReadDirectOwner(
	ctx context.Context,
	client directOwnerReader,
	registryID, packageID, skillID string,
) (DirectOwner, bool, error) {
	if client == nil {
		return DirectOwner{}, false, errors.New("workspace is not reachable")
	}
	registryID, packageID, skillID, err := normalizeRegistrySkillIdentity(registryID, packageID, skillID)
	if err != nil {
		return DirectOwner{}, false, errors.New("registry Skill identity is invalid")
	}
	markerPath, err := DirectOwnerPathForIDs(registryID, packageID, skillID)
	if err != nil {
		return DirectOwner{}, false, errors.New("registry Skill identity is invalid")
	}
	rc, err := client.ReadRaw(ctx, markerPath)
	if err != nil {
		if errors.Is(err, bridge.ErrNotFound) || errors.Is(err, io.EOF) {
			return DirectOwner{}, false, nil
		}
		return DirectOwner{}, false, fmt.Errorf("read direct Skill owner: %w", err)
	}
	defer func() { _ = rc.Close() }()
	payload, err := io.ReadAll(io.LimitReader(rc, maxDirectOwnerFileBytes+1))
	if err != nil {
		return DirectOwner{}, false, fmt.Errorf("read direct Skill owner: %w", err)
	}
	if len(payload) > maxDirectOwnerFileBytes {
		return DirectOwner{}, false, errors.New("direct Skill owner exceeds the size limit")
	}
	var owner DirectOwner
	if err := json.Unmarshal(payload, &owner); err != nil {
		return DirectOwner{}, false, fmt.Errorf("decode direct Skill owner: %w", err)
	}
	if owner.Version != directOwnerSchemaVersion ||
		owner.RegistryID != registryID ||
		owner.PackageID != packageID ||
		owner.SkillID != skillID {
		return DirectOwner{}, false, errors.New("direct Skill owner identity is invalid")
	}
	digest, err := hex.DecodeString(owner.ArtifactDigest)
	if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != owner.ArtifactDigest {
		return DirectOwner{}, false, errors.New("direct Skill owner Artifact digest is invalid")
	}
	return owner, true, nil
}

// DirectOwnerForSourcePath reads direct ownership for a canonical Registry
// Skill source path.
func DirectOwnerForSourcePath(
	ctx context.Context,
	client directOwnerReader,
	sourcePath string,
) (DirectOwner, bool, error) {
	registryID, packageID, skillID, ok := RegistrySkillIDs(sourcePath)
	if !ok {
		return DirectOwner{}, false, nil
	}
	return ReadDirectOwner(ctx, client, registryID, packageID, skillID)
}

func directOwnerBytes(
	ctx context.Context,
	client directOwnerReader,
	registryID, packageID, skillID string,
) ([]byte, bool, error) {
	owner, ok, err := ReadDirectOwner(ctx, client, registryID, packageID, skillID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	payload, err := directOwnerPayload(owner.RegistryID, owner.PackageID, owner.SkillID, owner.ArtifactDigest)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func directOwnerPayload(registryID, packageID, skillID, artifactDigest string) ([]byte, error) {
	registryID, packageID, skillID, err := normalizeRegistrySkillIdentity(registryID, packageID, skillID)
	if err != nil {
		return nil, errors.New("registry Skill identity is invalid")
	}
	artifactDigest = strings.TrimSpace(artifactDigest)
	digestBytes, err := hex.DecodeString(artifactDigest)
	if err != nil || len(digestBytes) != sha256.Size || hex.EncodeToString(digestBytes) != artifactDigest {
		return nil, errors.New("registry Skill Artifact digest is invalid")
	}
	payload, err := json.Marshal(DirectOwner{
		Version:        directOwnerSchemaVersion,
		RegistryID:     registryID,
		PackageID:      packageID,
		SkillID:        skillID,
		ArtifactDigest: artifactDigest,
	})
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func normalizeRegistrySkillIdentity(registryID, packageID, skillID string) (string, string, string, error) {
	registryID = strings.TrimSpace(registryID)
	packageID = strings.TrimSpace(packageID)
	skillID = strings.TrimSpace(skillID)
	if strings.EqualFold(registryID, UserSkillNamespace) {
		return "", "", "", bridge.ErrBadRequest
	}
	if _, err := SkillDirForIDs(registryID, packageID, skillID); err != nil {
		return "", "", "", bridge.ErrBadRequest
	}
	return registryID, packageID, skillID, nil
}
