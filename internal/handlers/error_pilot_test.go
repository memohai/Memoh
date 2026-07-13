package handlers

import (
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestCreateBotHTTPErrorMapsNameConflictToStableCode(t *testing.T) {
	err := createBotHTTPError(bots.ErrBotNameTaken, true)
	if got := apperror.CodeOf(err); got != apperror.CodeBotNameTaken {
		t.Fatalf("code = %q, want %q", got, apperror.CodeBotNameTaken)
	}
	if got := apperror.ArgsOf(err)["field"]; got != "name" {
		t.Fatalf("field arg = %q, want name", got)
	}
}

func TestUpdateBotHTTPErrorMapsNameConflictToStableCode(t *testing.T) {
	err := updateBotHTTPError(bots.ErrBotNameTaken)
	if got := apperror.CodeOf(err); got != apperror.CodeBotNameTaken {
		t.Fatalf("code = %q, want %q", got, apperror.CodeBotNameTaken)
	}
}

func TestFSHTTPErrorKeepsUnavailableCausePrivate(t *testing.T) {
	cause := errors.Join(bridge.ErrUnavailable, errors.New("connection refused"))
	err := fsHTTPError(cause)
	if got := apperror.CodeOf(err); got != apperror.CodeWorkspaceUnreachable {
		t.Fatalf("code = %q, want %q", got, apperror.CodeWorkspaceUnreachable)
	}
	if got := apperror.CauseOf(err); !errors.Is(got, bridge.ErrUnavailable) {
		t.Fatalf("cause = %v, want bridge unavailable", got)
	}
}

func TestDisplayPrepareAppErrorUsesSharedWorkspaceCode(t *testing.T) {
	event := newDisplayPrepareAppError(
		"checking",
		apperror.Wrap(apperror.CodeWorkspaceUnreachable, errors.New("private cause"), nil),
		"req-1",
	)
	if event.Code != string(apperror.CodeWorkspaceUnreachable) {
		t.Fatalf("code = %q", event.Code)
	}
	if event.I18nKey != "" {
		t.Fatalf("new AppError event exposed i18n_key = %q", event.I18nKey)
	}
	if event.Message != "The workspace could not be reached." {
		t.Fatalf("message = %q", event.Message)
	}
	if event.Detail != event.Message {
		t.Fatalf("detail = %q, message = %q", event.Detail, event.Message)
	}
	if event.RequestID != "req-1" {
		t.Fatalf("request_id = %q", event.RequestID)
	}
}
