import { describe, test, expect } from 'bun:test'
import type { ModelMessage } from 'ai'
import {
  parseAttachmentPaths,
  extractAttachmentsFromText,
  stripAttachmentsFromMessages,
  dedupeAttachments,
  AttachmentsStreamExtractor,
} from '../utils/attachments'
import { buildNativeImageParts } from '../agent'
import type { ContainerFileAttachment, GatewayInputAttachment } from '../types/attachment'

// ---------------------------------------------------------------------------
// parseAttachmentPaths
// ---------------------------------------------------------------------------

describe('parseAttachmentPaths', () => {
  test('parses standard list', () => {
    const input = `
- /path/to/file.pdf
- /path/to/video.mp4
    `
    expect(parseAttachmentPaths(input)).toEqual([
      '/path/to/file.pdf',
      '/path/to/video.mp4',
    ])
  })

  test('ignores lines without leading dash', () => {
    const input = `
some random text
- /valid/path.txt
not a path
- /another/path.png
    `
    expect(parseAttachmentPaths(input)).toEqual([
      '/valid/path.txt',
      '/another/path.png',
    ])
  })

  test('returns empty array for empty input', () => {
    expect(parseAttachmentPaths('')).toEqual([])
  })

  test('handles extra whitespace around paths', () => {
    const input = '  -   /spaced/path.txt  '
    expect(parseAttachmentPaths(input)).toEqual(['/spaced/path.txt'])
  })
})

// ---------------------------------------------------------------------------
// extractAttachmentsFromText
// ---------------------------------------------------------------------------

describe('extractAttachmentsFromText', () => {
  test('extracts a single block', () => {
    const text = 'Hello world\n<attachments>\n- /file.pdf\n</attachments>\nGoodbye'
    const { cleanedText, attachments } = extractAttachmentsFromText(text)
    expect(attachments).toEqual([{ type: 'file', path: '/file.pdf' }])
    expect(cleanedText).toBe('Hello world\n\nGoodbye')
  })

  test('extracts multiple blocks', () => {
    const text = [
      'Start',
      '<attachments>',
      '- /a.txt',
      '</attachments>',
      'Middle',
      '<attachments>',
      '- /b.txt',
      '</attachments>',
      'End',
    ].join('\n')
    const { cleanedText, attachments } = extractAttachmentsFromText(text)
    expect(attachments).toHaveLength(2)
    expect(attachments.map(a => a.path)).toEqual(['/a.txt', '/b.txt'])
    expect(cleanedText).toContain('Start')
    expect(cleanedText).toContain('Middle')
    expect(cleanedText).toContain('End')
    expect(cleanedText).not.toContain('<attachments>')
  })

  test('deduplicates paths across blocks', () => {
    const text = [
      '<attachments>',
      '- /dup.txt',
      '</attachments>',
      '<attachments>',
      '- /dup.txt',
      '</attachments>',
    ].join('\n')
    const { attachments } = extractAttachmentsFromText(text)
    expect(attachments).toHaveLength(1)
    expect(attachments[0].path).toBe('/dup.txt')
  })

  test('returns original text when no blocks present', () => {
    const text = 'No attachments here'
    const { cleanedText, attachments } = extractAttachmentsFromText(text)
    expect(cleanedText).toBe('No attachments here')
    expect(attachments).toEqual([])
  })

  test('collapses excessive newlines left by removal', () => {
    const text = 'Line1\n\n\n<attachments>\n- /f.txt\n</attachments>\n\n\nLine2'
    const { cleanedText } = extractAttachmentsFromText(text)
    // Should not have more than two consecutive newlines
    expect(cleanedText).not.toMatch(/\n{3,}/)
  })
})

// ---------------------------------------------------------------------------
// stripAttachmentsFromMessages
// ---------------------------------------------------------------------------

