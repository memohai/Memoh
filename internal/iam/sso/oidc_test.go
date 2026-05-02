package sso

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"github.com/zitadel/oidc/v3/pkg/oidc"
)

func TestBuildOIDCAuthRedirect(t *testing.T) { //nolint:gosec // test client IDs and endpoints are not credentials.
	// #nosec G101 -- test client IDs and endpoints are not credentials.
	redirect, err := BuildOIDCAuthRedirect(context.Background(), OIDCProviderConfig{
		ClientID:              "client-1",
		RedirectURL:           "https://memoh.example.com/auth/sso/google/callback",
		Scopes:                []string{"openid", "profile", "email", "groups"},
		AuthorizationEndpoint: "https://idp.example.com/oauth2/auth",
		TokenEndpoint:         "https://idp.example.com/oauth2/token",
	}, "state-1", "nonce-1", "verifier-1")
	if err != nil {
		t.Fatalf("build redirect: %v", err)
	}
	parsed, err := url.Parse(redirect.URL)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	query := parsed.Query()
	if parsed.String() == "" || query.Get("state") != "state-1" || query.Get("nonce") != "nonce-1" {
		t.Fatalf("redirect missing state/nonce: %s", redirect.URL)
	}
	if query.Get("code_challenge") != redirect.CodeChallenge || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("redirect missing pkce: %s", redirect.URL)
	}
	if redirect.CodeVerifier != "verifier-1" {
		t.Fatalf("code verifier = %q", redirect.CodeVerifier)
	}
}

func TestNormalizeOIDCClaims(t *testing.T) {
	provider := testProvider()
	provider.AttributeMapping = AttributeMapping{
		Groups: []string{"groups", "realm_access.roles"},
	}
	claims := &oidc.IDTokenClaims{
		TokenClaims: oidc.TokenClaims{
			Issuer:  "https://accounts.example.com",
			Subject: "subject-1",
		},
		UserInfoProfile: oidc.UserInfoProfile{
			Name:              "Jane Doe",
			PreferredUsername: "jane",
			Picture:           "https://example.com/avatar.png",
		},
		UserInfoEmail: oidc.UserInfoEmail{
			Email:         "Jane@Example.COM",
			EmailVerified: true,
		},
		Claims: map[string]any{
			"groups": []any{"engineering", "ops"},
			"realm_access": map[string]any{
				"roles": []any{"admin", "engineering"},
			},
		},
	}

	profile, err := NormalizeOIDCClaims(provider, claims)
	if err != nil {
		t.Fatalf("normalize claims: %v", err)
	}
	if profile.Subject != "https://accounts.example.com|subject-1" {
		t.Fatalf("subject = %q", profile.Subject)
	}
	if profile.Email != "jane@example.com" || !profile.EmailVerified {
		t.Fatalf("email = %q verified=%v", profile.Email, profile.EmailVerified)
	}
	if profile.Username != "jane" || profile.DisplayName != "Jane Doe" || profile.AvatarURL == "" {
		t.Fatalf("profile fields = %#v", profile)
	}
	if !reflect.DeepEqual(profile.Groups, []string{"engineering", "ops", "admin"}) {
		t.Fatalf("groups = %#v", profile.Groups)
	}
}
