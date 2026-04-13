package providers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	memohcopilot "github.com/memohai/memoh/internal/copilot"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
)

const (
	defaultOpenAICodexClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultOpenAIAuthorizeURL     = "https://auth.openai.com/oauth/authorize"
	defaultOpenAITokenURL         = "https://auth.openai.com/oauth/token" //nolint:gosec // OAuth endpoint URL, not a credential
	defaultOpenAICallbackURL      = "http://localhost:1455/auth/callback"
	defaultOpenAIOAuthScopes      = "openid profile email offline_access"
	defaultGitHubDeviceCodeURL    = "https://github.com/login/device/code"        //nolint:gosec // OAuth endpoint URL, not a credential
	defaultGitHubTokenURL         = "https://github.com/login/oauth/access_token" //nolint:gosec // OAuth endpoint URL, not a credential
	defaultGitHubUserURL          = "https://api.github.com/user"                 //nolint:gosec // OAuth endpoint URL, not a credential
	defaultGitHubUserEmailsURL    = "https://api.github.com/user/emails"          //nolint:gosec // OAuth endpoint URL, not a credential
	oauthExpirySkew               = 30 * time.Second
	providerOAuthHTTPTimeout      = 15 * time.Second
	metadataOAuthClientIDKey      = "oauth_client_id"
	metadataOAuthAuthorizeURLKey  = "oauth_authorize_url"
	metadataOAuthDeviceCodeURLKey = "oauth_device_code_url"
	metadataOAuthTokenURLKey      = "oauth_token_url" //nolint:gosec // metadata key name, not a credential
	metadataOAuthRedirectURIKey   = "oauth_redirect_uri"
	metadataOAuthScopesKey        = "oauth_scopes"
	metadataOAuthAudienceKey      = "oauth_audience"
	metadataOAuthUseIDOrgsFlagKey = "oauth_id_token_add_organizations"
	metadataDeviceCodeKey         = "device_code"
	metadataDeviceUserCodeKey     = "device_user_code"
	metadataDeviceVerifyURIKey    = "device_verification_uri"
	metadataDeviceIntervalKey     = "device_interval_seconds"
	metadataDeviceExpiresAtKey    = "device_expires_at"
	metadataAccountLabelKey       = "account_label"
	metadataAccountLoginKey       = "account_login"
	metadataAccountNameKey        = "account_name"
	metadataAccountEmailKey       = "account_email"
	metadataAccountAvatarURLKey   = "account_avatar_url"
	metadataAccountProfileURLKey  = "account_profile_url"
	configOAuthClientSecretKey    = "oauth_client_secret" //nolint:gosec // Metadata key name, not a credential literal.
)

type oauthTokenRecord struct {
	ProviderID       string
	UserID           string
	AccessToken      string //nolint:gosec // Runtime token payload persisted encrypted at rest.
	RefreshToken     string //nolint:gosec // Runtime token payload persisted encrypted at rest.
	ExpiresAt        time.Time
	Scope            string
	TokenType        string
	State            string
	PKCECodeVerifier string
	Metadata         map[string]any
}

type oauthConfig struct {
	ClientType              models.ClientType
	ClientID                string
	ClientSecret            string //nolint:gosec // Runtime OAuth client secret from provider metadata.
	AuthorizeURL            string
	DeviceCodeURL           string
	TokenURL                string
	RedirectURI             string
	Scopes                  string
	Audience                string
	UsePKCE                 bool
	IDTokenAddOrganizations bool
}

type deviceAuthorizationResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int64  `json:"expires_in"`
	Interval        int64  `json:"interval"`
	Error           string `json:"error"`
	Description     string `json:"error_description"`
}

func providerMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return map[string]any{}
	}
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func oauthLogAttrs(providerID, userID string, err error) []any {
	attrs := []any{}
	if strings.TrimSpace(providerID) != "" {
		attrs = append(attrs, slog.String("provider_id", providerID))
	}
	if strings.TrimSpace(userID) != "" {
		attrs = append(attrs, slog.String("user_id", userID))
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	return attrs
}

func (s *Service) oauthConfigForProvider(provider sqlc.Provider) oauthConfig {
	metadata := providerMetadata(provider.Metadata)

	switch models.ClientType(provider.ClientType) {
	case models.ClientTypeGitHubCopilot:
		result := oauthConfig{
			ClientType:    models.ClientTypeGitHubCopilot,
			ClientID:      memohcopilot.GitHubOAuthClientID,
			DeviceCodeURL: defaultGitHubDeviceCodeURL,
			TokenURL:      defaultGitHubTokenURL,
			Scopes:        memohcopilot.GitHubOAuthScope,
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthDeviceCodeURLKey)); v != "" {
			result.DeviceCodeURL = v
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthTokenURLKey)); v != "" {
			result.TokenURL = v
		}
		return result

	default:
		result := oauthConfig{
			ClientType:              models.ClientTypeOpenAICodex,
			ClientID:                defaultOpenAICodexClientID,
			AuthorizeURL:            defaultOpenAIAuthorizeURL,
			TokenURL:                defaultOpenAITokenURL,
			RedirectURI:             firstNonEmpty(s.callbackURL, defaultOpenAICallbackURL),
			Scopes:                  defaultOpenAIOAuthScopes,
			Audience:                strings.TrimSpace(stringValue(metadata, metadataOAuthAudienceKey)),
			UsePKCE:                 true,
			IDTokenAddOrganizations: true,
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthClientIDKey)); v != "" {
			result.ClientID = v
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthAuthorizeURLKey)); v != "" {
			result.AuthorizeURL = v
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthTokenURLKey)); v != "" {
			result.TokenURL = v
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthRedirectURIKey)); v != "" {
			result.RedirectURI = v
		}
		if v := strings.TrimSpace(stringValue(metadata, metadataOAuthScopesKey)); v != "" {
			result.Scopes = v
		}
		if v, ok := metadata[metadataOAuthUseIDOrgsFlagKey].(bool); ok {
			result.IDTokenAddOrganizations = v
		}
		return result
	}
}

