package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type stubSessionValidator struct {
	err       error
	userID    string
	sessionID string
	calls     int
}

func (s *stubSessionValidator) ValidateSession(_ context.Context, userID string, sessionID string) error {
	s.calls++
	s.userID = userID
	s.sessionID = sessionID
	return s.err
}

func TestSessionMiddlewareValidatesSession(t *testing.T) {
	validator := &stubSessionValidator{}
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimUserID:    "user-1",
			claimSessionID: "session-1",
		},
	})

	called := false
	handler := SessionMiddleware(validator)(func(echo.Context) error {
		called = true
		return nil
	})
	if err := handler(c); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	if validator.calls != 1 || validator.userID != "user-1" || validator.sessionID != "session-1" {
		t.Fatalf("validator call = (%d, %q, %q)", validator.calls, validator.userID, validator.sessionID)
	}
}

func TestSessionMiddlewareRejectsInvalidSession(t *testing.T) {
	validator := &stubSessionValidator{err: errors.New("revoked")}
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimUserID:    "user-1",
			claimSessionID: "session-1",
		},
	})

	err := SessionMiddleware(validator)(func(echo.Context) error {
		t.Fatal("next handler should not be called")
		return nil
	})(c)
	if err == nil {
		t.Fatal("middleware returned nil error")
	}
	var httpErr *echo.HTTPError
	ok := errors.As(err, &httpErr)
	if !ok {
		t.Fatalf("error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", httpErr.Code)
	}
}

func TestSessionMiddlewareSkipsChatToken(t *testing.T) {
	validator := &stubSessionValidator{err: errors.New("should not be called")}
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimType:   chatTokenType,
			claimUserID: "channel-user-1",
		},
	})

	called := false
	err := SessionMiddleware(validator)(func(echo.Context) error {
		called = true
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	if validator.calls != 0 {
		t.Fatalf("validator calls = %d, want 0", validator.calls)
	}
}
