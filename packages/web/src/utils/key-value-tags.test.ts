import { describe, expect, it } from 'vitest'
import { tagsToRecord, recordToTags } from './key-value-tags'

describe('tagsToRecord', () => {
  it('converts simple key:value pairs', () => {
    expect(tagsToRecord(['env:production', 'tier:free'])).toEqual({
      env: 'production',
      tier: 'free',
    })
  })

  it('preserves colons inside values (e.g. URLs)', () => {
    expect(tagsToRecord(['webhook:https://example.com/hook'])).toEqual({
      webhook: 'https://example.com/hook',
    })
  })

  it('preserves multiple colons in value', () => {
    expect(tagsToRecord(['ts:2026-03-01T10:00:00Z'])).toEqual({
      ts: '2026-03-01T10:00:00Z',
    })
  })

  it('skips entries without a colon', () => {
    expect(tagsToRecord(['nocoion'])).toEqual({})
  })

  it('skips entries with an empty key (leading colon)', () => {
    expect(tagsToRecord([':value'])).toEqual({})
  })

  it('skips entries with an empty value (trailing colon)', () => {
    expect(tagsToRecord(['key:'])).toEqual({})
  })

  it('returns empty record for empty array', () => {
    expect(tagsToRecord([])).toEqual({})
  })

  it('last writer wins on duplicate keys', () => {
    expect(tagsToRecord(['env:staging', 'env:production'])).toEqual({ env: 'production' })
  })
})

describe('recordToTags', () => {
  it('converts a record back to tags', () => {
    const tags = recordToTags({ env: 'production', tier: 'free' })
    expect(tags).toContain('env:production')
    expect(tags).toContain('tier:free')
    expect(tags).toHaveLength(2)
  })

  it('returns empty array for null', () => {
    expect(recordToTags(null)).toEqual([])
  })

  it('returns empty array for undefined', () => {
    expect(recordToTags(undefined)).toEqual([])
  })

  it('round-trips tags containing colons in values', () => {
    const original = { webhook: 'https://example.com/hook' }
    const tags = recordToTags(original)
    expect(tagsToRecord(tags)).toEqual(original)
  })
})
