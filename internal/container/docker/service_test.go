package docker

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"

	containerapi "github.com/memohai/memoh/internal/container"
)

type testStatusErr struct {
	code int
}

func (e testStatusErr) Error() string { return http.StatusText(e.code) }

func (e testStatusErr) StatusCode() int { return e.code }

func TestDockerSnapshotImageRefSanitizesRuntimeName(t *testing.T) {
	ref := dockerSnapshotImageRef("workspace/foo:snapshot@123")
	if !strings.HasPrefix(ref, snapshotImageRepository+":") {
		t.Fatalf("snapshot image ref = %q, want repo prefix", ref)
	}
	if strings.ContainsAny(strings.TrimPrefix(ref, snapshotImageRepository+":"), "/:@") {
		t.Fatalf("snapshot image ref contains invalid tag chars: %q", ref)
	}
}

func TestContainerInfoKeepsActiveStorageRefAsContainerID(t *testing.T) {
	info := containerInfoFromInspect(dockercontainer.InspectResponse{
		ContainerJSONBase: &dockercontainer.ContainerJSONBase{
			ID:      "docker-container-id",
			Name:    "/workspace-bot-1",
			Created: "2026-01-02T03:04:05Z",
		},
		Config: &dockercontainer.Config{
			Image: "debian:bookworm-slim",
			Labels: map[string]string{
				containerapi.StorageKeyLabel: "workspace-active-1",
			},
		},
	})
	if info.StorageRef.Key != "docker-container-id" {
		t.Fatalf("StorageRef.Key = %q, want container ID", info.StorageRef.Key)
	}
	if info.ID != "workspace-bot-1" {
		t.Fatalf("ID = %q, want container name", info.ID)
	}
	if info.StorageRef.Driver != "docker" {
		t.Fatalf("StorageRef.Driver = %q, want docker", info.StorageRef.Driver)
	}
	if info.StorageRef.Kind != "container" {
		t.Fatalf("StorageRef.Kind = %q, want container", info.StorageRef.Kind)
	}
	if info.Labels[containerapi.StorageKeyLabel] != "workspace-active-1" {
		t.Fatalf("storage label = %q, want workspace-active-1", info.Labels[containerapi.StorageKeyLabel])
	}
}

func TestMapDockerErrMapsConflictToAlreadyExists(t *testing.T) {
	err := mapDockerErr(testStatusErr{code: http.StatusConflict})
	if !containerapi.IsAlreadyExists(err) {
		t.Fatalf("mapDockerErr conflict = %v, want already exists", err)
	}

	err = mapDockerErr(errors.New("Conflict. The container name is already in use"))
	if !containerapi.IsAlreadyExists(err) {
		t.Fatalf("mapDockerErr text conflict = %v, want already exists", err)
	}
}

func TestDockerDoesNotExposeHostSnapshotCapabilities(t *testing.T) {
	type snapshotMountProvider interface {
		SnapshotMounts(context.Context, string, string) ([]containerapi.MountInfo, error)
	}
	var svc any = &Service{}
	if _, ok := svc.(snapshotMountProvider); ok {
		t.Fatal("docker service should not expose host-side snapshot mounts")
	}
}
