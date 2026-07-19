package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	dbstore "github.com/memohai/memoh/internal/db/store"
)

type testAccountStore struct {
	created      dbstore.CreateAccountInput
	record       dbstore.AccountRecord
	getErr       error
	adminUpdated dbstore.UpdateAccountAdminInput
	password     dbstore.UpdateAccountPasswordInput
	passwordErr  error
}

func TestCreatePersistsAccountWithoutProvisioningProviderInstances(t *testing.T) {
	t.Parallel()

	store := &testAccountStore{}
	service := NewService(nil, store)
	account, err := service.Create(context.Background(), "user-1", CreateAccountRequest{
		Username: "alice",
		Password: "secret",
		Role:     "member",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if account.ID != "user-1" || store.created.UserID != "user-1" || store.created.Username != "alice" {
		t.Fatalf("created account = %#v, input = %#v", account, store.created)
	}
}

func (*testAccountStore) CountAccounts(context.Context) (int64, error) { return 0, nil }
func (s *testAccountStore) GetByUserID(context.Context, string) (dbstore.AccountRecord, error) {
	return s.record, s.getErr
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
func (s *testAccountStore) UpdateAdmin(_ context.Context, input dbstore.UpdateAccountAdminInput) (dbstore.AccountRecord, error) {
	s.adminUpdated = input
	return s.record, nil
}

func (*testAccountStore) UpdateProfile(context.Context, dbstore.UpdateAccountProfileInput) (dbstore.AccountRecord, error) {
	return dbstore.AccountRecord{}, errors.New("not implemented")
}

func (s *testAccountStore) UpdatePassword(_ context.Context, input dbstore.UpdateAccountPasswordInput) error {
	s.password = input
	return s.passwordErr
}

func (*testAccountStore) RemoveMember(context.Context, string) error {
	return errors.New("not implemented")
}

func TestValidateSessionAndIsAdminRequireActiveAccount(t *testing.T) {
	store := &testAccountStore{record: dbstore.AccountRecord{ID: "user-1", Role: "admin", IsActive: false}}
	svc := NewService(nil, store)

	if err := svc.ValidateSession(context.Background(), "user-1"); !errors.Is(err, ErrInactiveAccount) {
		t.Fatalf("ValidateSession() error = %v, want ErrInactiveAccount", err)
	}
	isAdmin, err := svc.IsAdmin(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("IsAdmin() error = %v", err)
	}
	if isAdmin {
		t.Fatal("inactive account must not retain admin authority")
	}

	store.record.IsActive = true
	if err := svc.ValidateSession(context.Background(), "user-1"); err != nil {
		t.Fatalf("ValidateSession() active error = %v", err)
	}
	isAdmin, err = svc.IsAdmin(context.Background(), "user-1")
	if err != nil || !isAdmin {
		t.Fatalf("IsAdmin() active = %v, %v", isAdmin, err)
	}
}

func TestUpdateAdminLeavesMembershipStateUnspecified(t *testing.T) {
	store := &testAccountStore{record: dbstore.AccountRecord{ID: "user-1", Role: "member", IsActive: false}}
	svc := NewService(nil, store)
	role := "admin"

	if _, err := svc.UpdateAdmin(context.Background(), "user-1", UpdateAccountRequest{Role: &role}); err != nil {
		t.Fatalf("UpdateAdmin() error = %v", err)
	}
	if store.adminUpdated.IsActive != nil {
		t.Fatalf("role-only update supplied membership state %v", *store.adminUpdated.IsActive)
	}
}

func TestResetPasswordHashesAndStoresNewCredential(t *testing.T) {
	store := &testAccountStore{}
	svc := NewService(nil, store)

	if err := svc.ResetPassword(context.Background(), "user-1", "new-secret"); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if store.password.UserID != "user-1" {
		t.Fatalf("UpdatePassword() user = %q, want user-1", store.password.UserID)
	}
	if store.password.PasswordHash == "new-secret" {
		t.Fatal("ResetPassword() stored the plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(store.password.PasswordHash), []byte("new-secret")); err != nil {
		t.Fatalf("stored password hash does not match new credential: %v", err)
	}
}

func TestResetPasswordRejectsBlankCredential(t *testing.T) {
	store := &testAccountStore{}
	svc := NewService(nil, store)

	if err := svc.ResetPassword(context.Background(), "user-1", "  "); err == nil {
		t.Fatal("ResetPassword() should reject a blank password")
	}
	if store.password.UserID != "" {
		t.Fatal("blank password must not reach the account store")
	}
}
