package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ChunkerMode selects the text chunking strategy.
type ChunkerMode string

const (
	ChunkerModeText     ChunkerMode = "text"
	ChunkerModeMarkdown ChunkerMode = "markdown"
)

const streamFinalFirstChunkTimeout = 3 * time.Second

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
	TextChunkLimit      int           `json:"text_chunk_limit,omitempty"`
	ChunkerMode         ChunkerMode   `json:"chunker_mode,omitempty"`
	Chunker             Chunker       `json:"-"`
	MediaOrder          OutboundOrder `json:"media_order,omitempty"`
	InlineTextWithMedia bool          `json:"inline_text_with_media,omitempty"`
	RetryMax            int           `json:"retry_max,omitempty"`
	RetryBackoffMs      int           `json:"retry_backoff_ms,omitempty"`
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
	return utf8.RuneCountInString(value)
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
		return nil, errors.New("message is required")
	}
	normalized := normalizeOutboundMessage(msg.Message)
	attachments := append([]Attachment(nil), normalized.Attachments...)
	chunker := policy.Chunker
	if normalized.Format == MessageFormatMarkdown {
		chunker = ChunkMarkdownText
	}
	base := normalized
	if shouldInlineTextWithMedia(policy, base, attachments) {
		attachments[0].Caption = strings.TrimSpace(base.Text)
		base.Text = ""
	}
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
			// Message.ID signals an edit operation; only the first chunk carries it
			// so subsequent chunks are delivered as new messages rather than repeated edits.
			var messageID string
			if idx == 0 {
				messageID = base.ID
			}
			item := OutboundMessage{
				Target: msg.Target,
				Message: Message{
					ID:          messageID,
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
		return nil, errors.New("message is required")
	}
	if policy.MediaOrder == OutboundOrderTextFirst {
		return append(textMessages, attachmentMessages...), nil
	}
	return append(attachmentMessages, textMessages...), nil
}

func shouldInlineTextWithMedia(policy OutboundPolicy, msg Message, attachments []Attachment) bool {
	if !policy.InlineTextWithMedia {
		return false
	}
	if strings.TrimSpace(msg.Text) == "" || len(msg.Parts) > 0 || len(attachments) == 0 {
		return false
	}
	if strings.TrimSpace(attachments[0].Caption) != "" {
		return false
	}
	switch attachments[0].Type {
	case AttachmentImage, AttachmentGIF, AttachmentVideo, AttachmentAudio, AttachmentVoice:
		return true
	default:
		return false
	}
}

func normalizeOutboundMessage(msg Message) Message {
	if msg.Format == "" {
		if len(msg.Parts) > 0 {
			msg.Format = MessageFormatRich
		} else if strings.TrimSpace(msg.Text) != "" {
			if ContainsMarkdown(msg.Text) {
				msg.Format = MessageFormatMarkdown
			} else {
				msg.Format = MessageFormatPlain
			}
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
			return errors.New("channel does not support plain text")
		}
	case MessageFormatMarkdown:
		if !caps.Markdown && !caps.RichText {
			return errors.New("channel does not support markdown")
		}
	case MessageFormatRich:
		if !caps.RichText {
			return errors.New("channel does not support rich text")
		}
	}
	if len(msg.Parts) > 0 && !caps.RichText {
		return errors.New("channel does not support rich text")
	}
	if len(msg.Attachments) > 0 && !caps.Attachments {
		return errors.New("channel does not support attachments")
	}
	if len(msg.Attachments) > 0 && requiresMedia(msg.Attachments) && !caps.Media {
		return errors.New("channel does not support media")
	}
	if len(msg.Actions) > 0 && !caps.Buttons {
		return errors.New("channel does not support actions")
	}
	if msg.Thread != nil && !caps.Threads {
		return errors.New("channel does not support threads")
	}
	if msg.Reply != nil && !caps.Reply {
		return errors.New("channel does not support reply")
	}
	if strings.TrimSpace(msg.ID) != "" && !caps.Edit {
		return errors.New("channel does not support edit")
	}
	return nil
}

