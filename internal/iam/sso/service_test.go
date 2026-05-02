package sso

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeStore struct {
	identity     Identity
	identityUser User
	identityErr  error

	userByEmail    User
	userByEmailErr error

	createdIdentity Identity
	createdUser     User
	linkedIdentity  Identity

	updatedIdentityID string
	updatedProfile    NormalizedProfile
	linkedUserID      string
	linkedProfile     NormalizedProfile
	createdProfile    NormalizedProfile

	mappings              []GroupMapping
	mappingProviderID     string
	mappingExternalGroups []string
	replacedUserID        string
	replacedProviderID    string
	replacedGroupIDs      []string
}

func (s *fakeStore) FindIdentity(context.Context, ProviderType, string, string) (Identity, User, error) {
	return s.identity, s.identityUser, s.identityErr
}

func (s *fakeStore) FindUserByEmail(context.Context, string) (User, error) {
	return s.userByEmail, s.userByEmailErr
}

func (s *fakeStore) CreateUserWithIdentity(_ context.Context, profile NormalizedProfile) (Identity, User, error) {
	s.createdProfile = profile
	return s.createdIdentity, s.createdUser, nil
}

func (s *fakeStore) LinkIdentity(_ context.Context, userID string, profile NormalizedProfile) (Identity, error) {
	s.linkedUserID = userID
	s.linkedProfile = profile
	return s.linkedIdentity, nil
}

func (s *fakeStore) UpdateIdentityProfile(_ context.Context, identityID string, profile NormalizedProfile) error {
	s.updatedIdentityID = identityID
	s.updatedProfile = profile
	return nil
}

func (s *fakeStore) FindGroupMappings(_ context.Context, providerID string, externalGroups []string) ([]GroupMapping, error) {
	s.mappingProviderID = providerID
	s.mappingExternalGroups = append([]string(nil), externalGroups...)
	return s.mappings, nil
}

func (s *fakeStore) ReplaceSSOGroupMemberships(_ context.Context, userID string, providerID string, groupIDs []string) error {
	s.replacedUserID = userID
	s.replacedProviderID = providerID
	s.replacedGroupIDs = append([]string(nil), groupIDs...)
	return nil
}

func testProvider() Provider {
	return Provider{
		ID:                 "provider-1",
		Type:               ProviderTypeOIDC,
		Enabled:            true,
		JITEnabled:         true,
		EmailLinkingPolicy: EmailLinkingPolicyLinkExisting,
		TrustEmail:         true,
	}
}

func TestFindOrProvisionUserUpdatesExistingIdentity(t *testing.T) {
	store := &fakeStore{
		identity:     Identity{ID: "identity-1", UserID: "user-1"},
		identityUser: User{ID: "user-1"},
		identityErr:  nil,
	}
	service := NewService(store)

	result, err := service.FindOrProvisionUser(context.Background(), testProvider(), NormalizedProfile{Subject: "issuer|sub", Email: "User@Example.COM"})
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if result.User.ID != "user-1" || result.Identity.ID != "identity-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if store.updatedIdentityID != "identity-1" {
		t.Fatalf("identity was not updated")
	}
	if store.updatedProfile.Email != "user@example.com" {
		t.Fatalf("email not normalized: %q", store.updatedProfile.Email)
	}
}

func TestFindOrProvisionUserLinksExistingEmailWhenTrustedOIDCVerified(t *testing.T) {
	store := &fakeStore{
		identityErr:    ErrNotFound,
		userByEmail:    User{ID: "user-1", Email: "user@example.com"},
		userByEmailErr: nil,
		linkedIdentity: Identity{ID: "identity-2", UserID: "user-1"},
	}
	service := NewService(store)

	result, err := service.FindOrProvisionUser(context.Background(), testProvider(), NormalizedProfile{
		Subject:       "issuer|sub",
		Email:         "User@Example.COM",
		EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("link user: %v", err)
	}
	if !result.Linked || store.linkedUserID != "user-1" {
		t.Fatalf("expected linked existing user, result=%#v linkedUserID=%q", result, store.linkedUserID)
	}
}

func TestFindOrProvisionUserDoesNotLinkUnverifiedOIDCEmail(t *testing.T) {
	store := &fakeStore{
		identityErr:     ErrNotFound,
		userByEmail:     User{ID: "user-1", Email: "user@example.com"},
		userByEmailErr:  nil,
		createdIdentity: Identity{ID: "identity-3", UserID: "user-2"},
		createdUser:     User{ID: "user-2"},
	}
	service := NewService(store)

	result, err := service.FindOrProvisionUser(context.Background(), testProvider(), NormalizedProfile{
		Subject:       "issuer|sub",
		Email:         "user@example.com",
		EmailVerified: false,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if !result.Created || store.linkedUserID != "" {
		t.Fatalf("expected create without link, result=%#v linkedUserID=%q", result, store.linkedUserID)
	}
}

func TestFindOrProvisionUserRejectsExistingEmail(t *testing.T) {
	store := &fakeStore{
		identityErr:    ErrNotFound,
		userByEmail:    User{ID: "user-1"},
		userByEmailErr: nil,
	}
	provider := testProvider()
	provider.EmailLinkingPolicy = EmailLinkingPolicyRejectExisting
	service := NewService(store)

	_, err := service.FindOrProvisionUser(context.Background(), provider, NormalizedProfile{
		Subject: "issuer|sub",
		Email:   "user@example.com",
	})
	if !errors.Is(err, ErrEmailAlreadyBound) {
		t.Fatalf("expected ErrEmailAlreadyBound, got %v", err)
	}
}

func TestSyncMappedGroupsReplacesOnlyMappedGroups(t *testing.T) {
	store := &fakeStore{
		mappings: []GroupMapping{
			{ExternalGroup: "engineering", GroupID: "group-1"},
			{ExternalGroup: "ops", GroupID: "group-2"},
			{ExternalGroup: "ops-duplicate", GroupID: "group-2"},
		},
	}
	service := NewService(store)

	err := service.SyncMappedGroups(context.Background(), testProvider(), "user-1", []string{"engineering", "ops", "engineering", "unmapped"})
	if err != nil {
		t.Fatalf("sync groups: %v", err)
	}
	if store.mappingProviderID != "provider-1" {
		t.Fatalf("mapping provider = %q", store.mappingProviderID)
	}
	if !reflect.DeepEqual(store.mappingExternalGroups, []string{"engineering", "ops", "unmapped"}) {
		t.Fatalf("external groups = %#v", store.mappingExternalGroups)
	}
	if !reflect.DeepEqual(store.replacedGroupIDs, []string{"group-1", "group-2"}) {
		t.Fatalf("replaced groups = %#v", store.replacedGroupIDs)
	}
}
