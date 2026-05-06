package rbac

import (
	"context"
	"errors"
	"testing"
)

type fakeBootstrapStore struct {
	permissions     []PermissionSeed
	roles           []RoleSeed
	rolePermissions []RolePermissionSeed
	errAt           string
}

func (s *fakeBootstrapStore) EnsurePermissions(_ context.Context, permissions []PermissionSeed) error {
	if s.errAt == "permissions" {
		return errors.New("permissions failed")
	}
	s.permissions = append([]PermissionSeed(nil), permissions...)
	return nil
}

func (s *fakeBootstrapStore) EnsureRoles(_ context.Context, roles []RoleSeed) error {
	if s.errAt == "roles" {
		return errors.New("roles failed")
	}
	s.roles = append([]RoleSeed(nil), roles...)
	return nil
}

func (s *fakeBootstrapStore) EnsureRolePermissions(_ context.Context, rolePermissions []RolePermissionSeed) error {
	if s.errAt == "role_permissions" {
		return errors.New("role permissions failed")
	}
	s.rolePermissions = append([]RolePermissionSeed(nil), rolePermissions...)
	return nil
}

func TestBootstrapSeedsBuiltinRBAC(t *testing.T) {
	store := &fakeBootstrapStore{}
	if err := Bootstrap(context.Background(), store); err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if len(store.permissions) != len(BuiltinPermissions) {
		t.Fatalf("permissions = %d, want %d", len(store.permissions), len(BuiltinPermissions))
	}
	if len(store.roles) != len(BuiltinRoles) {
		t.Fatalf("roles = %d, want %d", len(store.roles), len(BuiltinRoles))
	}
	if len(store.rolePermissions) != len(BuiltinRolePermissions) {
		t.Fatalf("role permissions = %d, want %d", len(store.rolePermissions), len(BuiltinRolePermissions))
	}
}

func TestBootstrapStopsOnPermissionError(t *testing.T) {
	store := &fakeBootstrapStore{errAt: "permissions"}
	err := Bootstrap(context.Background(), store)
	if err == nil {
		t.Fatal("Bootstrap returned nil error")
	}
	if len(store.roles) != 0 || len(store.rolePermissions) != 0 {
		t.Fatal("Bootstrap continued after permissions error")
	}
}
