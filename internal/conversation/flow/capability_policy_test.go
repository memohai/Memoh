package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteAttachmentsByCapability_AllSupported(t *testing.T) {
	modalities := []string{"text", "image", "audio"}
	attachments := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
		{Type: "audio", Transport: gatewayTransportToolFileRef, Payload: "/data/voice.wav"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 1)
	assert.Len(t, result.Fallback, 1)
	assert.Equal(t, "image", result.Native[0].Type)
	assert.Equal(t, "audio", result.Fallback[0].Type)
}

func TestRouteAttachmentsByCapability_TextOnly(t *testing.T) {
	modalities := []string{"text"}
	attachments := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
		{Type: "video", Transport: gatewayTransportToolFileRef, Payload: "/data/video.mp4"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 0)
	assert.Len(t, result.Fallback, 2)
}

func TestRouteAttachmentsByCapability_Mixed(t *testing.T) {
	modalities := []string{"text", "image"}
	attachments := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
		{Type: "video", Transport: gatewayTransportToolFileRef, Payload: "/data/video.mp4"},
		{Type: "audio", Transport: gatewayTransportToolFileRef, Payload: "/data/audio.mp3"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 1)
	assert.Equal(t, "image", result.Native[0].Type)
	assert.Len(t, result.Fallback, 2)
}

func TestRouteAttachmentsByCapability_ImagePathOnlyFallsBack(t *testing.T) {
	modalities := []string{"text", "image"}
	attachments := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportToolFileRef, Payload: "/data/image.png"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 0)
	assert.Len(t, result.Fallback, 1)
	assert.Equal(t, "image", result.Fallback[0].Type)
}

func TestRouteAttachmentsByCapability_ImageURLIsNative(t *testing.T) {
	modalities := []string{"text", "image"}
	attachments := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportPublicURL, Payload: "https://example.com/image.png"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 1)
	assert.Len(t, result.Fallback, 0)
	assert.Equal(t, "image", result.Native[0].Type)
}

func TestRouteAttachmentsByCapability_UnknownType(t *testing.T) {
	modalities := []string{"text", "image"}
	attachments := []gatewayAttachment{
		{Type: "hologram", Transport: gatewayTransportToolFileRef, Payload: "/data/holo.dat"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 0)
	assert.Len(t, result.Fallback, 1)
}

func TestRouteAttachmentsByCapability_Empty(t *testing.T) {
	result := routeAttachmentsByCapability([]string{"text"}, nil)
	assert.Len(t, result.Native, 0)
	assert.Len(t, result.Fallback, 0)
}

func TestAttachmentsToAny(t *testing.T) {
	atts := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
		{Type: "file", Transport: gatewayTransportToolFileRef, Payload: "/data/doc.pdf"},
	}
	result := attachmentsToAny(atts)
	assert.Len(t, result, 2)
}
