import { describe, expect, it } from 'vitest'

import { validateConfig } from '../src/config'

const key = `mrk_${'a'.repeat(64)}`

describe('runtime config', () => {
  it('rejects unsafe URLs and relative workspace roots', () => {
    expect(() => validateConfig({ serverUrl: 'file:///tmp/socket', key, workspaceBase: '/tmp' })).toThrow('http')
    expect(() => validateConfig({ serverUrl: 'https://user:secret@example.test/api', key, workspaceBase: '/tmp' }))
      .toThrow('credentials')
    expect(() => validateConfig({ serverUrl: 'https://example.test/api', key, workspaceBase: 'relative' }))
      .toThrow('absolute')
  })
})
