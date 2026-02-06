package channel

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
)

// mockAdapter 专门用于 Manager 路由测试
type mockAdapter struct {
	sentMessages []OutboundMessage
}

func (m *mockAdapter) Type() ChannelType { return ChannelType("test") }
func (m *mockAdapter) Send(ctx context.Context, cfg ChannelConfig, msg OutboundMessage) error {
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

type fakeInboundProcessor struct {
	resp   *OutboundMessage
	err    error
	gotCfg ChannelConfig
	gotMsg InboundMessage
}

func (f *fakeInboundProcessor) HandleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage, sender ReplySender) error {
	f.gotCfg = cfg
	f.gotMsg = msg
	if f.err != nil {
		return f.err
	}
	if f.resp == nil {
		return nil
	}
	if sender == nil {
		return fmt.Errorf("sender missing")
	}
	return sender.Send(ctx, *f.resp)
}

func TestManager_HandleInbound_CoreLogic(t *testing.T) {
	logger := slog.Default()

	t.Run("返回回复_发送成功", func(t *testing.T) {
		processor := &fakeInboundProcessor{
			resp: &OutboundMessage{
				Target: "target-id",
				Message: Message{
					Text: "AI回复内容",
				},
			},
		}

		m := NewManager(logger, &fakeConfigStore{}, processor)
		adapter := &mockAdapter{}
		m.RegisterAdapter(adapter)

		cfg := ChannelConfig{ID: "bot-1", BotID: "bot-1", ChannelType: ChannelType("test")}
		msg := InboundMessage{
			Channel:     ChannelType("test"),
			Message:     Message{Text: "你好"},
			ReplyTarget: "target-id",
			Conversation: Conversation{
				ID:   "chat-1",
				Type: "p2p",
			},
		}

		err := m.handleInbound(context.Background(), cfg, msg)
		if err != nil {
			t.Fatalf("不应报错: %v", err)
		}

		// 验证: 是否正确调用了 Adapter 发送回复
		if len(adapter.sentMessages) != 1 {
			t.Fatalf("应该发送 1 条回复，实际发送: %d", len(adapter.sentMessages))
		}
		if adapter.sentMessages[0].Message.PlainText() != "AI回复内容" {
			t.Errorf("回复内容错误: %s", adapter.sentMessages[0].Message.PlainText())
		}
		if adapter.sentMessages[0].Target != "target-id" {
			t.Errorf("回复目标错误: %s", adapter.sentMessages[0].Target)
		}
	})

	t.Run("无回复_不发送", func(t *testing.T) {
		processor := &fakeInboundProcessor{resp: nil}
		m := NewManager(logger, &fakeConfigStore{}, processor)
		adapter := &mockAdapter{}
		m.RegisterAdapter(adapter)

		cfg := ChannelConfig{ID: "bot-1", BotID: "bot-1", ChannelType: ChannelType("test")}
		msg := InboundMessage{
			Channel:     ChannelType("test"),
			Message:     Message{Text: "你好"},
			ReplyTarget: "target-id",
		}

		err := m.handleInbound(context.Background(), cfg, msg)
		if err != nil {
			t.Fatalf("不应报错: %v", err)
		}

		if len(adapter.sentMessages) != 0 {
			t.Errorf("不应发送回复，实际发送: %+v", adapter.sentMessages)
		}
	})

	t.Run("处理失败_返回错误", func(t *testing.T) {
		processor := &fakeInboundProcessor{err: context.Canceled}
		m := NewManager(logger, &fakeConfigStore{}, processor)
		cfg := ChannelConfig{ID: "bot-1"}
		msg := InboundMessage{Message: Message{Text: "  "}} // 空格消息

		err := m.handleInbound(context.Background(), cfg, msg)
		if err == nil {
			t.Errorf("应返回处理错误")
		}
	})
}
