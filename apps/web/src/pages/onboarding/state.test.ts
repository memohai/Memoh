// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ONBOARDING_KEYS } from './constants'

describe('onboarding runtime state', () => {
  beforeEach(() => {
    vi.resetModules()
    sessionStorage.clear()
  })

  it('replaces AI selections and never keeps an unsuccessfully saved model', async () => {
    const state = await import('./state')
    state.commitOnboardingProvider('provider-id')
    state.commitOnboardingBotResult({
      botId: 'old-bot',
      selectedModelId: 'old-model',
      settingsApplied: true,
    })

    state.commitOnboardingACP({ agentId: 'Codex', setupMode: 'oauth', managed: {} })
    expect(state.onboardingRuntimeState.value.botResult).toBeUndefined()

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

  it('resumes OAuth until the user explicitly skips it', async () => {
    const state = await import('./state')
    state.commitOnboardingACP({ agentId: 'codex', setupMode: 'oauth', managed: {} })
    state.commitOnboardingBotResult({
      botId: 'bot-id',
      settingsApplied: true,
      acpLaunchAgentId: 'codex',
    })

    expect(state.onboardingOAuthResumeBotId()).toBe('bot-id')
    state.disableOnboardingACPLaunch()
    expect(state.onboardingOAuthResumeBotId()).toBe('')
    expect(state.onboardingRuntimeState.value.botResult?.botId).toBe('bot-id')
  })

  it('restores valid session state and rejects malformed storage', async () => {
    sessionStorage.setItem(ONBOARDING_KEYS.runtimeState, JSON.stringify({
      selection: { kind: 'provider', providerId: 'provider-id' },
      botResult: { botId: 'bot-id', selectedModelId: 'model-id', settingsApplied: true },
    }))
    let state = await import('./state')
    expect(state.onboardingRuntimeState.value.botResult?.selectedModelId).toBe('model-id')

    vi.resetModules()
    sessionStorage.setItem(ONBOARDING_KEYS.runtimeState, '{broken')
    state = await import('./state')
    expect(state.onboardingRuntimeState.value.selection.kind).toBe('none')
  })
})
