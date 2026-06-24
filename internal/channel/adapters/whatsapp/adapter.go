package whatsapp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/media"
)

const (
	StatusDisconnected = "disconnected"
	StatusQRPending    = "qr_pending"
	StatusConnected    = "connected"
	StatusReconnecting = "reconnecting"
	StatusLoggedOut    = "logged_out"
	StatusTerminal     = "terminal"

	sendConnectionWait = 5 * time.Second

	whatsAppDownloadOverheadBytes int64 = 1024 * 1024
)

type RuntimeStatus struct {
	ConfigID  string    `json:"config_id,omitempty"`
	BotID     string    `json:"bot_id,omitempty"`
	Status    string    `json:"status"`
	Running   bool      `json:"running"`
	LastError string    `json:"last_error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type runtimeEntry struct {
	cfg         channel.ChannelConfig
	container   interface{ Close() error }
	client      *whatsmeow.Client
	handlerID   uint32
	conn        *channel.BaseConnection
	cancel      context.CancelFunc
	stoppedOnce sync.Once
}

type Adapter struct {
	logger   *slog.Logger
	dataRoot string

	mu              sync.Mutex
	runtimes        map[string]*runtimeEntry
	statuses        map[string]RuntimeStatus
	terminalHandler func(context.Context, channel.ChannelConfig, string, error)
}

func NewAdapter(log *slog.Logger, dataRoot string) *Adapter {
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{
		logger:   log.With(slog.String("adapter", "whatsapp")),
		dataRoot: dataRoot,
		runtimes: map[string]*runtimeEntry{},
		statuses: map[string]RuntimeStatus{},
	}
}

func (*Adapter) Type() channel.ChannelType { return Type }

func (a *Adapter) SetTerminalHandler(handler func(context.Context, channel.ChannelConfig, string, error)) {
	a.mu.Lock()
	a.terminalHandler = handler
	a.mu.Unlock()
}

func (*Adapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "WhatsApp",
		SetupMode:   channel.SetupModeQR,
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Attachments:    true,
			Reply:          true,
			BlockStreaming: true,
			ChatTypes:      []string{channel.ConversationTypePrivate},
		},
		OutboundPolicy: channel.OutboundPolicy{
			TextChunkLimit: 4000,
			ChunkerMode:    channel.ChunkerModeText,
		},
		ConfigSchema: channel.ConfigSchema{Version: 1, Fields: map[string]channel.FieldSchema{}},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"jid":   {Type: channel.FieldString, Title: "WhatsApp JID", Example: "1234567890@s.whatsapp.net"},
				"phone": {Type: channel.FieldString, Title: "Phone", Example: "+1234567890"},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "phone | jid",
			Hints: []channel.TargetHint{
				{Label: "Phone", Example: "+1234567890"},
				{Label: "JID", Example: "1234567890@s.whatsapp.net"},
			},
		},
	}
}

func (*Adapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

func (*Adapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

func (*Adapter) NormalizeTarget(raw string) string { return normalizeTarget(raw) }

func (*Adapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

func (*Adapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

func (*Adapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

func (a *Adapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	waCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	if waCfg.NeedsRelink {
		return nil, errors.New("whatsapp channel needs QR relink")
	}
	if strings.TrimSpace(waCfg.StoreID) == "" {
		return nil, errors.New("whatsapp storeId is required")
	}
	paths := finalStorePaths(a.dataRoot, waCfg.StoreID)
	container, client, err := openClientStore(ctx, paths, waLog.Noop)
	if err != nil {
		a.setStatus(cfg, StatusTerminal, false, err)
		return nil, err
	}
	if client.Store.ID == nil {
		_ = container.Close()
		err := errors.New("whatsapp store is not linked; scan QR again")
		a.setStatus(cfg, StatusLoggedOut, false, err)
		if terminalHandler := a.getTerminalHandler(); terminalHandler != nil {
			go terminalHandler(context.WithoutCancel(ctx), cfg, StatusLoggedOut, err)
		}
		return nil, err
	}

	connCtx, cancel := context.WithCancel(ctx)
	entry := &runtimeEntry{cfg: cfg, container: container, client: client, cancel: cancel}
	entry.handlerID = client.AddEventHandler(func(evt any) {
		a.handleEvent(connCtx, entry, handler, evt)
	})
	conn := channel.NewConnection(cfg, func(stopCtx context.Context) error {
		a.stopRuntime(stopCtx, cfg.ID)
		return nil
	})
	entry.conn = conn

	a.mu.Lock()
	if old := a.runtimes[cfg.ID]; old != nil {
		a.mu.Unlock()
		a.stopEntry(ctx, old)
		a.mu.Lock()
	}
	a.runtimes[cfg.ID] = entry
	a.mu.Unlock()

	if err := client.Connect(); err != nil {
		a.stopRuntime(ctx, cfg.ID)
		a.setStatus(cfg, StatusTerminal, false, err)
		return nil, err
	}
	a.setStatus(cfg, StatusReconnecting, true, nil)
	return conn, nil
}

func (a *Adapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
	jid, err := parsePrivateTarget(msg.Target)
	if err != nil {
		return err
	}
	client, err := a.clientForConfig(cfg)
	if err != nil {
		return err
	}
	return sendWhatsAppPreparedMessage(ctx, a.logger, client, jid, msg.Message, cfg.ID, cfg.BotID)
}

func sendWhatsAppPreparedMessage(ctx context.Context, logger *slog.Logger, client whatsAppMediaSender, jid types.JID, msg channel.PreparedMessage, configID, botID string) error {
	text := strings.TrimSpace(msg.Message.PlainText())
	replyContext := whatsAppReplyContext(msg.Message.Reply, jid)
	if len(msg.Attachments) > 0 {
		sendable, skipped := partitionWhatsAppSendableAttachments(msg.Attachments)
		if len(skipped) > 0 && logger != nil {
			logger.Warn("skip unsupported whatsapp attachments",
				slog.String("config_id", configID),
				slog.String("bot_id", botID),
				slog.Int("skipped", len(skipped)),
			)
		}
		for i, att := range sendable {
			caption := strings.TrimSpace(att.Logical.Caption)
			if i == 0 && text != "" {
				caption = text
			}
			contextInfo := replyContext
			if i > 0 {
				contextInfo = nil
			}
			if err := sendWhatsAppAttachment(ctx, client, jid, att, caption, contextInfo); err != nil {
				return err
			}
		}
		if len(sendable) > 0 {
			return nil
		}
	}
	if text == "" {
		return errors.New("whatsapp text message is required")
	}
	return sendWhatsAppText(ctx, client, jid, text, replyContext)
}

func sendWhatsAppText(ctx context.Context, client whatsAppMediaSender, jid types.JID, text string, replyContext *waE2E.ContextInfo) error {
	out := &waE2E.Message{Conversation: proto.String(text)}
	if replyContext != nil {
		out = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: replyContext,
		}}
	}
	_, err := client.SendMessage(ctx, jid, out)
	return err
}

type whatsAppMediaSender interface {
	Upload(ctx context.Context, plaintext []byte, appInfo whatsmeow.MediaType) (whatsmeow.UploadResponse, error)
	SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

func sendWhatsAppAttachment(ctx context.Context, client whatsAppMediaSender, jid types.JID, att channel.PreparedAttachment, caption string, contextInfo *waE2E.ContextInfo) error {
	attType := inferWhatsAppPreparedAttachmentType(att)
	switch attType {
	case channel.AttachmentImage:
		return sendWhatsAppImageAttachment(ctx, client, jid, att, caption, contextInfo)
	case channel.AttachmentAudio, channel.AttachmentGIF, channel.AttachmentVideo, channel.AttachmentVoice:
		return fmt.Errorf("whatsapp unsupported attachment type: %s", attType)
	default:
		return sendWhatsAppDocumentAttachment(ctx, client, jid, att, caption, contextInfo)
	}
}

func partitionWhatsAppSendableAttachments(attachments []channel.PreparedAttachment) ([]channel.PreparedAttachment, []channel.PreparedAttachment) {
	sendable := make([]channel.PreparedAttachment, 0, len(attachments))
	skipped := make([]channel.PreparedAttachment, 0)
	for _, att := range attachments {
		switch inferWhatsAppPreparedAttachmentType(att) {
		case channel.AttachmentAudio, channel.AttachmentGIF, channel.AttachmentVideo, channel.AttachmentVoice:
			skipped = append(skipped, att)
		default:
			sendable = append(sendable, att)
		}
	}
	return sendable, skipped
}

func inferWhatsAppPreparedAttachmentType(att channel.PreparedAttachment) channel.AttachmentType {
	return channel.InferAttachmentType(att.Logical.Type, firstNonEmpty(att.Mime, att.Logical.Mime), firstNonEmpty(att.Name, att.Logical.Name))
}

func sendWhatsAppImageAttachment(ctx context.Context, client whatsAppMediaSender, jid types.JID, att channel.PreparedAttachment, caption string, contextInfo *waE2E.ContextInfo) error {
	if client == nil {
		return errors.New("whatsapp client is not configured")
	}
	attType := inferWhatsAppPreparedAttachmentType(att)
	if attType != channel.AttachmentImage {
		return fmt.Errorf("whatsapp unsupported attachment type: %s", attType)
	}
	if att.Kind != channel.PreparedAttachmentUpload || att.Open == nil {
		return fmt.Errorf("whatsapp image attachment requires upload source, got %s", att.Kind)
	}
	reader, err := att.Open(ctx)
	if err != nil {
		return fmt.Errorf("open whatsapp image attachment: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	data, err := media.ReadAllWithLimit(reader, media.MaxAssetBytes)
	if err != nil {
		return fmt.Errorf("read whatsapp image attachment: %w", err)
	}
	upload, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload whatsapp image: %w", err)
	}
	image := &waE2E.ImageMessage{
		URL:           proto.String(upload.URL),
		DirectPath:    proto.String(upload.DirectPath),
		MediaKey:      upload.MediaKey,
		FileEncSHA256: upload.FileEncSHA256,
		FileSHA256:    upload.FileSHA256,
		FileLength:    proto.Uint64(upload.FileLength),
		Mimetype:      proto.String(resolveWhatsAppImageMIME(att, data)),
	}
	if caption = strings.TrimSpace(caption); caption != "" {
		image.Caption = proto.String(caption)
	}
	if contextInfo != nil {
		image.ContextInfo = contextInfo
	}
	if err := applyWhatsAppImageDimensions(image, att.Logical.Width, att.Logical.Height); err != nil {
		return err
	}
	_, err = client.SendMessage(ctx, jid, &waE2E.Message{ImageMessage: image})
	return err
}

func sendWhatsAppDocumentAttachment(ctx context.Context, client whatsAppMediaSender, jid types.JID, att channel.PreparedAttachment, caption string, contextInfo *waE2E.ContextInfo) error {
	if client == nil {
		return errors.New("whatsapp client is not configured")
	}
	if att.Kind != channel.PreparedAttachmentUpload || att.Open == nil {
		return fmt.Errorf("whatsapp document attachment requires upload source, got %s", att.Kind)
	}
	reader, err := att.Open(ctx)
	if err != nil {
		return fmt.Errorf("open whatsapp document attachment: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	data, err := media.ReadAllWithLimit(reader, media.MaxAssetBytes)
	if err != nil {
		return fmt.Errorf("read whatsapp document attachment: %w", err)
	}
	upload, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload whatsapp document: %w", err)
	}
	name := whatsAppDocumentName(att)
	document := &waE2E.DocumentMessage{
		URL:           proto.String(upload.URL),
		DirectPath:    proto.String(upload.DirectPath),
		MediaKey:      upload.MediaKey,
		FileEncSHA256: upload.FileEncSHA256,
		FileSHA256:    upload.FileSHA256,
		FileLength:    proto.Uint64(upload.FileLength),
		Mimetype:      proto.String(resolveWhatsAppDocumentMIME(att, data)),
		FileName:      proto.String(name),
		Title:         proto.String(name),
	}
	if caption = strings.TrimSpace(caption); caption != "" {
		document.Caption = proto.String(caption)
	}
	if contextInfo != nil {
		document.ContextInfo = contextInfo
	}
	_, err = client.SendMessage(ctx, jid, &waE2E.Message{DocumentMessage: document})
	return err
}

func whatsAppReplyContext(reply *channel.ReplyRef, fallback types.JID) *waE2E.ContextInfo {
	if reply == nil || strings.TrimSpace(reply.MessageID) == "" {
		return nil
	}
	replyJID := fallback.ToNonAD()
	if target := strings.TrimSpace(reply.Target); target != "" {
		if parsed, err := parsePrivateTarget(target); err == nil {
			replyJID = parsed.ToNonAD()
		}
	}
	ctx := &waE2E.ContextInfo{
		StanzaID:    proto.String(strings.TrimSpace(reply.MessageID)),
		RemoteJID:   proto.String(replyJID.String()),
		Participant: proto.String(replyJID.String()),
	}
	if preview := strings.TrimSpace(reply.Preview); preview != "" {
		ctx.QuotedMessage = &waE2E.Message{Conversation: proto.String(preview)}
	}
	return ctx
}

func resolveWhatsAppImageMIME(att channel.PreparedAttachment, data []byte) string {
	mimeType := strings.TrimSpace(att.Mime)
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return mimeType
	}
	if logical := strings.TrimSpace(att.Logical.Mime); strings.HasPrefix(strings.ToLower(logical), "image/") {
		return logical
	}
	if len(data) > 0 {
		detected := http.DetectContentType(data)
		if strings.HasPrefix(strings.ToLower(detected), "image/") {
			return detected
		}
	}
	return "image/jpeg"
}

func resolveWhatsAppDocumentMIME(att channel.PreparedAttachment, data []byte) string {
	if mimeType := strings.TrimSpace(att.Mime); mimeType != "" {
		return mimeType
	}
	if logical := strings.TrimSpace(att.Logical.Mime); logical != "" {
		return logical
	}
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}

func whatsAppDocumentName(att channel.PreparedAttachment) string {
	for _, value := range []string{att.Name, att.Logical.Name} {
		if name := strings.TrimSpace(value); name != "" {
			return name
		}
	}
	return "attachment"
}

type finalOnlyStream struct {
	cfg   channel.ChannelConfig
	to    string
	reply *channel.ReplyRef
	send  func(context.Context, channel.ChannelConfig, channel.PreparedOutboundMessage) error

	mu          sync.Mutex
	closed      bool
	sent        bool
	finalTried  bool
	textBuilder strings.Builder
	attachments []channel.PreparedAttachment
}

func (a *Adapter) OpenStream(_ context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.PreparedOutboundStream, error) {
	jid, err := parsePrivateTarget(target)
	if err != nil {
		return nil, err
	}
	reply := cloneReplyRef(opts.Reply)
	if reply == nil && strings.TrimSpace(opts.SourceMessageID) != "" {
		reply = &channel.ReplyRef{Target: jid.String(), MessageID: strings.TrimSpace(opts.SourceMessageID)}
	}
	return &finalOnlyStream{cfg: cfg, to: jid.String(), reply: reply, send: a.Send}, nil
}

func (s *finalOnlyStream) Push(ctx context.Context, event channel.PreparedStreamEvent) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("whatsapp stream is closed")
	}
	switch event.Type {
	case channel.StreamEventDelta:
		if strings.TrimSpace(event.Delta) != "" && event.Phase != channel.StreamPhaseReasoning {
			s.textBuilder.WriteString(event.Delta)
		}
		s.mu.Unlock()
	case channel.StreamEventAttachment:
		if len(event.Attachments) == 0 {
			s.mu.Unlock()
			return nil
		}
		s.attachments = append(s.attachments, event.Attachments...)
		s.mu.Unlock()
		return nil
	case channel.StreamEventFinal:
		var final *channel.PreparedMessage
		if event.Final != nil {
			msg := event.Final.Message
			final = &msg
		}
		msg, ok := s.messageForFlushLocked(final)
		if ok {
			s.finalTried = true
		}
		s.mu.Unlock()
		if !ok {
			s.markSent()
			return nil
		}
		if s.send == nil {
			return errors.New("whatsapp stream sender is not configured")
		}
		if err := s.send(ctx, s.cfg, channel.PreparedOutboundMessage{Target: s.to, Message: msg}); err != nil {
			return err
		}
		s.markSent()
		return nil
	default:
		s.mu.Unlock()
		return nil
	}
	return nil
}

func (s *finalOnlyStream) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.sent || s.finalTried {
		s.mu.Unlock()
		return nil
	}
	msg, ok := s.messageForFlushLocked(nil)
	s.mu.Unlock()

	if !ok {
		return nil
	}
	if s.send == nil {
		return errors.New("whatsapp stream sender is not configured")
	}
	return s.send(ctx, s.cfg, channel.PreparedOutboundMessage{Target: s.to, Message: msg})
}

func (s *finalOnlyStream) messageForFlushLocked(final *channel.PreparedMessage) (channel.PreparedMessage, bool) {
	msg := channel.PreparedMessage{Message: channel.Message{Format: channel.MessageFormatPlain}}
	if final != nil {
		msg = *final
	}
	if strings.TrimSpace(msg.Message.Text) == "" {
		msg.Message.Text = strings.TrimSpace(s.textBuilder.String())
	}
	if msg.Message.Reply == nil && s.reply != nil {
		msg.Message.Reply = cloneReplyRef(s.reply)
	}
	if len(s.attachments) > 0 {
		msg.Attachments = appendPreparedAttachments(s.attachments, msg.Attachments)
		msg.Message.Attachments = whatsAppLogicalAttachments(msg.Attachments)
	}
	return msg, !msg.Message.IsEmpty() || len(msg.Attachments) > 0
}

func (s *finalOnlyStream) markSent() {
	s.mu.Lock()
	s.sent = true
	s.textBuilder.Reset()
	s.attachments = nil
	s.mu.Unlock()
}

func cloneReplyRef(reply *channel.ReplyRef) *channel.ReplyRef {
	if reply == nil {
		return nil
	}
	out := *reply
	if len(reply.Attachments) > 0 {
		out.Attachments = append([]channel.Attachment(nil), reply.Attachments...)
	}
	return &out
}

func appendPreparedAttachments(prefix, suffix []channel.PreparedAttachment) []channel.PreparedAttachment {
	if len(prefix) == 0 {
		return append([]channel.PreparedAttachment(nil), suffix...)
	}
	out := make([]channel.PreparedAttachment, 0, len(prefix)+len(suffix))
	out = append(out, prefix...)
	out = append(out, suffix...)
	return out
}

func whatsAppLogicalAttachments(attachments []channel.PreparedAttachment) []channel.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	logical := make([]channel.Attachment, 0, len(attachments))
	for _, att := range attachments {
		logical = append(logical, att.Logical)
	}
	return logical
}

func (a *Adapter) clientForConfig(cfg channel.ChannelConfig) (*whatsmeow.Client, error) {
	a.mu.Lock()
	if entry := a.runtimes[cfg.ID]; entry != nil && entry.client != nil {
		client := entry.client
		a.mu.Unlock()
		if client.WaitForConnection(sendConnectionWait) {
			return client, nil
		}
		return nil, errors.New("whatsapp channel is not connected")
	}
	a.mu.Unlock()
	return nil, errors.New("whatsapp channel is not connected")
}

func (a *Adapter) handleEvent(ctx context.Context, entry *runtimeEntry, handler channel.InboundHandler, evt any) {
	switch v := evt.(type) {
	case *events.Connected:
		a.setStatus(entry.cfg, StatusConnected, true, nil)
	case *events.KeepAliveTimeout:
		a.setStatus(entry.cfg, StatusReconnecting, true, fmt.Errorf("keepalive timeout: %d", v.ErrorCount))
	case *events.KeepAliveRestored:
		a.setStatus(entry.cfg, StatusConnected, true, nil)
	case *events.Disconnected:
		a.setStatus(entry.cfg, StatusReconnecting, true, nil)
	case *events.LoggedOut:
		a.markTerminal(ctx, entry, StatusLoggedOut, errors.New(v.Reason.String()))
	case *events.StreamReplaced:
		a.markTerminal(ctx, entry, StatusTerminal, errors.New("stream replaced"))
	case *events.ClientOutdated:
		a.markTerminal(ctx, entry, StatusTerminal, errors.New("client outdated"))
	case *events.TemporaryBan:
		a.markTerminal(ctx, entry, StatusTerminal, errors.New(v.String()))
	case *events.Message:
		a.handleMessage(ctx, entry.cfg, v, handler)
	}
}

func (a *Adapter) markTerminal(ctx context.Context, entry *runtimeEntry, status string, err error) {
	a.setStatus(entry.cfg, status, false, err)
	if entry.conn != nil {
		entry.conn.MarkStopped()
	}
	cleanupCtx := context.WithoutCancel(ctx)
	handler := a.getTerminalHandler()
	if handler != nil {
		go handler(cleanupCtx, entry.cfg, status, err)
	}
	go a.stopRuntimeEntry(cleanupCtx, entry)
}

func (a *Adapter) handleMessage(ctx context.Context, cfg channel.ChannelConfig, evt *events.Message, handler channel.InboundHandler) {
	if evt == nil || evt.Message == nil || handler == nil {
		return
	}
	info := evt.Info
	if info.IsFromMe {
		a.logIgnoredMessage(cfg, info, "from_self")
		return
	}
	if info.IsGroup {
		a.logIgnoredMessage(cfg, info, "group")
		return
	}
	if info.Chat.IsBroadcastList() {
		a.logIgnoredMessage(cfg, info, "broadcast_list")
		return
	}
	if info.Chat == types.StatusBroadcastJID {
		a.logIgnoredMessage(cfg, info, "status_broadcast")
		return
	}
	if !isPrivateUserServer(info.Chat.Server) {
		a.logIgnoredMessage(cfg, info, "unsupported_server")
		return
	}
	text, attachments := extractContent(evt.Message)
	if text == "" && len(attachments) == 0 {
		a.logIgnoredMessage(cfg, info, "non_text")
		return
	}
	chat := info.Chat.ToNonAD()
	sender := info.Sender.ToNonAD()
	if sender.IsEmpty() {
		sender = chat
	}
	receivedAt := info.Timestamp
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	senderAttrs := identityAttributes(sender, strings.TrimSpace(info.PushName))
	chatMeta := jidMetadata(chat)
	msg := channel.InboundMessage{
		Channel:     Type,
		BotID:       cfg.BotID,
		ReplyTarget: chat.String(),
		RouteKey:    channel.GenerateRoutingKey(Type.String(), cfg.BotID, chat.String(), channel.ConversationTypePrivate, sender.String()),
		Sender: channel.Identity{
			SubjectID:   sender.String(),
			DisplayName: strings.TrimSpace(info.PushName),
			Attributes:  senderAttrs,
		},
		Conversation: channel.Conversation{
			ID:       chat.String(),
			Type:     channel.ConversationTypePrivate,
			Name:     strings.TrimSpace(info.PushName),
			Metadata: chatMeta,
		},
		Message: channel.Message{
			ID:          info.ID,
			Format:      channel.MessageFormatPlain,
			Text:        text,
			Attachments: attachments,
			Reply:       extractReplyRef(evt.Message, chat.String()),
		},
		ReceivedAt: receivedAt,
		Source:     Type.String(),
		Metadata: map[string]any{
			"message_id": info.ID,
			"chat_jid":   chat.String(),
			"sender_jid": sender.String(),
		},
	}
	if a.logger != nil {
		a.logger.Debug("whatsapp inbound message accepted",
			slog.String("config_id", cfg.ID),
			slog.String("bot_id", cfg.BotID),
			slog.String("chat", chat.String()),
			slog.String("sender", sender.String()),
			slog.String("message_id", info.ID),
		)
	}
	if err := handler(ctx, cfg, msg); err != nil && a.logger != nil {
		a.logger.Warn("handle whatsapp inbound failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
	}
}

func (a *Adapter) logIgnoredMessage(cfg channel.ChannelConfig, info types.MessageInfo, reason string) {
	if a.logger == nil {
		return
	}
	a.logger.Debug("whatsapp inbound message ignored",
		slog.String("reason", reason),
		slog.String("config_id", cfg.ID),
		slog.String("bot_id", cfg.BotID),
		slog.String("chat", info.Chat.String()),
		slog.String("sender", info.Sender.String()),
		slog.String("message_id", info.ID),
	)
}

func isPrivateUserServer(server string) bool {
	return server == types.DefaultUserServer || server == types.HiddenUserServer
}

func identityAttributes(jid types.JID, pushName string) map[string]string {
	attrs := map[string]string{"jid": jid.String()}
	if pushName != "" {
		attrs["push_name"] = pushName
	}
	switch jid.Server {
	case types.DefaultUserServer:
		attrs["phone"] = jid.User
	case types.HiddenUserServer:
		attrs["lid"] = jid.User
	}
	return attrs
}

func jidMetadata(jid types.JID) map[string]any {
	meta := map[string]any{"jid": jid.String()}
	switch jid.Server {
	case types.DefaultUserServer:
		meta["phone"] = jid.User
	case types.HiddenUserServer:
		meta["lid"] = jid.User
	}
	return meta
}

func extractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if text := strings.TrimSpace(msg.GetConversation()); text != "" {
		return text
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return strings.TrimSpace(ext.GetText())
	}
	return ""
}

func extractContent(msg *waE2E.Message) (string, []channel.Attachment) {
	text := extractText(msg)
	attachments := extractAttachments(msg)
	if text == "" {
		if image := msg.GetImageMessage(); image != nil {
			text = strings.TrimSpace(image.GetCaption())
		}
	}
	if text == "" {
		if document := msg.GetDocumentMessage(); document != nil {
			text = strings.TrimSpace(document.GetCaption())
		}
	}
	return text, attachments
}

func extractReplyRef(msg *waE2E.Message, fallbackTarget string) *channel.ReplyRef {
	ctx := extractContextInfo(msg)
	if ctx == nil {
		return nil
	}
	messageID := strings.TrimSpace(ctx.GetStanzaID())
	if messageID == "" {
		return nil
	}
	target := strings.TrimSpace(ctx.GetRemoteJID())
	if target == "" {
		target = strings.TrimSpace(fallbackTarget)
	}
	if parsed, err := parsePrivateTarget(target); err == nil {
		target = parsed.String()
	}
	preview, attachments := extractContent(ctx.GetQuotedMessage())
	ref := &channel.ReplyRef{
		Target:      target,
		MessageID:   messageID,
		Sender:      strings.TrimSpace(ctx.GetParticipant()),
		Preview:     preview,
		Attachments: attachments,
	}
	return ref
}

func extractContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		if ctx := ext.GetContextInfo(); ctx != nil {
			return ctx
		}
	}
	if image := msg.GetImageMessage(); image != nil {
		if ctx := image.GetContextInfo(); ctx != nil {
			return ctx
		}
	}
	if document := msg.GetDocumentMessage(); document != nil {
		if ctx := document.GetContextInfo(); ctx != nil {
			return ctx
		}
	}
	return nil
}

func extractAttachments(msg *waE2E.Message) []channel.Attachment {
	if msg == nil {
		return nil
	}
	var attachments []channel.Attachment
	if image := msg.GetImageMessage(); image != nil {
		if att, ok := imageAttachment(image); ok {
			attachments = append(attachments, att)
		}
	}
	if document := msg.GetDocumentMessage(); document != nil {
		if att, ok := documentAttachment(document); ok {
			attachments = append(attachments, att)
		}
	}
	return attachments
}

func imageAttachment(image *waE2E.ImageMessage) (channel.Attachment, bool) {
	if image == nil || strings.TrimSpace(image.GetDirectPath()) == "" {
		return channel.Attachment{}, false
	}
	size := whatsAppFileLengthToSize(image.GetFileLength())
	att := channel.Attachment{
		Type:           channel.AttachmentImage,
		PlatformKey:    strings.TrimSpace(image.GetDirectPath()),
		SourcePlatform: Type.String(),
		Caption:        strings.TrimSpace(image.GetCaption()),
		Mime:           strings.TrimSpace(image.GetMimetype()),
		Size:           size,
		Width:          int(image.GetWidth()),
		Height:         int(image.GetHeight()),
		Metadata: map[string]any{
			"media_type":      "image",
			"direct_path":     strings.TrimSpace(image.GetDirectPath()),
			"media_key":       encodeWhatsAppMediaBytes(image.GetMediaKey()),
			"file_sha256":     encodeWhatsAppMediaBytes(image.GetFileSHA256()),
			"file_enc_sha256": encodeWhatsAppMediaBytes(image.GetFileEncSHA256()),
		},
	}
	return channel.NormalizeInboundChannelAttachment(att), true
}

func documentAttachment(document *waE2E.DocumentMessage) (channel.Attachment, bool) {
	if document == nil || strings.TrimSpace(document.GetDirectPath()) == "" {
		return channel.Attachment{}, false
	}
	name := strings.TrimSpace(document.GetFileName())
	if name == "" {
		name = strings.TrimSpace(document.GetTitle())
	}
	att := channel.Attachment{
		Type:           channel.AttachmentFile,
		PlatformKey:    strings.TrimSpace(document.GetDirectPath()),
		SourcePlatform: Type.String(),
		Caption:        strings.TrimSpace(document.GetCaption()),
		Name:           name,
		Mime:           strings.TrimSpace(document.GetMimetype()),
		Size:           whatsAppFileLengthToSize(document.GetFileLength()),
		Metadata: map[string]any{
			"media_type":      "document",
			"direct_path":     strings.TrimSpace(document.GetDirectPath()),
			"media_key":       encodeWhatsAppMediaBytes(document.GetMediaKey()),
			"file_sha256":     encodeWhatsAppMediaBytes(document.GetFileSHA256()),
			"file_enc_sha256": encodeWhatsAppMediaBytes(document.GetFileEncSHA256()),
			"file_name":       name,
		},
	}
	return channel.NormalizeInboundChannelAttachment(att), true
}

func encodeWhatsAppMediaBytes(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(value)
}

func (a *Adapter) ResolveAttachment(ctx context.Context, cfg channel.ChannelConfig, attachment channel.Attachment) (channel.AttachmentPayload, error) {
	client, err := a.clientForConfig(cfg)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	return resolveWhatsAppAttachment(ctx, client, attachment)
}

type whatsAppMediaDownloader interface {
	DownloadToFile(ctx context.Context, msg whatsmeow.DownloadableMessage, file whatsmeow.File) error
}

func resolveWhatsAppAttachment(ctx context.Context, client whatsAppMediaDownloader, attachment channel.Attachment) (channel.AttachmentPayload, error) {
	return resolveWhatsAppAttachmentWithLimit(ctx, client, attachment, media.MaxAssetBytes)
}

func resolveWhatsAppAttachmentWithLimit(ctx context.Context, client whatsAppMediaDownloader, attachment channel.Attachment, maxBytes int64) (channel.AttachmentPayload, error) {
	if client == nil {
		return channel.AttachmentPayload{}, errors.New("whatsapp client is not configured")
	}
	if maxBytes <= 0 {
		return channel.AttachmentPayload{}, errors.New("max bytes must be greater than 0")
	}
	if attachment.Size > maxBytes {
		return channel.AttachmentPayload{}, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, maxBytes)
	}
	downloadable, err := whatsappDownloadableMessage(attachment)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	tmp, err := os.CreateTemp("", "memoh-whatsapp-attachment-*")
	if err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("create whatsapp attachment temp file: %w", err)
	}
	path := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(path)
		}
	}()
	capped := &cappedWhatsAppFile{
		file:       tmp,
		writeLimit: maxBytes + whatsAppDownloadOverheadBytes,
		assetLimit: maxBytes,
	}
	if err := client.DownloadToFile(ctx, downloadable, capped); err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("download whatsapp attachment: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("seek whatsapp attachment: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("stat whatsapp attachment: %w", err)
	}
	size := info.Size()
	if size > maxBytes {
		return channel.AttachmentPayload{}, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, maxBytes)
	}
	mimeType := strings.TrimSpace(attachment.Mime)
	if mimeType == "" && size > 0 {
		mimeType, err = detectWhatsAppAttachmentMIME(tmp)
		if err != nil {
			return channel.AttachmentPayload{}, err
		}
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("seek whatsapp attachment: %w", err)
	}
	cleanup = false
	return channel.AttachmentPayload{
		Reader: &deleteOnCloseFile{File: tmp, path: path},
		Mime:   mimeType,
		Name:   whatsappAttachmentName(attachment, mimeType),
		Size:   size,
	}, nil
}

func detectWhatsAppAttachmentMIME(file *os.File) (string, error) {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("detect whatsapp attachment mime: %w", err)
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return "", fmt.Errorf("seek whatsapp attachment: %w", seekErr)
	}
	if n == 0 {
		return "", nil
	}
	return http.DetectContentType(buf[:n]), nil
}

type cappedWhatsAppFile struct {
	file       *os.File
	writeLimit int64
	assetLimit int64
}

func (f *cappedWhatsAppFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *cappedWhatsAppFile) Write(p []byte) (int, error) {
	offset, err := f.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	if offset >= f.writeLimit {
		return 0, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, f.assetLimit)
	}
	remaining := f.writeLimit - offset
	if int64(len(p)) > remaining {
		n, writeErr := f.file.Write(p[:remaining])
		if writeErr != nil {
			return n, writeErr
		}
		return n, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, f.assetLimit)
	}
	return f.file.Write(p)
}

func (f *cappedWhatsAppFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *cappedWhatsAppFile) ReadAt(p []byte, off int64) (int, error) {
	return f.file.ReadAt(p, off)
}

func (f *cappedWhatsAppFile) WriteAt(p []byte, off int64) (int, error) {
	if off+int64(len(p)) > f.writeLimit {
		return 0, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, f.assetLimit)
	}
	return f.file.WriteAt(p, off)
}

func (f *cappedWhatsAppFile) Truncate(size int64) error {
	if size > f.writeLimit {
		return fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, f.assetLimit)
	}
	return f.file.Truncate(size)
}

func (f *cappedWhatsAppFile) Stat() (os.FileInfo, error) {
	return f.file.Stat()
}

type deleteOnCloseFile struct {
	*os.File
	path string
}

func (f *deleteOnCloseFile) Close() error {
	err := f.File.Close()
	if removeErr := os.Remove(f.path); err == nil {
		err = removeErr
	}
	return err
}

func whatsappDownloadableMessage(attachment channel.Attachment) (whatsmeow.DownloadableMessage, error) {
	switch mediaType := metadataString(attachment.Metadata, "media_type"); mediaType {
	case "image":
		return whatsappImageDownloadable(attachment)
	case "document":
		return whatsappDocumentDownloadable(attachment)
	}
	switch attachment.Type {
	case channel.AttachmentImage:
		return whatsappImageDownloadable(attachment)
	case channel.AttachmentFile, channel.AttachmentGIF:
		return whatsappDocumentDownloadable(attachment)
	default:
		return nil, fmt.Errorf("whatsapp unsupported attachment type: %s", attachment.Type)
	}
}

func whatsappImageDownloadable(attachment channel.Attachment) (*waE2E.ImageMessage, error) {
	directPath := metadataString(attachment.Metadata, "direct_path")
	if directPath == "" {
		directPath = strings.TrimSpace(attachment.PlatformKey)
	}
	if directPath == "" {
		return nil, errors.New("whatsapp image direct path is required")
	}
	mediaKey, err := decodeMetadataBytes(attachment.Metadata, "media_key")
	if err != nil {
		return nil, err
	}
	if len(mediaKey) == 0 {
		return nil, errors.New("whatsapp image media key is required")
	}
	fileSHA256, err := decodeMetadataBytes(attachment.Metadata, "file_sha256")
	if err != nil {
		return nil, err
	}
	fileEncSHA256, err := decodeMetadataBytes(attachment.Metadata, "file_enc_sha256")
	if err != nil {
		return nil, err
	}
	image := &waE2E.ImageMessage{
		DirectPath:    proto.String(directPath),
		MediaKey:      mediaKey,
		FileSHA256:    fileSHA256,
		FileEncSHA256: fileEncSHA256,
	}
	if mimeType := strings.TrimSpace(attachment.Mime); mimeType != "" {
		image.Mimetype = proto.String(mimeType)
	}
	if attachment.Size > 0 {
		image.FileLength = proto.Uint64(uint64(attachment.Size))
	}
	if err := applyWhatsAppImageDimensions(image, attachment.Width, attachment.Height); err != nil {
		return nil, err
	}
	return image, nil
}

func whatsappDocumentDownloadable(attachment channel.Attachment) (*waE2E.DocumentMessage, error) {
	directPath := metadataString(attachment.Metadata, "direct_path")
	if directPath == "" {
		directPath = strings.TrimSpace(attachment.PlatformKey)
	}
	if directPath == "" {
		return nil, errors.New("whatsapp document direct path is required")
	}
	mediaKey, err := decodeMetadataBytes(attachment.Metadata, "media_key")
	if err != nil {
		return nil, err
	}
	if len(mediaKey) == 0 {
		return nil, errors.New("whatsapp document media key is required")
	}
	fileSHA256, err := decodeMetadataBytes(attachment.Metadata, "file_sha256")
	if err != nil {
		return nil, err
	}
	fileEncSHA256, err := decodeMetadataBytes(attachment.Metadata, "file_enc_sha256")
	if err != nil {
		return nil, err
	}
	document := &waE2E.DocumentMessage{
		DirectPath:    proto.String(directPath),
		MediaKey:      mediaKey,
		FileSHA256:    fileSHA256,
		FileEncSHA256: fileEncSHA256,
	}
	if mimeType := strings.TrimSpace(attachment.Mime); mimeType != "" {
		document.Mimetype = proto.String(mimeType)
	}
	if name := firstNonEmpty(attachment.Name, metadataString(attachment.Metadata, "file_name")); name != "" {
		document.FileName = proto.String(name)
		document.Title = proto.String(name)
	}
	if attachment.Size > 0 {
		document.FileLength = proto.Uint64(int64ToUint64(attachment.Size))
	}
	if caption := strings.TrimSpace(attachment.Caption); caption != "" {
		document.Caption = proto.String(caption)
	}
	return document, nil
}

func applyWhatsAppImageDimensions(image *waE2E.ImageMessage, width, height int) error {
	if image == nil {
		return errors.New("whatsapp image message is required")
	}
	if width > 0 {
		if width > math.MaxUint32 {
			return fmt.Errorf("whatsapp image width exceeds uint32: %d", width)
		}
		checkedWidth := uint32(width) //nolint:gosec // guarded by MaxUint32 check above.
		image.Width = proto.Uint32(checkedWidth)
	}
	if height > 0 {
		if height > math.MaxUint32 {
			return fmt.Errorf("whatsapp image height exceeds uint32: %d", height)
		}
		checkedHeight := uint32(height) //nolint:gosec // guarded by MaxUint32 check above.
		image.Height = proto.Uint32(checkedHeight)
	}
	return nil
}

func whatsAppFileLengthToSize(length uint64) int64 {
	if length > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(length) //nolint:gosec // guarded by MaxInt64 check above.
}

func int64ToUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value) //nolint:gosec // guarded by positive check above.
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	switch value := metadata[key].(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func decodeMetadataBytes(metadata map[string]any, key string) ([]byte, error) {
	raw := metadataString(metadata, key)
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode whatsapp %s: %w", key, err)
	}
	return decoded, nil
}

func whatsappAttachmentName(attachment channel.Attachment, mimeType string) string {
	if name := strings.TrimSpace(attachment.Name); name != "" {
		return name
	}
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "whatsapp-image.png"
	case "image/webp":
		return "whatsapp-image.webp"
	default:
		if metadataString(attachment.Metadata, "media_type") == "document" {
			return firstNonEmpty(attachment.Name, metadataString(attachment.Metadata, "file_name"), "whatsapp-document")
		}
		return "whatsapp-image.jpg"
	}
}

func (a *Adapter) stopRuntime(ctx context.Context, configID string) {
	a.mu.Lock()
	entry := a.runtimes[configID]
	delete(a.runtimes, configID)
	a.mu.Unlock()
	a.stopEntry(ctx, entry)
}

func (a *Adapter) stopRuntimeEntry(ctx context.Context, entry *runtimeEntry) {
	if entry == nil {
		return
	}
	a.mu.Lock()
	if a.runtimes[entry.cfg.ID] == entry {
		delete(a.runtimes, entry.cfg.ID)
	}
	a.mu.Unlock()
	a.stopEntry(ctx, entry)
}

func (*Adapter) stopEntry(_ context.Context, entry *runtimeEntry) {
	if entry == nil {
		return
	}
	entry.stoppedOnce.Do(func() {
		if entry.cancel != nil {
			entry.cancel()
		}
		if entry.client != nil {
			if entry.handlerID != 0 {
				entry.client.RemoveEventHandler(entry.handlerID)
			}
			entry.client.Disconnect()
		}
		if entry.container != nil {
			_ = entry.container.Close()
		}
		if entry.conn != nil {
			entry.conn.MarkStopped()
		}
	})
}

func (a *Adapter) setStatus(cfg channel.ChannelConfig, status string, running bool, err error) {
	if strings.TrimSpace(cfg.ID) == "" {
		return
	}
	item := RuntimeStatus{
		ConfigID:  cfg.ID,
		BotID:     cfg.BotID,
		Status:    status,
		Running:   running,
		UpdatedAt: time.Now().UTC(),
	}
	if err != nil {
		item.LastError = err.Error()
	}
	a.mu.Lock()
	a.statuses[cfg.ID] = item
	a.mu.Unlock()
}

func (a *Adapter) Status(configID string) (RuntimeStatus, bool) {
	configID = strings.TrimSpace(configID)
	if configID == "" {
		return RuntimeStatus{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	item, ok := a.statuses[configID]
	return item, ok
}

func (a *Adapter) getTerminalHandler() func(context.Context, channel.ChannelConfig, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.terminalHandler
}

func (a *Adapter) MarkStatus(cfg channel.ChannelConfig, status string, err error) {
	a.setStatus(cfg, status, false, err)
}

func (a *Adapter) CloseRuntime(ctx context.Context, configID string) {
	a.stopRuntime(ctx, configID)
}

func (a *Adapter) Logout(ctx context.Context, cfg channel.ChannelConfig) error {
	a.mu.Lock()
	entry := a.runtimes[cfg.ID]
	if entry != nil {
		delete(a.runtimes, cfg.ID)
	}
	a.mu.Unlock()
	if entry != nil {
		defer a.stopEntry(ctx, entry)
		if entry.client != nil && entry.client.Store != nil && entry.client.Store.ID != nil {
			if entry.client.WaitForConnection(10 * time.Second) {
				return entry.client.Logout(ctx)
			}
			return errors.New("whatsapp client is not connected")
		}
		return nil
	}

	waCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	if strings.TrimSpace(waCfg.StoreID) == "" {
		return nil
	}
	paths := finalStorePaths(a.dataRoot, waCfg.StoreID)
	container, client, err := openClientStore(ctx, paths, waLog.Noop)
	if err != nil {
		return err
	}
	defer func() { _ = container.Close() }()
	if client.Store.ID == nil {
		return nil
	}
	if err := client.Connect(); err != nil {
		client.Disconnect()
		return err
	}
	defer client.Disconnect()
	if !client.WaitForConnection(15 * time.Second) {
		return errors.New("whatsapp client did not connect for logout")
	}
	return client.Logout(ctx)
}
