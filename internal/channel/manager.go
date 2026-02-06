package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type ConfigStore interface {
	ResolveEffectiveConfig(ctx context.Context, botID string, channelType ChannelType) (ChannelConfig, error)
	GetUserConfig(ctx context.Context, actorUserID string, channelType ChannelType) (ChannelUserBinding, error)
	UpsertUserConfig(ctx context.Context, actorUserID string, channelType ChannelType, req UpsertUserConfigRequest) (ChannelUserBinding, error)
	ListConfigsByType(ctx context.Context, channelType ChannelType) ([]ChannelConfig, error)
	ResolveUserBinding(ctx context.Context, channelType ChannelType, criteria BindingCriteria) (string, error)
	ListSessionsByBotPlatform(ctx context.Context, botID string, platform string) ([]ChannelSession, error)
	GetChannelSession(ctx context.Context, sessionID string) (ChannelSession, error)
	UpsertChannelSession(ctx context.Context, sessionID string, botID string, channelConfigID string, userID string, contactID string, platform string, replyTarget string, threadID string, metadata map[string]any) error
}

// Middleware 消息处理中间件定义
type Middleware func(next InboundHandler) InboundHandler

type Manager struct {
	service         ConfigStore
	processor       InboundProcessor
	adapters        map[ChannelType]Adapter
	senders         map[ChannelType]Sender
	receivers       map[ChannelType]Receiver
	refreshInterval time.Duration
	logger          *slog.Logger
	middlewares     []Middleware

	inboundQueue   chan inboundTask
	inboundWorkers int
	inboundOnce    sync.Once
	inboundCtx     context.Context
	inboundCancel  context.CancelFunc
	adapterMu      sync.RWMutex
	mu             sync.Mutex
	connections    map[string]*connectionEntry
}

type connectionEntry struct {
	config     ChannelConfig
	connection Connection
}

func NewManager(log *slog.Logger, service ConfigStore, processor InboundProcessor) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		service:         service,
		processor:       processor,
		adapters:        map[ChannelType]Adapter{},
		senders:         map[ChannelType]Sender{},
		receivers:       map[ChannelType]Receiver{},
		refreshInterval: 30 * time.Second,
		connections:     map[string]*connectionEntry{},
		logger:          log.With(slog.String("component", "channel")),
		middlewares:     []Middleware{},
		inboundQueue:    make(chan inboundTask, 256),
		inboundWorkers:  4,
	}
}

// Use 注册中间件
func (m *Manager) Use(mw ...Middleware) {
	m.middlewares = append(m.middlewares, mw...)
}

func (m *Manager) RegisterAdapter(adapter Adapter) {
	if adapter == nil {
		return
	}
	m.adapterMu.Lock()
	m.adapters[adapter.Type()] = adapter
	if sender, ok := adapter.(Sender); ok {
		m.senders[adapter.Type()] = sender
	}
	if receiver, ok := adapter.(Receiver); ok {
		m.receivers[adapter.Type()] = receiver
	}
	m.adapterMu.Unlock()
	if m.logger != nil {
		m.logger.Info("adapter registered", slog.String("channel", adapter.Type().String()))
	}
}

// AddAdapter 注册适配器并触发一次刷新（便于热插拔）。
func (m *Manager) AddAdapter(ctx context.Context, adapter Adapter) {
	m.RegisterAdapter(adapter)
	if ctx != nil {
		m.refresh(ctx)
	}
}

// RemoveAdapter 移除适配器并停止其连接（便于热插拔）。
func (m *Manager) RemoveAdapter(ctx context.Context, channelType ChannelType) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := normalizeChannelType(channelType.String())
	if normalized == "" {
		return
	}
	m.mu.Lock()
	for id, entry := range m.connections {
		if entry != nil && entry.config.ChannelType == normalized {
			if entry.connection != nil {
				_ = entry.connection.Stop(ctx)
			}
			delete(m.connections, id)
		}
	}
	m.mu.Unlock()

	m.adapterMu.Lock()
	delete(m.adapters, normalized)
	delete(m.senders, normalized)
	delete(m.receivers, normalized)
	m.adapterMu.Unlock()
}

func (m *Manager) Start(ctx context.Context) {
	if m.logger != nil {
		m.logger.Info("manager start")
	}
	m.startInboundWorkers(ctx)
	go func() {
		m.refresh(ctx)
		ticker := time.NewTicker(m.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				if m.logger != nil {
					m.logger.Info("manager stop")
				}
				m.stopAll(ctx)
				return
			case <-ticker.C:
				m.refresh(ctx)
			}
		}
	}()
}

