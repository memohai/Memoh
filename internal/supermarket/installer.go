package supermarket

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	pluginspkg "github.com/memohai/memoh/internal/plugins"
	"github.com/memohai/memoh/internal/skillpackages"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const maxConcurrentPackagePreparations = 2

var packagePreparationTokens = make(chan struct{}, maxConcurrentPackagePreparations)

type PluginInstaller interface {
	WithBotMutation(context.Context, string, func(context.Context) error) error
	Install(context.Context, string, pluginspkg.InstallRequest) (pluginspkg.Installation, error)
	InstalledPluginState(context.Context, string, string) (pluginspkg.InstalledPluginState, bool, error)
}

type Installer struct {
	client     *Client
	plugins    PluginInstaller
	packages   *skillpackages.Service
	containers bridge.Provider
	workspaces *workspace.Manager
	logger     *slog.Logger
}

func NewInstaller(client *Client, plugins PluginInstaller, packages *skillpackages.Service, containers bridge.Provider, workspaces *workspace.Manager, logger *slog.Logger) *Installer {
	return &Installer{
		client: client, plugins: plugins, packages: packages, containers: containers, workspaces: workspaces, logger: logger,
	}
}

type StatusError struct {
	Status  int
	Message string
	Err     error
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return http.StatusText(e.Status)
}

func (e *StatusError) Unwrap() error { return e.Err }

type WorkspaceTargetError struct{ Err error }

func (e *WorkspaceTargetError) Error() string { return e.Err.Error() }
func (e *WorkspaceTargetError) Unwrap() error { return e.Err }

func withStatus(status int, err error) error { return &StatusError{Status: status, Err: err} }

func (i *Installer) withBotMutation(ctx context.Context, botID string, fn func(context.Context) error) error {
	if i.plugins == nil {
		return fn(ctx)
	}
	return i.plugins.WithBotMutation(ctx, botID, fn)
}

func (i *Installer) acquirePreparation(ctx context.Context) (func(), error) {
	if i == nil {
		return nil, errors.New("supermarket installer is not configured")
	}
	select {
	case packagePreparationTokens <- struct{}{}:
		return func() { <-packagePreparationTokens }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
