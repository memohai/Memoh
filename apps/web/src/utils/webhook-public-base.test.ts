import { describe, expect, it } from 'vitest'
import { normalizePublicWebhookBase } from './webhook-public-base'

describe('normalizePublicWebhookBase', () => {
  it('accepts public HTTPS origins and trims trailing slashes', () => {
    expect(normalizePublicWebhookBase('https://example.com/')).toBe('https://example.com')
    expect(normalizePublicWebhookBase('https://example.com./')).toBe('https://example.com')
  })

  it('rejects local, private, and metadata hosts', () => {
    for (const raw of [
      'https://localhost',
      'https://localhost.',
      'https://foo.localhost.',
      'https://app.local',
      'https://app.local.',
      'https://office',
      'https://memoh.internal',
      'https://memoh.internal.',
      'https://foo.test',
      'https://foo.invalid',
      'https://foo.example',
      'https://router.home.arpa',
      'https://127.0.0.1',
      'https://10.0.0.1',
      'https://172.16.0.1',
      'https://192.168.1.1',
      'https://169.254.169.254',
      'https://100.64.0.1',
      'https://0177.0.0.1',
      'https://010.0.0.1',
      'https://001.002.003.004',
    ]) {
      expect(normalizePublicWebhookBase(raw), raw).toBe('')
    }
  })

  it('rejects IPv6 literal hosts', () => {
    for (const raw of [
      'https://[::]',
      'https://[::1]',
      'https://[fc00::1]',
      'https://[fd00::1]',
      'https://[fe80::1]',
      'https://[fe90::1]',
      'https://[febf::1]',
      'https://[2001:db8::1]',
      'https://[2606:4700:4700::1111]',
      'https://[::10.0.0.1]',
      'https://[::ffff:127.0.0.1]',
      'https://[::ffff:192.168.1.1]',
      'https://[::ffff:8.8.8.8]',
    ]) {
      expect(normalizePublicWebhookBase(raw), raw).toBe('')
    }
  })

  it('rejects userinfo, query, and fragment', () => {
    for (const raw of [
      'https://user:pass@example.com',
      'https://example.com?x=1',
      'https://example.com#frag',
    ]) {
      expect(normalizePublicWebhookBase(raw), raw).toBe('')
    }
  })

  it('rejects path and port', () => {
    for (const raw of [
      'https://example.com/base',
      'https://example.com/base/',
      'https://example.com:443',
      'https://example.com:8443',
    ]) {
      expect(normalizePublicWebhookBase(raw), raw).toBe('')
    }
  })
})
