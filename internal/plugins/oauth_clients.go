package plugins

import (
	"log/slog"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/oauthclients"
)

type (
	OAuthClient         = oauthclients.Client
	OAuthClientRegistry = oauthclients.Registry
)

func NewOAuthClientRegistry(log *slog.Logger, cfg config.Config) *OAuthClientRegistry {
	return oauthclients.NewRegistry(log, cfg)
}
