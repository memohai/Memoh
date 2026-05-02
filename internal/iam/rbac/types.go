package rbac

type (
	PermissionKey    string
	RoleKey          string
	ResourceType     string
	PrincipalType    string
	AssignmentSource string
)

const (
	PermissionSystemLogin PermissionKey = "system.login"
	PermissionSystemAdmin PermissionKey = "system.admin"

	PermissionBotRead              PermissionKey = "bot.read"
	PermissionBotChat              PermissionKey = "bot.chat"
	PermissionBotUpdate            PermissionKey = "bot.update"
	PermissionBotDelete            PermissionKey = "bot.delete"
	PermissionBotPermissionsManage PermissionKey = "bot.permissions.manage"
)

const (
	RoleMember      RoleKey = "member"
	RoleAdmin       RoleKey = "admin"
	RoleBotViewer   RoleKey = "bot_viewer"
	RoleBotOperator RoleKey = "bot_operator"
	RoleBotOwner    RoleKey = "bot_owner"
)

const (
	ResourceSystem ResourceType = "system"
	ResourceBot    ResourceType = "bot"
)

const (
	PrincipalUser  PrincipalType = "user"
	PrincipalGroup PrincipalType = "group"
)

const (
	SourceSystem AssignmentSource = "system"
	SourceManual AssignmentSource = "manual"
	SourceSSO    AssignmentSource = "sso"
	SourceSCIM   AssignmentSource = "scim"
)

type Check struct {
	UserID        string
	PermissionKey PermissionKey
	ResourceType  ResourceType
	ResourceID    string
}
