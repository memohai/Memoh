package sso

import (
	"errors"
	"strings"
	"time"
)

type ProviderType string

const (
	ProviderTypeOIDC ProviderType = "oidc"
	ProviderTypeSAML ProviderType = "saml"
)

type EmailLinkingPolicy string

const (
	EmailLinkingPolicyLinkExisting   EmailLinkingPolicy = "link_existing"
	EmailLinkingPolicyRejectExisting EmailLinkingPolicy = "reject_existing"
)

type OIDCProviderConfig struct {
	Issuer                string   `json:"issuer"`
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret"` //nolint:gosec // provider configuration must carry the OAuth client secret.
	RedirectURL           string   `json:"redirect_url"`
	Scopes                []string `json:"scopes"`
	AuthorizationEndpoint string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string   `json:"token_endpoint,omitempty"`
}

type SAMLProviderConfig struct {
	EntityID    string `json:"entity_id"`
	MetadataXML string `json:"metadata_xml"`
	MetadataURL string `json:"metadata_url,omitempty"`
	ACSURL      string `json:"acs_url"`
}

type Provider struct {
	ID                 string             `json:"id"`
	Type               ProviderType       `json:"type"`
	Key                string             `json:"key"`
	Name               string             `json:"name"`
	Enabled            bool               `json:"enabled"`
	OIDC               OIDCProviderConfig `json:"oidc,omitempty"`
	SAML               SAMLProviderConfig `json:"saml,omitempty"`
	AttributeMapping   AttributeMapping   `json:"attribute_mapping"`
	JITEnabled         bool               `json:"jit_enabled"`
	EmailLinkingPolicy EmailLinkingPolicy `json:"email_linking_policy"`
	TrustEmail         bool               `json:"trust_email"`
}

func (p Provider) normalizedEmailLinkingPolicy() EmailLinkingPolicy {
	switch p.EmailLinkingPolicy {
	case "", EmailLinkingPolicyLinkExisting:
		return EmailLinkingPolicyLinkExisting
	case EmailLinkingPolicyRejectExisting:
		return EmailLinkingPolicyRejectExisting
	default:
		return p.EmailLinkingPolicy
	}
}

type AttributeMapping struct {
	Subject     string   `json:"subject,omitempty"`
	Email       string   `json:"email,omitempty"`
	Username    string   `json:"username,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	AvatarURL   string   `json:"avatar_url,omitempty"`
	Groups      []string `json:"groups,omitempty"`
}

type NormalizedProfile struct {
	ProviderType  ProviderType      `json:"provider_type"`
	ProviderID    string            `json:"provider_id"`
	Subject       string            `json:"subject"`
	Email         string            `json:"email,omitempty"`
	EmailVerified bool              `json:"email_verified,omitempty"`
	Username      string            `json:"username,omitempty"`
	DisplayName   string            `json:"display_name,omitempty"`
	AvatarURL     string            `json:"avatar_url,omitempty"`
	Groups        []string          `json:"groups,omitempty"`
	RawClaims     map[string]any    `json:"raw_claims,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

func (p NormalizedProfile) NormalizedEmail() string {
	return NormalizeEmail(p.Email)
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type User struct {
	ID          string
	Email       string
	Username    string
	DisplayName string
	AvatarURL   string
	IsActive    bool
}

type Identity struct {
	ID           string
	UserID       string
	ProviderType ProviderType
	ProviderID   string
	Subject      string
}

type GroupMapping struct {
	ExternalGroup string
	GroupID       string
}

type ProvisionResult struct {
	User     User
	Identity Identity
	Created  bool
	Linked   bool
}

type LoginCode struct {
	Code        string
	UserID      string
	SessionID   string
	RedirectURL string
}

type SAMLAuthRedirect struct {
	URL        string
	RelayState string
	RequestID  string
}

var (
	ErrNotFound          = errors.New("sso: not found")
	ErrProviderDisabled  = errors.New("sso: provider disabled")
	ErrInvalidProvider   = errors.New("sso: invalid provider")
	ErrInvalidProfile    = errors.New("sso: invalid profile")
	ErrJITDisabled       = errors.New("sso: jit disabled")
	ErrEmailAlreadyBound = errors.New("sso: email already bound")
)

const (
	OIDCStateCookieName = "memoh_oidc_state"
	OIDCStateCookieTTL  = 10 * time.Minute
	SAMLStateCookieName = "memoh_saml_state"
	SAMLStateCookieTTL  = 10 * time.Minute
	LoginCodeBytes      = 32
	LoginCodeTTL        = 2 * time.Minute
)
