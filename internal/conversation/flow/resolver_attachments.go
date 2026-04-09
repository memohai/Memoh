package flow

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
)

const (
	gatewayInlineAttachmentMaxBytes int64 = 20 * 1024 * 1024
)

// routeAndMergeAttachments applies CapabilityFallbackPolicy to split
// request attachments by model input modalities, then merges the results
// into a single []any for the gateway request.
func (r *Resolver) routeAndMergeAttachments(ctx context.Context, model models.GetResponse, req conversation.ChatRequest) []any {
	if len(req.Attachments) == 0 {
		return []any{}
	}
	typed := r.prepareGatewayAttachments(ctx, req)
	routed := routeAttachmentsByCapability(model.Config.Compatibilities, typed)
	for i := range routed.Fallback {
		fallbackPath := strings.TrimSpace(routed.Fallback[i].FallbackPath)
		if fallbackPath == "" {
			if r != nil && r.logger != nil {
				r.logger.Warn(
					"drop attachment without fallback path",
					slog.String("type", strings.TrimSpace(routed.Fallback[i].Type)),
					slog.String("transport", strings.TrimSpace(routed.Fallback[i].Transport)),
					slog.String("content_hash", strings.TrimSpace(routed.Fallback[i].ContentHash)),
					slog.Bool("has_payload", strings.TrimSpace(routed.Fallback[i].Payload) != ""),
				)
			}
			routed.Fallback[i] = gatewayAttachment{}
			continue
		}
		routed.Fallback[i].Type = "file"
		routed.Fallback[i].Transport = gatewayTransportToolFileRef
		routed.Fallback[i].Payload = fallbackPath
	}
	merged := make([]any, 0, len(routed.Native)+len(routed.Fallback))
	merged = append(merged, attachmentsToAny(routed.Native)...)
	for _, fb := range routed.Fallback {
		if fb.Type == "" || strings.TrimSpace(fb.Transport) == "" || strings.TrimSpace(fb.Payload) == "" {
			continue
		}
		merged = append(merged, fb)
	}
	if len(merged) == 0 {
		return []any{}
	}
	return merged
}

func (r *Resolver) prepareGatewayAttachments(ctx context.Context, req conversation.ChatRequest) []gatewayAttachment {
	if len(req.Attachments) == 0 {
		return nil
	}
	prepared := make([]gatewayAttachment, 0, len(req.Attachments))
	for _, raw := range req.Attachments {
		attachmentType := strings.ToLower(strings.TrimSpace(raw.Type))
		payload := strings.TrimSpace(raw.Base64)
		transport := ""
		fallbackPath := strings.TrimSpace(raw.Path)
		if payload != "" {
			transport = gatewayTransportInlineDataURL
		} else {
			rawURL := strings.TrimSpace(raw.URL)
			contentHash := strings.TrimSpace(raw.ContentHash)
			switch {
			case isDataURL(rawURL):
				payload = rawURL
				transport = gatewayTransportInlineDataURL
			case isLikelyPublicURL(rawURL) && contentHash == "":
				// Only treat a public HTTP URL as direct vision input when the
				// attachment has not been persisted yet. If ContentHash is set,
				// the file is already in the media store and will be inlined
				// by inlineImageAttachmentAssetIfNeeded below — prefer that path
				// so we never expose ephemeral or credentialed platform URLs
				// directly to the model.
				payload = rawURL
				transport = gatewayTransportPublicURL
			case rawURL != "" && fallbackPath == "":
				// URL is either a persisted local path (contentHash set) or an
				// unresolvable reference; store it as fallbackPath so the agent
				// can access it via the file tool if needed.
				fallbackPath = rawURL
			}
		}
		item := gatewayAttachment{
			ContentHash:  strings.TrimSpace(raw.ContentHash),
			Type:         attachmentType,
			Mime:         strings.TrimSpace(raw.Mime),
			Size:         raw.Size,
			Name:         strings.TrimSpace(raw.Name),
			Transport:    transport,
			Payload:      payload,
			Metadata:     raw.Metadata,
			FallbackPath: fallbackPath,
		}
		item = normalizeGatewayAttachmentPayload(item)
		item = r.inlineImageAttachmentAssetIfNeeded(ctx, strings.TrimSpace(req.BotID), item)
		prepared = append(prepared, item)
	}
	return prepared
}