func (m *Manager) sendWithConfig(ctx context.Context, sender Sender, cfg ChannelConfig, msg OutboundMessage, policy OutboundPolicy) error {
	if sender == nil {
		return fmt.Errorf("unsupported channel type: %s", cfg.ChannelType)
	}
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return errors.New("target is required")
	}
	if msg.Message.IsEmpty() {
		return errors.New("message is required")
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
	prepared, err := PrepareOutboundMessage(ctx, m.attachmentStore, cfg, OutboundMessage{
		Target:  target,
		Message: normalized.Message,
	})
	if err != nil {
		return err
	}
	editor, _ := m.registry.GetMessageEditor(cfg.ChannelType)
	if strings.TrimSpace(normalized.Message.ID) != "" {
		if editor == nil {
			return errors.New("channel does not support edit")
		}
		var lastErr error
		for i := 0; i < policy.RetryMax; i++ {
			err := editor.Update(ctx, cfg, target, strings.TrimSpace(normalized.Message.ID), prepared.Message)
			if err == nil {
				if m.logger != nil {
					m.logger.Debug("edit outbound success",
						slog.String("channel", cfg.ChannelType.String()),
						slog.String("bot_id", cfg.BotID),
						slog.String("target", target),
					)
				}
				return nil
			}
			lastErr = err
			if m.logger != nil {
				m.logger.Warn("edit outbound retry",
					slog.String("channel", cfg.ChannelType.String()),
					slog.Int("attempt", i+1),
					slog.Any("error", err))
			}
			if !sleepWithContext(ctx, time.Duration(i+1)*time.Duration(policy.RetryBackoffMs)*time.Millisecond) {
				return fmt.Errorf("edit outbound cancelled: %w", ctx.Err())
			}
		}
		return fmt.Errorf("edit outbound failed after retries: %w", lastErr)
	}
	var lastErr error
	for i := 0; i < policy.RetryMax; i++ {
		err := sender.Send(ctx, cfg, prepared)
		if err == nil {
			if m.logger != nil {
				m.logger.Debug("send outbound success",
					slog.String("channel", cfg.ChannelType.String()),
					slog.String("bot_id", cfg.BotID),
					slog.String("target", target),
				)
			}
			return nil
		}
		lastErr = err
		if m.logger != nil {
			m.logger.Warn("send outbound retry",
				slog.String("channel", cfg.ChannelType.String()),
				slog.Int("attempt", i+1),
				slog.Any("error", err))
		}
		if !sleepWithContext(ctx, time.Duration(i+1)*time.Duration(policy.RetryBackoffMs)*time.Millisecond) {
			return fmt.Errorf("send outbound cancelled: %w", ctx.Err())
		}
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
		item.ContentHash = strings.TrimSpace(item.ContentHash)
		item.Base64 = strings.TrimSpace(item.Base64)
		item.SourcePlatform = strings.TrimSpace(item.SourcePlatform)
		if item.SourcePlatform == "" && item.PlatformKey != "" {
			item.SourcePlatform = defaultPlatform.String()
		}
		if item.URL == "" && item.PlatformKey == "" && item.ContentHash == "" && item.Base64 == "" {
			return nil, errors.New("attachment reference is required")
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
			return errors.New("stream status is required")
		}
	case StreamEventDelta:
		if !caps.Streaming && !caps.BlockStreaming {
			return errors.New("channel does not support streaming")
		}
	case StreamEventPhaseStart, StreamEventPhaseEnd:
		if !caps.Streaming && !caps.BlockStreaming {
			return errors.New("channel does not support streaming")
		}
	case StreamEventToolCallStart, StreamEventToolCallEnd:
		if !caps.Streaming && !caps.BlockStreaming {
			return errors.New("channel does not support streaming")
		}
		if event.ToolCall == nil {
			return errors.New("stream tool call payload is required")
		}
	case StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return errors.New("stream attachments are required")
		}
		if _, err := normalizeAttachmentRefs(event.Attachments, channelType); err != nil {
			return err
		}
	case StreamEventAgentStart, StreamEventAgentEnd, StreamEventProcessingStarted, StreamEventProcessingCompleted:
		return nil
	case StreamEventProcessingFailed:
		if strings.TrimSpace(event.Error) == "" {
			return errors.New("processing failure error is required")
		}
	case StreamEventFinal:
		if event.Final == nil {
			return errors.New("stream final payload is required")
		}
		if err := validateMessageCapabilities(registry, channelType, event.Final.Message); err != nil {
			return err
		}
		if _, err := normalizeAttachmentRefs(event.Final.Message.Attachments, channelType); err != nil {
			return err
		}
	case StreamEventError:
		if strings.TrimSpace(event.Error) == "" {
			return errors.New("stream error is required")
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
		return errors.New("channel manager not configured")
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
		return nil, errors.New("channel manager not configured")
	}
	if s.streamSender == nil {
		return nil, errors.New("channel stream sender not configured")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("target is required")
	}
	caps, _ := s.manager.registry.GetCapabilities(s.channelType)
	if !caps.Streaming && !caps.BlockStreaming {
		return nil, errors.New("channel does not support streaming")
	}
	stream, err := s.streamSender.OpenStream(ctx, s.config, target, opts)
	if err != nil {
		return nil, err
	}
	return &managerOutboundStream{
		manager:     s.manager,
		config:      s.config,
		stream:      stream,
		channelType: s.channelType,
		policy:      s.manager.resolveOutboundPolicy(s.channelType),
		send: func(ctx context.Context, msg OutboundMessage) error {
			msg.Target = target
			return s.Send(ctx, msg)
		},
		reopen: func(ctx context.Context) (PreparedOutboundStream, error) {
			return s.streamSender.OpenStream(ctx, s.config, target, StreamOptions{
				SourceMessageID: opts.SourceMessageID,
				Metadata:        opts.Metadata,
			})
		},
	}, nil
}

