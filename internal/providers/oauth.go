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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
)

const (
	defaultOpenAICodexClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultOpenAIAuthorizeURL     = "https://auth.openai.com/oauth/authorize"
	defaultOpenAITokenURL         = "https://auth.openai.com/oauth/token" //nolint:gosec // OAuth endpoint URL, not a credential
	defaultOpenAICallbackURL      = "http://localhost:1455/auth/callback"
	defaultOpenAIOAuthScopes      = "openid profile email offline_access"
	oauthExpirySkew               = 30 * time.Second
	providerOAuthHTTPTimeout      = 15 * time.Second
	metadataOAuthClientIDKey      = "oauth_client_id"
	metadataOAuthAuthorizeURLKey  = "oauth_authorize_url"
	metadataOAuthTokenURLKey      = "oauth_token_url" //nolint:gosec // metadata key name, not a credential
	metadataOAuthRedirectURIKey   = "oauth_redirect_uri"
	metadataOAuthScopesKey        = "oauth_scopes"
	metadataOAuthAudienceKey      = "oauth_audience"
	metadataOAuthUseIDOrgsFlagKey = "oauth_id_token_add_organizations"
)

type providerOAuthToken struct {
	ProviderID       string    `json:"provider_id"`
	AccessToken      string    `json:"access_token"`  //nolint:gosec // runtime credential storage
	RefreshToken     string    `json:"refresh_token"` //nolint:gosec // runtime credential storage
	ExpiresAt        time.Time `json:"expires_at"`
	Scope            string    `json:"scope"`
	TokenType        string    `json:"token_type"`
	State            string    `json:"state"`
	PKCECodeVerifier string    `json:"pkce_code_verifier"`
}

type openAIOAuthConfig struct {
	ClientID                string
	AuthorizeURL            string
	TokenURL                string
	RedirectURI             string
	Scopes                  string
	IDTokenAddOrganizations bool
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

func (s *Service) oauthConfig(metadata map[string]any) openAIOAuthConfig {
	cfg := openAIOAuthConfig{
		ClientID:                defaultOpenAICodexClientID,
		AuthorizeURL:            defaultOpenAIAuthorizeURL,
		TokenURL:                defaultOpenAITokenURL,
		RedirectURI:             firstNonEmpty(s.callbackURL, defaultOpenAICallbackURL),
		Scopes:                  defaultOpenAIOAuthScopes,
		IDTokenAddOrganizations: true,
	}
	if v, _ := metadata[metadataOAuthClientIDKey].(string); strings.TrimSpace(v) != "" {
		cfg.ClientID = strings.TrimSpace(v)
	}
	if v, _ := metadata[metadataOAuthAuthorizeURLKey].(string); strings.TrimSpace(v) != "" {
		cfg.AuthorizeURL = strings.TrimSpace(v)
	}
	if v, _ := metadata[metadataOAuthTokenURLKey].(string); strings.TrimSpace(v) != "" {
		cfg.TokenURL = strings.TrimSpace(v)
	}
	if v, _ := metadata[metadataOAuthRedirectURIKey].(string); strings.TrimSpace(v) != "" {
		cfg.RedirectURI = strings.TrimSpace(v)
	}
	if v, _ := metadata[metadataOAuthScopesKey].(string); strings.TrimSpace(v) != "" {
		cfg.Scopes = strings.TrimSpace(v)
	}
	if v, ok := metadata[metadataOAuthUseIDOrgsFlagKey].(bool); ok {
		cfg.IDTokenAddOrganizations = v
	}
	return cfg
}

func supportsOAuth(provider sqlc.LlmProvider) bool {
	return models.ClientType(provider.ClientType) == models.ClientTypeOpenAICodex
}

func (s *Service) StartOAuthAuthorization(ctx context.Context, providerID string) (string, error) {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return "", err
	}
	provider, err := s.queries.GetLlmProviderByID(ctx, providerUUID)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	if !supportsOAuth(provider) {
		return "", errors.New("provider does not support oauth")
	}

	cfg := s.oauthConfig(providerMetadata(provider.Metadata))
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("generate code verifier: %w", err)
	}
	state, err := generateState()
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	if err := s.updateOAuthState(ctx, providerID, state, codeVerifier); err != nil {
		return "", err
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {cfg.RedirectURI},
		"scope":                 {cfg.Scopes},
		"code_challenge":        {computeCodeChallenge(codeVerifier)},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	if cfg.IDTokenAddOrganizations {
		params.Set("id_token_add_organizations", "true")
	}
	params.Set("codex_cli_simplified_flow", "true")

	return cfg.AuthorizeURL + "?" + params.Encode(), nil
}

