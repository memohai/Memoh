package conversation

import attachmentpkg "github.com/memohai/memoh/internal/attachment"

// BundleFromChatAttachment converts a chat request attachment to the shared
// internal bundle shape.
func BundleFromChatAttachment(att ChatAttachment) attachmentpkg.Bundle {
	return attachmentpkg.Bundle{
		Type:        att.Type,
		Base64:      att.Base64,
		Path:        att.Path,
		URL:         att.URL,
		PlatformKey: att.PlatformKey,
		ContentHash: att.ContentHash,
		Name:        att.Name,
		Mime:        att.Mime,
		Size:        att.Size,
		Metadata:    att.Metadata,
	}.Normalize()
}

// ChatAttachmentFromBundle converts the shared internal bundle shape to a chat
// request attachment without changing the public conversation DTO.
// Callers must guarantee bundle is already normalized (produced by BundleFromXxx or Normalize()).
func ChatAttachmentFromBundle(bundle attachmentpkg.Bundle) ChatAttachment {
	return ChatAttachment{
		Type:        bundle.Type,
		Base64:      bundle.Base64,
		Path:        bundle.Path,
		URL:         bundle.URL,
		PlatformKey: bundle.PlatformKey,
		ContentHash: bundle.ContentHash,
		Name:        bundle.Name,
		Mime:        bundle.Mime,
		Size:        bundle.Size,
		Metadata:    bundle.Metadata,
	}
}
