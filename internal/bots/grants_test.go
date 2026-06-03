package bots

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizePermissionsExpandsWorkspaceWrite(t *testing.T) {
	got, err := normalizePermissions([]string{PermissionWorkspaceWrite})
	if err != nil {
		t.Fatalf("normalizePermissions() error = %v", err)
	}
	want := []string{PermissionWorkspaceRead, PermissionWorkspaceWrite}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizePermissions() = %#v, want %#v", got, want)
	}
}

func TestNormalizePermissionsExpandsManage(t *testing.T) {
	got, err := normalizePermissions([]string{PermissionManage})
	if err != nil {
		t.Fatalf("normalizePermissions() error = %v", err)
	}
	want := []string{
		PermissionChat,
		PermissionWorkspaceRead,
		PermissionWorkspaceWrite,
		PermissionWorkspaceExec,
		PermissionManage,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizePermissions() = %#v, want %#v", got, want)
	}
}

func TestNormalizePermissionsRejectsInvalidPermission(t *testing.T) {
	_, err := normalizePermissions([]string{"workspace_admin"})
	if !errors.Is(err, ErrInvalidPermission) {
		t.Fatalf("normalizePermissions() error = %v, want ErrInvalidPermission", err)
	}
}
