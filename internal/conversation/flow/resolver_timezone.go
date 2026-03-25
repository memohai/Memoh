package flow

import (
	"context"
	"log/slog"
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
			r.logger.Warn(
				"resolve user timezone failed",
				slog.String("user_id", userID),
				slog.String("timezone", account.Timezone),
				slog.Any("error", err),
			)
		}
		return fallbackName, fallbackLocation
	}
	return name, loc
}
