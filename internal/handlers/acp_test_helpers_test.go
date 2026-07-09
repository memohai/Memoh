package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/teams"
)

func testUUID(value string) pgtype.UUID {
	var out pgtype.UUID
	if err := out.Scan(value); err != nil {
		panic(err)
	}
	return out
}

func testJSON(value map[string]any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

// testAuthContext builds an Echo context with a JWT token for userID.
// The team scope role is left empty (no admin). Use testAuthContextWithRole for
// tests that need a specific role injected into the team scope.
func testAuthContext(e *echo.Echo, req *http.Request, rec http.ResponseWriter, userID string) echo.Context {
	return testAuthContextWithRole(e, req, rec, userID, "")
}

// testAuthContextWithRole builds an Echo context with a JWT token and a team
// scope carrying the given role ("admin", "owner", "member", etc.). Pass an
// empty role to omit the team scope (anonymous / no admin).
func testAuthContextWithRole(e *echo.Echo, req *http.Request, rec http.ResponseWriter, userID, role string) echo.Context {
	ctx := e.NewContext(req, rec)
	ctx.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"sub":     userID,
			"user_id": userID,
		},
	})
	if role != "" {
		scopedCtx := teams.WithScope(req.Context(), teams.Scope{
			TeamID: teams.DefaultTeamID,
			UserID: userID,
			Role:   role,
		})
		ctx.SetRequest(req.WithContext(scopedCtx))
	}
	return ctx
}

// newTestAdminAccountService returns an accounts.Service backed by a fake store.
// Note: accounts.Service.IsAdmin now reads the team scope from context rather
// than querying the store, so this service is mainly used for AuthorizeAccess
// helpers that call GetByUserID. The role parameter is kept for call-site
// compatibility but is no longer stored.
func newTestAdminAccountService(_ string) *accounts.Service {
	return accounts.NewService(nil, testAdminAccountStore{})
}

type testAdminAccountStore struct {
	dbstore.AccountStore
}

func (testAdminAccountStore) GetByUserID(_ context.Context, _ string) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{IsActive: true}, nil
}
