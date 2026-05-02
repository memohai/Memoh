package sso

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

type OIDCAuthRedirect struct {
	URL           string
	CodeVerifier  string
	CodeChallenge string
}

func BuildOIDCAuthRedirect(ctx context.Context, cfg OIDCProviderConfig, state string, nonce string, codeVerifier string) (OIDCAuthRedirect, error) {
	if state == "" || nonce == "" || codeVerifier == "" {
		return OIDCAuthRedirect{}, ErrInvalidProfile
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	relyingParty, err := relyingPartyForConfig(ctx, cfg, scopes)
	if err != nil {
		return OIDCAuthRedirect{}, err
	}
	codeChallenge := oidc.NewSHACodeChallenge(codeVerifier)
	authURL := rp.AuthURL(
		state,
		relyingParty,
		rp.WithCodeChallenge(codeChallenge),
		rp.AuthURLOpt(rp.WithURLParam("nonce", nonce)),
	)
	return OIDCAuthRedirect{URL: authURL, CodeVerifier: codeVerifier, CodeChallenge: codeChallenge}, nil
}

func ExchangeOIDCCode(ctx context.Context, cfg OIDCProviderConfig, code string, codeVerifier string) (*oidc.IDTokenClaims, error) {
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	relyingParty, err := relyingPartyForConfig(ctx, cfg, scopes)
	if err != nil {
		return nil, err
	}
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, relyingParty, rp.WithCodeVerifier(codeVerifier))
	if err != nil {
		return nil, err
	}
	return tokens.IDTokenClaims, nil
}

func relyingPartyForConfig(ctx context.Context, cfg OIDCProviderConfig, scopes []string) (rp.RelyingParty, error) {
	if cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, ErrInvalidProvider
	}
	if cfg.AuthorizationEndpoint != "" {
		oauthConfig := &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  cfg.AuthorizationEndpoint,
				TokenURL: cfg.TokenEndpoint,
			},
		}
		return rp.NewRelyingPartyOAuth(oauthConfig)
	}
	if cfg.Issuer == "" {
		return nil, ErrInvalidProvider
	}
	return rp.NewRelyingPartyOIDC(ctx, cfg.Issuer, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL, scopes)
}

func NormalizeOIDCClaims(provider Provider, claims *oidc.IDTokenClaims) (NormalizedProfile, error) {
	if claims == nil {
		return NormalizedProfile{}, ErrInvalidProfile
	}
	issuer := strings.TrimSpace(claims.Issuer)
	subject := strings.TrimSpace(claims.Subject)
	if issuer == "" || subject == "" {
		return NormalizedProfile{}, ErrInvalidProfile
	}
	rawClaims := map[string]any{}
	if claims.Claims != nil {
		for key, value := range claims.Claims {
			rawClaims[key] = value
		}
	}
	rawClaims["iss"] = issuer
	rawClaims["sub"] = subject
	rawClaims["email"] = claims.Email
	rawClaims["email_verified"] = bool(claims.EmailVerified)
	rawClaims["preferred_username"] = claims.PreferredUsername
	rawClaims["name"] = claims.Name
	rawClaims["picture"] = claims.Picture

	mapping := provider.AttributeMapping
	email := firstClaimString(rawClaims, mapping.Email, "email")
	username := firstClaimString(rawClaims, mapping.Username, "preferred_username", "nickname", "email")
	displayName := firstClaimString(rawClaims, mapping.DisplayName, "name", "preferred_username", "email")
	avatarURL := firstClaimString(rawClaims, mapping.AvatarURL, "picture")
	groupKeys := mapping.Groups
	if len(groupKeys) == 0 {
		groupKeys = []string{"groups", "roles", "memberOf"}
	}

	return NormalizedProfile{
		ProviderType:  ProviderTypeOIDC,
		ProviderID:    provider.ID,
		Subject:       issuer + "|" + subject,
		Email:         NormalizeEmail(email),
		EmailVerified: bool(claims.EmailVerified),
		Username:      username,
		DisplayName:   displayName,
		AvatarURL:     avatarURL,
		Groups:        extractClaimStrings(rawClaims, groupKeys...),
		RawClaims:     rawClaims,
	}, nil
}

func firstClaimString(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		values := extractClaimStrings(claims, key)
		if len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func extractClaimStrings(claims map[string]any, keys ...string) []string {
	out := make([]string, 0)
	for _, key := range keys {
		value, ok := nestedClaim(claims, key)
		if !ok {
			continue
		}
		out = append(out, stringifyClaim(value)...)
	}
	return dedupeStrings(out)
}

func nestedClaim(claims map[string]any, key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	parts := strings.Split(key, ".")
	var current any = claims
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringifyClaim(value any) []string {
	switch v := value.(type) {
	case string:
		if strings.Contains(v, ",") {
			return strings.Split(v, ",")
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringifyClaim(item)...)
		}
		return out
	case json.Number:
		return []string{v.String()}
	default:
		return nil
	}
}