func supportsOAuth(provider sqlc.Provider) bool {
	switch models.ClientType(provider.ClientType) {
	case models.ClientTypeOpenAICodex, models.ClientTypeGitHubCopilot:
		return true
	default:
		return false
	}
}

func isUserScopedOAuthProvider(provider sqlc.Provider) bool {
	return models.ClientType(provider.ClientType) == models.ClientTypeGitHubCopilot
}

func (s *Service) StartOAuthAuthorization(ctx context.Context, providerID string) (*OAuthAuthorizeResponse, error) {
	provider, err := s.loadOAuthProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	cfg := s.oauthConfigForProvider(provider)

	if isUserScopedOAuthProvider(provider) {
		userID := oauthctx.UserIDFromContext(ctx)
		if userID == "" {
			return nil, errors.New("github copilot oauth requires a current user")
		}
		device, err := s.startGitHubDeviceAuthorization(ctx, providerID, userID, cfg)
		if err != nil {
			return nil, err
		}
		return &OAuthAuthorizeResponse{
			Mode:   "device",
			Device: device,
		}, nil
	}

	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {cfg.ClientID},
		"redirect_uri":  {cfg.RedirectURI},
		"state":         {state},
	}
	if cfg.Scopes != "" {
		params.Set("scope", cfg.Scopes)
	}
	if cfg.Audience != "" {
		params.Set("audience", cfg.Audience)
	}

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}
	if err := s.updateOAuthState(ctx, providerID, state, codeVerifier); err != nil {
		return nil, err
	}
	params.Set("scope", cfg.Scopes)
	params.Set("code_challenge", computeCodeChallenge(codeVerifier))
	params.Set("code_challenge_method", "S256")
	if cfg.IDTokenAddOrganizations {
		params.Set("id_token_add_organizations", "true")
	}
	params.Set("codex_cli_simplified_flow", "true")

	return &OAuthAuthorizeResponse{
		Mode:    "web",
		AuthURL: cfg.AuthorizeURL + "?" + params.Encode(),
	}, nil
}

