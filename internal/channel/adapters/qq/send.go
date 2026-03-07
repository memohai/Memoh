package qq

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/media"
)

const (
	qqMediaTypeImage = 1
	qqMediaTypeVideo = 2
	qqMediaTypeVoice = 3
	qqMediaTypeFile  = 4
)

type qqTargetKind string

const (
	qqTargetC2C     qqTargetKind = "c2c"
	qqTargetGroup   qqTargetKind = "group"
	qqTargetChannel qqTargetKind = "channel"
)

type qqTarget struct {
	Kind qqTargetKind
	ID   string
}

var qqUUIDTargetPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type attachmentUpload struct {
	Base64   string
	FileName string
	Mime     string
}

func (a *QQAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	resolvedTarget, err := a.resolveTarget(ctx, msg.Target)
	if err != nil {
		return err
	}
	target, err := parseTarget(resolvedTarget)
	if err != nil {
		return err
	}
	client := a.getOrCreateClient(cfg, parsed)
	replyTo := ""
	if msg.Message.Reply != nil {
		replyTo = strings.TrimSpace(msg.Message.Reply.MessageID)
	}

	text := strings.TrimSpace(msg.Message.PlainText())
	if text != "" {
		useMarkdown := parsed.MarkdownSupport && msg.Message.Format == channel.MessageFormatMarkdown && target.Kind != qqTargetChannel
		if err := client.sendText(ctx, target, text, replyTo, useMarkdown); err != nil {
			return err
		}
	}

	for _, att := range msg.Message.Attachments {
		if err := a.sendAttachment(ctx, cfg, client, target, replyTo, att); err != nil {
			return err
		}
	}
	return nil
}

func parseTarget(raw string) (qqTarget, error) {
	normalized := normalizeTarget(raw)
	switch {
	case strings.HasPrefix(normalized, "c2c:"):
		id := strings.TrimSpace(strings.TrimPrefix(normalized, "c2c:"))
		if id == "" {
			return qqTarget{}, errors.New("qq target c2c id is required")
		}
		if err := validateQQC2CTarget(id); err != nil {
			return qqTarget{}, err
		}
		return qqTarget{Kind: qqTargetC2C, ID: id}, nil
	case strings.HasPrefix(normalized, "group:"):
		id := strings.TrimSpace(strings.TrimPrefix(normalized, "group:"))
		if id == "" {
			return qqTarget{}, errors.New("qq target group id is required")
		}
		return qqTarget{Kind: qqTargetGroup, ID: id}, nil
	case strings.HasPrefix(normalized, "channel:"):
		id := strings.TrimSpace(strings.TrimPrefix(normalized, "channel:"))
		if id == "" {
			return qqTarget{}, errors.New("qq target channel id is required")
		}
		return qqTarget{Kind: qqTargetChannel, ID: id}, nil
	default:
		return qqTarget{}, errors.New("unsupported qq target")
	}
}

func validateQQC2CTarget(id string) error {
	if qqUUIDTargetPattern.MatchString(strings.TrimSpace(id)) {
		return errors.New("qq c2c target must be user_openid, not an internal UUID; use c2c:<user_openid>")
	}
	return nil
}

func (a *QQAdapter) sendAttachment(ctx context.Context, cfg channel.ChannelConfig, client *qqClient, target qqTarget, replyTo string, att channel.Attachment) error {
	if target.Kind == qqTargetChannel {
		switch att.Type {
		case channel.AttachmentImage, channel.AttachmentGIF:
			return errors.New("qq channel does not support image attachments")
		case channel.AttachmentVideo:
			return errors.New("qq channel does not support video attachments")
		case channel.AttachmentVoice, channel.AttachmentAudio:
			return errors.New("qq channel does not support voice attachments")
		case channel.AttachmentFile, "":
			return errors.New("qq channel does not support file attachments")
		default:
			return fmt.Errorf("unsupported qq attachment type: %s", att.Type)
		}
	}

	upload, err := a.prepareAttachmentUpload(ctx, cfg.BotID, att)
	if err != nil {
		return err
	}

	switch att.Type {
	case channel.AttachmentImage, channel.AttachmentGIF:
		fileInfo, err := client.uploadMedia(ctx, target, qqMediaTypeImage, upload.Base64, "")
		if err != nil {
			return err
		}
		return client.sendMedia(ctx, target, fileInfo, replyTo, att.Caption)
	case channel.AttachmentVideo:
		fileInfo, err := client.uploadMedia(ctx, target, qqMediaTypeVideo, upload.Base64, "")
		if err != nil {
			return err
		}
		return client.sendMedia(ctx, target, fileInfo, replyTo, att.Caption)
	case channel.AttachmentVoice, channel.AttachmentAudio:
		if !supportsQQVoiceUpload(att, upload.FileName, upload.Mime) {
			return errors.New("qq voice attachments require SILK/WAV/MP3/AMR input")
		}
		fileInfo, err := client.uploadMedia(ctx, target, qqMediaTypeVoice, upload.Base64, "")
		if err != nil {
			return err
		}
		return client.sendMedia(ctx, target, fileInfo, replyTo, att.Caption)
	case channel.AttachmentFile, "":
		fileInfo, err := client.uploadMedia(ctx, target, qqMediaTypeFile, upload.Base64, upload.FileName)
		if err != nil {
			return err
		}
		return client.sendMedia(ctx, target, fileInfo, replyTo, att.Caption)
	default:
		return fmt.Errorf("unsupported qq attachment type: %s", att.Type)
	}
}