describe('stripAttachmentsFromMessages', () => {
  test('strips from assistant message with string content', () => {
    const messages: ModelMessage[] = [
      { role: 'user', content: 'hi' },
      {
        role: 'assistant',
        content: 'Here you go\n<attachments>\n- /result.pdf\n</attachments>',
      },
    ]
    const { messages: stripped, attachments } = stripAttachmentsFromMessages(messages)
    expect(attachments).toEqual([{ type: 'file', path: '/result.pdf' }])
    const assistantMsg = stripped.find(m => m.role === 'assistant')!
    expect((assistantMsg as { content: string }).content).not.toContain('<attachments>')
  })

  test('strips from assistant message with array content containing TextPart', () => {
    const messages: ModelMessage[] = [
      {
        role: 'assistant',
        content: [
          { type: 'text', text: 'Check this\n<attachments>\n- /img.png\n</attachments>' },
        ],
      },
    ]
    const { messages: stripped, attachments } = stripAttachmentsFromMessages(messages)
    expect(attachments).toEqual([{ type: 'file', path: '/img.png' }])
    const content = (stripped[0] as { content: Array<{ type: string; text?: string }> }).content
    expect(content[0].text).not.toContain('<attachments>')
  })

  test('does not modify user or tool messages', () => {
    const messages: ModelMessage[] = [
      { role: 'user', content: '<attachments>\n- /should-stay.txt\n</attachments>' },
    ]
    const { messages: stripped, attachments } = stripAttachmentsFromMessages(messages)
    expect(attachments).toEqual([])
    expect((stripped[0] as { content: string }).content).toContain('<attachments>')
  })

  test('deduplicates attachments across messages', () => {
    const messages: ModelMessage[] = [
      { role: 'assistant', content: '<attachments>\n- /same.txt\n</attachments>' },
      { role: 'assistant', content: '<attachments>\n- /same.txt\n</attachments>' },
    ]
    const { attachments } = stripAttachmentsFromMessages(messages)
    expect(attachments).toHaveLength(1)
  })
})

// ---------------------------------------------------------------------------
// dedupeAttachments
// ---------------------------------------------------------------------------

describe('dedupeAttachments', () => {
  test('deduplicates file attachments by path', () => {
    const items: ContainerFileAttachment[] = [
      { type: 'file', path: '/a.txt' },
      { type: 'file', path: '/b.txt' },
      { type: 'file', path: '/a.txt' },
    ]
    const result = dedupeAttachments(items)
    expect(result).toHaveLength(2)
  })

  test('deduplicates image attachments by base64 prefix', () => {
    const base64 = 'a'.repeat(100)
    const result = dedupeAttachments([
      { type: 'image', base64 },
      { type: 'image', base64 },
    ])
    expect(result).toHaveLength(1)
  })

  test('keeps different types separate', () => {
    const result = dedupeAttachments([
      { type: 'file', path: '/a.txt' },
      { type: 'image', base64: 'abc' },
    ])
    expect(result).toHaveLength(2)
  })
})

describe('buildNativeImageParts', () => {
  test('keeps inline data url and public url images', () => {
    const attachments: GatewayInputAttachment[] = [
      { type: 'image', transport: 'inline_data_url', payload: 'data:image/png;base64,AAAA' },
      { type: 'image', transport: 'public_url', payload: 'https://example.com/demo.png' },
    ]
    const parts = buildNativeImageParts(attachments)
    expect(parts).toHaveLength(2)
    expect(parts[0].image).toBe('data:image/png;base64,AAAA')
    expect(parts[1].image).toBe('https://example.com/demo.png')
  })

  test('drops tool_file_ref images', () => {
    const attachments: GatewayInputAttachment[] = [
      { type: 'image', transport: 'tool_file_ref', payload: '/data/media/image/demo.png' },
    ]
    const parts = buildNativeImageParts(attachments)
    expect(parts).toEqual([])
  })
})

// ---------------------------------------------------------------------------
// AttachmentsStreamExtractor
// ---------------------------------------------------------------------------

