package sso

import (
	"reflect"
	"testing"

	"github.com/crewjam/saml"
)

func TestNormalizeSAMLAssertion(t *testing.T) {
	provider := Provider{
		ID:      "provider-1",
		Type:    ProviderTypeSAML,
		Enabled: true,
		AttributeMapping: AttributeMapping{
			Groups: []string{"memberOf"},
		},
	}
	assertion := &saml.Assertion{
		Subject: &saml.Subject{NameID: &saml.NameID{Value: "name-id-1"}},
		AttributeStatements: []saml.AttributeStatement{
			{
				Attributes: []saml.Attribute{
					{Name: "email", Values: []saml.AttributeValue{{Value: "User@Example.COM"}}},
					{Name: "displayName", Values: []saml.AttributeValue{{Value: "Jane Doe"}}},
					{Name: "memberOf", Values: []saml.AttributeValue{{Value: "engineering"}, {Value: "ops"}, {Value: "engineering"}}},
				},
			},
		},
	}

	profile, err := NormalizeSAMLAssertion(provider, assertion)
	if err != nil {
		t.Fatalf("normalize assertion: %v", err)
	}
	if profile.Subject != "name-id-1" || profile.Email != "user@example.com" || profile.DisplayName != "Jane Doe" {
		t.Fatalf("profile = %#v", profile)
	}
	if !reflect.DeepEqual(profile.Groups, []string{"engineering", "ops"}) {
		t.Fatalf("groups = %#v", profile.Groups)
	}
}
