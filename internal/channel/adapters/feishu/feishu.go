package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/common"
)

type FeishuAdapter struct {
	logger *slog.Logger
}

func NewFeishuAdapter(log *slog.Logger) *FeishuAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &FeishuAdapter{
		logger: log.With(slog.String("adapter", "feishu")),
	}
}

func (a *FeishuAdapter) Type() channel.ChannelType {
	return Type
}

func (a *FeishuAdapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	if a.logger != nil {
		a.logger.Info("start", slog.String("config_id", cfg.ID))
	}
	feishuCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("decode config failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return nil, err
	}
	connCtx, cancel := context.WithCancel(ctx)
	eventDispatcher := dispatcher.NewEventDispatcher(
		feishuCfg.VerificationToken,
		feishuCfg.EncryptKey,
	)
	eventDispatcher.OnP2MessageReceiveV1(func(_ context.Context, event *larkim.P2MessageReceiveV1) error {
		msg := extractFeishuInbound(event)
		text := msg.Message.PlainText()
		if text == "" && len(msg.Message.Attachments) == 0 {
			return nil
		}
		msg.BotID = cfg.BotID
		if a.logger != nil {
			a.logger.Info(
				"inbound received",
				slog.String("config_id", cfg.ID),
				slog.String("session_id", msg.SessionID()),
				slog.String("chat_type", msg.Conversation.Type),
				slog.String("text", common.SummarizeText(text)),
			)
		}
		go func() {
			if err := handler(connCtx, cfg, msg); err != nil && a.logger != nil {
				a.logger.Error("handle inbound failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
			}
		}()
		return nil
	})
	eventDispatcher.OnP2MessageReadV1(func(_ context.Context, _ *larkim.P2MessageReadV1) error {
		return nil
	})

	client := larkws.NewClient(
		feishuCfg.AppID,
		feishuCfg.AppSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogger(newLarkSlogLogger(a.logger)),
		larkws.WithLogLevel(larkcore.LogLevelDebug),
	)

	go func() {
		if err := client.Start(connCtx); err != nil && a.logger != nil {
			a.logger.Error("client start failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
	}()

	stop := func(context.Context) error {
		cancel()
		return nil
	}
	return channel.NewConnection(cfg, stop), nil
}

func (a *FeishuAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	feishuCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("decode config failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return err
	}

	receiveID, receiveType, err := resolveFeishuReceiveID(strings.TrimSpace(msg.Target))
	if err != nil {
		return err
	}

	client := lark.NewClient(feishuCfg.AppID, feishuCfg.AppSecret)

	// 1. 处理附件
	if len(msg.Message.Attachments) > 0 {
		for _, att := range msg.Message.Attachments {
			if err := a.sendAttachment(ctx, client, receiveID, receiveType, att, msg.Message.Text); err != nil {
				return err
			}
		}
		return nil
	}

	// 2. 处理富文本或普通文本
	var msgType string
	var content string

	if len(msg.Message.Parts) > 1 {
		msgType = larkim.MsgTypePost
		content, err = a.buildPostContent(msg.Message)
	} else {
		msgType = larkim.MsgTypeText
		text := strings.TrimSpace(msg.Message.PlainText())
		if text == "" {
			return fmt.Errorf("message is required")
		}
		payload, _ := json.Marshal(map[string]string{"text": text})
		content = string(payload)
	}

	if err != nil {
		return err
	}

	reqBuilder := larkim.NewCreateMessageReqBodyBuilder().
		ReceiveId(receiveID).
		MsgType(msgType).
		Content(content).
		Uuid(uuid.NewString())

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveType).
		Body(reqBuilder.Build()).
		Build()

	// 处理回复
	if msg.Message.Reply != nil && msg.Message.Reply.MessageID != "" {
		replyReq := larkim.NewReplyMessageReqBuilder().
			MessageId(msg.Message.Reply.MessageID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				Content(content).
				MsgType(msgType).
				Uuid(uuid.NewString()).
				Build()).
			Build()
		resp, err := client.Im.V1.Message.Reply(ctx, replyReq)
		return a.handleReplyResponse(cfg.ID, resp, err)
	}

	resp, err := client.Im.V1.Message.Create(ctx, req)
	return a.handleResponse(cfg.ID, resp, err)
}

func (a *FeishuAdapter) handleReplyResponse(configID string, resp *larkim.ReplyMessageResp, err error) error {
	if err != nil {
		if a.logger != nil {
			a.logger.Error("reply failed", slog.String("config_id", configID), slog.Any("error", err))
		}
		return err
	}
	if resp == nil || !resp.Success() {
		code := 0
		msg := ""
		if resp != nil {
			code = resp.Code
			msg = resp.Msg
		}
		if a.logger != nil {
			a.logger.Error("reply failed", slog.String("config_id", configID), slog.Int("code", code), slog.String("msg", msg))
		}
		return fmt.Errorf("feishu reply failed: %s (code: %d)", msg, code)
	}
	if a.logger != nil {
		a.logger.Info("reply success", slog.String("config_id", configID))
	}
	return nil
}

func (a *FeishuAdapter) handleResponse(configID string, resp *larkim.CreateMessageResp, err error) error {
	if err != nil {
		if a.logger != nil {
			a.logger.Error("send failed", slog.String("config_id", configID), slog.Any("error", err))
		}
		return err
	}
	if resp == nil || !resp.Success() {
		code := 0
		msg := ""
		if resp != nil {
			code = resp.Code
			msg = resp.Msg
		}
		if a.logger != nil {
			a.logger.Error("send failed", slog.String("config_id", configID), slog.Int("code", code), slog.String("msg", msg))
		}
		return fmt.Errorf("feishu send failed: %s (code: %d)", msg, code)
	}
	if a.logger != nil {
		a.logger.Info("send success", slog.String("config_id", configID))
	}
	return nil
}

func (a *FeishuAdapter) sendAttachment(ctx context.Context, client *lark.Client, receiveID, receiveType string, att channel.Attachment, text string) error {
	// 下载文件
	resp, err := http.Get(att.URL)
	if err != nil {
		return fmt.Errorf("failed to download attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download attachment, status: %d", resp.StatusCode)
	}

	var msgType string
	var contentMap map[string]string

	if strings.HasPrefix(att.Mime, "image/") || att.Type == channel.AttachmentImage {
		// 上传图片
		uploadReq := larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().
				ImageType(larkim.ImageTypeMessage).
				Image(resp.Body).
				Build()).
			Build()
		uploadResp, err := client.Im.V1.Image.Create(ctx, uploadReq)
		if err != nil || !uploadResp.Success() {
			return fmt.Errorf("failed to upload image: %w", err)
		}
		msgType = larkim.MsgTypeImage
		contentMap = map[string]string{"image_key": *uploadResp.Data.ImageKey}
	} else {
		// 上传文件
		uploadReq := larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(larkim.FileTypePdf). // 默认为 pdf，飞书支持 mp4, doc, xls, ppt, pdf, zip
				FileName(att.Name).
				File(resp.Body).
				Build()).
			Build()
		uploadResp, err := client.Im.V1.File.Create(ctx, uploadReq)
		if err != nil || !uploadResp.Success() {
			return fmt.Errorf("failed to upload file: %w", err)
		}
		msgType = larkim.MsgTypeFile
		contentMap = map[string]string{"file_key": *uploadResp.Data.FileKey}
	}

	content, _ := json.Marshal(contentMap)
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(msgType).
			Content(string(content)).
			Uuid(uuid.NewString()).
			Build()).
		Build()

	_, err = client.Im.V1.Message.Create(ctx, req)
	return err
}

