import { describe, expect, it } from 'vitest'
import { filterBotDetailsTabs, type BotDetailsTabRule } from './bot-detail-tabs'

const tabs = [
  { value: 'overview' },
  { value: 'general' },
  { value: 'desktop', containerWorkspaceOnly: true },
  { value: 'container', containerWorkspaceOnly: true },
  { value: 'network', containerWorkspaceOnly: true },
] satisfies BotDetailsTabRule[]

describe('bot details tab policy', () => {
  it('limits unmanaged users to overview', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      canManageBot: false,
      botWorkspaceBackend: 'container',
    }).map(tab => tab.value)).toEqual(['overview'])
  })

  it('hides container runtime tabs for local workspace bots', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      canManageBot: true,
      botWorkspaceBackend: 'local',
    }).map(tab => tab.value)).not.toEqual(expect.arrayContaining(['desktop', 'container', 'network']))
  })

  it('keeps container runtime tabs for container workspace bots', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      canManageBot: true,
      botWorkspaceBackend: 'container',
    }).map(tab => tab.value)).toEqual(expect.arrayContaining(['desktop', 'container', 'network']))
  })

  it('hides container runtime tabs for remote runtime bots', () => {
    expect(filterBotDetailsTabs(tabs, {
      host: 'desktop',
      canManageBot: true,
      botWorkspaceBackend: 'remote',
    }).map(tab => tab.value)).toEqual(['overview', 'general'])
  })
})