func (s *Service) HandleOAuthCallback(ctx context.Context, state, code string) (string, error) {
	if userToken, err := s.getUserOAuthTokenByState(ctx, state); err == nil {
		return s.handleUserScopedOAuthCallback(ctx, userToken, code)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	token, err := s.getOAuthTokenByState(ctx, state)
	if err != nil {
		return "", err
	}
	providerUUID, err := db.ParseUUID(token.ProviderID)
	if err != nil {
		return "", err
	}
	provider, err := s.queries.GetProviderByID(ctx, providerUUID)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	if !supportsOAuth(provider) {
		return "", errors.New("provider does not support oauth")
	}

	cfg := s.oauthConfigForProvider(provider)
	resp, err := s.exchangeCode(ctx, cfg, code, token.PKCECodeVerifier)
	if err != nil {
		return "", err
	}
	if err := s.saveOAuthToken(ctx, provider.ID.String(), oauthTokenRecord{
		ProviderID:       provider.ID.String(),
		AccessToken:      resp.AccessToken,
		RefreshToken:     firstNonEmpty(resp.RefreshToken, token.RefreshToken),
		ExpiresAt:        expiresAtFromNow(resp.ExpiresIn),
		Scope:            firstNonEmpty(resp.Scope, cfg.Scopes),
		TokenType:        firstNonEmpty(resp.TokenType, "Bearer"),
		State:            "",
		PKCECodeVerifier: "",
	}); err != nil {
		return "", err
	}
	return provider.ID.String(), nil
}

func (s *Service) handleUserScopedOAuthCallback(ctx context.Context, token *oauthTokenRecord, code string) (string, error) {
	providerUUID, err := db.ParseUUID(token.ProviderID)
	if err != nil {
		return "", err
	}
	provider, err := s.queries.GetProviderByID(ctx, providerUUID)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	if !isUserScopedOAuthProvider(provider) {
		return "", errors.New("provider does not use user-scoped oauth")
	}

	cfg := s.oauthConfigForProvider(provider)
	resp, err := s.exchangeCode(ctx, cfg, code, token.PKCECodeVerifier)
	if err != nil {
		return "", err
	}
	if err := s.saveUserOAuthToken(ctx, token.ProviderID, token.UserID, oauthTokenRecord{
		ProviderID:  token.ProviderID,
		UserID:      token.UserID,
		AccessToken: resp.AccessToken,
		RefreshToken: firstNonEmpty(
			resp.RefreshToken,
			token.RefreshToken,
		),
		ExpiresAt:        expiresAtFromNow(resp.ExpiresIn),
		Scope:            firstNonEmpty(resp.Scope, cfg.Scopes),
		TokenType:        firstNonEmpty(resp.TokenType, "bearer"),
		State:            "",
		PKCECodeVerifier: "",
		Metadata:         token.Metadata,
	}); err != nil {
		return "", err
	}
	return provider.ID.String(), nil
}

func (s *Service) startGitHubDeviceAuthorization(ctx context.Context, providerID, userID string, cfg oauthConfig) (*OAuthDeviceStatus, error) {
	resp, err := s.requestDeviceAuthorization(ctx, cfg)
	if err != nil {
		return nil, err
	}

	device := oauthDeviceMetadata{
		DeviceCode:      resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		ExpiresAt:       expiresAtFromNow(resp.ExpiresIn),
		IntervalSeconds: resp.Interval,
	}
	if err := s.updateUserOAuthState(ctx, providerID, userID, "", "", device.toMetadata()); err != nil {
		return nil, err
	}
	return device.toStatus(), nil
}

func (s *Service) GetOAuthStatus(ctx context.Context, providerID string) (*OAuthStatus, error) {
	provider, err := s.loadOAuthProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}

	status := &OAuthStatus{
		Configured:  supportsOAuth(provider),
		Mode:        "web",
		CallbackURL: s.oauthConfigForProvider(provider).RedirectURI,
	}
	if !status.Configured {
		return status, nil
	}
	if isUserScopedOAuthProvider(provider) {
		status.Mode = "device"
		status.CallbackURL = ""
	}

	userID := ""
	var token *oauthTokenRecord
	if isUserScopedOAuthProvider(provider) {
		userID = oauthctx.UserIDFromContext(ctx)
		if userID == "" {
			return nil, errors.New("github copilot oauth requires a current user")
		}
		token, err = s.getUserOAuthToken(ctx, providerID, userID)
	} else {
		token, err = s.getOAuthToken(ctx, providerID)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return status, nil
		}
		return nil, err
	}

	status.HasToken = strings.TrimSpace(token.AccessToken) != ""
	if !token.ExpiresAt.IsZero() {
		expiresAt := token.ExpiresAt
		status.ExpiresAt = &expiresAt
		status.Expired = time.Now().After(token.ExpiresAt)
	}
	if isUserScopedOAuthProvider(provider) {
		status.Device = deviceMetadataFromMap(token.Metadata).toStatus()
		account, accountErr := s.resolveGitHubOAuthAccount(ctx, providerID, userID, token)
		if accountErr != nil {
			return nil, accountErr
		}
		status.Account = account
	}
	return status, nil
}

