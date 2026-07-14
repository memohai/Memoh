package tenant

import (
	"context"

	"github.com/google/uuid"
)

// This package carries the request-scoped tenant identity in Go. Note the
// DATABASE-level tenant scope is enforced separately by the PostgreSQL session
// GUC app.tenant_id (bound on connect via db.SetDefaultTenantOnConnect, and per
// transaction by Cloud's trusted-context adapter): tenant queries scope
// themselves with app.current_tenant_id() and tenant_id defaults to it, so the
// Go ID here is not what filters rows — the GUC is. This context type is for
// application logic and for the adapter that sets the GUC, not a substitute for
// it.

// ID is a tenant identifier. In upstream single-tenant installs it is always
// DefaultTenantID; Cloud binds it per request from trusted context.
type ID = uuid.UUID

// DefaultTenantUUID is DefaultTenantID parsed as a uuid.UUID. Upstream code that
// needs a tenant id but has no multi-tenant context uses this singleton.
var DefaultTenantUUID = uuid.MustParse(DefaultTenantID)

type contextKey struct{}

// WithTenantID returns a context carrying the given tenant id. Callers must only
// derive the id from server-side trusted context (never from a client-supplied
// request body or query parameter).
func WithTenantID(ctx context.Context, id ID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the tenant id bound to the context, if any.
func FromContext(ctx context.Context) (ID, bool) {
	id, ok := ctx.Value(contextKey{}).(ID)
	return id, ok
}

// FromContextOrDefault returns the tenant id bound to the context, or the
// default singleton tenant when none is bound. Upstream single-tenant installs
// have no per-request tenant context, so repository/query call sites use this to
// obtain the tenant id to pass as the tenant_id query parameter. Cloud binds a
// real tenant id via WithTenantID before reaching these call sites.
func FromContextOrDefault(ctx context.Context) ID {
	if id, ok := FromContext(ctx); ok {
		return id
	}
	return DefaultTenantUUID
}
