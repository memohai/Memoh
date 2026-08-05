// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ONBOARDING_KEYS } from './constants'

describe('onboarding runtime state', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.resetModules()
    sessionStorage.clear()
  })

  it('keeps the active state in memory when session storage cannot persist it', async () => {
    const state = await import('./state')
    vi.spyOn(Storage.prototype, 'setItem').mockImplementationOnce(() => {
      throw new DOMException('blocked', 'SecurityError')
    })

    state.commitOnboardingProvider('provider-id')

    expect(state.onboardingRuntimeState.value.selection).toEqual({
      kind: 'provider',
      providerId: 'provider-id',
    })
  })

  it('replaces Provider and ACP selections atomically and clears stale bot results', async () => {
    const state = await import('./state')
    state.commitOnboardingProvider('provider-id')
    state.commitOnboardingBotResult({
      botId: 'bot-id',
      selectedModelId: 'model-id',
      settingsApplied: true,
    })

    state.commitOnboardingACP({ agentId: 'Codex', setupMode: 'oauth', managed: {} })

    expect(state.onboardingRuntimeState.value).toEqual({
      selection: {
        kind: 'acp',
        selection: { agentId: 'codex', setupMode: 'oauth', managed: {} },
      },
      botResult: undefined,
    })
  })

  it('records a model only when Step 4 reports that settings were applied', async () => {
    const state = await import('./state')
    state.commitOnboardingProvider('provider-id')
    state.beginOnboardingBotCreation()
    state.commitOnboardingBotResult({
      botId: 'bot-id',
      selectedModelId: 'must-not-survive',
      settingsApplied: false,
    })

    expect(state.onboardingRuntimeState.value.botResult).toEqual({
      botId: 'bot-id',
      settingsApplied: false,
    })
  })

  it('can suppress the ACP launch redirect without losing the created bot', async () => {
    const state = await import('./state')
    state.commitOnboardingACP({ agentId: 'codex', setupMode: 'oauth', managed: {} })
    state.commitOnboardingBotResult({
      botId: 'bot-id',
      settingsApplied: true,
      acpLaunchAgentId: 'codex',
    })

    state.disableOnboardingACPLaunch()

    expect(state.onboardingRuntimeState.value.botResult).toEqual({
      botId: 'bot-id',
      settingsApplied: true,
    })
    expect(state.onboardingOAuthResumeBotId()).toBe('')
  })

  it('restores the OAuth phase only for the matching ACP bot', async () => {
    const state = await import('./state')
    state.commitOnboardingACP({ agentId: 'codex', setupMode: 'oauth', managed: {} })
    state.commitOnboardingBotResult({
      botId: 'bot-id',
      settingsApplied: true,
      acpLaunchAgentId: 'codex',
    })

    expect(state.onboardingOAuthResumeBotId()).toBe('bot-id')
  })

  it('restores a valid state and rejects malformed storage', async () => {
    sessionStorage.setItem(ONBOARDING_KEYS.runtimeState, JSON.stringify({
      selection: { kind: 'provider', providerId: 'provider-id' },
      botResult: { botId: 'bot-id', selectedModelId: 'model-id', settingsApplied: true },
    }))
    let state = await import('./state')
    expect(state.onboardingRuntimeState.value.botResult?.selectedModelId).toBe('model-id')

    vi.resetModules()
    sessionStorage.setItem(ONBOARDING_KEYS.runtimeState, '{broken')
    state = await import('./state')
    expect(state.onboardingRuntimeState.value).toEqual({ selection: { kind: 'none' } })
  })
})
