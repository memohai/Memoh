import { describe, expect, it } from 'vitest'
import { oauthStatusTextKey } from './status-text'

describe('oauthStatusTextKey', () => {
  it('keeps the text stable while authorization is pending', () => {
    expect(oauthStatusTextKey({
      loading: true,
      authorizing: true,
      status: { configured: true, has_token: false },
      unavailableKey: 'unavailable',
    })).toBe('provider.oauth.status.oauthing')

    expect(oauthStatusTextKey({
      loading: false,
      authorizing: true,
      status: { configured: true, has_token: false },
      unavailableKey: 'unavailable',
    })).toBe('provider.oauth.status.oauthing')
  })

  it('does not hide authorized or unavailable states behind pending text', () => {
    expect(oauthStatusTextKey({
      loading: true,
      authorizing: true,
      status: { configured: true, has_token: true },
      unavailableKey: 'unavailable',
    })).toBe('provider.oauth.status.authorized')

    expect(oauthStatusTextKey({
      loading: true,
      authorizing: true,
      status: { configured: false, has_token: false },
      unavailableKey: 'unavailable',
    })).toBe('unavailable')
  })

  it('keeps the existing idle status order outside authorization', () => {
    expect(oauthStatusTextKey({
      loading: true,
      status: null,
      unavailableKey: 'unavailable',
    })).toBe('provider.oauth.status.checking')

    expect(oauthStatusTextKey({
      loading: false,
      status: { configured: true, has_token: false },
      unavailableKey: 'unavailable',
    })).toBe('provider.oauth.status.missing')
  })
})
