package teams

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

type stubResolver struct {
	scope Scope
	err   error
}

func (s stubResolver) Resolve(context.Context, string) (Scope, error) { return s.scope, s.err }

func newCtx(t *testing.T) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots", nil)
	return e.NewContext(req, httptest.NewRecorder()), httptest.NewRecorder()
}

func fixedUserID(id string, err error) func(echo.Context) (string, error) {
	return func(echo.Context) (string, error) { return id, err }
}

func TestMiddlewareInjectsScopeForMember(t *testing.T) {
	c, _ := newCtx(t)
	mw := ResolveTeamMiddleware(stubResolver{scope: Scope{TeamID: DefaultTeamID, UserID: "u1", Role: "owner"}}, fixedUserID("u1", nil), nil)
	var got Scope
	err := mw(func(c echo.Context) error {
		s, e := ScopeFromContext(c.Request().Context())
		got = s
		return e
	})(c)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if got.UserID != "u1" || got.Role != "owner" {
		t.Fatalf("scope = %+v", got)
	}
}

func TestMiddlewareNonMemberReturns403(t *testing.T) {
	c, _ := newCtx(t)
	mw := ResolveTeamMiddleware(stubResolver{err: ErrNotTeamMember}, fixedUserID("u1", nil), nil)
	err := mw(func(echo.Context) error { return nil })(c)
	he := new(echo.HTTPError)
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("err = %v, want 403", err)
	}
}

func TestMiddlewareSkipperBypasses(t *testing.T) {
	c, _ := newCtx(t)
	called := false
	mw := ResolveTeamMiddleware(stubResolver{err: ErrNotTeamMember}, fixedUserID("", errors.New("no user")), func(echo.Context) bool { return true })
	err := mw(func(echo.Context) error { called = true; return nil })(c)
	if err != nil || !called {
		t.Fatalf("skipper should pass through untouched (err=%v called=%v)", err, called)
	}
}

func TestMiddlewareResolverErrorReturns500(t *testing.T) {
	c, _ := newCtx(t)
	mw := ResolveTeamMiddleware(stubResolver{err: errors.New("db down")}, fixedUserID("u1", nil), nil)
	err := mw(func(echo.Context) error { return nil })(c)
	he := new(echo.HTTPError)
	if !errors.As(err, &he) || he.Code != http.StatusInternalServerError {
		t.Fatalf("err = %v, want 500", err)
	}
}
