import { describe, expect, it } from 'vitest'

import { normalizeRuntimeServerUrl } from '../src/server-url'

describe('runtime server URL', () => {
  it('normalizes equivalent HTTP and WebSocket endpoint spellings', () => {
    expect(normalizeRuntimeServerUrl('ws://EXAMPLE.test:80/api/')).toBe('http://example.test/api')
    expect(normalizeRuntimeServerUrl('wss://EXAMPLE.test/api/?token=ignored#fragment')).toBe('https://example.test/api')
  })
})
