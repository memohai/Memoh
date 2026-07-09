package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/teams"
)

func mount(q membershipQuery, uid string) *echo.Echo {
	e := echo.New()
	resolver := teams.NewSingleTeamResolver(membershipReader{q: q})
	e.Use(teams.ResolveTeamMiddleware(resolver, func(echo.Context) (string, error) { return uid, nil }, nil))
	e.GET("/whoami", func(c echo.Context) error {
		s, _ := teams.ScopeFromContext(c.Request().Context())
		return c.String(http.StatusOK, s.Role)
	})
	return e
}

const testUserID = "00000000-0000-0000-0000-000000000099"

func TestE2EMemberPasses(t *testing.T) {
	e := mount(fakeMembershipQueries{row: dbsqlc.GetTeamMembershipRow{Role: "owner"}}, testUserID)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/whoami", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "owner" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestE2ENonMemberForbidden(t *testing.T) {
	e := mount(fakeMembershipQueries{err: pgx.ErrNoRows}, testUserID)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/whoami", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, want 403", rec.Code)
	}
}