func (m *Manager) Send(ctx context.Context, botID string, channelType ChannelType, req SendRequest) error {
	if m.service == nil {
		return fmt.Errorf("channel manager not configured")
	}
	m.adapterMu.RLock()
	sender := m.senders[channelType]
	m.adapterMu.RUnlock()
	if sender == nil {
		return fmt.Errorf("unsupported channel type: %s", channelType)
	}
	config, err := m.service.ResolveEffectiveConfig(ctx, botID, channelType)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		targetUserID := strings.TrimSpace(req.UserID)
		if targetUserID == "" {
			return fmt.Errorf("target or user_id is required")
		}
		userCfg, err := m.service.GetUserConfig(ctx, targetUserID, channelType)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("channel binding missing", slog.String("channel", channelType.String()), slog.String("user_id", targetUserID))
			}
			return fmt.Errorf("channel binding required")
		}
		target, err = ResolveTargetFromUserConfig(channelType, userCfg.Config)
		if err != nil {
			return err
		}
	}
	if normalized, ok := NormalizeTarget(channelType, target); ok {
		target = normalized
	}
	if req.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}
	if m.logger != nil {
		m.logger.Info("send outbound", slog.String("channel", channelType.String()), slog.String("bot_id", botID))
	}
	policy := m.resolveOutboundPolicy(channelType)
	outbound, err := buildOutboundMessages(OutboundMessage{
		Target:  target,
		Message: req.Message,
	}, policy)
	if err != nil {
		return err
	}
	for _, item := range outbound {
		if err := m.sendWithConfig(ctx, sender, config, item, policy); err != nil {
			if m.logger != nil {
				m.logger.Error("send outbound failed", slog.String("channel", channelType.String()), slog.String("bot_id", botID), slog.Any("error", err))
			}
			return err
		}
	}
	return nil
}

func (m *Manager) HandleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error {
	if m.processor == nil {
		return fmt.Errorf("inbound processor not configured")
	}
	m.startInboundWorkers(ctx)
	if m.inboundCtx != nil && m.inboundCtx.Err() != nil {
		return fmt.Errorf("inbound dispatcher stopped")
	}
	taskCtx := ctx
	if ctx != nil {
		taskCtx = context.WithoutCancel(ctx)
	}
	task := inboundTask{
		ctx: taskCtx,
		cfg: cfg,
		msg: msg,
	}
	select {
	case m.inboundQueue <- task:
		return nil
	default:
		return fmt.Errorf("inbound queue full")
	}
}

func (m *Manager) refresh(ctx context.Context) {
	if m.service == nil {
		return
	}
	configs := make([]ChannelConfig, 0)
	channelTypes := m.listAdapterTypes()
	for _, channelType := range channelTypes {
		items, err := m.service.ListConfigsByType(ctx, channelType)
		if err != nil {
			if m.logger != nil {
				m.logger.Error("list configs failed", slog.String("channel", channelType.String()), slog.Any("error", err))
			}
			continue
		}
		configs = append(configs, items...)
	}
	m.reconcile(ctx, configs)
}

func (m *Manager) reconcile(ctx context.Context, configs []ChannelConfig) {
	active := map[string]ChannelConfig{}
	for _, cfg := range configs {
		if cfg.ID == "" {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(cfg.Status))
		if status != "" && status != "active" && status != "verified" {
			continue
		}
		active[cfg.ID] = cfg
		if err := m.ensureConnection(ctx, cfg); err != nil {
			if m.logger != nil {
				m.logger.Error("adapter start failed", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID), slog.Any("error", err))
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if _, ok := active[id]; ok {
			continue
		}
		if entry != nil && entry.connection != nil {
			if m.logger != nil {
				m.logger.Info("adapter stop", slog.String("channel", entry.config.ChannelType.String()), slog.String("config_id", id))
			}
			_ = entry.connection.Stop(ctx)
		}
		delete(m.connections, id)
	}
}

func (m *Manager) ensureConnection(ctx context.Context, cfg ChannelConfig) error {
	m.adapterMu.RLock()
	receiver := m.receivers[cfg.ChannelType]
	m.adapterMu.RUnlock()
	if receiver == nil {
		return nil
	}
	m.mu.Lock()
	entry := m.connections[cfg.ID]
	m.mu.Unlock()

	if entry != nil {
		if entry.config.UpdatedAt.Equal(cfg.UpdatedAt) {
			return nil
		}
		if m.logger != nil {
			m.logger.Info("adapter restart", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID))
		}
		if err := entry.connection.Stop(ctx); err != nil {
			if errors.Is(err, ErrStopNotSupported) {
				if m.logger != nil {
					m.logger.Warn("adapter restart skipped", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID))
				}
				return nil
			}
			return err
		}
		m.mu.Lock()
		delete(m.connections, cfg.ID)
		m.mu.Unlock()
	}
	if m.logger != nil {
		m.logger.Info("adapter start", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID))
	}

	// 包装中间件
	handler := m.handleInbound
	for i := len(m.middlewares) - 1; i >= 0; i-- {
		handler = m.middlewares[i](handler)
	}

	conn, err := receiver.Connect(ctx, cfg, handler)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.connections[cfg.ID] = &connectionEntry{
		config:     cfg,
		connection: conn,
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) stopAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if entry != nil && entry.connection != nil {
			if m.logger != nil {
				m.logger.Info("adapter stop", slog.String("channel", entry.config.ChannelType.String()), slog.String("config_id", id))
			}
			_ = entry.connection.Stop(ctx)
		}
		delete(m.connections, id)
	}
}

