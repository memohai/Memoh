package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type AuthService struct {
	queries     dbstore.Queries
	core        *Service
	sessionTTL  time.Duration
	redirectURL string
}

func NewAuthService(queries dbstore.Queries, sessionTTL time.Duration, redirectURL string) *AuthService {
	svc := &AuthService{
		queries:     queries,
		sessionTTL:  sessionTTL,
		redirectURL: redirectURL,
	}
	svc.core = NewService((*authStore)(svc))
	return svc
}

func (s *AuthService) ListEnabledProviders(ctx context.Context) ([]Provider, error) {
	rows, err := s.queries.ListEnabledSSOProviders(ctx)
	if err != nil {
		return nil, err
	}
	providers := make([]Provider, 0, len(rows))
	for _, row := range rows {
		provider, err := providerFromRow(row)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func (s *AuthService) GetProvider(ctx context.Context, providerID string) (Provider, error) {
	id, err := db.ParseUUID(providerID)
	if err == nil {
		row, err := s.queries.GetSSOProviderByID(ctx, id)
		if err != nil {
			return Provider{}, mapSSOErr(err)
		}
		return providerFromRow(row)
	}
	row, err := s.queries.GetSSOProviderByKey(ctx, providerID)
	if err != nil {
		return Provider{}, mapSSOErr(err)
	}
	return providerFromRow(row)
}

func (*AuthService) BuildOIDCAuthRedirect(ctx context.Context, provider Provider, state string, nonce string, codeVerifier string) (OIDCAuthRedirect, error) {
	return BuildOIDCAuthRedirect(ctx, provider.OIDC, state, nonce, codeVerifier)
}

func (s *AuthService) CompleteOIDCCallback(ctx context.Context, provider Provider, code string, state OIDCState) (LoginCode, error) {
	claims, err := ExchangeOIDCCode(ctx, provider.OIDC, code, state.CodeVerifier)
	if err != nil {
		return LoginCode{}, err
	}
	profile, err := NormalizeOIDCClaims(provider, claims)
	if err != nil {
		return LoginCode{}, err
	}
	return s.issueLoginCode(ctx, provider, profile)
}

func (*AuthService) BuildSAMLAuthRedirect(_ context.Context, provider Provider) (SAMLAuthRedirect, error) {
	sp, err := buildSAMLServiceProvider(provider)
	if err != nil {
		return SAMLAuthRedirect{}, err
	}
	authReq, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		return SAMLAuthRedirect{}, err
	}
	relayState, err := GenerateState()
	if err != nil {
		return SAMLAuthRedirect{}, err
	}
	redirectURL, err := authReq.Redirect(relayState, sp)
	if err != nil {
		return SAMLAuthRedirect{}, err
	}
	return SAMLAuthRedirect{URL: redirectURL.String(), RelayState: relayState, RequestID: authReq.ID}, nil
}

func (s *AuthService) CompleteSAMLACS(ctx context.Context, provider Provider, r *http.Request, state SAMLState) (LoginCode, error) {
	if err := r.ParseForm(); err != nil {
		return LoginCode{}, err
	}
	if r.Form.Get("RelayState") != state.RelayState {
		return LoginCode{}, ErrInvalidProfile
	}
	sp, err := buildSAMLServiceProvider(provider)
	if err != nil {
		return LoginCode{}, err
	}
	assertion, err := sp.ParseResponse(r, []string{state.RequestID})
	if err != nil {
		return LoginCode{}, err
	}
	profile, err := NormalizeSAMLAssertion(provider, assertion)
	if err != nil {
		return LoginCode{}, err
	}
	return s.issueLoginCode(ctx, provider, profile)
}

func (*AuthService) BuildSAMLMetadata(_ context.Context, provider Provider) (string, error) {
	sp, err := buildSAMLServiceProvider(provider)
	if err != nil {
		return "", err
	}
	buf, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func buildSAMLServiceProvider(provider Provider) (*saml.ServiceProvider, error) {
	cfg := provider.SAML
	acsURL, err := parseRequiredURL(cfg.ACSURL)
	if err != nil {
		return nil, err
	}
	metadataURLValue := cfg.MetadataURL
	if metadataURLValue == "" {
		metadataURLValue = deriveSAMLMetadataURL(cfg.ACSURL)
	}
	metadataURL, err := parseRequiredURL(metadataURLValue)
	if err != nil {
		return nil, err
	}
	idpMetadata, err := samlsp.ParseMetadata([]byte(cfg.MetadataXML))
	if err != nil {
		return nil, err
	}
	return &saml.ServiceProvider{
		EntityID:          cfg.EntityID,
		MetadataURL:       *metadataURL,
		AcsURL:            *acsURL,
		IDPMetadata:       idpMetadata,
		HTTPClient:        http.DefaultClient,
		AuthnNameIDFormat: saml.PersistentNameIDFormat,
	}, nil
}

func parseRequiredURL(value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ErrInvalidProvider
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, ErrInvalidProvider
	}
	return parsed, nil
}

func deriveSAMLMetadataURL(acsURL string) string {
	if strings.HasSuffix(acsURL, "/acs") {
		return strings.TrimSuffix(acsURL, "/acs") + "/metadata"
	}
	return strings.TrimRight(acsURL, "/") + "/metadata"
}

func GenerateState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *AuthService) ExchangeLoginCode(ctx context.Context, code string) (LoginCode, error) {
	row, err := s.queries.UseIAMLoginCode(ctx, HashLoginCode(code))
	if err != nil {
		return LoginCode{}, mapSSOErr(err)
	}
	return LoginCode{
		Code:      code,
		UserID:    row.UserID.String(),
		SessionID: row.SessionID.String(),
	}, nil
}

