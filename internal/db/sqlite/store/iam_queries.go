package store

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

func (q *Queries) AssignPrincipalRole(ctx context.Context, arg pgsqlc.AssignPrincipalRoleParams) (pgsqlc.IamPrincipalRole, error) {
	var sqliteArg sqlitesqlc.AssignPrincipalRoleParams
	var result pgsqlc.IamPrincipalRole
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.AssignPrincipalRole(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) CreateIAMLoginCode(ctx context.Context, arg pgsqlc.CreateIAMLoginCodeParams) (pgsqlc.IamLoginCode, error) {
	var sqliteArg sqlitesqlc.CreateIAMLoginCodeParams
	var result pgsqlc.IamLoginCode
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.CreateIAMLoginCode(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) CreateIAMSession(ctx context.Context, arg pgsqlc.CreateIAMSessionParams) (pgsqlc.IamSession, error) {
	var sqliteArg sqlitesqlc.CreateIAMSessionParams
	var result pgsqlc.IamSession
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.CreateIAMSession(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) CreatePasswordIdentity(ctx context.Context, arg pgsqlc.CreatePasswordIdentityParams) (pgsqlc.IamIdentity, error) {
	var sqliteArg sqlitesqlc.CreatePasswordIdentityParams
	var result pgsqlc.IamIdentity
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.CreatePasswordIdentity(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) DeletePrincipalRole(ctx context.Context, id pgtype.UUID) error {
	var sqliteID string
	if err := q.convertArg(id, &sqliteID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeletePrincipalRole(ctx, sqliteID))
}

func (q *Queries) DeletePrincipalRoleAssignment(ctx context.Context, arg pgsqlc.DeletePrincipalRoleAssignmentParams) error {
	var sqliteArg sqlitesqlc.DeletePrincipalRoleAssignmentParams
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeletePrincipalRoleAssignment(ctx, sqliteArg))
}

func (q *Queries) DeleteIAMGroup(ctx context.Context, id pgtype.UUID) error {
	var sqliteID string
	if err := q.convertArg(id, &sqliteID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteIAMGroup(ctx, sqliteID))
}

func (q *Queries) DeleteIAMGroupMember(ctx context.Context, arg pgsqlc.DeleteIAMGroupMemberParams) error {
	var sqliteArg sqlitesqlc.DeleteIAMGroupMemberParams
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteIAMGroupMember(ctx, sqliteArg))
}

func (q *Queries) DeleteSSOGroupMapping(ctx context.Context, arg pgsqlc.DeleteSSOGroupMappingParams) error {
	var sqliteArg sqlitesqlc.DeleteSSOGroupMappingParams
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteSSOGroupMapping(ctx, sqliteArg))
}

func (q *Queries) DeleteSSOProvider(ctx context.Context, id pgtype.UUID) error {
	var sqliteID string
	if err := q.convertArg(id, &sqliteID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteSSOProvider(ctx, sqliteID))
}

func (q *Queries) ExtendIAMSession(ctx context.Context, arg pgsqlc.ExtendIAMSessionParams) (pgsqlc.IamSession, error) {
	var sqliteArg sqlitesqlc.ExtendIAMSessionParams
	var result pgsqlc.IamSession
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.ExtendIAMSession(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetIAMSessionByID(ctx context.Context, id pgtype.UUID) (pgsqlc.GetIAMSessionByIDRow, error) {
	var sqliteID string
	var result pgsqlc.GetIAMSessionByIDRow
	if err := q.convertArg(id, &sqliteID); err != nil {
		return result, err
	}
	out, err := q.store.queries.GetIAMSessionByID(ctx, sqliteID)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetIAMGroupByID(ctx context.Context, id pgtype.UUID) (pgsqlc.IamGroup, error) {
	var sqliteID string
	var result pgsqlc.IamGroup
	if err := q.convertArg(id, &sqliteID); err != nil {
		return result, err
	}
	out, err := q.store.queries.GetIAMGroupByID(ctx, sqliteID)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetIdentityByProviderSubject(ctx context.Context, arg pgsqlc.GetIdentityByProviderSubjectParams) (pgsqlc.IamIdentity, error) {
	var sqliteArg sqlitesqlc.GetIdentityByProviderSubjectParams
	var result pgsqlc.IamIdentity
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.GetIdentityByProviderSubject(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetPasswordIdentityBySubject(ctx context.Context, subject string) (pgsqlc.IamIdentity, error) {
	var result pgsqlc.IamIdentity
	out, err := q.store.queries.GetPasswordIdentityBySubject(ctx, subject)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetRoleByKey(ctx context.Context, key string) (pgsqlc.IamRole, error) {
	var result pgsqlc.IamRole
	out, err := q.store.queries.GetRoleByKey(ctx, key)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetSSOProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.IamSsoProvider, error) {
	var sqliteID string
	var result pgsqlc.IamSsoProvider
	if err := q.convertArg(id, &sqliteID); err != nil {
		return result, err
	}
	out, err := q.store.queries.GetSSOProviderByID(ctx, sqliteID)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) GetSSOProviderByKey(ctx context.Context, key string) (pgsqlc.IamSsoProvider, error) {
	var result pgsqlc.IamSsoProvider
	out, err := q.store.queries.GetSSOProviderByKey(ctx, key)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) HasPermission(ctx context.Context, arg pgsqlc.HasPermissionParams) (bool, error) {
	var sqliteArg sqlitesqlc.HasPermissionParams
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return false, err
	}
	return q.store.queries.HasPermission(ctx, sqliteArg)
}

func (q *Queries) ListEnabledSSOProviders(ctx context.Context) ([]pgsqlc.IamSsoProvider, error) {
	out, err := q.store.queries.ListEnabledSSOProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.IamSsoProvider
	return result, convertValue(out, &result)
}

func (q *Queries) ListIAMGroups(ctx context.Context) ([]pgsqlc.IamGroup, error) {
	out, err := q.store.queries.ListIAMGroups(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.IamGroup
	return result, convertValue(out, &result)
}

func (q *Queries) ListIAMGroupMembers(ctx context.Context, groupID pgtype.UUID) ([]pgsqlc.ListIAMGroupMembersRow, error) {
	var sqliteGroupID string
	if err := q.convertArg(groupID, &sqliteGroupID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListIAMGroupMembers(ctx, sqliteGroupID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListIAMGroupMembersRow
	return result, convertValue(out, &result)
}

func (q *Queries) ListRoles(ctx context.Context) ([]pgsqlc.IamRole, error) {
	out, err := q.store.queries.ListRoles(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.IamRole
	return result, convertValue(out, &result)
}

func (q *Queries) ListSSOGroupMappingsByProvider(ctx context.Context, providerID pgtype.UUID) ([]pgsqlc.ListSSOGroupMappingsByProviderRow, error) {
	var sqliteProviderID string
	if err := q.convertArg(providerID, &sqliteProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSSOGroupMappingsByProvider(ctx, sqliteProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSSOGroupMappingsByProviderRow
	return result, convertValue(out, &result)
}

func (q *Queries) ListSSOProviders(ctx context.Context) ([]pgsqlc.IamSsoProvider, error) {
	out, err := q.store.queries.ListSSOProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.IamSsoProvider
	return result, convertValue(out, &result)
}

func (q *Queries) ListPrincipalRoles(ctx context.Context, arg pgsqlc.ListPrincipalRolesParams) ([]pgsqlc.ListPrincipalRolesRow, error) {
	var sqliteArg sqlitesqlc.ListPrincipalRolesParams
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListPrincipalRoles(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListPrincipalRolesRow
	return result, convertValue(out, &result)
}

func (q *Queries) ReplaceSSOGroupMemberships(ctx context.Context, arg pgsqlc.ReplaceSSOGroupMembershipsParams) error {
	var userID string
	var providerID string
	if err := q.convertArg(arg.UserID, &userID); err != nil {
		return err
	}
	if err := q.convertArg(arg.ProviderID, &providerID); err != nil {
		return err
	}
	if err := q.store.queries.ClearSSOGroupMemberships(ctx, sqlitesqlc.ClearSSOGroupMembershipsParams{
		UserID:     userID,
		ProviderID: sql.NullString{String: providerID, Valid: providerID != ""},
	}); err != nil {
		return mapQueryErr(err)
	}
	for _, groupID := range arg.GroupIds {
		var sqliteGroupID string
		if err := q.convertArg(groupID, &sqliteGroupID); err != nil {
			return err
		}
		if err := q.store.queries.AddSSOGroupMembership(ctx, sqlitesqlc.AddSSOGroupMembershipParams{
			UserID:     userID,
			GroupID:    sqliteGroupID,
			ProviderID: sql.NullString{String: providerID, Valid: providerID != ""},
		}); err != nil {
			return mapQueryErr(err)
		}
	}
	return nil
}

func (q *Queries) RevokeIAMSession(ctx context.Context, id pgtype.UUID) error {
	var sqliteID string
	if err := q.convertArg(id, &sqliteID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.RevokeIAMSession(ctx, sqliteID))
}

func (q *Queries) UpdateIdentityLastLogin(ctx context.Context, id pgtype.UUID) error {
	var sqliteID string
	if err := q.convertArg(id, &sqliteID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.UpdateIdentityLastLogin(ctx, sqliteID))
}

func (q *Queries) UpsertExternalIdentity(ctx context.Context, arg pgsqlc.UpsertExternalIdentityParams) (pgsqlc.IamIdentity, error) {
	var sqliteArg sqlitesqlc.UpsertExternalIdentityParams
	var result pgsqlc.IamIdentity
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.UpsertExternalIdentity(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) UpsertIAMGroup(ctx context.Context, arg pgsqlc.UpsertIAMGroupParams) (pgsqlc.IamGroup, error) {
	var sqliteArg sqlitesqlc.UpsertIAMGroupParams
	var result pgsqlc.IamGroup
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.UpsertIAMGroup(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) UpsertIAMGroupMember(ctx context.Context, arg pgsqlc.UpsertIAMGroupMemberParams) (pgsqlc.IamGroupMember, error) {
	var sqliteArg sqlitesqlc.UpsertIAMGroupMemberParams
	var result pgsqlc.IamGroupMember
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.UpsertIAMGroupMember(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) UpsertSSOGroupMapping(ctx context.Context, arg pgsqlc.UpsertSSOGroupMappingParams) (pgsqlc.IamSsoGroupMapping, error) {
	var sqliteArg sqlitesqlc.UpsertSSOGroupMappingParams
	var result pgsqlc.IamSsoGroupMapping
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.UpsertSSOGroupMapping(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) UpsertSSOProvider(ctx context.Context, arg pgsqlc.UpsertSSOProviderParams) (pgsqlc.IamSsoProvider, error) {
	var sqliteArg sqlitesqlc.UpsertSSOProviderParams
	var result pgsqlc.IamSsoProvider
	if err := q.convertArg(arg, &sqliteArg); err != nil {
		return result, err
	}
	out, err := q.store.queries.UpsertSSOProvider(ctx, sqliteArg)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) UseIAMLoginCode(ctx context.Context, codeHash string) (pgsqlc.IamLoginCode, error) {
	var result pgsqlc.IamLoginCode
	out, err := q.store.queries.UseIAMLoginCode(ctx, codeHash)
	if err != nil {
		return result, mapQueryErr(err)
	}
	return result, convertValue(out, &result)
}

func (q *Queries) convertArg(src any, dst any) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	return convertValue(src, dst)
}
