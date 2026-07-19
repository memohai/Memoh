package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	dbstore "github.com/memohai/memoh/internal/db/store"
)

type testAccountStore struct {
	created dbstore.CreateAccountInput
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