// managerOutboundStream wraps a PreparedOutboundStream and adds text-chunking,
// stream-splitting, and attachment fallback on top of the raw adapter stream.
//
// Push and Close must be called from a single goroutine; this type is not
// safe for concurrent use.
type managerOutboundStream struct {
	manager     *Manager
	config      ChannelConfig
	stream      PreparedOutboundStream
	channelType ChannelType
	policy      OutboundPolicy // cached at open time; immutable after creation
	send        func(ctx context.Context, msg OutboundMessage) error
	reopen      func(ctx context.Context) (PreparedOutboundStream, error)
	deltaRunes  int
	deltaText   strings.Builder
	splitCount  int
}

func (s *managerOutboundStream) Push(ctx context.Context, event StreamEvent) error {
	if s.manager == nil || s.stream == nil {
		return errors.New("stream is not configured")
	}
	if err := validateStreamEvent(s.manager.registry, s.channelType, event); err != nil {
		return err
	}

	if event.Type == StreamEventDelta && event.Delta != "" && event.Phase != StreamPhaseReasoning {
		return s.pushDelta(ctx, event)
	}

	if event.Type == StreamEventFinal && event.Final != nil && s.send != nil {
		if s.splitCount > 0 {
			return s.pushFinalAfterSplit(ctx, event)
		}
		return s.pushFinalWithChunking(ctx, event)
	}
	return s.pushPrepared(ctx, event)
}

// streamSplitSoftRatio controls the soft-limit window. The soft limit is
// hardLimit - hardLimit/streamSplitSoftRatio (75% of hard limit). Between
// soft and hard the manager watches for natural break points to split
// gracefully; if none is found it force-splits at the hard limit.
const streamSplitSoftRatio = 4

// pushDelta forwards a text delta and splits the stream into a new message
// when accumulated text approaches the platform's TextChunkLimit. Between
// the soft and hard limits it looks for natural break points (sentence ends,
// line breaks) so messages don't get cut mid-sentence.
func (s *managerOutboundStream) pushDelta(ctx context.Context, event StreamEvent) error {
	policy := s.policy
	if policy.TextChunkLimit <= 0 || s.reopen == nil {
		s.deltaRunes += runeLen(event.Delta)
		return s.pushPrepared(ctx, event)
	}

	newRunes := runeLen(event.Delta)
	afterRunes := s.deltaRunes + newRunes
	hardLimit := policy.TextChunkLimit
	softLimit := hardLimit - hardLimit/streamSplitSoftRatio

	if afterRunes <= softLimit {
		s.deltaRunes = afterRunes
		s.deltaText.WriteString(event.Delta)
		return s.pushPrepared(ctx, event)
	}

	if afterRunes <= hardLimit {
		s.deltaRunes = afterRunes
		s.deltaText.WriteString(event.Delta)
		if err := s.pushPrepared(ctx, event); err != nil {
			return err
		}
		if isNaturalBreakPoint(s.deltaText.String()) {
			s.deltaRunes = 0
			s.deltaText.Reset()
			return s.splitStream(ctx)
		}
		return nil
	}

	if err := s.splitStream(ctx); err != nil {
		return err
	}
	s.deltaRunes = newRunes
	s.deltaText.Reset()
	s.deltaText.WriteString(event.Delta)
	return s.pushPrepared(ctx, event)
}

