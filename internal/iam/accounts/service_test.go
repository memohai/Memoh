package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestLoginSuccess(t *testing.T) {
	store := newFakeStore(t)
	service := NewService(nil, store)

	account, err := service.Login(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if account.ID != "user-1" {
		t.Fatalf("Login() account ID = %q, want user-1", account.ID)
	}
	if store.touchUserID != "user-1" || store.touchIdentityID != "identity-1" {
		t.Fatalf("TouchLogin() = (%q, %q), want (user-1, identity-1)", store.touchUserID, store.touchIdentityID)
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	store := newFakeStore(t)
	service := NewService(nil, store)

	_, err := service.Login(context.Background(), "alice", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestLoginInactive(t *testing.T) {
	store := newFakeStore(t)
	account := store.accounts["user-1"]
	account.IsActive = false
	store.accounts["user-1"] = account
	service := NewService(nil, store)

	_, err := service.Login(context.Background(), "alice", "secret")
	if !errors.Is(err, ErrInactiveAccount) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInactiveAccount)
	}
}

func TestLoginNoPasswordIdentity(t *testing.T) {
	store := newFakeStore(t)
	delete(store.identitiesBySubject, "alice")
	service := NewService(nil, store)

	_, err := service.Login(context.Background(), "alice", "secret")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestCreateWritesPasswordIdentity(t *testing.T) {
	store := &fakeStore{
		accounts:            map[string]Account{},
		identitiesBySubject: map[string]PasswordIdentity{},
		identitiesByEmail:   map[string]PasswordIdentity{},
		identitiesByUserID:  map[string]PasswordIdentity{},
	}
	service := NewService(nil, store)

	account, err := service.Create(context.Background(), "user-2", CreateAccountRequest{
		Username: "bob",
		Password: "secret",
		Email:    "BOB@example.com",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if account.DisplayName != "bob" {
		t.Fatalf("Create() display name = %q, want bob", account.DisplayName)
	}
	identity := store.identitiesByUserID["user-2"]
	if identity.Subject != "bob" {
		t.Fatalf("password identity subject = %q, want bob", identity.Subject)
	}
	if identity.Email != "bob@example.com" {
		t.Fatalf("password identity email = %q, want bob@example.com", identity.Email)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(identity.CredentialSecret), []byte("secret")); err != nil {
		t.Fatalf("password identity hash does not match: %v", err)
	}
}

func TestUpdatePasswordVerifiesCurrentPassword(t *testing.T) {
	store := newFakeStore(t)
	service := NewService(nil, store)

	if err := service.UpdatePassword(context.Background(), "user-1", "wrong", "new-secret"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("UpdatePassword() wrong current error = %v, want %v", err, ErrInvalidPassword)
	}
	if store.updatedPasswordIdentityID != "" {
		t.Fatalf("UpdatePassword() updated identity after wrong password")
	}

	if err := service.UpdatePassword(context.Background(), "user-1", "secret", "new-secret"); err != nil {
		t.Fatalf("UpdatePassword() error = %v", err)
	}
	if store.updatedPasswordIdentityID != "identity-1" {
		t.Fatalf("updated identity = %q, want identity-1", store.updatedPasswordIdentityID)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(store.updatedPasswordSecret), []byte("new-secret")); err != nil {
		t.Fatalf("updated password hash does not match: %v", err)
	}
}

func newFakeStore(t *testing.T) *fakeStore {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	account := Account{
		ID:          "user-1",
		Username:    "alice",
		Email:       "alice@example.com",
		DisplayName: "Alice",
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	identity := PasswordIdentity{
		ID:               "identity-1",
		UserID:           "user-1",
		Subject:          "alice",
		Email:            "alice@example.com",
		Username:         "alice",
		CredentialSecret: string(hash),
	}
	return &fakeStore{
		accounts: map[string]Account{
			account.ID: account,
		},
		identitiesBySubject: map[string]PasswordIdentity{
			identity.Subject: identity,
		},
		identitiesByEmail: map[string]PasswordIdentity{
			identity.Email: identity,
		},
		identitiesByUserID: map[string]PasswordIdentity{
			identity.UserID: identity,
		},
	}
}

type fakeStore struct {
	accounts            map[string]Account
	identitiesBySubject map[string]PasswordIdentity
	identitiesByEmail   map[string]PasswordIdentity
	identitiesByUserID  map[string]PasswordIdentity

	touchUserID               string
	touchIdentityID           string
	updatedPasswordIdentityID string
	updatedPasswordSecret     string
}

func (s *fakeStore) Get(_ context.Context, userID string) (Account, error) {
	account, ok := s.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	return account, nil
}

func (s *fakeStore) GetPasswordIdentityBySubject(_ context.Context, subject string) (PasswordIdentity, error) {
	identity, ok := s.identitiesBySubject[subject]
	if !ok {
		return PasswordIdentity{}, ErrNotFound
	}
	return identity, nil
}

func (s *fakeStore) GetPasswordIdentityByEmail(_ context.Context, email string) (PasswordIdentity, error) {
	identity, ok := s.identitiesByEmail[email]
	if !ok {
		return PasswordIdentity{}, ErrNotFound
	}
	return identity, nil
}

func (s *fakeStore) GetPasswordIdentityByUserID(_ context.Context, userID string) (PasswordIdentity, error) {
	identity, ok := s.identitiesByUserID[userID]
	if !ok {
		return PasswordIdentity{}, ErrNotFound
	}
	return identity, nil
}

func (s *fakeStore) ListAccounts(context.Context) ([]Account, error) {
	out := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		out = append(out, account)
	}
	return out, nil
}

func (s *fakeStore) SearchAccounts(ctx context.Context, _ string, _ int) ([]Account, error) {
	return s.ListAccounts(ctx)
}

func (s *fakeStore) UpsertAccount(_ context.Context, input UpsertAccountInput) (Account, error) {
	account := Account{
		ID:          input.UserID,
		Username:    input.Username,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		AvatarURL:   input.AvatarURL,
		IsActive:    input.IsActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.accounts[account.ID] = account
	return account, nil
}

func (s *fakeStore) CreateHuman(ctx context.Context, input CreateHumanInput) (Account, error) {
	return s.UpsertAccount(ctx, UpsertAccountInput{
		UserID:      "created-user",
		Username:    input.Username,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		AvatarURL:   input.AvatarURL,
		IsActive:    input.IsActive,
	})
}

func (s *fakeStore) UpsertPasswordIdentity(_ context.Context, input UpsertPasswordIdentityInput) (PasswordIdentity, error) {
	identity := PasswordIdentity{
		ID:               "identity-" + input.UserID,
		UserID:           input.UserID,
		Subject:          input.Subject,
		Email:            input.Email,
		Username:         input.Username,
		CredentialSecret: input.CredentialSecret,
	}
	s.identitiesBySubject[identity.Subject] = identity
	s.identitiesByEmail[identity.Email] = identity
	s.identitiesByUserID[identity.UserID] = identity
	return identity, nil
}

func (s *fakeStore) UpdateAdmin(_ context.Context, input UpdateAdminInput) (Account, error) {
	account, ok := s.accounts[input.UserID]
	if !ok {
		return Account{}, ErrNotFound
	}
	account.DisplayName = input.DisplayName
	account.AvatarURL = input.AvatarURL
	account.IsActive = input.IsActive
	s.accounts[input.UserID] = account
	return account, nil
}

func (s *fakeStore) UpdateProfile(_ context.Context, input UpdateProfileInput) (Account, error) {
	account, ok := s.accounts[input.UserID]
	if !ok {
		return Account{}, ErrNotFound
	}
	account.DisplayName = input.DisplayName
	account.AvatarURL = input.AvatarURL
	account.Timezone = input.Timezone
	s.accounts[input.UserID] = account
	return account, nil
}

func (s *fakeStore) UpdatePasswordIdentity(_ context.Context, identityID string, credentialSecret string) error {
	s.updatedPasswordIdentityID = identityID
	s.updatedPasswordSecret = credentialSecret
	return nil
}

func (s *fakeStore) TouchLogin(_ context.Context, userID string, identityID string) error {
	s.touchUserID = userID
	s.touchIdentityID = identityID
	return nil
}
