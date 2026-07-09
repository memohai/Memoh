package registry

import (
	"context"

	"github.com/memohai/memoh/internal/models"
)

// setRegistryTeamID injects the active tenant scope into a sqlc params struct
// so registry sync writes land in the caller's team instead of silently
// falling back to the open-source default team via the query-side COALESCE.
func setRegistryTeamID(ctx context.Context, arg any) {
	models.SetTeamIDParam(arg, models.TeamIDFromContext(ctx))
}
