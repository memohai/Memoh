package workspace

import ctr "github.com/memohai/memoh/internal/container"

// runtimeService is the workspace-facing façade over the container runtime.
// Workspace depends only on the capability groups it actually consumes.
type runtimeService interface {
	ctr.ImageService
	ctr.ContainerService
	ctr.TaskService
	ctr.NetworkService
	ctr.SnapshotService
}
