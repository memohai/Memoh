import type { ImagePart } from 'ai'
import type { GatewayInputAttachment } from '../types/attachment'

type NativeImageAttachment = GatewayInputAttachment & {
  type: 'image'
  transport: 'inline_data_url' | 'public_url'
}

type ImagePartPayload = string | Uint8Array | URL

const normalizeMediaType = (value?: string): string | undefined => {
  const mediaType = typeof value === 'string' ? value.trim() : ''
  return mediaType || undefined
}

const createImagePart = (image: ImagePartPayload, mediaType?: string): ImagePart => {
  const normalizedMediaType = normalizeMediaType(mediaType)
  if (normalizedMediaType == null) {
    return { type: 'image', image }
  }
  return { type: 'image', image, mediaType: normalizedMediaType }
}

const parseDataUrl = (payload: string): { bytes: Uint8Array; mediaType?: string } | null => {
  const trimmed = payload.trim()
  if (!trimmed.toLowerCase().startsWith('data:')) {
    return null
  }

  const commaIndex = trimmed.indexOf(',')
  if (commaIndex < 0) {
    return null
  }

  const header = trimmed.slice(5, commaIndex)
  const body = trimmed.slice(commaIndex + 1)
  const segments = header.split(';').map((segment) => segment.trim()).filter(Boolean)
  const mediaType = normalizeMediaType(segments.find((segment) => segment.includes('/')))
  const isBase64 = segments.some((segment) => segment.toLowerCase() === 'base64')
  const buffer = isBase64
    ? Buffer.from(body, 'base64')
    : Buffer.from(decodeURIComponent(body), 'utf8')

  return {
    bytes: new Uint8Array(buffer),
    mediaType,
  }
}

const isNativeImageAttachment = (
  attachment: GatewayInputAttachment,
): attachment is NativeImageAttachment => {
  if (attachment.type !== 'image') {
    return false
  }
  if (attachment.transport !== 'inline_data_url' && attachment.transport !== 'public_url') {
    return false
  }
  return typeof attachment.payload === 'string' && attachment.payload.trim() !== ''
}

const createInlineDataImagePart = (payload: string, mediaType?: string): ImagePart => {
  const parsed = parseDataUrl(payload)
  if (parsed != null) {
    return createImagePart(parsed.bytes, mediaType ?? parsed.mediaType)
  }
  return createImagePart(payload, mediaType)
}

export const createBinaryImagePart = (bytes: Uint8Array, mediaType?: string): ImagePart => {
  return createImagePart(bytes, mediaType)
}

export const createImagePartFromAttachment = (
  attachment: GatewayInputAttachment,
): ImagePart | null => {
  if (!isNativeImageAttachment(attachment)) {
    return null
  }

  const payload = attachment.payload.trim()
  switch (attachment.transport) {
    case 'public_url':
      return createImagePart(new URL(payload), attachment.mime)
    case 'inline_data_url':
      return createInlineDataImagePart(payload, attachment.mime)
  }
}
