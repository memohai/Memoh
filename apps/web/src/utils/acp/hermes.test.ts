import { describe, expect, it } from 'vitest'
import {
  HERMES_CUSTOM_MODEL_VALUE,
  ensureHermesManagedDefaults,
  hermesAPIKeyPlaceholder,
  hermesDefaultModel,
  hermesModelSelectValue,
  hermesProviderValue,
  isHermesCustomProvider,
  normalizeHermesProvider,
} from './hermes'

describe('hermes acp presets', () => {
  it('normalizes provider aliases to the values accepted by managed Hermes config', () => {
    expect(normalizeHermesProvider('google-ai-studio')).toBe('gemini')
    expect(normalizeHermesProvider('google')).toBe('gemini')
    expect(normalizeHermesProvider('openai')).toBe('openai-api')
    expect(hermesProviderValue('')).toBe('gemini')
    expect(hermesProviderValue('unknown')).toBe('unknown')
  })

  it('uses Gemini 3.5 Flash as the managed default', () => {
    expect(hermesDefaultModel('gemini')).toBe('gemini-3.5-flash')
    expect(hermesModelSelectValue('gemini', '')).toBe('gemini-3.5-flash')
  })

  it('keeps non-preset model ids editable through the custom model option', () => {
    expect(hermesModelSelectValue('openrouter', 'vendor/custom-model')).toBe(HERMES_CUSTOM_MODEL_VALUE)
  })

  it('clears base_url outside the custom provider path', () => {
    const managed = {
      provider: 'google-ai-studio',
      model: '',
      base_url: 'https://api.example.test/v1',
      api_key: '',
    }

    ensureHermesManagedDefaults(managed)

    expect(managed).toMatchObject({
      provider: 'gemini',
      model: 'gemini-3.5-flash',
      base_url: '',
    })
    expect(isHermesCustomProvider(managed.provider)).toBe(false)
  })

  it('keeps custom provider base_url when applying defaults', () => {
    const managed = {
      provider: 'custom',
      model: 'my-model',
      base_url: 'https://llm.example/v1',
      api_key: '',
    }

    ensureHermesManagedDefaults(managed)

    expect(managed).toMatchObject({
      provider: 'custom',
      model: 'my-model',
      base_url: 'https://llm.example/v1',
    })
  })

  it('uses provider-specific API key placeholders', () => {
    expect(hermesAPIKeyPlaceholder('gemini')).toBe('AIza...')
    expect(hermesAPIKeyPlaceholder('openrouter')).toBe('sk-or-v1-...')
    expect(hermesAPIKeyPlaceholder('openai')).toBe('sk-...')
    expect(hermesAPIKeyPlaceholder('unknown')).toBeUndefined()
    expect(hermesAPIKeyPlaceholder('unknown', 'Fallback key')).toBe('Fallback key')
  })
})