func (m *Manager) handleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error {
	if m.processor == nil {
		return fmt.Errorf("inbound processor not configured")
	}
	sender := m.newReplySender(cfg, msg.Channel)
	if err := m.processor.HandleInbound(ctx, cfg, msg, sender); err != nil {
		if m.logger != nil {
			m.logger.Error("inbound processing failed", slog.String("channel", msg.Channel.String()), slog.Any("error", err))
		}
		return err
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context, configID string) error {
	configID = strings.TrimSpace(configID)
	if configID == "" {
		return fmt.Errorf("config id is required")
	}
	m.mu.Lock()
	entry := m.connections[configID]
	m.mu.Unlock()
	if entry == nil || entry.connection == nil {
		return nil
	}
	return entry.connection.Stop(ctx)
}

func (m *Manager) StopByBot(ctx context.Context, botID string) error {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return fmt.Errorf("bot id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if entry != nil && entry.config.BotID == botID {
			if entry.connection != nil {
				_ = entry.connection.Stop(ctx)
			}
			delete(m.connections, id)
		}
	}
	return nil
}

func (m *Manager) Shutdown(ctx context.Context) error {
	if m.inboundCancel != nil {
		m.inboundCancel()
	}
	m.stopAll(ctx)
	return nil
}

func (m *Manager) newReplySender(cfg ChannelConfig, channelType ChannelType) ReplySender {
	m.adapterMu.RLock()
	sender := m.senders[channelType]
	m.adapterMu.RUnlock()
	return &managerReplySender{
		manager:     m,
		sender:      sender,
		channelType: channelType,
		config:      cfg,
	}
}

func (m *Manager) listAdapterTypes() []ChannelType {
	m.adapterMu.RLock()
	defer m.adapterMu.RUnlock()
	items := make([]ChannelType, 0, len(m.adapters))
	for channelType := range m.adapters {
		items = append(items, channelType)
	}
	return items
}

type inboundTask struct {
	ctx context.Context
	cfg ChannelConfig
	msg InboundMessage
}

func (m *Manager) startInboundWorkers(ctx context.Context) {
	m.inboundOnce.Do(func() {
		workerCtx := ctx
		if workerCtx == nil {
			workerCtx = context.Background()
		}
		m.inboundCtx, m.inboundCancel = context.WithCancel(workerCtx)
		for i := 0; i < m.inboundWorkers; i++ {
			go m.runInboundWorker(m.inboundCtx)
		}
	})
}

func (m *Manager) runInboundWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-m.inboundQueue:
			if err := m.handleInbound(task.ctx, task.cfg, task.msg); err != nil {
				if m.logger != nil {
					m.logger.Error("inbound processing failed", slog.String("channel", task.msg.Channel.String()), slog.Any("error", err))
				}
			}
		}
	}
}

func (m *Manager) resolveOutboundPolicy(channelType ChannelType) OutboundPolicy {
	policy, ok := GetChannelOutboundPolicy(channelType)
	if !ok {
		policy = OutboundPolicy{}
	}
	return NormalizeOutboundPolicy(policy)
}