func (a *QQAdapter) prepareAttachmentUpload(ctx context.Context, fallbackBotID string, att channel.Attachment) (attachmentUpload, error) {
	if remoteURL := strings.TrimSpace(att.URL); strings.HasPrefix(strings.ToLower(remoteURL), "http://") || strings.HasPrefix(strings.ToLower(remoteURL), "https://") {
		return a.prepareRemoteAttachmentUpload(ctx, att, remoteURL)
	}

	if rawBase64 := extractRawBase64(att); rawBase64 != "" {
		return attachmentUpload{
			Base64:   rawBase64,
			FileName: deriveAttachmentName(att),
			Mime:     strings.TrimSpace(att.Mime),
		}, nil
	}

	contentHash := strings.TrimSpace(att.ContentHash)
	if contentHash == "" || a.assets == nil {
		return attachmentUpload{}, errors.New("qq attachment requires http(s) URL, base64, or content_hash")
	}

	botID := strings.TrimSpace(fallbackBotID)
	if att.Metadata != nil {
		if override, ok := att.Metadata["bot_id"].(string); ok && strings.TrimSpace(override) != "" {
			botID = strings.TrimSpace(override)
		}
	}
	if botID == "" {
		return attachmentUpload{}, errors.New("qq attachment content_hash requires bot_id context")
	}

	reader, asset, err := a.assets.Open(ctx, botID, contentHash)
	if err != nil {
		return attachmentUpload{}, err
	}
	defer func() { _ = reader.Close() }()

	data, err := media.ReadAllWithLimit(reader, media.MaxAssetBytes)
	if err != nil {
		return attachmentUpload{}, err
	}

	fileName := deriveAttachmentName(att)
	if fileName == "" {
		fileName = deriveFileNameFromMime(asset.Mime, att.Type)
	}
	return attachmentUpload{
		Base64:   base64.StdEncoding.EncodeToString(data),
		FileName: fileName,
		Mime:     strings.TrimSpace(asset.Mime),
	}, nil
}

func (a *QQAdapter) prepareRemoteAttachmentUpload(ctx context.Context, att channel.Attachment, remoteURL string) (attachmentUpload, error) {
	u, err := url.Parse(remoteURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return attachmentUpload{}, fmt.Errorf("invalid attachment url: %s", remoteURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return attachmentUpload{}, err
	}
	resp, err := a.httpClient.Do(req) //nolint:gosec // remote URL is validated to http(s) with non-empty host above
	if err != nil {
		return attachmentUpload{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return attachmentUpload{}, fmt.Errorf("qq attachment fetch failed: status=%d", resp.StatusCode)
	}

	data, err := media.ReadAllWithLimit(resp.Body, media.MaxAssetBytes)
	if err != nil {
		return attachmentUpload{}, err
	}

	mimeType := strings.TrimSpace(att.Mime)
	if mimeType == "" {
		mimeType = strings.TrimSpace(resp.Header.Get("Content-Type"))
		if idx := strings.Index(mimeType, ";"); idx >= 0 {
			mimeType = strings.TrimSpace(mimeType[:idx])
		}
	}

	fileName := deriveAttachmentName(att)
	if fileName == "" {
		fileName = deriveFileNameFromMime(mimeType, att.Type)
	}

	return attachmentUpload{
		Base64:   base64.StdEncoding.EncodeToString(data),
		FileName: fileName,
		Mime:     mimeType,
	}, nil
}

func extractRawBase64(att channel.Attachment) string {
	if candidate := strings.TrimSpace(att.Base64); candidate != "" {
		if strings.HasPrefix(strings.ToLower(candidate), "data:") {
			if idx := strings.Index(candidate, ","); idx >= 0 && idx < len(candidate)-1 {
				return candidate[idx+1:]
			}
			return ""
		}
		return candidate
	}

	candidate := strings.TrimSpace(att.URL)
	if strings.HasPrefix(strings.ToLower(candidate), "data:") {
		if idx := strings.Index(candidate, ","); idx >= 0 && idx < len(candidate)-1 {
			return candidate[idx+1:]
		}
	}
	return ""
}

func deriveAttachmentName(att channel.Attachment) string {
	if name := strings.TrimSpace(att.Name); name != "" {
		return name
	}
	if rawURL := strings.TrimSpace(att.URL); rawURL != "" && !strings.HasPrefix(strings.ToLower(rawURL), "data:") {
		if base := filepath.Base(rawURL); base != "." && base != "/" && base != "" {
			return base
		}
	}
	return deriveFileNameFromMime(att.Mime, att.Type)
}

func deriveFileNameFromMime(mimeType string, attType channel.AttachmentType) string {
	ext := mimeExtension(mimeType)
	base := "attachment"
	switch attType {
	case channel.AttachmentImage, channel.AttachmentGIF:
		base = "image"
	case channel.AttachmentVideo:
		base = "video"
	case channel.AttachmentVoice, channel.AttachmentAudio:
		base = "audio"
	case channel.AttachmentFile:
		base = "file"
	}
	return base + ext
}

func mimeExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/amr":
		return ".amr"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

func supportsQQVoiceUpload(att channel.Attachment, fileName string, resolvedMime string) bool {
	check := strings.ToLower(strings.TrimSpace(fileName))
	if check == "" {
		check = strings.ToLower(strings.TrimSpace(att.Name))
	}
	for _, ext := range []string{".silk", ".slk", ".amr", ".wav", ".mp3"} {
		if strings.HasSuffix(check, ext) {
			return true
		}
	}
	mimeType := strings.ToLower(strings.TrimSpace(resolvedMime))
	if mimeType == "" {
		mimeType = strings.ToLower(strings.TrimSpace(att.Mime))
	}
	switch mimeType {
	case "audio/silk", "audio/amr", "audio/wav", "audio/x-wav", "audio/mpeg", "audio/mp3":
		return true
	default:
		return false
	}
}