func (s *Service) HandleOAuthCallback(ctx context.Context, state, code string) (string, error) {
	token, err := s.getOAuthTokenByState(ctx, state)
	if err != nil {
		return "", err
	}
	providerUUID, err := db.ParseUUID(token.ProviderID)
	if err != nil {
		return "", err
	}
	provider, err := s.queries.GetLlmProviderByID(ctx, providerUUID)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	if !supportsOAuth(provider) {
		return "", errors.New("provider does not support oauth")
	}

	cfg := s.oauthConfig(providerMetadata(provider.Metadata))
	resp, err := s.exchangeCode(ctx, cfg, code, token.PKCECodeVerifier)
	if err != nil {
		return "", err
	}
	if err := s.saveOAuthToken(ctx, provider.ID.String(), providerOAuthToken{
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

func (s *Service) GetOAuthStatus(ctx context.Context, providerID string) (*OAuthStatus, error) {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	provider, err := s.queries.GetLlmProviderByID(ctx, providerUUID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}
	status := &OAuthStatus{
		Configured:  supportsOAuth(provider),
		CallbackURL: s.oauthConfig(providerMetadata(provider.Metadata)).RedirectURI,
	}
	if !status.Configured {
		return status, nil
	}

	token, err := s.getOAuthToken(ctx, providerID)
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
	return status, nil
}

func (s *Service) RevokeOAuthToken(ctx context.Context, providerID string) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	provider, err := s.queries.GetLlmProviderByID(ctx, providerUUID)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}
	if !supportsOAuth(provider) {
		return errors.New("provider does not support oauth")
	}
	return s.queries.DeleteLlmProviderOAuthToken(ctx, providerUUID)
}

func (s *Service) GetValidAccessToken(ctx context.Context, providerID string) (string, error) {
	token, err := s.getOAuthToken(ctx, providerID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", errors.New("oauth token is missing access token")
	}
	if token.ExpiresAt.IsZero() || time.Now().Add(oauthExpirySkew).Before(token.ExpiresAt) {
		return token.AccessToken, nil
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return "", errors.New("oauth token expired and no refresh token is available")
	}

	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return "", err
	}
	provider, err := s.queries.GetLlmProviderByID(ctx, providerUUID)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	cfg := s.oauthConfig(providerMetadata(provider.Metadata))
	refreshed, err := s.refreshAccessToken(ctx, cfg, token.RefreshToken)
	if err != nil {
		return "", err
	}
	saved := providerOAuthToken{
		ProviderID:       providerID,
		AccessToken:      refreshed.AccessToken,
		RefreshToken:     firstNonEmpty(refreshed.RefreshToken, token.RefreshToken),
		ExpiresAt:        expiresAtFromNow(refreshed.ExpiresIn),
		Scope:            firstNonEmpty(refreshed.Scope, token.Scope),
		TokenType:        firstNonEmpty(refreshed.TokenType, token.TokenType),
		State:            token.State,
		PKCECodeVerifier: token.PKCECodeVerifier,
	}
	if err := s.saveOAuthToken(ctx, providerID, saved); err != nil {
		return "", err
	}
	return saved.AccessToken, nil
}

func (s *Service) getOAuthToken(ctx context.Context, providerID string) (*providerOAuthToken, error) {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetLlmProviderOAuthTokenByProvider(ctx, providerUUID)
	if err != nil {
		return nil, err
	}
	return toProviderOAuthToken(row), nil
}

func (s *Service) getOAuthTokenByState(ctx context.Context, state string) (*providerOAuthToken, error) {
	row, err := s.queries.GetLlmProviderOAuthTokenByState(ctx, state)
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
	return s.queries.UpdateLlmProviderOAuthState(ctx, sqlc.UpdateLlmProviderOAuthStateParams{
		LlmProviderID:    providerUUID,
		State:            state,
		PkceCodeVerifier: codeVerifier,
	})
}

func (s *Service) saveOAuthToken(ctx context.Context, providerID string, token providerOAuthToken) error {
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	var expiresAt pgtype.Timestamptz
	if !token.ExpiresAt.IsZero() {
		expiresAt = pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true}
	}
	_, err = s.queries.UpsertLlmProviderOAuthToken(ctx, sqlc.UpsertLlmProviderOAuthTokenParams{
		LlmProviderID:    providerUUID,
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

func toProviderOAuthToken(row sqlc.LlmProviderOauthToken) *providerOAuthToken {
	token := &providerOAuthToken{
		ProviderID:       row.LlmProviderID.String(),
		AccessToken:      row.AccessToken,
		RefreshToken:     row.RefreshToken,
		Scope:            row.Scope,
		TokenType:        row.TokenType,
		State:            row.State,
		PKCECodeVerifier: row.PkceCodeVerifier,
	}
	if row.ExpiresAt.Valid {
		token.ExpiresAt = row.ExpiresAt.Time
	}
	return token
}

type openAITokenResponse struct {
	AccessToken  string `json:"access_token"`  //nolint:gosec // OAuth response payload carries runtime access token
	RefreshToken string `json:"refresh_token"` //nolint:gosec // OAuth response payload carries runtime refresh token
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func (s *Service) exchangeCode(ctx context.Context, cfg openAIOAuthConfig, code, codeVerifier string) (*openAITokenResponse, error) {
	values := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"redirect_uri":  {cfg.RedirectURI},
		"code_verifier": {codeVerifier},
	}
	return s.postTokenRequest(ctx, cfg.TokenURL, values)
}

func (s *Service) refreshAccessToken(ctx context.Context, cfg openAIOAuthConfig, refreshToken string) (*openAITokenResponse, error) {
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {cfg.ClientID},
	}
	return s.postTokenRequest(ctx, cfg.TokenURL, values)
}

func (s *Service) postTokenRequest(ctx context.Context, tokenURL string, body url.Values) (*openAITokenResponse, error) {
	if err := validateOAuthTokenURL(tokenURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create oauth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	//nolint:gosec // tokenURL is restricted to the fixed OpenAI OAuth host by validateOAuthTokenURL above
	resp, err := s.httpClient.Do(req)
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

	var tokenResp openAITokenResponse
	if err := json.Unmarshal(payload, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode oauth response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oauth token request failed: %s", firstNonEmpty(tokenResp.Description, tokenResp.Error))
	}
	return &tokenResp, nil
}

func validateOAuthTokenURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid oauth token url: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("oauth token url must use https")
	}
	if !strings.EqualFold(parsed.Hostname(), "auth.openai.com") {
		return errors.New("oauth token url host must be auth.openai.com")
	}
	return nil
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
