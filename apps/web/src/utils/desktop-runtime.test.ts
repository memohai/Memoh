import { describe, expect, it } from 'vitest'
import { canCreateLocalWorkspace } from './desktop-runtime'

describe('desktop runtime policy', () => {
  it('allows local workspace creation in web when the server supports it', () => {
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: true,
      host: 'web',
    })).toBe(true)
  })

  it('does not allow local workspace creation in desktop', () => {
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: true,
      host: 'desktop',
    })).toBe(false)
  })

  it('does not allow local workspace creation without server support', () => {
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: false,
      host: 'web',
    })).toBe(false)
  })
})
