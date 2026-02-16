package flow

import "github.com/memohai/memoh/internal/models"

// attachmentModality maps an attachment type string to the input modality it requires.
var attachmentModality = map[string]string{
	"image": models.ModelInputImage,
	"audio": models.ModelInputAudio,
	"video": models.ModelInputVideo,
	"file":  models.ModelInputFile,
}

// gatewayAttachment is the structured attachment payload sent to the agent gateway.
// Only fields consumable by the agent/LLM are serialized; internal references
// (asset_id, platform_key, url) are stripped before dispatch.
type gatewayAttachment struct {
	Type     string         `json:"type"`
	Base64   string         `json:"base64,omitempty"`
	Path     string         `json:"path,omitempty"`
	Mime     string         `json:"mime,omitempty"`
	Name     string         `json:"name,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// capabilityRouteResult holds the outcome of splitting attachments by model capability.
type capabilityRouteResult struct {
	// Native are attachments the model can consume directly as multimodal input.
	Native []gatewayAttachment
	// Fallback are attachments whose modality is unsupported; they are converted
	// to container file path references for the LLM to access via tools.
	Fallback []gatewayAttachment
}

// routeAttachmentsByCapability splits attachments based on the model's supported
// input modalities. Supported modalities produce native multimodal input; unsupported
// modalities produce container path references for tool-based access.
func routeAttachmentsByCapability(modalities []string, attachments []gatewayAttachment) capabilityRouteResult {
	supported := make(map[string]struct{}, len(modalities))
	for _, m := range modalities {
		supported[m] = struct{}{}
	}

	result := capabilityRouteResult{
		Native:   make([]gatewayAttachment, 0, len(attachments)),
		Fallback: make([]gatewayAttachment, 0),
	}
	for _, att := range attachments {
		requiredModality, known := attachmentModality[att.Type]
		if !known {
			// Unknown attachment types always go through fallback path.
			result.Fallback = append(result.Fallback, att)
			continue
		}
		if _, ok := supported[requiredModality]; ok {
			result.Native = append(result.Native, att)
		} else {
			result.Fallback = append(result.Fallback, att)
		}
	}
	return result
}

// attachmentsToAny converts typed gateway attachments to []any for JSON serialization.
func attachmentsToAny(atts []gatewayAttachment) []any {
	out := make([]any, 0, len(atts))
	for _, a := range atts {
		out = append(out, a)
	}
	return out
}
