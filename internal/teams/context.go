package teams

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	DefaultTeamID   = "00000000-0000-0000-0000-000000000001"
	DefaultTeamSlug = "default"
)

var ErrMissingScope = errors.New("team scope missing from context")

type Scope struct {
	TeamID string
	// UserID is the acting principal; empty for system/background contexts.
	UserID string
	// Role is the caller's team_members.role in TeamID: owner|admin|member.
	Role string
}

type contextKey struct{}

func DefaultScope() Scope {
	return Scope{TeamID: DefaultTeamID}
}

func (s Scope) IsZero() bool {
	return strings.TrimSpace(s.TeamID) == ""
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, scope)
}

func ScopeFromContext(ctx context.Context) (Scope, error) {
	if ctx == nil {
		return Scope{}, ErrMissingScope
	}
	scope, ok := ctx.Value(contextKey{}).(Scope)
	if !ok || scope.IsZero() {
		return Scope{}, ErrMissingScope
	}
	return scope, nil
}

func ScopeOrDefault(ctx context.Context) Scope {
	scope, err := ScopeFromContext(ctx)
	if err != nil {
		return DefaultScope()
	}
	return scope
}

func DefaultMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c == nil || c.Request() == nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "request context not configured")
			}
			req := c.Request()
			c.SetRequest(req.WithContext(WithScope(req.Context(), DefaultScope())))
			return next(c)
		}
	}
}