describe('AttachmentsStreamExtractor', () => {
  /** Helper: simulates streaming by feeding one character at a time. */
  const feedCharByChar = (extractor: AttachmentsStreamExtractor, text: string) => {
    let visibleText = ''
    const attachments: ContainerFileAttachment[] = []
    for (const ch of text) {
      const result = extractor.push(ch)
      visibleText += result.visibleText
      attachments.push(...result.attachments)
    }
    const remainder = extractor.flushRemainder()
    visibleText += remainder.visibleText
    attachments.push(...remainder.attachments)
    return { visibleText, attachments }
  }

  /** Helper: simulates streaming by feeding the entire string at once. */
  const feedAtOnce = (extractor: AttachmentsStreamExtractor, text: string) => {
    const result = extractor.push(text)
    const remainder = extractor.flushRemainder()
    return {
      visibleText: result.visibleText + remainder.visibleText,
      attachments: [...result.attachments, ...remainder.attachments],
    }
  }

  test('passes through plain text (char-by-char)', () => {
    const ext = new AttachmentsStreamExtractor()
    const { visibleText, attachments } = feedCharByChar(ext, 'Hello world')
    expect(visibleText).toBe('Hello world')
    expect(attachments).toEqual([])
  })

  test('passes through plain text (all-at-once)', () => {
    const ext = new AttachmentsStreamExtractor()
    const { visibleText, attachments } = feedAtOnce(ext, 'Hello world')
    expect(visibleText).toBe('Hello world')
    expect(attachments).toEqual([])
  })

  test('extracts attachments block (char-by-char)', () => {
    const ext = new AttachmentsStreamExtractor()
    const input = 'Before<attachments>\n- /file.pdf\n</attachments>After'
    const { visibleText, attachments } = feedCharByChar(ext, input)
    expect(visibleText).toBe('BeforeAfter')
    expect(attachments).toEqual([{ type: 'file', path: '/file.pdf' }])
  })

  test('extracts attachments block (all-at-once)', () => {
    const ext = new AttachmentsStreamExtractor()
    const input = 'Before<attachments>\n- /file.pdf\n</attachments>After'
    const { visibleText, attachments } = feedAtOnce(ext, input)
    expect(visibleText).toBe('BeforeAfter')
    expect(attachments).toEqual([{ type: 'file', path: '/file.pdf' }])
  })

  test('extracts multiple paths from one block', () => {
    const ext = new AttachmentsStreamExtractor()
    const input = '<attachments>\n- /a.txt\n- /b.txt\n</attachments>'
    const { attachments } = feedCharByChar(ext, input)
    expect(attachments.map(a => a.path)).toEqual(['/a.txt', '/b.txt'])
  })

  test('handles multiple blocks in one stream', () => {
    const ext = new AttachmentsStreamExtractor()
    const input = 'A<attachments>\n- /x.txt\n</attachments>B<attachments>\n- /y.txt\n</attachments>C'
    const { visibleText, attachments } = feedCharByChar(ext, input)
    expect(visibleText).toBe('ABC')
    expect(attachments.map(a => a.path)).toEqual(['/x.txt', '/y.txt'])
  })

  test('handles chunk boundaries splitting the opening tag', () => {
    const ext = new AttachmentsStreamExtractor()
    let visible = ''
    const attachments: ContainerFileAttachment[] = []

    // Feed the opening tag across two chunks
    let r = ext.push('Hello <attach')
    visible += r.visibleText
    attachments.push(...r.attachments)

    r = ext.push('ments>\n- /split.txt\n</attachments> Done')
    visible += r.visibleText
    attachments.push(...r.attachments)

    const remainder = ext.flushRemainder()
    visible += remainder.visibleText
    attachments.push(...remainder.attachments)

    expect(visible).toBe('Hello  Done')
    expect(attachments).toEqual([{ type: 'file', path: '/split.txt' }])
  })

  test('handles chunk boundaries splitting the closing tag', () => {
    const ext = new AttachmentsStreamExtractor()
    let visible = ''
    const attachments: ContainerFileAttachment[] = []

    let r = ext.push('<attachments>\n- /f.txt\n</attach')
    visible += r.visibleText
    attachments.push(...r.attachments)

    r = ext.push('ments>Tail')
    visible += r.visibleText
    attachments.push(...r.attachments)

    const remainder = ext.flushRemainder()
    visible += remainder.visibleText
    attachments.push(...remainder.attachments)

    expect(visible).toBe('Tail')
    expect(attachments).toEqual([{ type: 'file', path: '/f.txt' }])
  })

  test('flushRemainder returns raw text for unclosed block', () => {
    const ext = new AttachmentsStreamExtractor()
    ext.push('<attachments>\n- /orphan.txt\n')
    const remainder = ext.flushRemainder()
    // Unclosed block should be returned as visible text
    expect(remainder.visibleText).toContain('<attachments>')
    expect(remainder.visibleText).toContain('/orphan.txt')
    expect(remainder.attachments).toEqual([])
  })

  test('text without any angle brackets passes through immediately', () => {
    const ext = new AttachmentsStreamExtractor()
    const r = ext.push('simple text without tags')
    // Most of the text should be emitted (minus a small buffered tail)
    const remainder = ext.flushRemainder()
    const full = r.visibleText + remainder.visibleText
    expect(full).toBe('simple text without tags')
  })
})