// splitStream finalizes the current adapter stream, opens a continuation
// stream, and sends Status(Started) so the adapter creates a new platform
// message before the first delta arrives.
func (s *managerOutboundStream) splitStream(ctx context.Context) error {
	if err := s.pushPrepared(ctx, StreamEvent{
		Type:  StreamEventFinal,
		Final: &StreamFinalizePayload{},
	}); err != nil {
		return err
	}
	if err := s.stream.Close(ctx); err != nil {
		return err
	}

	newStream, err := s.reopen(ctx)
	if err != nil {
		return err
	}
	s.stream = newStream
	s.splitCount++

	return s.pushPrepared(ctx, StreamEvent{
		Type:   StreamEventStatus,
		Status: StreamStatusStarted,
	})
}

const sentenceTerminators = ".。!！?？…⋯;；"

// isNaturalBreakPoint reports whether text ends at a position suitable for
// splitting a message — a line break or sentence-ending punctuation.
func isNaturalBreakPoint(text string) bool {
	if strings.HasSuffix(text, "\n") {
		return true
	}
	trimmed := strings.TrimRightFunc(text, unicode.IsSpace)
	if trimmed == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(trimmed)
	return strings.ContainsRune(sentenceTerminators, last)
}

// pushFinalAfterSplit handles StreamEventFinal when the adapter has already
// sent earlier portions of the response during streaming. It passes an
// empty-text Final so the adapter finalizes its internal buffer, then
// delivers any remaining attachments / actions via the non-streaming path.
func (s *managerOutboundStream) pushFinalAfterSplit(ctx context.Context, event StreamEvent) error {
	bufferFinal := StreamEvent{
		Type:     StreamEventFinal,
		Final:    &StreamFinalizePayload{},
		Metadata: event.Metadata,
	}
	if err := s.pushPrepared(ctx, bufferFinal); err != nil {
		return err
	}

	if event.Final == nil {
		return nil
	}
	msg := event.Final.Message

	if len(msg.Attachments) > 0 {
		if err := s.send(ctx, OutboundMessage{
			Message: Message{
				Attachments: msg.Attachments,
				Thread:      msg.Thread,
				Actions:     msg.Actions,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *managerOutboundStream) pushFinalWithChunking(ctx context.Context, event StreamEvent) error {
	policy := s.policy
	if policy.TextChunkLimit <= 0 {
		if s.manager.logger != nil {
			s.manager.logger.Debug("stream final chunking skipped: non-positive chunk limit",
				slog.String("channel", s.channelType.String()),
				slog.Int("chunk_limit", policy.TextChunkLimit),
			)
		}
		return s.pushPrepared(ctx, event)
	}
	msg := normalizeOutboundMessage(event.Final.Message)
	text := strings.TrimSpace(msg.PlainText())
	textRunes := runeLen(text)
	if s.manager.logger != nil {
		s.manager.logger.Debug("stream final chunking evaluate",
			slog.String("channel", s.channelType.String()),
			slog.Int("chunk_limit", policy.TextChunkLimit),
			slog.Int("text_runes", textRunes),
			slog.Int("attachments", len(msg.Attachments)),
			slog.String("format", string(msg.Format)),
		)
	}
	if text == "" || runeLen(text) <= policy.TextChunkLimit {
		if s.manager.logger != nil {
			s.manager.logger.Debug("stream final chunking skipped: text within limit",
				slog.String("channel", s.channelType.String()),
				slog.Int("text_runes", textRunes),
				slog.Int("chunk_limit", policy.TextChunkLimit),
			)
		}
		return s.pushPrepared(ctx, event)
	}

	chunker := policy.Chunker
	if msg.Format == MessageFormatMarkdown {
		chunker = ChunkMarkdownText
	}
	chunks := chunker(text, policy.TextChunkLimit)
	if len(chunks) <= 1 {
		if s.manager.logger != nil {
			s.manager.logger.Debug("stream final chunking skipped: chunker returned single chunk",
				slog.String("channel", s.channelType.String()),
				slog.Int("chunks", len(chunks)),
			)
		}
		return s.pushPrepared(ctx, event)
	}

	hasAttachments := len(msg.Attachments) > 0
	if s.manager.logger != nil {
		s.manager.logger.Info("stream final chunking applied",
			slog.String("channel", s.channelType.String()),
			slog.Int("chunks", len(chunks)),
			slog.Bool("has_attachments", hasAttachments),
		)
	}

	firstMsg := msg
	firstMsg.Text = chunks[0]
	firstMsg.Parts = nil
	firstMsg.Attachments = nil
	firstMsg.Actions = nil
	firstChunkEvent := StreamEvent{
		Type:     StreamEventFinal,
		Final:    &StreamFinalizePayload{Message: firstMsg},
		Metadata: event.Metadata,
	}
	firstChunkCtx, cancelFirstChunk := context.WithTimeout(ctx, streamFinalFirstChunkTimeout)
	defer cancelFirstChunk()
	if err := s.pushPrepared(firstChunkCtx, firstChunkEvent); err != nil {
		if s.manager.logger != nil {
			s.manager.logger.Warn("stream final first chunk push failed, fallback to direct sends",
				slog.String("channel", s.channelType.String()),
				slog.Duration("timeout", streamFinalFirstChunkTimeout),
				slog.Any("error", err),
			)
		}
		return s.sendChunkedFinal(ctx, msg, chunks, 0, hasAttachments)
	}
	return s.sendChunkedFinal(ctx, msg, chunks, 1, hasAttachments)
}

func (s *managerOutboundStream) pushPrepared(ctx context.Context, event StreamEvent) error {
	if s.manager == nil || s.stream == nil {
		return errors.New("stream is not configured")
	}
	prepared, err := PrepareStreamEvent(ctx, s.manager.attachmentStore, s.config, event)
	if err != nil {
		return err
	}
	return s.stream.Push(ctx, prepared)
}

func (s *managerOutboundStream) sendChunkedFinal(ctx context.Context, msg Message, chunks []string, startIndex int, hasAttachments bool) error {
	if startIndex < 0 {
		startIndex = 0
	}
	for idx := startIndex; idx < len(chunks); idx++ {
		chunk := chunks[idx]
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		isLast := idx == len(chunks)-1
		var actions []Action
		if isLast && !hasAttachments {
			actions = msg.Actions
		}
		if err := s.send(ctx, OutboundMessage{
			Message: Message{
				Format:   msg.Format,
				Text:     chunk,
				Thread:   msg.Thread,
				Reply:    msg.Reply,
				Metadata: msg.Metadata,
				Actions:  actions,
			},
		}); err != nil {
			if s.manager.logger != nil {
				s.manager.logger.Error("stream final overflow chunk send failed",
					slog.String("channel", s.channelType.String()),
					slog.Int("chunk_index", idx+1),
					slog.Int("total_chunks", len(chunks)),
					slog.Any("error", err),
				)
			}
			return err
		}
	}

	if hasAttachments {
		if err := s.send(ctx, OutboundMessage{
			Message: Message{
				Attachments: msg.Attachments,
				Thread:      msg.Thread,
				Reply:       msg.Reply,
				Metadata:    msg.Metadata,
				Actions:     msg.Actions,
			},
		}); err != nil {
			if s.manager.logger != nil {
				s.manager.logger.Error("stream final attachments send failed",
					slog.String("channel", s.channelType.String()),
					slog.Int("attachments", len(msg.Attachments)),
					slog.Any("error", err),
				)
			}
			return err
		}
	}
	if s.manager.logger != nil {
		s.manager.logger.Info("stream final chunking completed",
			slog.String("channel", s.channelType.String()),
			slog.Int("chunks", len(chunks)),
			slog.Bool("has_attachments", hasAttachments),
		)
	}
	return nil
}

func (s *managerOutboundStream) Close(ctx context.Context) error {
	if s.stream == nil {
		return errors.New("stream is not configured")
	}
	return s.stream.Close(ctx)
}

// sleepWithContext waits for d or until ctx is cancelled.
// It returns true if the sleep completed normally, false if ctx was cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