func (s *Service) PollOAuthAuthorization(ctx context.Context, providerID string) (*OAuthStatus, error) {
	provider, err := s.loadOAuthProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !isUserScopedOAuthProvider(provider) {
		return nil, errors.New("device authorization is only supported for github copilot")
	}

	userID := oauthctx.UserIDFromContext(ctx)
	if userID == "" {
		return nil, errors.New("github copilot oauth requires a current user")
	}

	token, err := s.getUserOAuthToken(ctx, providerID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.GetOAuthStatus(ctx, providerID)
		}
		return nil, err
	}
	device := deviceMetadataFromMap(token.Metadata)
	if strings.TrimSpace(device.DeviceCode) == "" {
		return s.GetOAuthStatus(ctx, providerID)
	}
	if !device.ExpiresAt.IsZero() && time.Now().After(device.ExpiresAt) {
		if err := s.updateUserOAuthState(ctx, providerID, userID, "", "", nil); err != nil {
			return nil, err
		}
		return s.GetOAuthStatus(ctx, providerID)
	}

	cfg := s.oauthConfigForProvider(provider)
	resp, err := s.exchangeDeviceCode(ctx, cfg, device.DeviceCode)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		switch resp.Error {
		case "authorization_pending":
			return s.GetOAuthStatus(ctx, providerID)
		case "slow_down":
			if resp.Interval > 0 {
				device.IntervalSeconds = resp.Interval
				if err := s.updateUserOAuthState(ctx, providerID, userID, "", "", device.toMetadata()); err != nil {
					return nil, err
				}
			}
			return s.GetOAuthStatus(ctx, providerID)
		case "expired_token", "access_denied", "incorrect_device_code", "unsupported_grant_type":
			if err := s.updateUserOAuthState(ctx, providerID, userID, "", "", nil); err != nil {
				return nil, err
			}
			return s.GetOAuthStatus(ctx, providerID)
		default:
			return nil, fmt.Errorf("oauth device token request failed: %s", firstNonEmpty(resp.Description, resp.Error))
		}
	}

	account, err := s.fetchGitHubOAuthAccount(ctx, resp.AccessToken)
	if err != nil {
		s.logger.Warn("fetch github oauth account failed", oauthLogAttrs(providerID, userID, err)...)
	}

	if err := s.saveUserOAuthToken(ctx, providerID, userID, oauthTokenRecord{
		ProviderID:   providerID,
		UserID:       userID,
		AccessToken:  resp.AccessToken,
		RefreshToken: firstNonEmpty(resp.RefreshToken, token.RefreshToken),
		ExpiresAt:    expiresAtFromNow(resp.ExpiresIn),
		Scope:        firstNonEmpty(resp.Scope, token.Scope),
		TokenType:    firstNonEmpty(resp.TokenType, "bearer"),
		Metadata:     account.toMetadata(),
	}); err != nil {
		return nil, err
	}
	return s.GetOAuthStatus(ctx, providerID)
}

func (s *Service) RevokeOAuthToken(ctx context.Context, providerID string) error {
	provider, err := s.loadOAuthProvider(ctx, providerID)
	if err != nil {
		return err
	}
	if !supportsOAuth(provider) {
		return errors.New("provider does not support oauth")
	}

	if isUserScopedOAuthProvider(provider) {
		userID := oauthctx.UserIDFromContext(ctx)
		if userID == "" {
			return errors.New("github copilot oauth requires a current user")
		}
		return s.deleteUserOAuthToken(ctx, providerID, userID)
	}
	return s.queries.DeleteProviderOAuthToken(ctx, provider.ID)
}

func (s *Service) GetValidAccessToken(ctx context.Context, providerID string) (string, error) {
	provider, err := s.loadOAuthProvider(ctx, providerID)
	if err != nil {
		return "", err
	}
	cfg := s.oauthConfigForProvider(provider)

	if isUserScopedOAuthProvider(provider) {
		userID := oauthctx.UserIDFromContext(ctx)
		if userID == "" {
			return "", errors.New("github copilot requires a current user")
		}
		token, err := s.getUserOAuthToken(ctx, providerID, userID)
		if err != nil {
			return "", err
		}
		return s.resolveValidUserOAuthToken(ctx, cfg, token)
	}

	token, err := s.getOAuthToken(ctx, providerID)
	if err != nil {
		return "", err
	}
	return s.resolveValidProviderOAuthToken(ctx, cfg, token)
}

func (s *Service) resolveValidProviderOAuthToken(ctx context.Context, cfg oauthConfig, token *oauthTokenRecord) (string, error) {
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", errors.New("oauth token is missing access token")
	}
	if token.ExpiresAt.IsZero() || time.Now().Add(oauthExpirySkew).Before(token.ExpiresAt) {
		return token.AccessToken, nil
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return "", errors.New("oauth token expired and no refresh token is available")
	}

	refreshed, err := s.refreshAccessToken(ctx, cfg, token.RefreshToken)
	if err != nil {
		return "", err
	}
	saved := oauthTokenRecord{
		ProviderID:       token.ProviderID,
		AccessToken:      refreshed.AccessToken,
		RefreshToken:     firstNonEmpty(refreshed.RefreshToken, token.RefreshToken),
		ExpiresAt:        expiresAtFromNow(refreshed.ExpiresIn),
		Scope:            firstNonEmpty(refreshed.Scope, token.Scope),
		TokenType:        firstNonEmpty(refreshed.TokenType, token.TokenType),
		State:            token.State,
		PKCECodeVerifier: token.PKCECodeVerifier,
		Metadata:         token.Metadata,
	}
	if err := s.saveOAuthToken(ctx, token.ProviderID, saved); err != nil {
		return "", err
	}
	return saved.AccessToken, nil
}

