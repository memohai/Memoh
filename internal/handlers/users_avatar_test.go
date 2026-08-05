package handlers

import (
	"context"
	"testing"

	"github.com/memohai/memoh/internal/apperror"
	avatarpkg "github.com/memohai/memoh/internal/avatar"
	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/storage/providers/localfs"
)

func TestUsersHandlerMapsInvalidAvatarToStableCode(t *testing.T) {
	t.Parallel()

	handler := &UsersHandler{
		avatarService: avatarpkg.NewService(media.NewService(nil, localfs.New(t.TempDir()))),
	}
	_, err := handler.storeAvatarURL(context.Background(), "data:text/plain;base64,aGVsbG8=")
	if got := apperror.CodeOf(err); got != apperror.CodeAvatarInvalid {
		t.Fatalf("error code = %q, want %q", got, apperror.CodeAvatarInvalid)
	}
}
