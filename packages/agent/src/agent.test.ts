import { describe, expect, it } from 'vitest'
import { createImagePartFromAttachment, createBinaryImagePart, sanitizeMessagesForJson } from './utils/image-parts'
import type { ModelMessage } from 'ai'

describe('createImagePartFromAttachment', () => {
  it('keeps inline data URLs as strings for JSON safety', () => {
    const payload = 'data:image/png;base64,AQID'
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'inline_data_url',
      payload,
    })

    expect(part?.type).toBe('image')
    expect(typeof part?.image).toBe('string')
    expect(part?.image).toBe(payload)
    expect(part?.mediaType).toBe('image/png')
  })

  it('keeps public URLs as URL objects', () => {
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'public_url',
      payload: 'https://example.com/demo.png',
    })

    expect(part?.image).toBeInstanceOf(URL)
    expect(String(part?.image)).toBe('https://example.com/demo.png')
  })

  it('falls back to string payloads for malformed public URLs', () => {
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'public_url',
      payload: 'https://',
      mime: 'image/png',
    })

    expect(part?.image).toBe('https://')
    expect(part?.mediaType).toBe('image/png')
  })

  it('keeps inline payload strings when they are not data URLs', () => {
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'inline_data_url',
      payload: 'AQID',
      mime: 'image/png',
    })

    expect(part?.image).toBe('AQID')
    expect(part?.mediaType).toBe('image/png')
  })

  it('falls back to string payloads for malformed non-base64 data URLs', () => {
    const payload = 'data:image/png,a%ZZ'
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'inline_data_url',
      payload,
      mime: 'image/png',
    })

    expect(part?.image).toBe(payload)
    expect(part?.mediaType).toBe('image/png')
  })

  it('falls back to string payloads for malformed base64 data URLs', () => {
    const payload = 'data:image/png;base64,%%%'
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'inline_data_url',
      payload,
      mime: 'image/png',
    })

    expect(part?.image).toBe(payload)
    expect(part?.mediaType).toBe('image/png')
  })

  it('skips tool file references', () => {
    const part = createImagePartFromAttachment({
      type: 'image',
      transport: 'tool_file_ref',
      payload: '/data/media/demo.png',
    })

    expect(part).toBeNull()
  })
})

describe('createBinaryImagePart', () => {
  it('converts Uint8Array to base64 data URL string', () => {
    const bytes = new Uint8Array([1, 2, 3])
    const part = createBinaryImagePart(bytes, 'image/png')

    expect(part.type).toBe('image')
    expect(typeof part.image).toBe('string')
    expect(part.image).toBe('data:image/png;base64,AQID')
    expect(part.mediaType).toBe('image/png')
  })

  it('uses fallback MIME when none provided', () => {
    const bytes = new Uint8Array([0xff, 0xd8])
    const part = createBinaryImagePart(bytes)

    expect(typeof part.image).toBe('string')
    expect((part.image as string).startsWith('data:application/octet-stream;base64,')).toBe(true)
  })
})

describe('sanitizeMessagesForJson', () => {
  it('converts Uint8Array image data to base64 data URL', () => {
    const messages: ModelMessage[] = [
      {
        role: 'user',
        content: [
          { type: 'text', text: 'hello' },
          { type: 'image', image: new Uint8Array([1, 2, 3]) },
        ],
      },
    ]
    const sanitized = sanitizeMessagesForJson(messages)
    const content = (sanitized[0] as { content: { type: string; image?: unknown }[] }).content
    expect(content[0]).toEqual({ type: 'text', text: 'hello' })
    expect(typeof content[1].image).toBe('string')
    expect((content[1].image as string).startsWith('data:')).toBe(true)
  })

  it('converts URL objects to href strings', () => {
    const messages: ModelMessage[] = [
      {
        role: 'user',
        content: [
          { type: 'image', image: new URL('https://example.com/img.png') },
        ],
      },
    ]
    const sanitized = sanitizeMessagesForJson(messages)
    const content = (sanitized[0] as { content: { type: string; image?: unknown }[] }).content
    expect(content[0].image).toBe('https://example.com/img.png')
  })

  it('leaves string image data untouched', () => {
    const messages: ModelMessage[] = [
      {
        role: 'user',
        content: [
          { type: 'image', image: 'data:image/png;base64,AQID', mediaType: 'image/png' },
        ],
      },
    ]
    const sanitized = sanitizeMessagesForJson(messages)
    expect(sanitized[0]).toBe(messages[0])
  })

  it('passes through non-array content messages unchanged', () => {
    const messages: ModelMessage[] = [
      { role: 'assistant', content: [{ type: 'text', text: 'hi' }] },
    ]
    const sanitized = sanitizeMessagesForJson(messages)
    expect(sanitized[0]).toBe(messages[0])
  })
})
