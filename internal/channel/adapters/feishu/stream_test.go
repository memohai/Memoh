package feishu

import (
	"context"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/memohai/memoh/internal/channel"
)

type fakeFeishuStreamMessageAPI struct {
	createCalls []string
	replyCalls  []string
	patchCalls  []string
}

func (f *fakeFeishuStreamMessageAPI) Create(_ context.Context, req *larkim.CreateMessageReq, _ ...larkcore.RequestOptionFunc) (*larkim.CreateMessageResp, error) {
	content := ""
	if req != nil && req.Body != nil && req.Body.Content != nil {
		content = strings.TrimSpace(*req.Body.Content)
	}
	f.createCalls = append(f.createCalls, content)
	messageID := "om_stream_created"
	return &larkim.CreateMessageResp{
		CodeError: larkcore.CodeError{Code: 0},
		Data: &larkim.CreateMessageRespData{
			MessageId: &messageID,
		},
	}, nil
}

func (f *fakeFeishuStreamMessageAPI) Reply(_ context.Context, req *larkim.ReplyMessageReq, _ ...larkcore.RequestOptionFunc) (*larkim.ReplyMessageResp, error) {
	content := ""
	if req != nil && req.Body != nil && req.Body.Content != nil {
		content = strings.TrimSpace(*req.Body.Content)
	}
	f.replyCalls = append(f.replyCalls, content)
	messageID := "om_stream_reply"
	return &larkim.ReplyMessageResp{
		CodeError: larkcore.CodeError{Code: 0},
		Data: &larkim.ReplyMessageRespData{
			MessageId: &messageID,
		},
	}, nil
}

func (f *fakeFeishuStreamMessageAPI) Patch(_ context.Context, req *larkim.PatchMessageReq, _ ...larkcore.RequestOptionFunc) (*larkim.PatchMessageResp, error) {
	content := ""
	if req != nil && req.Body != nil && req.Body.Content != nil {
		content = strings.TrimSpace(*req.Body.Content)
	}
	f.patchCalls = append(f.patchCalls, content)
	return &larkim.PatchMessageResp{CodeError: larkcore.CodeError{Code: 0}}, nil
}

func TestExtractReadableFromJSON(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"json with text", `{"text":"extracted"}`, "extracted"},
		{"json with message", `{"message":"ok"}`, "ok"},
		{"json with content", `{"content":"result"}`, "result"},
		{"invalid json", `{invalid`, `{invalid`},
		{"empty object", `{}`, `{}`},
		{"array of strings", `["first"]`, "first"},
		{"array empty", `[]`, `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractReadableFromJSON(tc.in)
			if got != tc.want {
				t.Errorf("extractReadableFromJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFeishuOutboundStreamToolCallBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		run                func(context.Context, *feishuOutboundStream) error
		wantLastPatch      string
		wantAbsentFragment string
	}{
		{
			name: "post-tool delta keeps single card",
			run: func(ctx context.Context, stream *feishuOutboundStream) error {
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventStatus, Status: channel.StreamStatusStarted}); err != nil {
					return err
				}
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "hello"}); err != nil {
					return err
				}
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventToolCallStart}); err != nil {
					return err
				}
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventToolCallEnd}); err != nil {
					return err
				}
				return stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "world"})
			},
			wantLastPatch:      "world",
			wantAbsentFragment: "hello",
		},
		{
			name: "final text overrides pre-tool draft",
			run: func(ctx context.Context, stream *feishuOutboundStream) error {
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventStatus, Status: channel.StreamStatusStarted}); err != nil {
					return err
				}
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "draft before tool"}); err != nil {
					return err
				}
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventToolCallStart}); err != nil {
					return err
				}
				if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventToolCallEnd}); err != nil {
					return err
				}
				return stream.Push(ctx, channel.StreamEvent{
					Type: channel.StreamEventFinal,
					Final: &channel.StreamFinalizePayload{
						Message: channel.Message{Text: "final answer"},
					},
				})
			},
			wantLastPatch:      "final answer",
			wantAbsentFragment: "draft before tool",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := &fakeFeishuStreamMessageAPI{}
			stream := &feishuOutboundStream{
				adapter:       NewFeishuAdapter(nil),
				cfg:           channel.ChannelConfig{ID: "cfg-1"},
				target:        "chat_id:oc_group_1",
				messageAPI:    api,
				receiveID:     "oc_group_1",
				receiveType:   larkim.ReceiveIdTypeChatId,
				patchInterval: 0,
			}

			if err := tc.run(context.Background(), stream); err != nil {
				t.Fatalf("stream sequence failed: %v", err)
			}
			if len(api.createCalls) != 1 {
				t.Fatalf("expected exactly one card create, got %d", len(api.createCalls))
			}
			if len(api.replyCalls) != 0 {
				t.Fatalf("expected no reply call, got %d", len(api.replyCalls))
			}
			if len(api.patchCalls) < 2 {
				t.Fatalf("expected card patches across tool boundaries, got %d", len(api.patchCalls))
			}
			if stream.cardMessageID != "om_stream_created" {
				t.Fatalf("expected card message id to be preserved, got %q", stream.cardMessageID)
			}
			lastPatch := stream.lastPatched
			if !strings.Contains(lastPatch, tc.wantLastPatch) {
				t.Fatalf("expected last patch to contain %q, got %s", tc.wantLastPatch, lastPatch)
			}
			if tc.wantAbsentFragment != "" && strings.Contains(lastPatch, tc.wantAbsentFragment) {
				t.Fatalf("expected last patch to drop stale fragment %q, got %s", tc.wantAbsentFragment, lastPatch)
			}
		})
	}
}
