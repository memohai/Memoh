package plugins

import (
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/memohai/memoh/internal/config"
)

type OAuthClient struct {
	DisplayName           string   `toml:"display_name"`
	ClientID              string   `toml:"client_id"`
	ClientSecret          string   `toml:"client_secret"` //nolint:gosec // Server-side OAuth client secret loaded from operator config.
	AuthorizationEndpoint string   `toml:"authorization_endpoint"`
	TokenEndpoint         string   `toml:"token_endpoint"`
	RedirectURI           string   `toml:"redirect_uri"`
	AllowedScopes         []string `toml:"allowed_scopes"`
}

type OAuthClientRegistry struct {
	clients map[string]OAuthClient
	logger  *slog.Logger
}

func NewOAuthClientRegistry(log *slog.Logger, cfg config.Config) *OAuthClientRegistry {
	if log == nil {
		log = slog.Default()
	}
	registry := &OAuthClientRegistry{
		clients: map[string]OAuthClient{},
		logger:  log.With(slog.String("registry", "plugin_oauth_clients")),
	}
	path := cfg.OAuthClients.Path()
	if strings.TrimSpace(path) == "" {
		return registry
	}
	//nolint:gosec // OAuth client config path is controlled by server configuration.
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			registry.logger.Warn("failed to read OAuth clients config", slog.String("path", path), slog.Any("error", err))
		}
		return registry
	}
	var raw struct {
		Clients map[string]OAuthClient `toml:"clients"`
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		registry.logger.Warn("failed to parse OAuth clients config", slog.String("path", path), slog.Any("error", err))
		return registry
	}
	for key, client := range raw.Clients {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		client.DisplayName = expand(client.DisplayName)
		client.ClientID = expand(client.ClientID)
		client.ClientSecret = expand(client.ClientSecret)
		client.AuthorizationEndpoint = expand(client.AuthorizationEndpoint)
		client.TokenEndpoint = expand(client.TokenEndpoint)
		client.RedirectURI = expand(client.RedirectURI)
		registry.clients[key] = client
	}
	return registry
}

func (r *OAuthClientRegistry) Get(ref string) (OAuthClient, bool) {
	if r == nil {
		return OAuthClient{}, false
	}
	client, ok := r.clients[strings.TrimSpace(ref)]
	return client, ok
}

func (r *OAuthClientRegistry) HasUsableClient(ref string) bool {
	client, ok := r.Get(ref)
	return ok && strings.TrimSpace(client.ClientID) != ""
}

func expand(value string) string {
	return os.ExpandEnv(strings.TrimSpace(value))
}