func buildOutboundMessages(msg OutboundMessage, policy OutboundPolicy) ([]OutboundMessage, error) {
	policy = NormalizeOutboundPolicy(policy)
	if msg.Message.IsEmpty() {
		return nil, fmt.Errorf("message is required")
	}
	normalized := normalizeOutboundMessage(msg.Message)
	chunker := policy.Chunker
	if normalized.Format == MessageFormatMarkdown {
		chunker = ChunkMarkdownText
	}
	base := normalized
	base.Attachments = nil
	textMessages := make([]OutboundMessage, 0)
	shouldChunk := policy.TextChunkLimit > 0 && strings.TrimSpace(base.Text) != "" && len(base.Parts) == 0
	if shouldChunk {
		chunks := chunker(base.Text, policy.TextChunkLimit)
		for idx, chunk := range chunks {
			chunk = strings.TrimSpace(chunk)
			if chunk == "" {
				continue
			}
			actions := base.Actions
			if len(chunks) > 1 && idx < len(chunks)-1 {
				actions = nil
			}
			item := OutboundMessage{
				Target: msg.Target,
				Message: Message{
					ID:          base.ID,
					Format:      base.Format,
					Text:        chunk,
					Parts:       base.Parts,
					Attachments: nil,
					Actions:     actions,
					Thread:      base.Thread,
					Reply:       base.Reply,
					Metadata:    base.Metadata,
				},
			}
			textMessages = append(textMessages, item)
		}
	} else if !base.IsEmpty() {
		textMessages = append(textMessages, OutboundMessage{Target: msg.Target, Message: base})
	}

	attachments := normalized.Attachments
	attachmentMessages := make([]OutboundMessage, 0)
	if len(attachments) > 0 {
		media := normalized
		media.Format = ""
		media.Text = ""
		media.Parts = nil
		media.Actions = nil
		media.Attachments = attachments
		attachmentMessages = append(attachmentMessages, OutboundMessage{Target: msg.Target, Message: media})
	}

	if len(textMessages) == 0 && len(attachmentMessages) == 0 {
		return nil, fmt.Errorf("message is required")
	}
	if policy.MediaOrder == OutboundOrderTextFirst {
		return append(textMessages, attachmentMessages...), nil
	}
	return append(attachmentMessages, textMessages...), nil
}

func normalizeOutboundMessage(msg Message) Message {
	if msg.Format == "" {
		if len(msg.Parts) > 0 {
			msg.Format = MessageFormatRich
		} else if strings.TrimSpace(msg.Text) != "" {
			msg.Format = MessageFormatPlain
		}
	}
	return msg
}

func (m *Manager) sendWithConfig(ctx context.Context, sender Sender, cfg ChannelConfig, msg OutboundMessage, policy OutboundPolicy) error {
	if sender == nil {
		return fmt.Errorf("unsupported channel type: %s", cfg.ChannelType)
	}
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return fmt.Errorf("target is required")
	}
	if msg.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}
	if caps, ok := GetChannelCapabilities(cfg.ChannelType); ok {
		if msg.Message.Format == MessageFormatPlain && !caps.Text {
			return fmt.Errorf("channel does not support plain text")
		}
		if msg.Message.Format == MessageFormatMarkdown && !(caps.Markdown || caps.RichText) {
			return fmt.Errorf("channel does not support markdown")
		}
		if msg.Message.Format == MessageFormatRich && !caps.RichText {
			return fmt.Errorf("channel does not support rich text")
		}
		if len(msg.Message.Parts) > 0 && !caps.RichText {
			return fmt.Errorf("channel does not support rich text")
		}
		if len(msg.Message.Attachments) > 0 && !caps.Attachments {
			return fmt.Errorf("channel does not support attachments")
		}
		if len(msg.Message.Attachments) > 0 && requiresMedia(msg.Message.Attachments) && !caps.Media {
			return fmt.Errorf("channel does not support media")
		}
		if len(msg.Message.Actions) > 0 && !caps.Buttons {
			return fmt.Errorf("channel does not support actions")
		}
		if msg.Message.Thread != nil && !caps.Threads {
			return fmt.Errorf("channel does not support threads")
		}
		if msg.Message.Reply != nil && !caps.Reply {
			return fmt.Errorf("channel does not support reply")
		}
	}
	policy = NormalizeOutboundPolicy(policy)
	var lastErr error
	for i := 0; i < policy.RetryMax; i++ {
		err := sender.Send(ctx, cfg, OutboundMessage{Target: target, Message: msg.Message})
		if err == nil {
			return nil
		}
		lastErr = err
		if m.logger != nil {
			m.logger.Warn("send outbound retry",
				slog.String("channel", cfg.ChannelType.String()),
				slog.Int("attempt", i+1),
				slog.Any("error", err))
		}
		time.Sleep(time.Duration(i+1) * time.Duration(policy.RetryBackoffMs) * time.Millisecond)
	}
	return fmt.Errorf("send outbound failed after retries: %w", lastErr)
}

func requiresMedia(attachments []Attachment) bool {
	for _, att := range attachments {
		switch att.Type {
		case AttachmentAudio, AttachmentVideo, AttachmentVoice, AttachmentGIF:
			return true
		default:
			continue
		}
	}
	return false
}

type managerReplySender struct {
	manager     *Manager
	sender      Sender
	channelType ChannelType
	config      ChannelConfig
}

func (s *managerReplySender) Send(ctx context.Context, msg OutboundMessage) error {
	if s.manager == nil {
		return fmt.Errorf("channel manager not configured")
	}
	policy := s.manager.resolveOutboundPolicy(s.channelType)
	outbound, err := buildOutboundMessages(msg, policy)
	if err != nil {
		return err
	}
	for _, item := range outbound {
		if err := s.manager.sendWithConfig(ctx, s.sender, s.config, item, policy); err != nil {
			return err
		}
	}
	return nil
}
