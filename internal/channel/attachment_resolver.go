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

const attachmentSniffBytes = 512

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
	if body == nil {
		return nil, nil, "", errors.New("attachment body is required")
	}
	if maxBytes <= 0 {
		_ = body.Close()
		return nil, nil, "", errors.New("max bytes must be greater than 0")
	}

	limited := &io.LimitedReader{R: body, N: maxBytes + 1}
	sniffBytes := attachmentSniffBytes
	if remaining := int(maxBytes + 1); remaining < sniffBytes {
		sniffBytes = remaining
	}
	head := make([]byte, sniffBytes)
	n, err := io.ReadFull(limited, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		_ = body.Close()
		return nil, nil, "", fmt.Errorf("read attachment body: %w", err)
	}
	head = head[:n]
	if int64(len(head)) > maxBytes {
		_ = body.Close()
		return nil, nil, "", fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, maxBytes)
	}
	sniffed := strings.TrimSpace(http.DetectContentType(head))
	stream := &attachmentStreamReadCloser{
		reader: io.MultiReader(
			bytes.NewReader(head),
			&maxBytesReader{
				reader:   limited,
				maxBytes: maxBytes,
				emitted:  int64(len(head)),
			},
		),
		closer: body,
	}
	return head, stream, sniffed, nil
}

type attachmentStreamReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (r *attachmentStreamReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *attachmentStreamReadCloser) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

type maxBytesReader struct {
	reader   io.Reader
	maxBytes int64
	emitted  int64
}

func (r *maxBytesReader) Read(p []byte) (int, error) {
	if r.emitted >= r.maxBytes {
		return r.probeOverflow()
	}

	allowed := int(r.maxBytes - r.emitted)
	if allowed <= 0 {
		return r.probeOverflow()
	}
	if len(p) > allowed {
		p = p[:allowed]
	}

	n, err := r.reader.Read(p)
	r.emitted += int64(n)
	return n, err
}

func (r *maxBytesReader) probeOverflow() (int, error) {
	var probe [1]byte
	n, err := r.reader.Read(probe[:])
	if n > 0 {
		return 0, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, r.maxBytes)
	}
	if err != nil {
		return 0, err
	}
	return 0, io.EOF
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
