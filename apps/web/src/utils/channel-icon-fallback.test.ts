import { describe, expect, it } from 'vitest'
import { channelIconFallbackText } from './channel-icon-fallback'

describe('channelIconFallbackText', () => {
  it('returns empty text for empty channel keys', () => {
    expect(channelIconFallbackText('')).toBe('')
    expect(channelIconFallbackText('   ')).toBe('')
  })

  it('uses explicit built-in fallbacks for non-brand channels', () => {
    expect(channelIconFallbackText('local')).toBe('LC')
    expect(channelIconFallbackText('cli')).toBe('CLI')
    expect(channelIconFallbackText('web')).toBe('Web')
  })

  it('normalizes casing and whitespace', () => {
    expect(channelIconFallbackText('  Discord  ')).toBe('DI')
  })

  it('creates initials for multi-part unknown channels', () => {
    expect(channelIconFallbackText('custom-bridge')).toBe('CB')
    expect(channelIconFallbackText('foo_bar')).toBe('FB')
  })

  it('uses the first two alphanumeric characters for simple unknown channels', () => {
    expect(channelIconFallbackText('misskey')).toBe('MI')
    expect(channelIconFallbackText('qq')).toBe('QQ')
  })
})
