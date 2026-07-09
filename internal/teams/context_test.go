package teams

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestDefaultScopeRoundTripsThroughContext(t *testing.T) {
	ctx := WithScope(context.Background(), DefaultScope())

	got, err := ScopeFromContext(ctx)
	if err != nil {
		t.Fatalf("ScopeFromContext returned error: %v", err)
	}
	if got.TeamID != DefaultTeamID {
		t.Fatalf("team id = %q, want %q", got.TeamID, DefaultTeamID)
	}
}

func TestScopeSurvivesWithoutCancel(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{TeamID: DefaultTeamID})

	got, err := ScopeFromContext(context.WithoutCancel(ctx))
	if err != nil {
		t.Fatalf("ScopeFromContext returned error: %v", err)
	}
	if got.TeamID != DefaultTeamID {
		t.Fatalf("team id = %q, want %q", got.TeamID, DefaultTeamID)
	}
}

func TestScopeFromContextRequiresScope(t *testing.T) {
	if _, err := ScopeFromContext(context.Background()); err == nil {
		t.Fatal("expected missing scope error")
	}
}

func TestDefaultMiddlewareInjectsTeamScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var got Scope
	handler := DefaultMiddleware()(func(c echo.Context) error {
		scope, err := ScopeFromContext(c.Request().Context())
		if err != nil {
			return err
		}
		got = scope
		return c.NoContent(http.StatusNoContent)
	})

	if err := handler(c); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if got.TeamID != DefaultTeamID {
		t.Fatalf("team id = %q, want %q", got.TeamID, DefaultTeamID)
	}
}
