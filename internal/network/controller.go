package network

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotSupported indicates the selected runtime or overlay driver cannot
// report a requested capability yet.
var ErrNotSupported = errors.New("network operation not supported")

// Controller coordinates container runtime networking and optional overlay
// attachment behind a single workspace-facing surface.
type Controller interface {
	EnsureAttached(ctx context.Context, req AttachmentRequest) (AttachmentStatus, error)
	Detach(ctx context.Context, req AttachmentRequest) error
	Status(ctx context.Context, req AttachmentRequest) (AttachmentStatus, error)
}

type controller struct {
	runtime Runtime
	overlay OverlayDriver
}

// NewController creates a controller with the given runtime and optional
// overlay driver. When overlay is nil, a no-op overlay driver is used.
func NewController(runtime Runtime, overlay OverlayDriver) Controller {
	if overlay == nil {
		overlay = NoopOverlayDriver{}
	}
	return &controller{
		runtime: runtime,
		overlay: overlay,
	}
}

func (c *controller) EnsureAttached(ctx context.Context, req AttachmentRequest) (AttachmentStatus, error) {
	status := AttachmentStatus{}
	runtimeReady := false
	rollbackRuntime := func(cause error) error {
		if !runtimeReady || c.runtime == nil {
			return cause
		}
		if err := c.runtime.RemoveNetwork(ctx, req.Runtime); err != nil {
			return errors.Join(cause, fmt.Errorf("rollback runtime network: %w", err))
		}
		return cause
	}
	if c.runtime != nil {
		runtimeStatus, err := c.runtime.EnsureNetwork(ctx, req.Runtime)
		if err != nil {
			return AttachmentStatus{}, err
		}
		status.Runtime = runtimeStatus
		runtimeReady = true
	}
	if c.overlay != nil {
		overlayStatus, err := c.overlay.EnsureAttached(ctx, req)
		if err != nil {
			return AttachmentStatus{}, rollbackRuntime(err)
		}
		status.Overlay = overlayStatus
	}
	return status, nil
}

func (c *controller) Detach(ctx context.Context, req AttachmentRequest) error {
	if c.overlay != nil {
		if err := c.overlay.Detach(ctx, req); err != nil {
			return err
		}
	}
	if c.runtime != nil {
		return c.runtime.RemoveNetwork(ctx, req.Runtime)
	}
	return nil
}

func (c *controller) Status(ctx context.Context, req AttachmentRequest) (AttachmentStatus, error) {
	status := AttachmentStatus{}
	if c.runtime != nil {
		runtimeStatus, err := c.runtime.StatusNetwork(ctx, req.Runtime)
		if err != nil && !errors.Is(err, ErrNotSupported) {
			return AttachmentStatus{}, err
		}
		if err == nil {
			status.Runtime = runtimeStatus
		}
	}
	if c.overlay != nil {
		overlayStatus, err := c.overlay.Status(ctx, req)
		if err != nil && !errors.Is(err, ErrNotSupported) {
			return AttachmentStatus{}, err
		}
		if err == nil {
			status.Overlay = overlayStatus
		}
	}
	return status, nil
}
