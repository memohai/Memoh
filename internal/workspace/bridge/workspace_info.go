package bridge

import "context"

const (
	WorkspaceBackendContainer = "container"
	WorkspaceBackendLocal     = "local"
)

type WorkspaceInfo struct {
	Backend        string
	DefaultWorkDir string
}

type WorkspaceInfoProvider interface {
	WorkspaceInfo(ctx context.Context, botID string) (WorkspaceInfo, error)
}
