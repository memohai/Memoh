package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"

	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/workspace"
)

func TestBotRemoteRuntimeHTTPErrorDistinguishesUnusableRuntime(t *testing.T) {
	err := botRemoteRuntimeHTTPError(nil, workspace.ErrRemoteRuntimeNotUsable)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
		t.Fatalf("error = %v, want HTTP 404", err)
	}
	if httpErr.Message == "remote runtime binding not found" {
		t.Fatal("an unusable runtime must not be reported as a missing binding")
	}
}

type fakeWorkspaceStopper struct {
	calls int
	err   error
}

func (s *fakeWorkspaceStopper) StopBot(context.Context, string) error {
	s.calls++
	return s.err
}

func TestStopStaleContainerIsBestEffort(t *testing.T) {
	for name, stopErr := range map[string]error{
		"stopped":        nil,
		"already bound":  ctr.ErrNotSupported,
		"no container":   workspace.ErrContainerNotFound,
		"backend failed": errors.New("containerd unavailable"),
	} {
		t.Run(name, func(t *testing.T) {
			stopper := &fakeWorkspaceStopper{err: stopErr}
			handler := &BotRemoteRuntimeHandler{log: slog.Default(), workspaces: stopper}
			handler.stopStaleContainer(context.Background(), "bot-1")
			if stopper.calls != 1 {
				t.Fatalf("StopBot calls = %d, want 1", stopper.calls)
			}
		})
	}

	// A handler without a workspace manager must not panic.
	(&BotRemoteRuntimeHandler{log: slog.Default()}).stopStaleContainer(context.Background(), "bot-1")
}
