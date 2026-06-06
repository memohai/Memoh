import { describe, expect, it } from 'vitest'
import { onboardingProviderPresets, providerPresets } from '@/constants/provider-presets'
import { suggestProviderName } from './provider-presets'

describe('provider preset helpers', () => {
  it('keeps onboarding presets focused while settings can use the full preset catalog', () => {
    expect(onboardingProviderPresets.map(preset => preset.id)).toEqual([
      'openai',
      'anthropic',
      'openrouter',
      'google',
      'deepseek',
      'moonshot',
      'minimax',
      'xai',
    ])

    expect(providerPresets.some(preset => preset.id === 'ollama')).toBe(true)
    expect(providerPresets.some(preset => preset.id === 'github-copilot')).toBe(true)
  })

  it('keeps registry source metadata separate from provider instances', () => {
    const deepseek = providerPresets.find(preset => preset.id === 'deepseek')

    expect(deepseek?.source).toBe('deepseek.yaml')
  })

  it('suggests a unique provider instance name for repeat preset accounts', () => {
    expect(suggestProviderName('DeepSeek', [
      { name: 'OpenAI' },
      { name: 'DeepSeek' },
      { name: 'DeepSeek 2' },
    ])).toBe('DeepSeek 3')
  })
})
