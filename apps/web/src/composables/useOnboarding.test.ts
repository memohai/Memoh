// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  updateMe: vi.fn(),
  toastError: vi.fn(),
  user: { onboardingCompleted: false },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@memohai/sdk', () => ({
  putUsersMe: mocks.updateMe,
}))

vi.mock('@felinic/ui', () => ({
  toast: { error: mocks.toastError },
}))

vi.mock('@/store/user', () => ({
  useUserStore: () => mocks.user,
}))

import { resetOnboardingState, useOnboarding } from './useOnboarding'
import {
  commitOnboardingACP,
  commitOnboardingBotResult,
  onboardingRuntimeState,
  resetOnboardingRuntimeState,
} from '@/pages/onboarding/state'

describe('useOnboarding completion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.user.onboardingCompleted = false
    mocks.updateMe.mockResolvedValue({})
    mocks.replace.mockResolvedValue(undefined)
    resetOnboardingState()
    resetOnboardingRuntimeState()
  })

  it('keeps runtime state on navigation failure and clears it after retry', async () => {
    commitOnboardingACP({ agentId: 'codex', setupMode: 'oauth', managed: {} })
    commitOnboardingBotResult({
      botId: 'bot-id',
      settingsApplied: true,
      acpLaunchAgentId: 'codex',
    })

    mocks.replace.mockRejectedValue(new Error('navigation failed'))

    expect(await useOnboarding().complete()).toBe(false)
    expect(onboardingRuntimeState.value.botResult?.botId).toBe('bot-id')
    expect(mocks.toastError).toHaveBeenCalledWith('onboarding.complete.navigationFailed')

    mocks.replace.mockResolvedValue(undefined)
    expect(await useOnboarding().complete()).toBe(true)
    expect(mocks.replace).toHaveBeenLastCalledWith({
      name: 'bot',
      params: { botName: 'bot-id' },
      query: { acp: 'codex' },
    })
    expect(onboardingRuntimeState.value.selection.kind).toBe('none')
  })
})
