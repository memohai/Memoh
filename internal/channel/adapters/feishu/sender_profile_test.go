package feishu

import (
	"context"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type fakeSenderProfileLookup struct {
	calls        []string
	contact      feishuSenderProfile
	contactErr   error
	groupOpen    feishuSenderProfile
	groupOpenErr error
	groupUser    feishuSenderProfile
	groupUserErr error
}

func (f *fakeSenderProfileLookup) LookupContact(_ context.Context, openID, userID string) (feishuSenderProfile, error) {
	f.calls = append(f.calls, "contact:"+openID+":"+userID)
	return f.contact, f.contactErr
}

func (f *fakeSenderProfileLookup) LookupGroupMember(_ context.Context, chatID, memberIDType, memberID string) (feishuSenderProfile, error) {
	f.calls = append(f.calls, "group:"+chatID+":"+memberIDType+":"+memberID)
	switch memberIDType {
	case "open_id":
		return f.groupOpen, f.groupOpenErr
	case "user_id":
		return f.groupUser, f.groupUserErr
	default:
		return feishuSenderProfile{}, nil
	}
}

func TestApplySenderProfileFillDisplayAndUsername(t *testing.T) {
	msg := &channel.InboundMessage{
		Sender: channel.Identity{
			SubjectID:  "ou_test",
			Attributes: map[string]string{"open_id": "ou_test"},
		},
	}

	applySenderProfile(msg, feishuSenderProfile{
		displayName: "张三",
		username:    "zhangsan",
	})

	if got := msg.Sender.DisplayName; got != "张三" {
		t.Fatalf("expected display name 张三, got %q", got)
	}
	if got := msg.Sender.Attribute("display_name"); got != "张三" {
		t.Fatalf("expected attribute display_name 张三, got %q", got)
	}
	if got := msg.Sender.Attribute("name"); got != "张三" {
		t.Fatalf("expected attribute name 张三, got %q", got)
	}
	if got := msg.Sender.Attribute("username"); got != "zhangsan" {
		t.Fatalf("expected attribute username zhangsan, got %q", got)
	}
}

func TestApplySenderProfileKeepExistingIdentityFields(t *testing.T) {
	msg := &channel.InboundMessage{
		Sender: channel.Identity{
			SubjectID:   "ou_test",
			DisplayName: "原名",
			Attributes: map[string]string{
				"open_id":      "ou_test",
				"display_name": "原显示名",
				"name":         "原姓名",
				"username":     "old_user",
			},
		},
	}

	applySenderProfile(msg, feishuSenderProfile{
		displayName: "新名",
		username:    "new_user",
	})

	if got := msg.Sender.DisplayName; got != "原名" {
		t.Fatalf("expected original display name preserved, got %q", got)
	}
	if got := msg.Sender.Attribute("display_name"); got != "原显示名" {
		t.Fatalf("expected original attribute display_name preserved, got %q", got)
	}
	if got := msg.Sender.Attribute("name"); got != "原姓名" {
		t.Fatalf("expected original attribute name preserved, got %q", got)
	}
	if got := msg.Sender.Attribute("username"); got != "old_user" {
		t.Fatalf("expected original attribute username preserved, got %q", got)
	}
}

func TestLookupSenderProfileLookupOrder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		openID      string
		userID      string
		chatID      string
		lookup      *fakeSenderProfileLookup
		wantProfile feishuSenderProfile
		wantCalls   []string
	}{
		{
			name:   "contact without group context",
			openID: "ou_123",
			userID: "u_123",
			lookup: &fakeSenderProfileLookup{
				contact: feishuSenderProfile{
					displayName: "Alice Zhang",
					username:    "alice",
				},
			},
			wantProfile: feishuSenderProfile{displayName: "Alice Zhang", username: "alice"},
			wantCalls:   []string{"contact:ou_123:u_123"},
		},
		{
			name:   "group alias wins over contact profile",
			openID: "ou_456",
			userID: "u_456",
			chatID: "oc_group_2",
			lookup: &fakeSenderProfileLookup{
				contact: feishuSenderProfile{
					displayName: "Alice Global",
					username:    "alice",
				},
				groupOpen: feishuSenderProfile{
					displayName: "Alice In Group",
					username:    "alice-group",
				},
			},
			wantProfile: feishuSenderProfile{displayName: "Alice In Group", username: "alice-group"},
			wantCalls:   []string{"group:oc_group_2:open_id:ou_456"},
		},
		{
			name:   "group miss falls back to contact profile",
			openID: "ou_456",
			userID: "u_456",
			chatID: "oc_group_2",
			lookup: &fakeSenderProfileLookup{
				contact: feishuSenderProfile{
					displayName: "Bob Global",
					username:    "bob",
				},
			},
			wantProfile: feishuSenderProfile{displayName: "Bob Global", username: "bob"},
			wantCalls: []string{
				"group:oc_group_2:open_id:ou_456",
				"group:oc_group_2:user_id:u_456",
				"contact:ou_456:u_456",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			profile, err := lookupSenderProfileWithLookup(context.Background(), tc.lookup, tc.openID, tc.userID, tc.chatID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if profile != tc.wantProfile {
				t.Fatalf("unexpected profile: got %#v want %#v", profile, tc.wantProfile)
			}
			if len(tc.lookup.calls) != len(tc.wantCalls) {
				t.Fatalf("unexpected lookup count: %#v", tc.lookup.calls)
			}
			for i, want := range tc.wantCalls {
				if tc.lookup.calls[i] != want {
					t.Fatalf("unexpected lookup order: %#v", tc.lookup.calls)
				}
			}
		})
	}
}