func (s *AuthService) issueLoginCode(ctx context.Context, provider Provider, profile NormalizedProfile) (LoginCode, error) {
	result, err := s.core.FindOrProvisionUser(ctx, provider, profile)
	if err != nil {
		return LoginCode{}, err
	}
	if err := s.core.SyncMappedGroups(ctx, provider, result.User.ID, profile.Groups); err != nil {
		return LoginCode{}, err
	}
	userID, err := db.ParseUUID(result.User.ID)
	if err != nil {
		return LoginCode{}, err
	}
	identityID, err := db.ParseUUID(result.Identity.ID)
	if err != nil {
		return LoginCode{}, err
	}
	now := time.Now().UTC()
	session, err := s.queries.CreateIAMSession(ctx, sqlc.CreateIAMSessionParams{
		UserID:     userID,
		IdentityID: identityID,
		ExpiresAt:  pgtype.Timestamptz{Time: now.Add(s.sessionTTL), Valid: true},
		Metadata:   []byte("{}"),
	})
	if err != nil {
		return LoginCode{}, err
	}
	code, codeHash, err := GenerateLoginCode()
	if err != nil {
		return LoginCode{}, err
	}
	if _, err := s.queries.CreateIAMLoginCode(ctx, sqlc.CreateIAMLoginCodeParams{
		CodeHash:   codeHash,
		UserID:     userID,
		IdentityID: identityID,
		SessionID:  session.ID,
		ExpiresAt:  pgtype.Timestamptz{Time: now.Add(LoginCodeTTL), Valid: true},
	}); err != nil {
		return LoginCode{}, err
	}
	return LoginCode{Code: code, UserID: result.User.ID, SessionID: session.ID.String(), RedirectURL: s.redirectURL}, nil
}

type authStore AuthService

func (s *authStore) FindIdentity(ctx context.Context, providerType ProviderType, providerID string, subject string) (Identity, User, error) {
	pid, err := db.ParseUUID(providerID)
	if err != nil {
		return Identity{}, User{}, err
	}
	identity, err := s.queries.GetIdentityByProviderSubject(ctx, sqlc.GetIdentityByProviderSubjectParams{
		ProviderType: string(providerType),
		ProviderID:   pid,
		Subject:      subject,
	})
	if err != nil {
		return Identity{}, User{}, mapSSOErr(err)
	}
	user, err := s.queries.GetAccountByUserID(ctx, identity.UserID)
	if err != nil {
		return Identity{}, User{}, mapSSOErr(err)
	}
	return identityFromRow(identity), userFromRow(user), nil
}

func (s *authStore) FindUserByEmail(ctx context.Context, normalizedEmail string) (User, error) {
	rows, err := s.queries.SearchAccounts(ctx, sqlc.SearchAccountsParams{Query: normalizedEmail, LimitCount: 10})
	if err != nil {
		return User{}, err
	}
	for _, row := range rows {
		if NormalizeEmail(row.Email.String) == normalizedEmail {
			return userFromRow(row), nil
		}
	}
	return User{}, ErrNotFound
}

func (s *authStore) CreateUserWithIdentity(ctx context.Context, profile NormalizedProfile) (Identity, User, error) {
	row, err := s.queries.CreateUser(ctx, sqlc.CreateUserParams{IsActive: true, Metadata: []byte("{}")})
	if err != nil {
		return Identity{}, User{}, err
	}
	user := userFromRow(row)
	userID := row.ID
	username := profile.Username
	if username == "" {
		username = profile.Email
	}
	if username == "" {
		username = profile.Subject
	}
	account, err := s.queries.CreateAccount(ctx, sqlc.CreateAccountParams{
		UserID:      userID,
		Username:    pgtype.Text{String: username, Valid: username != ""},
		Email:       pgtype.Text{String: profile.Email, Valid: profile.Email != ""},
		DisplayName: pgtype.Text{String: profile.DisplayName, Valid: profile.DisplayName != ""},
		AvatarUrl:   pgtype.Text{String: profile.AvatarURL, Valid: profile.AvatarURL != ""},
		IsActive:    true,
	})
	if err == nil {
		user = userFromRow(account)
	}
	identity, err := s.upsertIdentity(ctx, userID, profile)
	if err != nil {
		return Identity{}, User{}, err
	}
	return identity, user, nil
}

