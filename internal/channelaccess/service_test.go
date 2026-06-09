package channelaccess

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestListManagersIncludesEveryoneManageBindings(t *testing.T) {
	ctx := context.Background()
	botID := "00000000-0000-0000-0000-000000000010"
	channelIdentityID := "00000000-0000-0000-0000-000000000020"

	svc := &Service{
		queries: &fakeChannelAccessQueries{
			allBindings: []sqlc.ListChannelIdentityBindingsRow{{
				ID:                         mustUUID(t, "00000000-0000-0000-0000-000000000021"),
				UserID:                     mustUUID(t, "00000000-0000-0000-0000-000000000022"),
				ChannelIdentityID:          mustUUID(t, channelIdentityID),
				ChannelType:                text("telegram"),
				ChannelSubjectID:           text("tg-1"),
				ChannelIdentityDisplayName: text("Alice"),
			}},
		},
		acl: &fakeManageOverrides{},
		bots: &fakeBotPermissions{
			grants: []bots.UserGrant{{
				BotID:       botID,
				SubjectType: bots.GrantSubjectEveryone,
				Permissions: []string{bots.PermissionManage},
			}},
		},
	}

	items, err := svc.ListManagers(ctx, botID)
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 manager, got %d: %#v", len(items), items)
	}
	item := items[0]
	if item.ChannelIdentityID != channelIdentityID {
		t.Fatalf("unexpected channel identity: %q", item.ChannelIdentityID)
	}
	if !item.Bound || !item.Inherited || !item.Manage {
		t.Fatalf("expected everyone Manage to be inherited and effective, got %#v", item)
	}
	if item.ChannelType != "telegram" || item.ChannelIdentityDisplayName != "Alice" {
		t.Fatalf("identity metadata was not preserved: %#v", item)
	}
}

func TestListManagersLocalOverrideSuppressesEveryoneManage(t *testing.T) {
	ctx := context.Background()
	botID := "00000000-0000-0000-0000-000000000010"
	channelIdentityID := "00000000-0000-0000-0000-000000000020"

	svc := &Service{
		queries: &fakeChannelAccessQueries{
			allBindings: []sqlc.ListChannelIdentityBindingsRow{{
				ID:                mustUUID(t, "00000000-0000-0000-0000-000000000021"),
				UserID:            mustUUID(t, "00000000-0000-0000-0000-000000000022"),
				ChannelIdentityID: mustUUID(t, channelIdentityID),
				ChannelType:       text("telegram"),
			}},
		},
		acl: &fakeManageOverrides{
			overrides: []acl.ManageOverride{{
				BotID:             botID,
				ChannelIdentityID: channelIdentityID,
				Granted:           false,
			}},
		},
		bots: &fakeBotPermissions{
			grants: []bots.UserGrant{{
				BotID:       botID,
				SubjectType: bots.GrantSubjectEveryone,
				Permissions: []string{bots.PermissionManage},
			}},
		},
	}

	items, err := svc.ListManagers(ctx, botID)
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 manager, got %d: %#v", len(items), items)
	}
	item := items[0]
	if !item.Bound || !item.Inherited || !item.HasOverride {
		t.Fatalf("expected bound inherited identity with override, got %#v", item)
	}
	if item.Manage {
		t.Fatalf("expected local deny override to suppress everyone Manage, got %#v", item)
	}
}

type fakeChannelAccessQueries struct {
	dbstore.Queries
	allBindings []sqlc.ListChannelIdentityBindingsRow
}

func (f *fakeChannelAccessQueries) ListChannelIdentityBindings(context.Context) ([]sqlc.ListChannelIdentityBindingsRow, error) {
	return f.allBindings, nil
}

type fakeManageOverrides struct {
	overrides []acl.ManageOverride
}

func (*fakeManageOverrides) GetManageOverride(context.Context, string, string) (bool, bool, error) {
	return false, false, nil
}

func (f *fakeManageOverrides) ListManageOverrides(context.Context, string) ([]acl.ManageOverride, error) {
	return f.overrides, nil
}

func (*fakeManageOverrides) SetManageOverride(context.Context, string, string, bool, string) (acl.ManageOverride, error) {
	return acl.ManageOverride{}, nil
}

func (*fakeManageOverrides) DeleteManageOverride(context.Context, string, string) error {
	return nil
}

type fakeBotPermissions struct {
	grants []bots.UserGrant
}

func (*fakeBotPermissions) ResolveUserPermissions(context.Context, string, string, bool) ([]string, error) {
	return nil, nil
}

func (f *fakeBotPermissions) ListUserGrants(context.Context, string) ([]bots.UserGrant, error) {
	return f.grants, nil
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}

func text(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