func (a *FeishuAdapter) buildPostContent(msg channel.Message) (string, error) {
	// 简单的 Post 构建逻辑
	type postContent struct {
		ZhCn struct {
			Title   string  `json:"title"`
			Content [][]any `json:"content"`
		} `json:"zh_cn"`
	}

	pc := postContent{}
	pc.ZhCn.Title = "" // 暂时不设标题

	line := []any{}
	for _, part := range msg.Parts {
		if part.Type == channel.MessagePartText {
			line = append(line, map[string]any{
				"tag":  "text",
				"text": part.Text,
			})
		}
	}
	pc.ZhCn.Content = [][]any{line}

	payload, err := json.Marshal(pc)
	return string(payload), err
}

func extractFeishuInbound(event *larkim.P2MessageReceiveV1) channel.InboundMessage {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return channel.InboundMessage{Channel: Type}
	}
	message := event.Event.Message

	var msg channel.Message
	if message.MessageId != nil {
		msg.ID = *message.MessageId
	}

	// 解析内容
	var contentMap map[string]any
	if message.Content != nil {
		_ = json.Unmarshal([]byte(*message.Content), &contentMap)
	}

	if message.MessageType != nil {
		switch *message.MessageType {
		case larkim.MsgTypeText:
			if txt, ok := contentMap["text"].(string); ok {
				msg.Text = txt
			}
		case larkim.MsgTypeImage:
			if key, ok := contentMap["image_key"].(string); ok {
				msg.Attachments = append(msg.Attachments, channel.Attachment{
					Type: channel.AttachmentImage,
					URL:  key, // 飞书内部 key，上层需注意
				})
			}
		case larkim.MsgTypeFile, larkim.MsgTypeAudio:
			if key, ok := contentMap["file_key"].(string); ok {
				name, _ := contentMap["file_name"].(string)
				msg.Attachments = append(msg.Attachments, channel.Attachment{
					Type: channel.AttachmentType(*message.MessageType),
					URL:  key,
					Name: name,
				})
			}
		}
	}

	// 处理回复引用
	if message.ParentId != nil && *message.ParentId != "" {
		msg.Reply = &channel.ReplyRef{
			MessageID: *message.ParentId,
		}
	}

	senderID, senderOpenID := "", ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil {
		if event.Event.Sender.SenderId.UserId != nil {
			senderID = strings.TrimSpace(*event.Event.Sender.SenderId.UserId)
		}
		if event.Event.Sender.SenderId.OpenId != nil {
			senderOpenID = strings.TrimSpace(*event.Event.Sender.SenderId.OpenId)
		}
	}
	chatID := ""
	chatType := ""
	if message.ChatId != nil {
		chatID = strings.TrimSpace(*message.ChatId)
	}
	if message.ChatType != nil {
		chatType = strings.TrimSpace(*message.ChatType)
	}
	replyTo := senderOpenID
	if replyTo == "" {
		replyTo = senderID
	}
	if chatType != "" && chatType != "p2p" && chatID != "" {
		replyTo = "chat_id:" + chatID
	}
	attrs := map[string]string{}
	if senderID != "" {
		attrs["user_id"] = senderID
	}
	if senderOpenID != "" {
		attrs["open_id"] = senderOpenID
	}
	externalID := senderOpenID
	if externalID == "" {
		externalID = senderID
	}

	return channel.InboundMessage{
		Channel:     Type,
		Message:     msg,
		ReplyTarget: replyTo,
		Sender: channel.Identity{
			ExternalID:  externalID,
			DisplayName: senderOpenID,
			Attributes:  attrs,
		},
		Conversation: channel.Conversation{
			ID:   chatID,
			Type: chatType,
		},
		ReceivedAt: time.Now().UTC(),
		Source:     "feishu",
	}
}

func resolveFeishuReceiveID(raw string) (string, string, error) {
	if raw == "" {
		return "", "", fmt.Errorf("feishu target is required")
	}
	if strings.HasPrefix(raw, "open_id:") {
		return strings.TrimPrefix(raw, "open_id:"), larkim.ReceiveIdTypeOpenId, nil
	}
	if strings.HasPrefix(raw, "user_id:") {
		return strings.TrimPrefix(raw, "user_id:"), larkim.ReceiveIdTypeUserId, nil
	}
	if strings.HasPrefix(raw, "chat_id:") {
		return strings.TrimPrefix(raw, "chat_id:"), larkim.ReceiveIdTypeChatId, nil
	}
	return raw, larkim.ReceiveIdTypeOpenId, nil
}