func (s *Service) resolveValidUserOAuthToken(ctx context.Context, cfg oauthConfig, token *oauthTokenRecord) (string, error) {
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", errors.New("oauth token is missing access token")
	}
	if token.ExpiresAt.IsZero() || time.Now().Add(oauthExpirySkew).Before(token.ExpiresAt) {
		return token.AccessToken, nil
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return "", errors.New("oauth token expired and no refresh token is available")
	}

	refreshed, err := s.refreshAccessToken(ctx, cfg, token.RefreshToken)
	if err != nil {
		return "", err
	}
	saved := oauthTokenRecord{
		ProviderID:       token.ProviderID,
		UserID:           token.UserID,
		AccessToken:      refreshed.AccessToken,
		RefreshToken:     firstNonEmpty(refreshed.RefreshToken, token.RefreshToken),
		ExpiresAt:        expiresAtFromNow(refreshed.ExpiresIn),
		Scope:            firstNonEmpty(refreshed.Scope, token.Scope),
		TokenType:        firstNonEmpty(refreshed.TokenType, token.TokenType),
		State:            token.State,
		PKCECodeVerifier: token.PKCECodeVerifier,
		Metadata:         token.Metadata,
	}
	if err := s.saveUserOAuthToken(ctx, token.ProviderID, token.UserID, saved); err != nil {
		return "", err
	}
	return saved.AccessToken, nil
}

func (s *Service) loadOAuthProvider(ctx context.Context, providerID string) (sqlc.Provider, error) {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return sqlc.Provider{}, err
	}
	provider, err := s.queries.GetProviderByID(ctx, providerUUID)
	if err != nil {
		return sqlc.Provider{}, fmt.Errorf("get provider: %w", err)
	}
	if !supportsOAuth(provider) {
		return sqlc.Provider{}, errors.New("provider does not support oauth")
	}
	return provider, nil
}

func (s *Service) getOAuthToken(ctx context.Context, providerID string) (*oauthTokenRecord, error) {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetProviderOAuthTokenByProvider(ctx, providerUUID)
	if err != nil {
		return nil, err
	}
	return toProviderOAuthToken(row), nil
}

func (s *Service) getOAuthTokenByState(ctx context.Context, state string) (*oauthTokenRecord, error) {
	row, err := s.queries.GetProviderOAuthTokenByState(ctx, state)
	if err != nil {
		return nil, err
	}
	return toProviderOAuthToken(row), nil
}

func (s *Service) updateOAuthState(ctx context.Context, providerID, state, codeVerifier string) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	return s.queries.UpdateProviderOAuthState(ctx, sqlc.UpdateProviderOAuthStateParams{
		ProviderID:       providerUUID,
		State:            state,
		PkceCodeVerifier: codeVerifier,
	})
}

func (s *Service) saveOAuthToken(ctx context.Context, providerID string, token oauthTokenRecord) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	var expiresAt pgtype.Timestamptz
	if !token.ExpiresAt.IsZero() {
		expiresAt = pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true}
	}
	_, err = s.queries.UpsertProviderOAuthToken(ctx, sqlc.UpsertProviderOAuthTokenParams{
		ProviderID:       providerUUID,
		AccessToken:      token.AccessToken,
		RefreshToken:     token.RefreshToken,
		ExpiresAt:        expiresAt,
		Scope:            token.Scope,
		TokenType:        token.TokenType,
		State:            token.State,
		PkceCodeVerifier: token.PKCECodeVerifier,
	})
	return err
}

func (s *Service) getUserOAuthToken(ctx context.Context, providerID, userID string) (*oauthTokenRecord, error) {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	userUUID, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetUserProviderOAuthToken(ctx, sqlc.GetUserProviderOAuthTokenParams{
		ProviderID: providerUUID,
		UserID:     userUUID,
	})
	if err != nil {
		return nil, err
	}
	return toUserProviderOAuthToken(row), nil
}

func (s *Service) getUserOAuthTokenByState(ctx context.Context, state string) (*oauthTokenRecord, error) {
	row, err := s.queries.GetUserProviderOAuthTokenByState(ctx, state)
	if err != nil {
		return nil, err
	}
	return toUserProviderOAuthToken(row), nil
}

func (s *Service) updateUserOAuthState(ctx context.Context, providerID, userID, state, codeVerifier string, metadata map[string]any) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	userUUID, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	return s.queries.UpdateUserProviderOAuthState(ctx, sqlc.UpdateUserProviderOAuthStateParams{
		ProviderID:       providerUUID,
		UserID:           userUUID,
		State:            state,
		PkceCodeVerifier: codeVerifier,
		Metadata:         metadataJSON(metadata),
	})
}

func (s *Service) saveUserOAuthToken(ctx context.Context, providerID, userID string, token oauthTokenRecord) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	userUUID, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	var expiresAt pgtype.Timestamptz
	if !token.ExpiresAt.IsZero() {
		expiresAt = pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true}
	}
	_, err = s.queries.UpsertUserProviderOAuthToken(ctx, sqlc.UpsertUserProviderOAuthTokenParams{
		ProviderID:       providerUUID,
		UserID:           userUUID,
		AccessToken:      token.AccessToken,
		RefreshToken:     token.RefreshToken,
		ExpiresAt:        expiresAt,
		Scope:            token.Scope,
		TokenType:        token.TokenType,
		State:            token.State,
		PkceCodeVerifier: token.PKCECodeVerifier,
		Metadata:         metadataJSON(token.Metadata),
	})
	return err
}

