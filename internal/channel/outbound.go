package channel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ChunkerMode selects the text chunking strategy.
type ChunkerMode string

const (
	ChunkerModeText     ChunkerMode = "text"
	ChunkerModeMarkdown ChunkerMode = "markdown"
)

// OutboundOrder controls the delivery order of text and media messages.
type OutboundOrder string

const (
	OutboundOrderMediaFirst OutboundOrder = "media_first"
	OutboundOrderTextFirst  OutboundOrder = "text_first"
)

// Chunker splits text into pieces that respect a character limit.
type Chunker func(text string, limit int) []string

// OutboundPolicy configures how outbound messages are chunked, ordered, and retried.
type OutboundPolicy struct {
	TextChunkLimit int           `json:"text_chunk_limit,omitempty"`
	ChunkerMode    ChunkerMode   `json:"chunker_mode,omitempty"`
	Chunker        Chunker       `json:"-"`
	MediaOrder     OutboundOrder `json:"media_order,omitempty"`
	RetryMax       int           `json:"retry_max,omitempty"`
	RetryBackoffMs int           `json:"retry_backoff_ms,omitempty"`
}

// NormalizeOutboundPolicy fills zero-value fields with sensible defaults.
func NormalizeOutboundPolicy(policy OutboundPolicy) OutboundPolicy {
	if policy.TextChunkLimit <= 0 {
		policy.TextChunkLimit = 2000
	}
	if policy.MediaOrder == "" {
		policy.MediaOrder = OutboundOrderMediaFirst
	}
	if policy.ChunkerMode == "" {
		policy.ChunkerMode = ChunkerModeText
	}
	if policy.RetryMax <= 0 {
		policy.RetryMax = 3
	}
	if policy.RetryBackoffMs <= 0 {
		policy.RetryBackoffMs = 500
	}
	if policy.Chunker == nil {
		policy.Chunker = DefaultChunker(policy.ChunkerMode)
	}
	return policy
}

// DefaultChunker returns the built-in Chunker for the given mode.
func DefaultChunker(mode ChunkerMode) Chunker {
	switch mode {
	case ChunkerModeMarkdown:
		return ChunkMarkdownText
	default:
		return ChunkText
	}
}

// ChunkText splits text at newline boundaries, respecting the rune limit.
func ChunkText(text string, limit int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if limit <= 0 || runeLen(trimmed) <= limit {
		return []string{trimmed}
	}
	lines := strings.Split(trimmed, "\n")
	chunks := make([]string, 0)
	buf := make([]string, 0, len(lines))
	bufLen := 0
	for _, line := range lines {
		lineLen := runeLen(line)
		sepLen := 0
		if len(buf) > 0 {
			sepLen = 1
		}
		if bufLen+sepLen+lineLen <= limit {
			buf = append(buf, line)
			bufLen += sepLen + lineLen
			continue
		}
		if len(buf) > 0 {
			chunks = append(chunks, strings.Join(buf, "\n"))
			buf = buf[:0]
			bufLen = 0
		}
		if lineLen <= limit {
			buf = append(buf, line)
			bufLen = lineLen
			continue
		}
		chunks = append(chunks, splitLongLine(line, limit)...)
	}
	if len(buf) > 0 {
		chunks = append(chunks, strings.Join(buf, "\n"))
	}
	return chunks
}

// ChunkMarkdownText splits text at paragraph boundaries (double newlines), respecting the rune limit.
func ChunkMarkdownText(text string, limit int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if limit <= 0 || runeLen(trimmed) <= limit {
		return []string{trimmed}
	}
	paragraphs := strings.Split(trimmed, "\n\n")
	chunks := make([]string, 0)
	buf := make([]string, 0, len(paragraphs))
	bufLen := 0
	for _, para := range paragraphs {
		paraLen := runeLen(para)
		sepLen := 0
		if len(buf) > 0 {
			sepLen = 2
		}
		if bufLen+sepLen+paraLen <= limit {
			buf = append(buf, para)
			bufLen += sepLen + paraLen
			continue
		}
		if len(buf) > 0 {
			chunks = append(chunks, strings.Join(buf, "\n\n"))
			buf = buf[:0]
			bufLen = 0
		}
		if paraLen <= limit {
			buf = append(buf, para)
			bufLen = paraLen
			continue
		}
		chunks = append(chunks, ChunkText(para, limit)...)
	}
	if len(buf) > 0 {
		chunks = append(chunks, strings.Join(buf, "\n\n"))
	}
	return chunks
}

