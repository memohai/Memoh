package auth

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
)

type SessionValidator interface {
	ValidateSession(ctx context.Context, userID string, sessionID string) error
}

func SessionMiddleware(validator SessionValidator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if validator == nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "session validator is required")
			}
			if IsChatTokenContext(c) {
				return next(c)
			}
			userID, err := UserIDFromContext(c)
			if err != nil {
				return err
			}
			sessionID, err := SessionIDFromContext(c)
			if err != nil {
				return err
			}
			if err := validator.ValidateSession(c.Request().Context(), userID, sessionID); err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
			}
			return next(c)
		}
	}
}
