package flow

import (
	"context"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/timezone"
)

func (r *Resolver) resolveUserTimezone(ctx context.Context, userID string) (string, *time.Location) {
	fallbackLocation := r.clockLocation
	fallbackName := timezone.DefaultName
	if fallbackLocation != nil {
		fallbackName = fallbackLocation.String()
	} else {
		fallbackLocation = timezone.MustResolve(fallbackName)
	}

	if r.accountService == nil {
		return fallbackName, fallbackLocation
	}
	account, err := r.accountService.Get(ctx, strings.TrimSpace(userID))
	if err != nil {
		return fallbackName, fallbackLocation
	}
	if strings.TrimSpace(account.Timezone) == "" {
		return fallbackName, fallbackLocation
	}
	loc, name, err := timezone.Resolve(account.Timezone)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("resolve user timezone failed", "user_id", userID, "timezone", account.Timezone, "error", err)
		}
		return fallbackName, fallbackLocation
	}
	return name, loc
}
