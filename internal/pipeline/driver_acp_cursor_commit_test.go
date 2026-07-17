package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestHandleReplyWithAgent_ACPDiscussCursorCommitOutcomes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		runtime     fakeDiscussRuntimeStreamer
		wantCursor  int64
		wantUpserts int
	}{
		{
			name:        "no durable response advances cursor separately",
			runtime:     fakeDiscussRuntimeStreamer{noDurableResponse: true},
			wantCursor:  200,
			wantUpserts: 1,
		},
		{
			name: "persistence failure retries without advancing",
			runtime: fakeDiscussRuntimeStreamer{
				noDurableResponse: true,
				postTerminalErr:   errors.New("persist ACP round"),
			},
		},
		{
			name: "persisted failed response is complete",
			runtime: fakeDiscussRuntimeStreamer{
				abort:          true,
				emitErrorEvent: true,
			},
			wantCursor: 200,
		},
		{
			name: "durable commit proof wins over trailing stream error",
			runtime: fakeDiscussRuntimeStreamer{
				postTerminalErr: errors.New("stream closed after commit"),
			},
			wantCursor: 200,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := tc.runtime
			resolver := &fakeRunConfigResolver{
				resolveResult:  ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent},
				compactionDone: make(chan struct{}, 1),
			}
			cursors := &fakeDiscussCursorStore{}
			driver := NewDiscussDriver(DiscussDriverDeps{
				Resolver:        resolver,
				RuntimeStreamer: &runtime,
				CursorStore:     cursors,
			})
			sess := &discussSession{config: DiscussSessionConfig{
				BotID:             "bot-1",
				SessionID:         "sess-1",
				RouteID:           "route-1",
				CurrentPlatform:   "telegram",
				ConversationType:  channel.ConversationTypeGroup,
				ChannelIdentityID: "acct-1",
			}}
			rc := RenderedContext{{
				ReceivedAtMs: 200,
				MentionsMe:   true,
				Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">please inspect the app</message>`}},
			}}

			driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})
			waitForFakeCompaction(t, resolver)

			if runtime.calls != 1 {
				t.Fatalf("runtime calls = %d, want 1", runtime.calls)
			}
			if sess.lastProcessedCursor != tc.wantCursor {
				t.Fatalf("session cursor = %d, want %d", sess.lastProcessedCursor, tc.wantCursor)
			}
			if cursors.upsertCalls != tc.wantUpserts {
				t.Fatalf("cursor-only upserts = %d, want %d", cursors.upsertCalls, tc.wantUpserts)
			}
			if tc.wantUpserts == 1 && cursors.upsertCursor.EventCursor != 200 {
				t.Fatalf("cursor-only position = %#v, want event cursor 200", cursors.upsertCursor)
			}
			if tc.wantCursor == 0 && sess.pendingCursor != nil {
				t.Fatalf("persistence failure left pending cursor = %#v, want retry without advance", sess.pendingCursor)
			}
		})
	}
}
