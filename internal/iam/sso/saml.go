package sso

import (
	"strings"

	"github.com/crewjam/saml"
)

func NormalizeSAMLAssertion(provider Provider, assertion *saml.Assertion) (NormalizedProfile, error) {
	if assertion == nil || assertion.Subject == nil || assertion.Subject.NameID == nil {
		return NormalizedProfile{}, ErrInvalidProfile
	}
	subject := strings.TrimSpace(assertion.Subject.NameID.Value)
	if subject == "" {
		return NormalizedProfile{}, ErrInvalidProfile
	}
	attributes := samlAttributes(assertion)
	mapping := provider.AttributeMapping

	email := firstAttribute(attributes, mapping.Email, "email", "Email", "mail")
	username := firstAttribute(attributes, mapping.Username, "username", "uid", "email", "Email", "mail")
	displayName := firstAttribute(attributes, mapping.DisplayName, "displayName", "DisplayName", "name", "cn")
	avatarURL := firstAttribute(attributes, mapping.AvatarURL, "avatar_url", "picture")
	groupKeys := mapping.Groups
	if len(groupKeys) == 0 {
		groupKeys = []string{"groups", "Groups", "memberOf"}
	}

	return NormalizedProfile{
		ProviderType: ProviderTypeSAML,
		ProviderID:   provider.ID,
		Subject:      subject,
		Email:        NormalizeEmail(email),
		Username:     username,
		DisplayName:  displayName,
		AvatarURL:    avatarURL,
		Groups:       extractAttributes(attributes, groupKeys...),
		Attributes:   firstAttributeValues(attributes),
	}, nil
}

func samlAttributes(assertion *saml.Assertion) map[string][]string {
	attributes := make(map[string][]string)
	for _, statement := range assertion.AttributeStatements {
		for _, attribute := range statement.Attributes {
			values := make([]string, 0, len(attribute.Values))
			for _, value := range attribute.Values {
				if strings.TrimSpace(value.Value) != "" {
					values = append(values, value.Value)
				}
				if value.NameID != nil && strings.TrimSpace(value.NameID.Value) != "" {
					values = append(values, value.NameID.Value)
				}
			}
			values = dedupeStrings(values)
			if len(values) == 0 {
				continue
			}
			if attribute.Name != "" {
				attributes[attribute.Name] = append(attributes[attribute.Name], values...)
			}
			if attribute.FriendlyName != "" {
				attributes[attribute.FriendlyName] = append(attributes[attribute.FriendlyName], values...)
			}
		}
	}
	for key, values := range attributes {
		attributes[key] = dedupeStrings(values)
	}
	return attributes
}

func firstAttribute(attributes map[string][]string, keys ...string) string {
	values := extractAttributes(attributes, keys...)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func extractAttributes(attributes map[string][]string, keys ...string) []string {
	out := make([]string, 0)
	for _, key := range keys {
		if key == "" {
			continue
		}
		out = append(out, attributes[key]...)
	}
	return dedupeStrings(out)
}

func firstAttributeValues(attributes map[string][]string) map[string]string {
	out := make(map[string]string, len(attributes))
	for key, values := range attributes {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}
