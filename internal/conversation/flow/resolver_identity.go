package flow

import (
	"context"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
)

// resolveDisplayName returns the best available display name for the request identity.
func (r *Resolver) resolveDisplayName(ctx context.Context, req conversation.ChatRequest) string {
	if name := strings.TrimSpace(req.DisplayName); name != "" {
		return name
	}
	if r.queries == nil {
		return "User"
	}
	channelIdentityID := strings.TrimSpace(req.SourceChannelIdentityID)
	if channelIdentityID == "" {
		return "User"
	}
	pgID, err := parseResolverUUID(channelIdentityID)
	if err != nil {
		return "User"
	}
	ci, err := r.queries.GetChannelIdentityByID(ctx, pgID)
	if err == nil && ci.DisplayName.Valid {
		if name := strings.TrimSpace(ci.DisplayName.String); name != "" {
			return name
		}
	}
	return "User"
}

func (r *Resolver) isExistingChannelIdentityID(ctx context.Context, id string) bool {
	if r.queries == nil {
		return false
	}
	pgID, err := parseResolverUUID(id)
	if err != nil {
		return false
	}
	_, err = r.queries.GetChannelIdentityByID(ctx, pgID)
	return err == nil
}

func (r *Resolver) isExistingUserID(ctx context.Context, id string) bool {
	if r.queries == nil {
		return false
	}
	pgID, err := parseResolverUUID(id)
	if err != nil {
		return false
	}
	_, err = r.queries.GetUserByID(ctx, pgID)
	return err == nil
}
