package feishu

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

// TestFeishuGateway_Integration 飞书通道集成测试
// 运行此测试需要设置环境变量:
// FEISHU_APP_ID: 飞书应用的 App ID
// FEISHU_APP_SECRET: 飞书应用的 App Secret
// FEISHU_ENCRYPT_KEY: (可选) 飞书应用的 Encrypt Key
// FEISHU_VERIFICATION_TOKEN: (可选) 飞书应用的 Verification Token
func TestFeishuGateway_Integration(t *testing.T) {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")

	if appID == "" || appSecret == "" {
		t.Skip("跳过集成测试: 未设置 FEISHU_APP_ID 或 FEISHU_APP_SECRET 环境变量")
	}

	encryptKey := os.Getenv("FEISHU_ENCRYPT_KEY")
	verificationToken := os.Getenv("FEISHU_VERIFICATION_TOKEN")

	// 使用更规范的日志配置
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	adapter := NewFeishuAdapter(logger)

	// 构造测试配置
	cfg := channel.ChannelConfig{
		ID: "integration-test-bot",
		Credentials: map[string]any{
			"app_id":             appID,
			"app_secret":         appSecret,
			"encrypt_key":        encryptKey,
			"verification_token": verificationToken,
		},
	}

	// 定义测试上下文，设置合理的超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 消息计数，用于验证是否收到消息
	receivedChan := make(chan channel.InboundMessage, 1)

	// 模拟 InboundHandler
	handler := func(ctx context.Context, c channel.ChannelConfig, msg channel.InboundMessage) error {
		plainText := msg.Message.PlainText()
		logger.Info("测试收到消息",
			slog.String("text", plainText),
			slog.String("user_id", msg.Sender.Attribute("user_id")),
			slog.String("session_id", msg.SessionID()))

		// 将消息放入通道，供主测试逻辑验证
		select {
		case receivedChan <- msg:
		default:
		}

		// 自动回复测试 (验证下行链路)
		reply := channel.OutboundMessage{
			Target: msg.ReplyTarget,
			Message: channel.Message{
				Text: fmt.Sprintf("【Memoh 集成测试】已收到消息: %s\n测试时间: %s", plainText, time.Now().Format("15:04:05")),
			},
		}

		if err := adapter.Send(ctx, c, reply); err != nil {
			return fmt.Errorf("failed to send reply: %w", err)
		}

		// 模拟异步主动推送测试
		go func() {
			time.Sleep(1 * time.Second)
			pushMsg := channel.OutboundMessage{
				Target: msg.ReplyTarget,
				Message: channel.Message{
					Text: "【Memoh 集成测试】主动推送验证成功。",
				},
			}
			_ = adapter.Send(context.Background(), c, pushMsg)
		}()

		return nil
	}

	// 启动适配器
	logger.Info("正在启动飞书适配器...", slog.String("app_id", appID))
	runner, err := adapter.Connect(ctx, cfg, handler)
	if err != nil {
		t.Fatalf("适配器启动失败: %v", err)
	}
	defer func() {
		_ = runner.Stop(context.Background())
	}()

	fmt.Println("==================================================================")
	fmt.Println("🚀 飞书集成测试已就绪!")
	fmt.Println("请在飞书客户端向机器人发送一条消息，以完成端到端验证。")
	fmt.Println("测试将在收到第一条消息或 10 分钟超时后结束。")
	fmt.Println("==================================================================")

	// 等待测试结果
	select {
	case msg := <-receivedChan:
		logger.Info("集成测试验证成功!", slog.String("received_text", msg.Message.PlainText()))
		// 给一点时间让异步推送完成
		time.Sleep(2 * time.Second)
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			t.Log("测试超时结束")
		}
	}
}
