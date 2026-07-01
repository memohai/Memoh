package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteAttachmentsByCapability(t *testing.T) {
	tests := []struct {
		name              string
		compatibilities   []string
		attachments       []gatewayAttachment
		wantNativeTypes   []string
		wantFallbackTypes []string
	}{
		{
			name:            "vision supported routes inline image natively",
			compatibilities: []string{"vision", "tool-call"},
			attachments: []gatewayAttachment{
				{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
				{Type: "audio", Transport: gatewayTransportToolFileRef, Payload: "/data/voice.wav"},
			},
			wantNativeTypes:   []string{"image"},
			wantFallbackTypes: []string{"audio"},
		},
		{
			name:            "no vision routes all to fallback",
			compatibilities: []string{"tool-call"},
			attachments: []gatewayAttachment{
				{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
				{Type: "video", Transport: gatewayTransportToolFileRef, Payload: "/data/video.mp4"},
			},
			wantFallbackTypes: []string{"image", "video"},
		},
		{
			name:            "image path only falls back",
			compatibilities: []string{"vision"},
			attachments: []gatewayAttachment{
				{Type: "image", Transport: gatewayTransportToolFileRef, Payload: "/data/image.png"},
			},
			wantFallbackTypes: []string{"image"},
		},
		{
			name:            "image public url is native",
			compatibilities: []string{"vision"},
			attachments: []gatewayAttachment{
				{Type: "image", Transport: gatewayTransportPublicURL, Payload: "https://example.com/image.png"},
			},
			wantNativeTypes: []string{"image"},
		},
		{
			name:            "unknown type falls back",
			compatibilities: []string{"vision"},
			attachments: []gatewayAttachment{
				{Type: "hologram", Transport: gatewayTransportToolFileRef, Payload: "/data/holo.dat"},
			},
			wantFallbackTypes: []string{"hologram"},
		},
		{
			name:            "empty input",
			compatibilities: []string{"vision"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := routeAttachmentsByCapability(tt.compatibilities, tt.attachments)
			assert.Equal(t, tt.wantNativeTypes, gatewayAttachmentTypes(result.Native))
			assert.Equal(t, tt.wantFallbackTypes, gatewayAttachmentTypes(result.Fallback))
		})
	}
}

func TestAttachmentsToAny(t *testing.T) {
	atts := []gatewayAttachment{
		{Type: "image", Transport: gatewayTransportInlineDataURL, Payload: "data:image/png;base64,abc"},
		{Type: "file", Transport: gatewayTransportToolFileRef, Payload: "/data/doc.pdf"},
	}
	result := attachmentsToAny(atts)
	assert.Len(t, result, 2)
}

func gatewayAttachmentTypes(attachments []gatewayAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	types := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		types = append(types, attachment.Type)
	}
	return types
}