func (s *Service) deleteUserOAuthToken(ctx context.Context, providerID, userID string) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	userUUID, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	return s.queries.DeleteUserProviderOAuthToken(ctx, sqlc.DeleteUserProviderOAuthTokenParams{
		ProviderID: providerUUID,
		UserID:     userUUID,
	})
}

func toProviderOAuthToken(row sqlc.ProviderOauthToken) *oauthTokenRecord {
	token := &oauthTokenRecord{
		ProviderID:       row.ProviderID.String(),
		AccessToken:      row.AccessToken,
		RefreshToken:     row.RefreshToken,
		Scope:            row.Scope,
		TokenType:        row.TokenType,
		State:            row.State,
		PKCECodeVerifier: row.PkceCodeVerifier,
		Metadata:         map[string]any{},
	}
	if row.ExpiresAt.Valid {
		token.ExpiresAt = row.ExpiresAt.Time
	}
	return token
}

func toUserProviderOAuthToken(row sqlc.UserProviderOauthToken) *oauthTokenRecord {
	token := &oauthTokenRecord{
		ProviderID:       row.ProviderID.String(),
		UserID:           row.UserID.String(),
		AccessToken:      row.AccessToken,
		RefreshToken:     row.RefreshToken,
		Scope:            row.Scope,
		TokenType:        row.TokenType,
		State:            row.State,
		PKCECodeVerifier: row.PkceCodeVerifier,
		Metadata:         providerMetadata(row.Metadata),
	}
	if row.ExpiresAt.Valid {
		token.ExpiresAt = row.ExpiresAt.Time
	}
	return token
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`  //nolint:gosec // OAuth response payload carries runtime access token
	RefreshToken string `json:"refresh_token"` //nolint:gosec // OAuth response payload carries runtime refresh token
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	Interval     int64  `json:"interval"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

type oauthDeviceMetadata struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresAt       time.Time
	IntervalSeconds int64
}

type oauthAccountMetadata struct {
	Label      string
	Login      string
	Name       string
	Email      string
	AvatarURL  string
	ProfileURL string
}

func deviceMetadataFromMap(metadata map[string]any) oauthDeviceMetadata {
	device := oauthDeviceMetadata{
		DeviceCode:      strings.TrimSpace(stringValue(metadata, metadataDeviceCodeKey)),
		UserCode:        strings.TrimSpace(stringValue(metadata, metadataDeviceUserCodeKey)),
		VerificationURI: strings.TrimSpace(stringValue(metadata, metadataDeviceVerifyURIKey)),
		IntervalSeconds: int64Value(metadata, metadataDeviceIntervalKey),
	}
	if raw := strings.TrimSpace(stringValue(metadata, metadataDeviceExpiresAtKey)); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			device.ExpiresAt = parsed
		}
	}
	return device
}

