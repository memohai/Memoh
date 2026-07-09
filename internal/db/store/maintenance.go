package store

// MaintenanceQueries is a distinct type wrapping the transitional Queries
// interface for the maintenance (owner / BYPASSRLS) database pool.
//
// It exists so dependency injection can distinguish the restricted, per-request
// Queries (which runs under the non-owner memoh_app role and sets app.team_id
// per statement so FORCE ROW LEVEL SECURITY isolates teams) from the owner-role
// Queries used by the small, enumerated set of process-wide startup paths that
// legitimately span all teams:
//
//   - ListAutoStartContainers (workspace container reconcile)
//   - ListEnabledSchedules (schedule bootstrap)
//   - ListHeartbeatEnabledBots (heartbeat bootstrap)
//   - ListBotChannelConfigsByType (channel manager refresh)
//
// Under FORCE RLS the restricted role returns zero rows for these all-team
// reads (app.team_id can only ever name one team), so they must run on the
// owner pool, which bypasses RLS. Startup writes on the reconcile path run on
// the same maintenance pool for simplicity and correctness (owner bypasses
// RLS), per the RLS enforcement plan.
type MaintenanceQueries struct {
	Queries
}

// NewMaintenanceQueries wraps an owner-pool Queries as MaintenanceQueries.
func NewMaintenanceQueries(q Queries) MaintenanceQueries {
	return MaintenanceQueries{Queries: q}
}
