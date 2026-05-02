package sso

import (
	"context"
	"errors"
)

type Store interface {
	FindIdentity(ctx context.Context, providerType ProviderType, providerID string, subject string) (Identity, User, error)
	FindUserByEmail(ctx context.Context, normalizedEmail string) (User, error)
	CreateUserWithIdentity(ctx context.Context, profile NormalizedProfile) (Identity, User, error)
	LinkIdentity(ctx context.Context, userID string, profile NormalizedProfile) (Identity, error)
	UpdateIdentityProfile(ctx context.Context, identityID string, profile NormalizedProfile) error
	FindGroupMappings(ctx context.Context, providerID string, externalGroups []string) ([]GroupMapping, error)
	ReplaceSSOGroupMemberships(ctx context.Context, userID string, providerID string, groupIDs []string) error
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) FindOrProvisionUser(ctx context.Context, provider Provider, profile NormalizedProfile) (ProvisionResult, error) {
	if s == nil || s.store == nil {
		return ProvisionResult{}, errors.New("sso: store is required")
	}
	if !provider.Enabled {
		return ProvisionResult{}, ErrProviderDisabled
	}
	if provider.ID == "" || provider.Type == "" {
		return ProvisionResult{}, ErrInvalidProvider
	}
	if profile.Subject == "" {
		return ProvisionResult{}, ErrInvalidProfile
	}
	profile.ProviderID = provider.ID
	profile.ProviderType = provider.Type
	profile.Email = NormalizeEmail(profile.Email)
	profile.Groups = dedupeStrings(profile.Groups)

	identity, user, err := s.store.FindIdentity(ctx, profile.ProviderType, profile.ProviderID, profile.Subject)
	if err == nil {
		if updateErr := s.store.UpdateIdentityProfile(ctx, identity.ID, profile); updateErr != nil {
			return ProvisionResult{}, updateErr
		}
		return ProvisionResult{User: user, Identity: identity}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ProvisionResult{}, err
	}

	if profile.Email != "" {
		switch provider.normalizedEmailLinkingPolicy() {
		case EmailLinkingPolicyLinkExisting:
			if providerCanLinkEmail(provider, profile) {
				existingUser, findErr := s.store.FindUserByEmail(ctx, profile.Email)
				if findErr == nil {
					identity, linkErr := s.store.LinkIdentity(ctx, existingUser.ID, profile)
					if linkErr != nil {
						return ProvisionResult{}, linkErr
					}
					return ProvisionResult{User: existingUser, Identity: identity, Linked: true}, nil
				}
				if !errors.Is(findErr, ErrNotFound) {
					return ProvisionResult{}, findErr
				}
			}
		case EmailLinkingPolicyRejectExisting:
			existingUser, findErr := s.store.FindUserByEmail(ctx, profile.Email)
			if findErr == nil && existingUser.ID != "" {
				return ProvisionResult{}, ErrEmailAlreadyBound
			}
			if findErr != nil && !errors.Is(findErr, ErrNotFound) {
				return ProvisionResult{}, findErr
			}
		default:
			return ProvisionResult{}, ErrInvalidProvider
		}
	}

	if !provider.JITEnabled {
		return ProvisionResult{}, ErrJITDisabled
	}
	identity, user, err = s.store.CreateUserWithIdentity(ctx, profile)
	if err != nil {
		return ProvisionResult{}, err
	}
	return ProvisionResult{User: user, Identity: identity, Created: true}, nil
}

func (s *Service) SyncMappedGroups(ctx context.Context, provider Provider, userID string, externalGroups []string) error {
	if s == nil || s.store == nil {
		return errors.New("sso: store is required")
	}
	if provider.ID == "" {
		return ErrInvalidProvider
	}
	if userID == "" {
		return ErrInvalidProfile
	}
	externalGroups = dedupeStrings(externalGroups)
	mappings, err := s.store.FindGroupMappings(ctx, provider.ID, externalGroups)
	if err != nil {
		return err
	}
	groupIDs := make([]string, 0, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping.GroupID == "" {
			continue
		}
		if _, ok := seen[mapping.GroupID]; ok {
			continue
		}
		seen[mapping.GroupID] = struct{}{}
		groupIDs = append(groupIDs, mapping.GroupID)
	}
	return s.store.ReplaceSSOGroupMemberships(ctx, userID, provider.ID, groupIDs)
}

func providerCanLinkEmail(provider Provider, profile NormalizedProfile) bool {
	if !provider.TrustEmail {
		return false
	}
	if profile.ProviderType == ProviderTypeOIDC && !profile.EmailVerified {
		return false
	}
	return true
}
