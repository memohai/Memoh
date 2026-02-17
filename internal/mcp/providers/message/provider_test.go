package message

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

type fakeSender struct {
	err     error
	lastReq channel.SendRequest
}

func (f *fakeSender) Send(_ context.Context, _ string, _ channel.Type, req channel.SendRequest) error {
	f.lastReq = req
	return f.err
}

type fakeReactor struct {
	err     error
	lastReq channel.ReactRequest
}

func (f *fakeReactor) React(_ context.Context, _ string, _ channel.Type, req channel.ReactRequest) error {
	f.lastReq = req
	return f.err
}

type fakeResolver struct {
	ct  channel.Type
	err error
}

func (f *fakeResolver) ParseChannelType(_ string) (channel.Type, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.ct, nil
}

// --- send tests ---

func TestExecutor_ListTools_NilDeps(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, nil)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools when deps nil, got %d", len(tools))
	}
}

func TestExecutor_ListTools(t *testing.T) {
	sender := &fakeSender{}
	reactor := &fakeReactor{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, reactor, resolver)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != toolSend {
		t.Errorf("tool[0] name = %q, want %q", tools[0].Name, toolSend)
	}
	if tools[1].Name != toolReact {
		t.Errorf("tool[1] name = %q, want %q", tools[1].Name, toolReact)
	}
}

func TestExecutor_ListTools_OnlySender(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != toolSend {
		t.Errorf("tool name = %q, want %q", tools[0].Name, toolSend)
	}
}

func TestExecutor_CallTool_NotFound(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	_, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, "other_tool", nil)
	if !errors.Is(err, mcpgw.ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}

func TestExecutor_CallTool_NilDeps(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, toolSend, map[string]any{
		"platform": "feishu", "target": "t1", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error result when deps nil")
	}
}

func TestExecutor_CallTool_NoBotID(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{}, toolSend, map[string]any{
		"platform": "feishu", "target": "t1", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot_id is missing")
	}
}

func TestExecutor_CallTool_BotIDMismatch(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"bot_id": "other", "platform": "feishu", "target": "t1", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot_id mismatch")
	}
}

func TestExecutor_CallTool_NoPlatform(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"target": "t1", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when platform is missing")
	}
}

func TestExecutor_CallTool_PlatformParseError(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{err: errors.New("unknown platform")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "feishu"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"platform": "bad", "target": "t1", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when platform parse fails")
	}
}

func TestExecutor_CallTool_NoMessage(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"platform": "feishu", "target": "t1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when message/text is missing")
	}
}

func TestExecutor_CallTool_NoTarget(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"platform": "feishu", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when target and channel_identity_id are missing")
	}
}

func TestExecutor_CallTool_SendError(t *testing.T) {
	sender := &fakeSender{err: errors.New("send failed")}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", ReplyTarget: "t1"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"platform": "feishu", "text": "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when Send fails")
	}
}

func TestExecutor_CallTool_Success(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("feishu")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "feishu", ReplyTarget: "chat1"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"text": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content == nil {
		t.Fatal("no structuredContent")
	}
	if content["ok"] != true {
		t.Errorf("ok = %v", content["ok"])
	}
	if content["platform"] != "feishu" {
		t.Errorf("platform = %v", content["platform"])
	}
}

func TestExecutor_CallTool_ReplyTo(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram", ReplyTarget: "123"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"text":     "reply text",
		"reply_to": "msg-789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	if sender.lastReq.Message.Reply == nil {
		t.Fatal("expected Reply to be set")
	}
	if sender.lastReq.Message.Reply.MessageID != "msg-789" {
		t.Errorf("Reply.MessageID = %q, want %q", sender.lastReq.Message.Reply.MessageID, "msg-789")
	}
}

func TestExecutor_CallTool_NoReplyTo(t *testing.T) {
	sender := &fakeSender{}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, sender, nil, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram", ReplyTarget: "123"}
	result, err := exec.CallTool(context.Background(), session, toolSend, map[string]any{
		"text": "no reply",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	if sender.lastReq.Message.Reply != nil {
		t.Error("expected Reply to be nil when reply_to is not provided")
	}
}

// --- react tests ---

func TestExecutor_React_NilReactor(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, toolReact, map[string]any{
		"platform": "telegram", "target": "123", "message_id": "456", "emoji": "👍",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when reactor is nil")
	}
}

func TestExecutor_React_NoMessageID(t *testing.T) {
	reactor := &fakeReactor{}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, nil, reactor, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram", ReplyTarget: "123"}
	result, err := exec.CallTool(context.Background(), session, toolReact, map[string]any{
		"emoji": "👍",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when message_id is missing")
	}
}

func TestExecutor_React_NoTarget(t *testing.T) {
	reactor := &fakeReactor{}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, nil, reactor, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram"}
	result, err := exec.CallTool(context.Background(), session, toolReact, map[string]any{
		"message_id": "456", "emoji": "👍",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when target is missing")
	}
}

func TestExecutor_React_Success(t *testing.T) {
	reactor := &fakeReactor{}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, nil, reactor, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram", ReplyTarget: "123"}
	result, err := exec.CallTool(context.Background(), session, toolReact, map[string]any{
		"message_id": "456", "emoji": "👍",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content == nil {
		t.Fatal("no structuredContent")
	}
	if content["ok"] != true {
		t.Errorf("ok = %v", content["ok"])
	}
	if content["action"] != "added" {
		t.Errorf("action = %v", content["action"])
	}
	if content["emoji"] != "👍" {
		t.Errorf("emoji = %v", content["emoji"])
	}
}

func TestExecutor_React_Remove(t *testing.T) {
	reactor := &fakeReactor{}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, nil, reactor, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram", ReplyTarget: "123"}
	result, err := exec.CallTool(context.Background(), session, toolReact, map[string]any{
		"message_id": "456", "remove": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content["action"] != "removed" {
		t.Errorf("action = %v", content["action"])
	}
	if reactor.lastReq.Remove != true {
		t.Error("expected Remove=true in request")
	}
}

func TestExecutor_React_Error(t *testing.T) {
	reactor := &fakeReactor{err: errors.New("reaction failed")}
	resolver := &fakeResolver{ct: channel.Type("telegram")}
	exec := NewExecutor(nil, nil, reactor, resolver)
	session := mcpgw.ToolSessionContext{BotID: "bot1", CurrentPlatform: "telegram", ReplyTarget: "123"}
	result, err := exec.CallTool(context.Background(), session, toolReact, map[string]any{
		"message_id": "456", "emoji": "👍",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when React fails")
	}
}

// --- parseOutboundMessage tests ---

func TestParseOutboundMessage(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		fallback  string
		wantEmpty bool
		wantErr   bool
	}{
		{"text fallback", map[string]any{}, "hello", false, false},
		{"message string", map[string]any{"message": "msg"}, "", false, false},
		{"message object", map[string]any{"message": map[string]any{"text": "obj"}}, "", false, false},
		{"empty", map[string]any{}, "", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parseOutboundMessage(tt.args, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOutboundMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantEmpty && !msg.IsEmpty() {
				t.Error("expected empty message")
			}
			if !tt.wantEmpty && msg.IsEmpty() {
				t.Error("expected non-empty message")
			}
		})
	}
}