func (d oauthDeviceMetadata) toMetadata() map[string]any {
	if strings.TrimSpace(d.DeviceCode) == "" {
		return nil
	}
	metadata := map[string]any{
		metadataDeviceCodeKey:      d.DeviceCode,
		metadataDeviceUserCodeKey:  d.UserCode,
		metadataDeviceVerifyURIKey: d.VerificationURI,
		metadataDeviceIntervalKey:  d.IntervalSeconds,
	}
	if !d.ExpiresAt.IsZero() {
		metadata[metadataDeviceExpiresAtKey] = d.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return metadata
}

func (d oauthDeviceMetadata) toStatus() *OAuthDeviceStatus {
	if strings.TrimSpace(d.DeviceCode) == "" {
		return nil
	}
	status := &OAuthDeviceStatus{
		Pending:         true,
		UserCode:        d.UserCode,
		VerificationURI: d.VerificationURI,
		IntervalSeconds: d.IntervalSeconds,
	}
	if !d.ExpiresAt.IsZero() {
		expiresAt := d.ExpiresAt
		status.ExpiresAt = &expiresAt
	}
	return status
}

func accountMetadataFromMap(metadata map[string]any) oauthAccountMetadata {
	account := oauthAccountMetadata{
		Label:      strings.TrimSpace(stringValue(metadata, metadataAccountLabelKey)),
		Login:      strings.TrimSpace(stringValue(metadata, metadataAccountLoginKey)),
		Name:       strings.TrimSpace(stringValue(metadata, metadataAccountNameKey)),
		Email:      strings.TrimSpace(stringValue(metadata, metadataAccountEmailKey)),
		AvatarURL:  strings.TrimSpace(stringValue(metadata, metadataAccountAvatarURLKey)),
		ProfileURL: strings.TrimSpace(stringValue(metadata, metadataAccountProfileURLKey)),
	}
	if account.Label == "" {
		account.Label = firstNonEmpty(account.Name, account.Login, account.Email)
	}
	return account
}

func (a oauthAccountMetadata) toMetadata() map[string]any {
	if a.isZero() {
		return map[string]any{}
	}
	metadata := map[string]any{}
	if a.Label != "" {
		metadata[metadataAccountLabelKey] = a.Label
	}
	if a.Login != "" {
		metadata[metadataAccountLoginKey] = a.Login
	}
	if a.Name != "" {
		metadata[metadataAccountNameKey] = a.Name
	}
	if a.Email != "" {
		metadata[metadataAccountEmailKey] = a.Email
	}
	if a.AvatarURL != "" {
		metadata[metadataAccountAvatarURLKey] = a.AvatarURL
	}
	if a.ProfileURL != "" {
		metadata[metadataAccountProfileURLKey] = a.ProfileURL
	}
	return metadata
}

func (a oauthAccountMetadata) toStatus() *OAuthAccount {
	if a.isZero() {
		return nil
	}
	return &OAuthAccount{
		Label:      a.Label,
		Login:      a.Login,
		Name:       a.Name,
		Email:      a.Email,
		AvatarURL:  a.AvatarURL,
		ProfileURL: a.ProfileURL,
	}
}

func (a oauthAccountMetadata) isZero() bool {
	return a.Label == "" && a.Login == "" && a.Name == "" && a.Email == "" && a.AvatarURL == "" && a.ProfileURL == ""
}

func (s *Service) resolveGitHubOAuthAccount(ctx context.Context, providerID, userID string, token *oauthTokenRecord) (*OAuthAccount, error) {
	account := accountMetadataFromMap(token.Metadata)
	if status := account.toStatus(); status != nil {
		return status, nil
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, nil
	}

	refreshedAccount, err := s.fetchGitHubOAuthAccount(ctx, token.AccessToken)
	if err != nil {
		s.logger.Warn("refresh github oauth account metadata failed", oauthLogAttrs(providerID, userID, err)...)
		return nil, nil
	}

	updatedToken := *token
	updatedToken.Metadata = refreshedAccount.toMetadata()
	if err := s.saveUserOAuthToken(ctx, providerID, userID, updatedToken); err != nil {
		return nil, err
	}
	return refreshedAccount.toStatus(), nil
}

func (s *Service) fetchGitHubOAuthAccount(ctx context.Context, accessToken string) (oauthAccountMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultGitHubUserURL, nil)
	if err != nil {
		return oauthAccountMetadata{}, fmt.Errorf("create github oauth account request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.httpClient.Do(req) //nolint:gosec // Request targets a fixed GitHub API endpoint.
	if err != nil {
		return oauthAccountMetadata{}, fmt.Errorf("execute github oauth account request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return oauthAccountMetadata{}, fmt.Errorf("read github oauth account response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthAccountMetadata{}, fmt.Errorf("github oauth account request failed: %s", strings.TrimSpace(string(payload)))
	}

	var profile struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
	}
	if err := json.Unmarshal(payload, &profile); err != nil {
		return oauthAccountMetadata{}, fmt.Errorf("decode github oauth account response: %w", err)
	}

	account := oauthAccountMetadata{
		Login:      strings.TrimSpace(profile.Login),
		Name:       strings.TrimSpace(profile.Name),
		Email:      strings.TrimSpace(profile.Email),
		AvatarURL:  strings.TrimSpace(profile.AvatarURL),
		ProfileURL: strings.TrimSpace(profile.HTMLURL),
	}
	if account.Email == "" {
		email, err := s.fetchGitHubPrimaryEmail(ctx, accessToken)
		if err != nil {
			s.logger.Warn("fetch github oauth primary email failed", slog.Any("error", err))
		} else {
			account.Email = email
		}
	}
	account.Label = firstNonEmpty(account.Email, account.Name, account.Login)
	if account.Label == "" {
		return oauthAccountMetadata{}, errors.New("github oauth account response did not include a usable account label")
	}
	return account, nil
}

func (s *Service) fetchGitHubPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultGitHubUserEmailsURL, nil)
	if err != nil {
		return "", fmt.Errorf("create github oauth emails request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.httpClient.Do(req) //nolint:gosec // Request targets a fixed GitHub API endpoint.
	if err != nil {
		return "", fmt.Errorf("execute github oauth emails request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read github oauth emails response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github oauth emails request failed: %s", strings.TrimSpace(string(payload)))
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(payload, &emails); err != nil {
		return "", fmt.Errorf("decode github oauth emails response: %w", err)
	}

	for _, candidate := range emails {
		email := strings.TrimSpace(candidate.Email)
		if candidate.Primary && candidate.Verified && email != "" {
			return email, nil
		}
	}
	for _, candidate := range emails {
		email := strings.TrimSpace(candidate.Email)
		if candidate.Primary && email != "" {
			return email, nil
		}
	}
	for _, candidate := range emails {
		email := strings.TrimSpace(candidate.Email)
		if email != "" {
			return email, nil
		}
	}

	return "", errors.New("github oauth emails response did not include a usable email")
}

func (s *Service) requestDeviceAuthorization(ctx context.Context, cfg oauthConfig) (*deviceAuthorizationResponse, error) {
	if err := validateOAuthTokenURL(cfg.ClientType, cfg.DeviceCodeURL); err != nil {
		return nil, err
	}

	values := url.Values{
		"client_id": {cfg.ClientID},
	}
	if cfg.Scopes != "" {
		values.Set("scope", cfg.Scopes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.DeviceCodeURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create oauth device request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is validated by validateOAuthTokenURL before request execution.
	if err != nil {
		return nil, fmt.Errorf("execute oauth device request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oauth device response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth device request failed: %s", strings.TrimSpace(string(payload)))
	}

	var deviceResp deviceAuthorizationResponse
	if err := json.Unmarshal(payload, &deviceResp); err != nil {
		return nil, fmt.Errorf("decode oauth device response: %w", err)
	}
	if deviceResp.Error != "" {
		return nil, fmt.Errorf("oauth device request failed: %s", firstNonEmpty(deviceResp.Description, deviceResp.Error))
	}
	if strings.TrimSpace(deviceResp.DeviceCode) == "" || strings.TrimSpace(deviceResp.UserCode) == "" || strings.TrimSpace(deviceResp.VerificationURI) == "" {
		return nil, errors.New("oauth device request returned incomplete device authorization data")
	}
	if deviceResp.Interval <= 0 {
		deviceResp.Interval = 5
	}
	return &deviceResp, nil
}

func (s *Service) exchangeDeviceCode(ctx context.Context, cfg oauthConfig, deviceCode string) (*oauthTokenResponse, error) {
	if err := validateOAuthTokenURL(cfg.ClientType, cfg.TokenURL); err != nil {
		return nil, err
	}

	values := url.Values{
		"client_id":   {cfg.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create oauth device token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is validated by validateOAuthTokenURL before request execution.
	if err != nil {
		return nil, fmt.Errorf("execute oauth device token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oauth device token response: %w", err)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(payload, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode oauth device token response: %w", err)
	}
	if tokenResp.Interval <= 0 {
		tokenResp.Interval = 5
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if tokenResp.Error != "" {
			return &tokenResp, nil
		}
		return nil, fmt.Errorf("oauth device token request failed: %s", strings.TrimSpace(string(payload)))
	}
	return &tokenResp, nil
}

func (s *Service) exchangeCode(ctx context.Context, cfg oauthConfig, code, codeVerifier string) (*oauthTokenResponse, error) {
	values := url.Values{
		"code":         {code},
		"client_id":    {cfg.ClientID},
		"redirect_uri": {cfg.RedirectURI},
	}
	if cfg.UsePKCE {
		values.Set("grant_type", "authorization_code")
		values.Set("code_verifier", codeVerifier)
	}
	if cfg.ClientSecret != "" {
		values.Set("client_secret", cfg.ClientSecret)
	}
	return s.postTokenRequest(ctx, cfg, values)
}

func (s *Service) refreshAccessToken(ctx context.Context, cfg oauthConfig, refreshToken string) (*oauthTokenResponse, error) {
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {cfg.ClientID},
	}
	if cfg.ClientSecret != "" {
		values.Set("client_secret", cfg.ClientSecret)
	}
	return s.postTokenRequest(ctx, cfg, values)
}

func (s *Service) postTokenRequest(ctx context.Context, cfg oauthConfig, body url.Values) (*oauthTokenResponse, error) {
	if err := validateOAuthTokenURL(cfg.ClientType, cfg.TokenURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create oauth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:gosec // URL is validated by validateOAuthTokenURL before request execution.
	if err != nil {
		return nil, fmt.Errorf("execute oauth request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oauth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth token request failed: %s", strings.TrimSpace(string(payload)))
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(payload, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode oauth response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oauth token request failed: %s", firstNonEmpty(tokenResp.Description, tokenResp.Error))
	}
	return &tokenResp, nil
}

func validateOAuthTokenURL(clientType models.ClientType, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid oauth token url: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("oauth token url must use https")
	}

	switch clientType {
	case models.ClientTypeOpenAICodex:
		if !strings.EqualFold(parsed.Hostname(), "auth.openai.com") {
			return errors.New("oauth token url host must be auth.openai.com")
		}
	case models.ClientTypeGitHubCopilot:
		if !strings.EqualFold(parsed.Hostname(), "github.com") {
			return errors.New("oauth token url host must be github.com")
		}
	default:
		return errors.New("unsupported oauth client type")
	}

	return nil
}

func stringValue(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return value
}

func int64Value(input map[string]any, key string) int64 {
	if input == nil {
		return 0
	}
	switch value := input[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func metadataJSON(metadata map[string]any) []byte {
	if len(metadata) == 0 {
		return []byte("{}")
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func computeCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func expiresAtFromNow(expiresIn int64) time.Time {
	if expiresIn <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(expiresIn) * time.Second)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
