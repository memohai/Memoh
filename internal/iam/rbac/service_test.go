package rbac

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	allowed    map[string]bool
	adminUsers map[string]bool
	err        error
	checks     int
	adminCalls int
}

func (s *fakeStore) HasPermission(_ context.Context, check Check) (bool, error) {
	s.checks++
	if s.err != nil {
		return false, s.err
	}
	return s.allowed[cacheKey(check)], nil
}

func (s *fakeStore) HasSystemAdmin(_ context.Context, userID string) (bool, error) {
	s.adminCalls++
	if s.err != nil {
		return false, s.err
	}
	return s.adminUsers[userID], nil
}

func TestHasPermissionUsesStoreAndCache(t *testing.T) {
	check := Check{
		UserID:        "user-1",
		PermissionKey: PermissionBotRead,
		ResourceType:  ResourceBot,
		ResourceID:    "bot-1",
	}
	store := &fakeStore{
		allowed:    map[string]bool{cacheKey(check): true},
		adminUsers: map[string]bool{},
	}
	service := NewServiceWithCache(store, 100, time.Minute)

	allowed, err := service.HasPermission(context.Background(), check)
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("allowed = false, want true")
	}
	allowed, err = service.HasPermission(context.Background(), check)
	if err != nil {
		t.Fatalf("second HasPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("second allowed = false, want true")
	}
	if store.checks != 1 {
		t.Fatalf("store checks = %d, want 1", store.checks)
	}
	if store.adminCalls != 1 {
		t.Fatalf("admin calls = %d, want 1", store.adminCalls)
	}
}

func TestHasPermissionAllowsSystemAdmin(t *testing.T) {
	store := &fakeStore{
		allowed:    map[string]bool{},
		adminUsers: map[string]bool{"admin-1": true},
	}
	service := NewServiceWithCache(store, 100, time.Minute)

	allowed, err := service.HasPermission(context.Background(), Check{
		UserID:        "admin-1",
		PermissionKey: PermissionBotDelete,
		ResourceType:  ResourceBot,
		ResourceID:    "bot-1",
	})
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("allowed = false, want true")
	}
	if store.checks != 0 {
		t.Fatalf("store checks = %d, want 0", store.checks)
	}
}

func TestHasPermissionDoesNotBypassSystemAdminCheck(t *testing.T) {
	check := Check{
		UserID:        "user-1",
		PermissionKey: PermissionSystemAdmin,
		ResourceType:  ResourceSystem,
	}
	store := &fakeStore{
		allowed:    map[string]bool{cacheKey(check): true},
		adminUsers: map[string]bool{"user-1": true},
	}
	service := NewServiceWithCache(store, 100, time.Minute)

	allowed, err := service.HasPermission(context.Background(), check)
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("allowed = false, want true")
	}
	if store.adminCalls != 0 {
		t.Fatalf("admin calls = %d, want 0", store.adminCalls)
	}
	if store.checks != 1 {
		t.Fatalf("store checks = %d, want 1", store.checks)
	}
}

func TestHasPermissionValidatesCheck(t *testing.T) {
	store := &fakeStore{allowed: map[string]bool{}, adminUsers: map[string]bool{}}
	service := NewServiceWithCache(store, 100, time.Minute)

	_, err := service.HasPermission(context.Background(), Check{
		UserID:        "user-1",
		PermissionKey: PermissionBotRead,
		ResourceType:  ResourceSystem,
		ResourceID:    "bot-1",
	})
	if err == nil {
		t.Fatal("HasPermission returned nil error")
	}
}

func TestHasPermissionPropagatesStoreError(t *testing.T) {
	store := &fakeStore{err: errors.New("db failed")}
	service := NewServiceWithCache(store, 100, time.Minute)

	_, err := service.HasPermission(context.Background(), Check{
		UserID:        "user-1",
		PermissionKey: PermissionBotRead,
		ResourceType:  ResourceBot,
		ResourceID:    "bot-1",
	})
	if err == nil {
		t.Fatal("HasPermission returned nil error")
	}
}
