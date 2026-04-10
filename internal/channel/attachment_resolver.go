package channel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/media"
)

type defaultAttachmentResolver struct{}

type effectiveAttachmentResolver struct {
	platform AttachmentResolver
	fallback AttachmentResolver
}

func newEffectiveAttachmentResolver(platform AttachmentResolver) AttachmentResolver {
	return effectiveAttachmentResolver{
		platform: platform,
		fallback: defaultAttachmentResolver{},
	}
}

func (r effectiveAttachmentResolver) ResolveAttachment(ctx context.Context, cfg ChannelConfig, attachment Attachment) (AttachmentPayload, error) {
	if r.platform != nil {
		result, err := r.platform.ResolveAttachment(ctx, cfg, attachment)
		if !errors.Is(err, ErrAttachmentNotResolvable) {
			return result, err
		}
	}
	if r.fallback != nil {
		return r.fallback.ResolveAttachment(ctx, cfg, attachment)
	}
	return AttachmentPayload{}, ErrAttachmentNotResolvable
}

func (defaultAttachmentResolver) ResolveAttachment(ctx context.Context, _ ChannelConfig, attachment Attachment) (AttachmentPayload, error) {
	if strings.TrimSpace(attachment.Base64) == "" && !isHTTPURL(strings.TrimSpace(attachment.URL)) {
		return AttachmentPayload{}, ErrAttachmentNotResolvable
	}

	rawBase64 := strings.TrimSpace(attachment.Base64)
	if rawBase64 != "" {
		decoded, err := attachmentpkg.DecodeBase64(rawBase64, media.MaxAssetBytes)
		if err != nil {
			return AttachmentPayload{}, fmt.Errorf("decode attachment base64: %w", err)
		}
		mimeType := strings.TrimSpace(attachment.Mime)
		if mimeType == "" {
			mimeType = strings.TrimSpace(attachmentpkg.MimeFromDataURL(rawBase64))
		}
		return AttachmentPayload{
			Reader: io.NopCloser(decoded),
			Mime:   mimeType,
			Name:   strings.TrimSpace(attachment.Name),
		}, nil
	}

	rawURL := strings.TrimSpace(attachment.URL)
	if !isHTTPURL(rawURL) {
		return AttachmentPayload{}, ErrAttachmentNotResolvable
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return AttachmentPayload{}, fmt.Errorf("build request: %w", err)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is an operator/user provided public attachment URL
	if err != nil {
		return AttachmentPayload{}, fmt.Errorf("download attachment: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = resp.Body.Close()
		return AttachmentPayload{}, fmt.Errorf("download attachment status: %d", resp.StatusCode)
	}
	if resp.ContentLength > media.MaxAssetBytes {
		_ = resp.Body.Close()
		return AttachmentPayload{}, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, media.MaxAssetBytes)
	}

	head, body, sniffed, err := readAttachmentBody(resp.Body, media.MaxAssetBytes)
	if err != nil {
		return AttachmentPayload{}, err
	}
	mime := normalizeResponseMime(resp.Header.Get("Content-Type"))
	if mime == "" {
		mime = sniffed
	}
	if looksLikeUnexpectedHTML(attachment, mime, sniffed, head) {
		_ = body.Close()
		return AttachmentPayload{}, errors.New("download attachment returned html instead of binary payload")
	}

	size := attachment.Size
	if size <= 0 && resp.ContentLength > 0 {
		size = resp.ContentLength
	}
	return AttachmentPayload{
		Reader: body,
		Mime:   mime,
		Name:   strings.TrimSpace(attachment.Name),
		Size:   size,
	}, nil
}

func readAttachmentBody(body io.ReadCloser, maxBytes int64) ([]byte, io.ReadCloser, string, error) {
	limited := io.LimitReader(body, maxBytes+1)
	data, err := io.ReadAll(limited)
	_ = body.Close()
	if err != nil {
		return nil, nil, "", fmt.Errorf("read attachment body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, nil, "", fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, maxBytes)
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	sniffed := strings.TrimSpace(http.DetectContentType(head))
	return head, io.NopCloser(bytes.NewReader(data)), sniffed, nil
}

func normalizeResponseMime(raw string) string {
	mime := strings.TrimSpace(raw)
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	return mime
}

func looksLikeUnexpectedHTML(att Attachment, headerMime, sniffedMime string, head []byte) bool {
	if !attachmentExpectsBinary(att) {
		return false
	}
	headerMime = strings.ToLower(strings.TrimSpace(headerMime))
	sniffedMime = strings.ToLower(strings.TrimSpace(sniffedMime))
	if headerMime == "text/html" || sniffedMime == "text/html" {
		return true
	}
	trimmed := strings.TrimSpace(strings.ToLower(string(head)))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html")
}

func attachmentExpectsBinary(att Attachment) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(att.Mime)), "image/") {
		return true
	}
	switch att.Type {
	case AttachmentImage, AttachmentVideo, AttachmentAudio, AttachmentGIF:
		return true
	default:
		return false
	}
}

func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	default:
		return false
	}
}