func (s *authStore) LinkIdentity(ctx context.Context, userID string, profile NormalizedProfile) (Identity, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return Identity{}, err
	}
	return s.upsertIdentity(ctx, uid, profile)
}

func (s *authStore) UpdateIdentityProfile(ctx context.Context, identityID string, _ NormalizedProfile) error {
	uid, err := db.ParseUUID(identityID)
	if err != nil {
		return err
	}
	if err := s.queries.UpdateIdentityLastLogin(ctx, uid); err != nil {
		return err
	}
	return nil
}

func (s *authStore) FindGroupMappings(ctx context.Context, providerID string, externalGroups []string) ([]GroupMapping, error) {
	pid, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSSOGroupMappingsByProvider(ctx, pid)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]struct{}, len(externalGroups))
	for _, group := range externalGroups {
		wanted[group] = struct{}{}
	}
	out := make([]GroupMapping, 0, len(rows))
	for _, row := range rows {
		if _, ok := wanted[row.ExternalGroup]; ok {
			out = append(out, GroupMapping{ExternalGroup: row.ExternalGroup, GroupID: row.GroupID.String()})
		}
	}
	return out, nil
}

func (s *authStore) ReplaceSSOGroupMemberships(ctx context.Context, userID string, providerID string, groupIDs []string) error {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	pid, err := db.ParseUUID(providerID)
	if err != nil {
		return err
	}
	gids := make([]pgtype.UUID, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		gid, err := db.ParseUUID(groupID)
		if err != nil {
			return err
		}
		gids = append(gids, gid)
	}
	return s.queries.ReplaceSSOGroupMemberships(ctx, sqlc.ReplaceSSOGroupMembershipsParams{UserID: uid, GroupIds: gids, ProviderID: pid})
}

func (s *authStore) upsertIdentity(ctx context.Context, userID pgtype.UUID, profile NormalizedProfile) (Identity, error) {
	pid, err := db.ParseUUID(profile.ProviderID)
	if err != nil {
		return Identity{}, err
	}
	rawClaims, err := json.Marshal(profile.RawClaims)
	if err != nil {
		return Identity{}, err
	}
	if len(rawClaims) == 0 || string(rawClaims) == "null" {
		rawClaims = []byte("{}")
	}
	row, err := s.queries.UpsertExternalIdentity(ctx, sqlc.UpsertExternalIdentityParams{
		UserID:       userID,
		ProviderType: string(profile.ProviderType),
		ProviderID:   pid,
		Subject:      profile.Subject,
		Email:        pgtype.Text{String: profile.Email, Valid: profile.Email != ""},
		Username:     pgtype.Text{String: profile.Username, Valid: profile.Username != ""},
		DisplayName:  pgtype.Text{String: profile.DisplayName, Valid: profile.DisplayName != ""},
		AvatarUrl:    pgtype.Text{String: profile.AvatarURL, Valid: profile.AvatarURL != ""},
		RawClaims:    rawClaims,
	})
	if err != nil {
		return Identity{}, err
	}
	return identityFromRow(row), nil
}

func providerFromRow(row sqlc.IamSsoProvider) (Provider, error) {
	var oidcCfg OIDCProviderConfig
	var samlCfg SAMLProviderConfig
	if row.Type == string(ProviderTypeOIDC) {
		if err := json.Unmarshal(row.Config, &oidcCfg); err != nil {
			return Provider{}, err
		}
	}
	if row.Type == string(ProviderTypeSAML) {
		if err := json.Unmarshal(row.Config, &samlCfg); err != nil {
			return Provider{}, err
		}
	}
	var mapping AttributeMapping
	if len(row.AttributeMapping) > 0 {
		if err := json.Unmarshal(row.AttributeMapping, &mapping); err != nil {
			return Provider{}, err
		}
	}
	return Provider{
		ID:                 row.ID.String(),
		Type:               ProviderType(row.Type),
		Key:                row.Key,
		Name:               row.Name,
		Enabled:            row.Enabled,
		OIDC:               oidcCfg,
		SAML:               samlCfg,
		AttributeMapping:   mapping,
		JITEnabled:         row.JitEnabled,
		EmailLinkingPolicy: EmailLinkingPolicy(row.EmailLinkingPolicy),
		TrustEmail:         row.TrustEmail,
	}, nil
}

func identityFromRow(row sqlc.IamIdentity) Identity {
	return Identity{
		ID:           row.ID.String(),
		UserID:       row.UserID.String(),
		ProviderType: ProviderType(row.ProviderType),
		ProviderID:   row.ProviderID.String(),
		Subject:      row.Subject,
	}
}

func userFromRow(row sqlc.IamUser) User {
	return User{
		ID:          row.ID.String(),
		Email:       row.Email.String,
		Username:    row.Username.String,
		DisplayName: row.DisplayName.String,
		AvatarURL:   row.AvatarUrl.String,
		IsActive:    row.IsActive,
	}
}

func mapSSOErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
