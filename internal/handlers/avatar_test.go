package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	avatarpkg "github.com/memohai/memoh/internal/avatar"
	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/storage/providers/localfs"
)

func TestAvatarHandlerServesPrivateStoredAvatar(t *testing.T) {
	t.Parallel()

	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	service := avatarpkg.NewService(media.NewService(nil, localfs.New(t.TempDir())))
	stored, err := service.StoreDataURL(
		context.Background(),
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString(png),
	)
	if err != nil {
		t.Fatalf("StoreDataURL() error = %v", err)
	}

	e := echo.New()
	NewAvatarHandler(nil, service).Register(e)
	req := httptest.NewRequest(http.MethodGet, stored.URL, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get(echo.HeaderContentType) != "image/png" {
		t.Fatalf("content type = %q", rec.Header().Get(echo.HeaderContentType))
	}
	if rec.Header().Get(echo.HeaderCacheControl) != "private, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", rec.Header().Get(echo.HeaderCacheControl))
	}
	if rec.Body.String() != string(png) {
		t.Fatalf("body = %x, want %x", rec.Body.Bytes(), png)
	}
}