func normalizeGatewayAttachmentPayload(item gatewayAttachment) gatewayAttachment {
	if item.Transport != gatewayTransportInlineDataURL {
		return item
	}
	payload := strings.TrimSpace(item.Payload)
	if payload == "" {
		return item
	}
	if strings.HasPrefix(strings.ToLower(payload), "data:") {
		mime := strings.TrimSpace(item.Mime)
		if mime == "" || strings.EqualFold(mime, "application/octet-stream") {
			if extracted := attachmentpkg.MimeFromDataURL(payload); extracted != "" {
				item.Mime = extracted
			}
		}
		item.Payload = payload
		return item
	}
	mime := strings.TrimSpace(item.Mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	item.Payload = attachmentpkg.NormalizeBase64DataURL(payload, mime)
	return item
}

func isLikelyPublicURL(raw string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

func isDataURL(raw string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(trimmed, "data:")
}

// inlineInjectAttachments converts image attachments from an injected message
// into sdk.ImagePart values for direct vision input. Non-image attachments and
// images that cannot be inlined are silently skipped.
func (r *Resolver) inlineInjectAttachments(ctx context.Context, botID string, atts []conversation.ChatAttachment) []sdk.ImagePart {
	var parts []sdk.ImagePart
	for _, att := range atts {
		if strings.ToLower(strings.TrimSpace(att.Type)) != "image" {
			continue
		}
		contentHash := strings.TrimSpace(att.ContentHash)
		if contentHash == "" {
			continue
		}
		dataURL, mime, err := r.inlineAssetAsDataURL(ctx, botID, contentHash, "image", strings.TrimSpace(att.Mime))
		if err != nil {
			if r != nil && r.logger != nil {
				r.logger.Warn(
					"inline inject image attachment failed",
					slog.Any("error", err),
					slog.String("bot_id", botID),
					slog.String("content_hash", contentHash),
				)
			}
			continue
		}
		parts = append(parts, sdk.ImagePart{
			Image:     dataURL,
			MediaType: mime,
		})
	}
	return parts
}

func (r *Resolver) inlineImageAttachmentAssetIfNeeded(ctx context.Context, botID string, item gatewayAttachment) gatewayAttachment {
	if item.Type != "image" {
		return item
	}
	if strings.TrimSpace(item.Payload) != "" &&
		(item.Transport == gatewayTransportInlineDataURL || item.Transport == gatewayTransportPublicURL) {
		return item
	}
	contentHash := strings.TrimSpace(item.ContentHash)
	if contentHash == "" {
		return item
	}
	dataURL, mime, err := r.inlineAssetAsDataURL(ctx, botID, contentHash, item.Type, item.Mime)
	if err != nil {
		if r != nil && r.logger != nil {
			r.logger.Warn(
				"inline gateway image attachment failed",
				slog.Any("error", err),
				slog.String("bot_id", botID),
				slog.String("content_hash", contentHash),
			)
		}
		return item
	}
	item.Transport = gatewayTransportInlineDataURL
	item.Payload = dataURL
	if strings.TrimSpace(item.Mime) == "" {
		item.Mime = mime
	}
	return item
}

func (r *Resolver) inlineAssetAsDataURL(ctx context.Context, botID, contentHash, attachmentType, fallbackMime string) (string, string, error) {
	if r == nil || r.assetLoader == nil {
		return "", "", errors.New("gateway asset loader not configured")
	}
	reader, assetMime, err := r.assetLoader.OpenForGateway(ctx, botID, contentHash)
	if err != nil {
		return "", "", fmt.Errorf("open asset: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	mime := strings.TrimSpace(fallbackMime)
	if mime == "" {
		mime = strings.TrimSpace(assetMime)
	}
	dataURL, resolvedMime, err := encodeReaderAsDataURL(reader, gatewayInlineAttachmentMaxBytes, attachmentType, mime)
	if err != nil {
		return "", "", err
	}
	return dataURL, resolvedMime, nil
}

func encodeReaderAsDataURL(reader io.Reader, maxBytes int64, attachmentType, fallbackMime string) (string, string, error) {
	if reader == nil {
		return "", "", errors.New("reader is required")
	}
	if maxBytes <= 0 {
		return "", "", errors.New("max bytes must be greater than 0")
	}
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	head := make([]byte, 512)
	n, err := limited.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", fmt.Errorf("read asset: %w", err)
	}
	head = head[:n]

	mime := strings.TrimSpace(fallbackMime)
	if strings.EqualFold(strings.TrimSpace(attachmentType), "image") &&
		(strings.TrimSpace(mime) == "" || strings.EqualFold(strings.TrimSpace(mime), "application/octet-stream")) {
		detected := strings.TrimSpace(http.DetectContentType(head))
		if strings.HasPrefix(strings.ToLower(detected), "image/") {
			mime = detected
		}
	}
	if mime == "" {
		mime = "application/octet-stream"
	}

	var encoded strings.Builder
	encoded.Grow(len("data:") + len(mime) + len(";base64,"))
	encoded.WriteString("data:")
	encoded.WriteString(mime)
	encoded.WriteString(";base64,")

	encoder := base64.NewEncoder(base64.StdEncoding, &encoded)
	if len(head) > 0 {
		if _, err := encoder.Write(head); err != nil {
			_ = encoder.Close()
			return "", "", fmt.Errorf("encode asset head: %w", err)
		}
	}
	copied, err := io.Copy(encoder, limited)
	if err != nil {
		_ = encoder.Close()
		return "", "", fmt.Errorf("encode asset body: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", "", fmt.Errorf("finalize asset encoding: %w", err)
	}

	total := int64(len(head)) + copied
	if total > maxBytes {
		return "", "", fmt.Errorf(
			"asset too large to inline: %d > %d",
			total,
			maxBytes,
		)
	}
	return encoded.String(), mime, nil
}