func runeLen(value string) int {
	return len([]rune(value))
}

func splitLongLine(line string, limit int) []string {
	if limit <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	chunks := make([]string, 0)
	for start := 0; start < len(runes); start += limit {
		end := start + limit
		if end > len(runes) {
			end = len(runes)
		}
		segment := strings.TrimSpace(string(runes[start:end]))
		if segment == "" {
			continue
		}
		chunks = append(chunks, segment)
	}
	return chunks
}

// --- Outbound pipeline methods (used by Manager) ---

func (m *Manager) resolveOutboundPolicy(channelType ChannelType) OutboundPolicy {
	policy, ok := m.registry.GetOutboundPolicy(channelType)
	if !ok {
		policy = OutboundPolicy{}
	}
	return NormalizeOutboundPolicy(policy)
}

// buildOutboundMessages splits an outbound message into multiple messages based on the policy.
func buildOutboundMessages(msg OutboundMessage, policy OutboundPolicy) ([]OutboundMessage, error) {
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

func validateMessageCapabilities(registry *Registry, channelType ChannelType, msg Message) error {
	caps, ok := registry.GetCapabilities(channelType)
	if !ok {
		return nil
	}
	switch msg.Format {
	case MessageFormatPlain:
		if !caps.Text {
			return fmt.Errorf("channel does not support plain text")
		}
	case MessageFormatMarkdown:
		if !caps.Markdown && !caps.RichText {
			return fmt.Errorf("channel does not support markdown")
		}
	case MessageFormatRich:
		if !caps.RichText {
			return fmt.Errorf("channel does not support rich text")
		}
	}
	if len(msg.Parts) > 0 && !caps.RichText {
		return fmt.Errorf("channel does not support rich text")
	}
	if len(msg.Attachments) > 0 && !caps.Attachments {
		return fmt.Errorf("channel does not support attachments")
	}
	if len(msg.Attachments) > 0 && requiresMedia(msg.Attachments) && !caps.Media {
		return fmt.Errorf("channel does not support media")
	}
	if len(msg.Actions) > 0 && !caps.Buttons {
		return fmt.Errorf("channel does not support actions")
	}
	if msg.Thread != nil && !caps.Threads {
		return fmt.Errorf("channel does not support threads")
	}
	if msg.Reply != nil && !caps.Reply {
		return fmt.Errorf("channel does not support reply")
	}
	if strings.TrimSpace(msg.ID) != "" && !caps.Edit {
		return fmt.Errorf("channel does not support edit")
	}
	return nil
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
	normalized := msg
	attachments, err := normalizeAttachmentRefs(msg.Message.Attachments, cfg.ChannelType)
	if err != nil {
		return err
	}
	normalized.Message.Attachments = attachments
	if err := validateMessageCapabilities(m.registry, cfg.ChannelType, normalized.Message); err != nil {
		return err
	}
	editor, _ := m.registry.GetMessageEditor(cfg.ChannelType)
	if strings.TrimSpace(normalized.Message.ID) != "" {
		if editor == nil {
			return fmt.Errorf("channel does not support edit")
		}
		var lastErr error
		for i := 0; i < policy.RetryMax; i++ {
			err := editor.Update(ctx, cfg, target, strings.TrimSpace(normalized.Message.ID), normalized.Message)
			if err == nil {
				return nil
			}
			lastErr = err
			if m.logger != nil {
				m.logger.Warn("edit outbound retry",
					slog.String("channel", cfg.ChannelType.String()),
					slog.Int("attempt", i+1),
					slog.Any("error", err))
			}
			time.Sleep(time.Duration(i+1) * time.Duration(policy.RetryBackoffMs) * time.Millisecond)
		}
		return fmt.Errorf("edit outbound failed after retries: %w", lastErr)
	}
	var lastErr error
	for i := 0; i < policy.RetryMax; i++ {
		err := sender.Send(ctx, cfg, OutboundMessage{Target: target, Message: normalized.Message})
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

func normalizeAttachmentRefs(attachments []Attachment, defaultPlatform ChannelType) ([]Attachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	normalized := make([]Attachment, 0, len(attachments))
	for _, att := range attachments {
		item := att
		item.URL = strings.TrimSpace(item.URL)
		item.PlatformKey = strings.TrimSpace(item.PlatformKey)
		item.AssetID = strings.TrimSpace(item.AssetID)
		item.SourcePlatform = strings.TrimSpace(item.SourcePlatform)
		if item.SourcePlatform == "" && item.PlatformKey != "" {
			item.SourcePlatform = defaultPlatform.String()
		}
		if item.URL == "" && item.PlatformKey == "" && item.AssetID == "" {
			return nil, fmt.Errorf("attachment reference is required")
		}
		// asset_id-only attachments require media resolution before dispatch.
		// Adapters expect url or platform_key; fail loudly if neither is available.
		if item.URL == "" && item.PlatformKey == "" && item.AssetID != "" {
			return nil, fmt.Errorf("attachment %s has asset_id but no sendable url or platform_key; media resolution required before dispatch", item.AssetID)
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
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

func validateStreamEvent(registry *Registry, channelType ChannelType, event StreamEvent) error {
	caps, _ := registry.GetCapabilities(channelType)
	switch event.Type {
	case StreamEventStatus:
		if event.Status == "" {
			return fmt.Errorf("stream status is required")
		}
	case StreamEventDelta:
		if !caps.Streaming && !caps.BlockStreaming {
			return fmt.Errorf("channel does not support streaming")
		}
	case StreamEventPhaseStart, StreamEventPhaseEnd:
		if !caps.Streaming && !caps.BlockStreaming {
			return fmt.Errorf("channel does not support streaming")
		}
	case StreamEventToolCallStart, StreamEventToolCallEnd:
		if !caps.Streaming && !caps.BlockStreaming {
			return fmt.Errorf("channel does not support streaming")
		}
		if event.ToolCall == nil {
			return fmt.Errorf("stream tool call payload is required")
		}
	case StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return fmt.Errorf("stream attachments are required")
		}
		if _, err := normalizeAttachmentRefs(event.Attachments, channelType); err != nil {
			return err
		}
	case StreamEventAgentStart, StreamEventAgentEnd, StreamEventProcessingStarted, StreamEventProcessingCompleted:
		return nil
	case StreamEventProcessingFailed:
		if strings.TrimSpace(event.Error) == "" {
			return fmt.Errorf("processing failure error is required")
		}
	case StreamEventFinal:
		if event.Final == nil {
			return fmt.Errorf("stream final payload is required")
		}
		if err := validateMessageCapabilities(registry, channelType, event.Final.Message); err != nil {
			return err
		}
		if _, err := normalizeAttachmentRefs(event.Final.Message.Attachments, channelType); err != nil {
			return err
		}
	case StreamEventError:
		if strings.TrimSpace(event.Error) == "" {
			return fmt.Errorf("stream error is required")
		}
	default:
		return fmt.Errorf("unsupported stream event type: %s", event.Type)
	}
	return nil
}

func (m *Manager) newReplySender(cfg ChannelConfig, channelType ChannelType) StreamReplySender {
	sender, _ := m.registry.GetSender(channelType)
	streamSender, _ := m.registry.GetStreamSender(channelType)
	return &managerReplySender{
		manager:      m,
		sender:       sender,
		streamSender: streamSender,
		channelType:  channelType,
		config:       cfg,
	}
}

type managerReplySender struct {
	manager      *Manager
	sender       Sender
	streamSender StreamSender
	channelType  ChannelType
	config       ChannelConfig
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

func (s *managerReplySender) OpenStream(ctx context.Context, target string, opts StreamOptions) (OutboundStream, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("channel manager not configured")
	}
	if s.streamSender == nil {
		return nil, fmt.Errorf("channel stream sender not configured")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}
	caps, _ := s.manager.registry.GetCapabilities(s.channelType)
	if !caps.Streaming && !caps.BlockStreaming {
		return nil, fmt.Errorf("channel does not support streaming")
	}
	stream, err := s.streamSender.OpenStream(ctx, s.config, target, opts)
	if err != nil {
		return nil, err
	}
	return &managerOutboundStream{
		manager:     s.manager,
		stream:      stream,
		channelType: s.channelType,
	}, nil
}

type managerOutboundStream struct {
	manager     *Manager
	stream      OutboundStream
	channelType ChannelType
}

func (s *managerOutboundStream) Push(ctx context.Context, event StreamEvent) error {
	if s.manager == nil || s.stream == nil {
		return fmt.Errorf("stream is not configured")
	}
	if err := validateStreamEvent(s.manager.registry, s.channelType, event); err != nil {
		return err
	}
	return s.stream.Push(ctx, event)
}

func (s *managerOutboundStream) Close(ctx context.Context) error {
	if s.stream == nil {
		return fmt.Errorf("stream is not configured")
	}
	return s.stream.Close(ctx)
}
