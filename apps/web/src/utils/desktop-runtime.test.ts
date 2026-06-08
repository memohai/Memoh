import { describe, expect, it } from 'vitest'
import { canCreateLocalWorkspace, normalizeDesktopRuntimeMode } from './desktop-runtime'

describe('desktop runtime policy', () => {
  it('normalizes only canonical runtime modes', () => {
    expect(normalizeDesktopRuntimeMode('local')).toBe('local')
    expect(normalizeDesktopRuntimeMode('remote')).toBe('remote')
    expect(normalizeDesktopRuntimeMode('online')).toBeNull()
    expect(normalizeDesktopRuntimeMode(undefined)).toBeNull()
  })

  it('allows local workspace creation in web when the server supports it', () => {
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: true,
      host: 'web',
      desktopRuntimeMode: null,
    })).toBe(true)
  })

  it('allows local workspace creation only for local desktop runtime', () => {
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: true,
      host: 'desktop',
      desktopRuntimeMode: 'local',
    })).toBe(true)
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: true,
      host: 'desktop',
      desktopRuntimeMode: 'remote',
    })).toBe(false)
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: true,
      host: 'desktop',
      desktopRuntimeMode: null,
    })).toBe(false)
  })

  it('does not allow local workspace creation without server support', () => {
    expect(canCreateLocalWorkspace({
      serverLocalWorkspaceEnabled: false,
      host: 'web',
      desktopRuntimeMode: null,
    })).toBe(false)
  })
})
