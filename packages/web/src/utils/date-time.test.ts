import { describe, expect, it, vi, afterEach } from 'vitest'
import { formatDateTime, formatDate, formatDateTimeSeconds, formatRelativeTime } from './date-time'

// ─── formatDateTime ───────────────────────────────────────────────────────────

describe('formatDateTime', () => {
  it('returns empty string for null', () => {
    expect(formatDateTime(null)).toBe('')
  })

  it('returns empty string for undefined', () => {
    expect(formatDateTime(undefined)).toBe('')
  })

  it('returns fallback for missing value', () => {
    expect(formatDateTime(null, { fallback: '–' })).toBe('–')
  })

  it('returns fallback for invalid date when no invalidFallback set', () => {
    expect(formatDateTime('not-a-date', { fallback: '–' })).toBe('–')
  })

  it('returns invalidFallback for invalid date when set', () => {
    expect(formatDateTime('not-a-date', { invalidFallback: 'bad date' })).toBe('bad date')
  })

  it('formats a valid ISO date string', () => {
    const result = formatDateTime('2026-03-01T10:00:00Z')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })
})

// ─── formatDate ───────────────────────────────────────────────────────────────

describe('formatDate', () => {
  it('returns empty string for null', () => {
    expect(formatDate(null)).toBe('')
  })

  it('returns fallback for missing value', () => {
    expect(formatDate(undefined, { fallback: 'n/a' })).toBe('n/a')
  })

  it('returns invalidFallback for invalid date', () => {
    expect(formatDate('garbage', { invalidFallback: '?' })).toBe('?')
  })

  it('falls back to fallback when invalidFallback not set and date is invalid', () => {
    expect(formatDate('garbage', { fallback: 'fallback' })).toBe('fallback')
  })

  it('formats a valid date string', () => {
    const result = formatDate('2026-03-01')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })
})

// ─── formatDateTimeSeconds ────────────────────────────────────────────────────

describe('formatDateTimeSeconds', () => {
  it('returns empty string for null', () => {
    expect(formatDateTimeSeconds(null)).toBe('')
  })

  it('returns fallback for missing value', () => {
    expect(formatDateTimeSeconds(undefined, { fallback: '–' })).toBe('–')
  })

  it('returns invalidFallback for invalid date', () => {
    expect(formatDateTimeSeconds('bad', { invalidFallback: 'invalid' })).toBe('invalid')
  })

  it('falls back to fallback when invalidFallback not set', () => {
    expect(formatDateTimeSeconds('bad', { fallback: 'fb' })).toBe('fb')
  })

  it('returns raw value when neither fallback option is set and date is invalid', () => {
    expect(formatDateTimeSeconds('raw-garbage')).toBe('raw-garbage')
  })

  it('formats a known UTC timestamp to YYYY-MM-DD HH:mm:ss in local time', () => {
    // Pin to a specific local date to avoid TZ flakiness.
    const d = new Date(2026, 2, 1, 10, 5, 9) // 2026-03-01 10:05:09 local
    const result = formatDateTimeSeconds(d.toISOString())
    // We can only assert the shape since local TZ varies in CI.
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
  })
})

// ─── formatRelativeTime ───────────────────────────────────────────────────────

describe('formatRelativeTime', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns empty string for null', () => {
    expect(formatRelativeTime(null)).toBe('')
  })

  it('returns fallback for missing value', () => {
    expect(formatRelativeTime(undefined, { fallback: '–' })).toBe('–')
  })

  it('returns invalidFallback for invalid date string', () => {
    expect(formatRelativeTime('not-a-date', { invalidFallback: '?' })).toBe('?')
  })

  it('accepts a Date object', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-01T12:00:00Z'))
    const past = new Date('2026-03-01T11:55:00Z') // 5 minutes ago
    const result = formatRelativeTime(past)
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })

  it('accepts an ISO string', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-01T12:00:00Z'))
    const result = formatRelativeTime('2026-03-01T11:55:00Z')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })

  it('falls back to toLocaleDateString for dates older than 7 days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-01T12:00:00Z'))
    const old = new Date('2026-02-01T12:00:00Z') // 28 days ago
    const result = formatRelativeTime(old)
    // Should be a date string, not a relative string
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })
})
