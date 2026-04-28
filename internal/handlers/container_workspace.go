package handlers

import (
	"context"
	"io"

	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

// containerWorkspace captures the subset of workspace capabilities required by
// container-related HTTP handlers. Keeping this private prevents handlers from
// depending on the full concrete workspace manager surface.
type containerWorkspace interface {
	bridge.Provider
	EnsureRunning(ctx context.Context, botID string) error
	ContainerID(ctx context.Context, botID string) (string, error)
	ResolveWorkspaceImage(ctx context.Context, botID string) (string, error)
	ResolveWorkspaceGPU(ctx context.Context, botID string) (workspace.WorkspaceGPUConfig, error)
	PullImage(ctx context.Context, image string, opts *ctr.PullImageOptions) (ctr.ImageInfo, error)
	HasPreservedData(botID string) bool
	StartWithResolvedConfig(ctx context.Context, botID, image string, gpu workspace.WorkspaceGPUConfig) error
	RememberWorkspaceImage(ctx context.Context, botID, image string) error
	RememberWorkspaceGPU(ctx context.Context, botID string, gpu workspace.WorkspaceGPUConfig) error
	RestorePreservedData(ctx context.Context, botID string) error
	RecordContainerRunning(ctx context.Context, botID, containerID, image string)
	GetContainerInfo(ctx context.Context, botID string) (*workspace.ContainerStatus, error)
	CleanupBotContainer(ctx context.Context, botID string, preserveData bool) error
	StopBot(ctx context.Context, botID string) error
	CreateSnapshot(ctx context.Context, botID, snapshotName, source string) (*workspace.SnapshotCreateInfo, error)
	ListBotSnapshotData(ctx context.Context, botID string) (*workspace.BotSnapshotData, error)
	RollbackVersion(ctx context.Context, botID string, version int) error
	ExportData(ctx context.Context, botID string) (io.ReadCloser, error)
	ImportData(ctx context.Context, botID string, r io.Reader) error
}
