package teams

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ResolveTeamMiddleware resolves the acting team for authenticated requests and
// rejects non-members. It must be registered AFTER the auth middleware so
// userID(c) can read the authenticated principal. skipper (may be nil) marks
// unauthenticated routes that carry no user and must pass through untouched.
func ResolveTeamMiddleware(
	resolver TeamResolver,
	userID func(echo.Context) (string, error),
	skipper func(echo.Context) bool,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper != nil && skipper(c) {
				return next(c)
			}
			uid, err := userID(c)
			if err != nil || uid == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "unauthenticated")
			}
			scope, err := resolver.Resolve(c.Request().Context(), uid)
			if err != nil {
				if errors.Is(err, ErrNotTeamMember) {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this team")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "team resolution failed")
			}
			req := c.Request()
			c.SetRequest(req.WithContext(WithScope(req.Context(), scope)))
			return next(c)
		}
	}
}
