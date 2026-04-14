package tools

import (
	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/channel"
)

func toolAttachmentFromBundle(bundle attachmentpkg.Bundle) Attachment {
	bundle = bundle.Normalize()
	return Attachment{
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

func toolAttachmentFromChannelAttachment(att channel.Attachment) Attachment {
	return toolAttachmentFromBundle(channel.BundleFromAttachment(att))
}
