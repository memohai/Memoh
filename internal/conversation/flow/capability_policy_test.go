package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteAttachmentsByCapability_AllSupported(t *testing.T) {
	modalities := []string{"text", "image", "audio"}
	attachments := []gatewayAttachment{
		{Type: "image", Base64: "abc"},
		{Type: "audio", Path: "/data/voice.wav"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 2)
	assert.Len(t, result.Fallback, 0)
}

func TestRouteAttachmentsByCapability_TextOnly(t *testing.T) {
	modalities := []string{"text"}
	attachments := []gatewayAttachment{
		{Type: "image", Base64: "abc"},
		{Type: "video", Path: "/data/video.mp4"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 0)
	assert.Len(t, result.Fallback, 2)
}

func TestRouteAttachmentsByCapability_Mixed(t *testing.T) {
	modalities := []string{"text", "image"}
	attachments := []gatewayAttachment{
		{Type: "image", Base64: "abc"},
		{Type: "video", Path: "/data/video.mp4"},
		{Type: "audio", Path: "/data/audio.mp3"},
	}
	result := routeAttachmentsByCapability(modalities, attachments)
	assert.Len(t, result.Native, 1)
	assert.Equal(t, "image", result.Native[0].Type)
	assert.Len(t, result.Fallback, 2)
}

func TestRouteAttachmentsByCapability_UnknownType(t *testing.T) {
	modalities := []string{"text", "image"}
	attachments := []gatewayAttachment{
		{Type: "hologram", Path: "/data/holo.dat"},
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
		{Type: "image", Base64: "abc"},
		{Type: "file", Path: "/data/doc.pdf"},
	}
	result := attachmentsToAny(atts)
	assert.Len(t, result, 2)
}
