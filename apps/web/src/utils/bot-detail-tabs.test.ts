import { describe, expect, it } from 'vitest'
import { filterBotDetailsTabs, type BotDetailsTabRule } from './bot-detail-tabs'

const tabs = [
  { value: 'overview' },
  { value: 'general' },
  { value: 'desktop', containerWorkspaceOnly: true },
  { value: 'container', containerWorkspaceOnly: true },
  { value: 'network', containerWorkspaceOnly: true },
  { value: 'remote-settings', remoteOnly: true },
  { value: 'local-settings', localOnly: true },
] satisfies BotDetailsTabRule[]

describe('bot details tab policy', () => {
  it('limits unmanaged users to overview', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      desktopRuntimeMode: 'local',
      canManageBot: false,
      botWorkspaceBackend: 'container',
    }).map(tab => tab.value)).toEqual(['overview'])
  })

  it('hides container runtime tabs for local workspace bots', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      desktopRuntimeMode: 'local',
      canManageBot: true,
      botWorkspaceBackend: 'local',
    }).map(tab => tab.value)).not.toEqual(expect.arrayContaining(['desktop', 'container', 'network']))
  })

  it('keeps container runtime tabs for container workspace bots', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      desktopRuntimeMode: 'remote',
      canManageBot: true,
      botWorkspaceBackend: 'container',
    }).map(tab => tab.value)).toEqual(expect.arrayContaining(['desktop', 'container', 'network']))
  })

  it('applies remote and local runtime flags', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      desktopRuntimeMode: 'remote',
      canManageBot: true,
      botWorkspaceBackend: 'container',
    }).map(tab => tab.value)).toContain('remote-settings')
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      desktopRuntimeMode: 'remote',
      canManageBot: true,
      botWorkspaceBackend: 'container',
    }).map(tab => tab.value)).not.toContain('local-settings')
  })
})
