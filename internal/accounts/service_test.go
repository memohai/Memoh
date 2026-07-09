package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/teams"
)

type testAccountStore struct {
	created dbstore.CreateAccountInput
}

func (*testAccountStore) CountAccounts(context.Context) (int64, error) { return 0, nil }
func (*testAccountStore) GetByUserID(context.Context, string) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{}, errors.New("not implemented")
}

func (*testAccountStore) GetByIdentity(context.Context, string) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{}, errors.New("not implemented")
}

func (*testAccountStore) List(context.Context) ([]dbstore.AccountRecord, error) { return nil, nil }

func (*testAccountStore) Search(context.Context, string, int32) ([]dbstore.AccountRecord, error) {
	return nil, nil
}

func (*testAccountStore) CreateUser(context.Context, dbstore.CreateUserInput) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{}, errors.New("not implemented")
}

func (s *testAccountStore) CreateAccount(_ context.Context, input dbstore.CreateAccountInput) (dbstore.AccountRecord, error) {
	s.created = input
	now := time.Now()
	return dbstore.AccountRecord{
		ID:              input.UserID,
		Username:        input.Username,
		Email:           input.Email,
		Role:            input.Role,
		DisplayName:     input.DisplayName,
		AvatarURL:       input.AvatarURL,
		PasswordHash:    input.PasswordHash,
		HasPasswordHash: true,
		IsActive:        input.IsActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}
func (*testAccountStore) UpdateLastLogin(context.Context, string) error { return nil }
func (*testAccountStore) UpdateAdmin(context.Context, dbstore.UpdateAccountAdminInput) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{}, errors.New("not implemented")
}

func (*testAccountStore) UpdateProfile(context.Context, dbstore.UpdateAccountProfileInput) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{}, errors.New("not implemented")
}

func (*testAccountStore) UpdatePassword(context.Context, dbstore.UpdateAccountPasswordInput) error {
	return errors.New("not implemented")
}

func (*testAccountStore) RemoveMember(context.Context, string) error {
	return errors.New("not implemented")
}

type testEmailBootstrapper struct {
	userID string
	err    error
}

func (b *testEmailBootstrapper) EnsureDefaultGmailProvider(_ context.Context, userID string) error {
	b.userID = userID
	return b.err
}

func TestIsAdminReadsScopeRole(t *testing.T) {
	svc := &Service{} // IsAdmin(ctx) does not need store
	for role, want := range map[string]bool{"owner": true, "admin": true, "member": false, "": false} {
		ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teams.DefaultTeamID, UserID: "u1", Role: role})
		got, err := svc.IsAdmin(ctx)
		if err != nil {
			t.Fatalf("role %q: %v", role, err)
		}
		if got != want {
			t.Fatalf("role %q: IsAdmin=%v want %v", role, got, want)
		}
	}
}

func TestCreateEnsuresDefaultGmailProvider(t *testing.T) {
	store := &testAccountStore{}
	bootstrapper := &testEmailBootstrapper{}
	svc := NewService(nil, store)
	svc.SetEmailProviderBootstrapper(bootstrapper)

	account, err := svc.Create(context.Background(), "user-1", CreateAccountRequest{
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if account.ID != "user-1" {
		t.Fatalf("expected created account id user-1, got %q", account.ID)
	}
	if bootstrapper.userID != "user-1" {
		t.Fatalf("expected default gmail bootstrap for user-1, got %q", bootstrapper.userID)
	}
}

func TestCreateReturnsBootstrapperError(t *testing.T) {
	bootstrapper := &testEmailBootstrapper{err: errors.New("boom")}
	svc := NewService(nil, &testAccountStore{})
	svc.SetEmailProviderBootstrapper(bootstrapper)

	_, err := svc.Create(context.Background(), "user-1", CreateAccountRequest{
		Username: "alice",
		Password: "secret",
	})
	if err == nil {
		t.Fatal("Create should return bootstrapper error")
	}
	if !errors.Is(err, bootstrapper.err) {
		t.Fatalf("expected bootstrapper error, got %v", err)
	}
}
