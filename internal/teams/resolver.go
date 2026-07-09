package teams

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotTeamMember is returned when the caller is not a member of the resolved team.
var ErrNotTeamMember = errors.New("user is not a member of the team")

// MembershipReader looks up a user's role within a team. found=false means the
// user has no membership row (not an error).
type MembershipReader interface {
	Membership(ctx context.Context, teamID, userID string) (role string, found bool, err error)
}

// TeamResolver resolves and authorizes the acting team for a request.
// SaaS supplies its own implementation; open-source uses SingleTeamResolver.
type TeamResolver interface {
	Resolve(ctx context.Context, userID string) (Scope, error)
}

// SingleTeamResolver resolves every caller to the single default team and
// requires the caller to be a member of it.
type SingleTeamResolver struct {
	members MembershipReader
}

func NewSingleTeamResolver(members MembershipReader) *SingleTeamResolver {
	return &SingleTeamResolver{members: members}
}

func (r *SingleTeamResolver) Resolve(ctx context.Context, userID string) (Scope, error) {
	role, found, err := r.members.Membership(ctx, DefaultTeamID, userID)
	if err != nil {
		return Scope{}, fmt.Errorf("resolve team membership: %w", err)
	}
	if !found {
		return Scope{}, ErrNotTeamMember
	}
	return Scope{TeamID: DefaultTeamID, UserID: userID, Role: role}, nil
}
