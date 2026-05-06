package rbac

import "context"

type PermissionSeed struct {
	Key      PermissionKey
	IsSystem bool
}

type RoleSeed struct {
	Key      RoleKey
	Scope    ResourceType
	IsSystem bool
}

type RolePermissionSeed struct {
	RoleKey       RoleKey
	PermissionKey PermissionKey
}

var BuiltinPermissions = []PermissionSeed{
	{Key: PermissionSystemLogin, IsSystem: true},
	{Key: PermissionSystemAdmin, IsSystem: true},
	{Key: PermissionBotRead, IsSystem: true},
	{Key: PermissionBotChat, IsSystem: true},
	{Key: PermissionBotUpdate, IsSystem: true},
	{Key: PermissionBotDelete, IsSystem: true},
	{Key: PermissionBotPermissionsManage, IsSystem: true},
}

var BuiltinRoles = []RoleSeed{
	{Key: RoleMember, Scope: ResourceSystem, IsSystem: true},
	{Key: RoleAdmin, Scope: ResourceSystem, IsSystem: true},
	{Key: RoleBotViewer, Scope: ResourceBot, IsSystem: true},
	{Key: RoleBotOperator, Scope: ResourceBot, IsSystem: true},
	{Key: RoleBotOwner, Scope: ResourceBot, IsSystem: true},
}

var BuiltinRolePermissions = []RolePermissionSeed{
	{RoleKey: RoleMember, PermissionKey: PermissionSystemLogin},
	{RoleKey: RoleAdmin, PermissionKey: PermissionSystemLogin},
	{RoleKey: RoleAdmin, PermissionKey: PermissionSystemAdmin},
	{RoleKey: RoleBotViewer, PermissionKey: PermissionBotRead},
	{RoleKey: RoleBotViewer, PermissionKey: PermissionBotChat},
	{RoleKey: RoleBotOperator, PermissionKey: PermissionBotRead},
	{RoleKey: RoleBotOperator, PermissionKey: PermissionBotChat},
	{RoleKey: RoleBotOperator, PermissionKey: PermissionBotUpdate},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotRead},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotChat},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotUpdate},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotDelete},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotPermissionsManage},
}

type BootstrapStore interface {
	EnsurePermissions(ctx context.Context, permissions []PermissionSeed) error
	EnsureRoles(ctx context.Context, roles []RoleSeed) error
	EnsureRolePermissions(ctx context.Context, rolePermissions []RolePermissionSeed) error
}

func Bootstrap(ctx context.Context, store BootstrapStore) error {
	if err := store.EnsurePermissions(ctx, BuiltinPermissions); err != nil {
		return err
	}
	if err := store.EnsureRoles(ctx, BuiltinRoles); err != nil {
		return err
	}
	return store.EnsureRolePermissions(ctx, BuiltinRolePermissions)
}