func TestFinalizeSenderProfileDoesNotCacheFallbackIdentity(t *testing.T) {
	t.Parallel()

	profile, cacheable := finalizeSenderProfile(feishuSenderProfile{}, "ou_789", "u_789")
	if !hasSenderProfile(profile) {
		t.Fatalf("expected fallback profile to be applied, got %#v", profile)
	}
	if cacheable {
		t.Fatal("expected synthetic fallback profile to remain non-cacheable")
	}
}

func TestSenderProfileCacheKeyIncludesAppScope(t *testing.T) {
	t.Parallel()

	keyA := senderProfileCacheKey(channel.ChannelConfig{
		ID: "cfg-1",
		Credentials: map[string]any{
			"app_id":     "app-a",
			"app_secret": "secret",
			"region":     "feishu",
		},
	}, "ou_789", "u_789", "chat_id:oc_group_1")
	keyB := senderProfileCacheKey(channel.ChannelConfig{
		ID: "cfg-1",
		Credentials: map[string]any{
			"app_id":     "app-b",
			"app_secret": "secret",
			"region":     "feishu",
		},
	}, "ou_789", "u_789", "oc_group_1")

	if keyA == keyB {
		t.Fatalf("expected cache key to change across app rotations, got %q", keyA)
	}
}

func TestStoreCachedSenderProfileSweepsExpiredEntries(t *testing.T) {
	t.Parallel()

	adapter := NewFeishuAdapter(nil)
	adapter.senderProfiles.Store("expired", cachedSenderProfile{
		profile:   feishuSenderProfile{displayName: "Old Name"},
		expiresAt: time.Now().Add(-time.Minute),
	})
	adapter.senderProfileSweepAt = time.Now().Add(-senderProfileSweepWindow - time.Second)

	adapter.storeCachedSenderProfile("fresh", feishuSenderProfile{displayName: "Fresh Name"})

	if _, ok := adapter.senderProfiles.Load("expired"); ok {
		t.Fatal("expected expired sender profile cache entry to be swept")
	}
	if cached, ok := adapter.loadCachedSenderProfile("fresh"); !ok || cached.displayName != "Fresh Name" {
		t.Fatalf("expected fresh sender profile cache entry to remain, got %#v ok=%v", cached, ok)
	}
}
