export const HERMES_DEFAULT_PROVIDER = 'gemini'
export const HERMES_CUSTOM_MODEL_VALUE = '__memoh_custom_model__'

export interface HermesProviderPreset {
  value: string
  labelKey: string
  defaultModel: string
}

export interface HermesModelPreset {
  value: string
  label: string
}

export const HERMES_PROVIDER_PRESETS: HermesProviderPreset[] = [
  {
    value: 'gemini',
    labelKey: 'bots.settings.acpHermesProviderGemini',
    defaultModel: 'gemini-3.5-flash',
  },
  {
    value: 'openrouter',
    labelKey: 'bots.settings.acpHermesProviderOpenRouter',
    defaultModel: '~anthropic/claude-sonnet-latest',
  },
  {
    value: 'openai-api',
    labelKey: 'bots.settings.acpHermesProviderOpenAI',
    defaultModel: 'gpt-5.5',
  },
  {
    value: 'custom',
    labelKey: 'bots.settings.acpHermesProviderCustom',
    defaultModel: '',
  },
]

const HERMES_PROVIDER_ALIASES: Record<string, string> = {
  google: 'gemini',
  'google-gemini': 'gemini',
  'google-ai-studio': 'gemini',
  openai: 'openai-api',
}

const HERMES_MODEL_PRESETS: Record<string, HermesModelPreset[]> = {
  gemini: [
    { value: 'gemini-3.5-flash', label: 'Gemini 3.5 Flash' },
    { value: 'gemini-3-flash-preview', label: 'Gemini 3 Flash Preview' },
  ],
  openrouter: [
    { value: '~anthropic/claude-sonnet-latest', label: 'Claude Sonnet latest' },
    { value: '~google/gemini-flash-latest', label: 'Gemini Flash latest' },
    { value: 'openrouter/auto', label: 'OpenRouter Auto' },
    { value: 'openrouter/pareto-code', label: 'Pareto Code Router' },
  ],
  'openai-api': [
    { value: 'gpt-5.5', label: 'GPT-5.5' },
  ],
  custom: [],
}

export function normalizeHermesProvider(value: unknown): string {
  const raw = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return HERMES_PROVIDER_ALIASES[raw] ?? raw
}

export function isHermesProviderPreset(value: unknown): boolean {
  const provider = normalizeHermesProvider(value)
  return HERMES_PROVIDER_PRESETS.some(preset => preset.value === provider)
}

export function hermesProviderValue(value: unknown): string {
  const provider = normalizeHermesProvider(value)
  if (!provider) return HERMES_DEFAULT_PROVIDER
  return provider
}

export function isHermesCustomProvider(value: unknown): boolean {
  return hermesProviderValue(value) === 'custom'
}

export function hermesModelPresets(provider: unknown): HermesModelPreset[] {
  return HERMES_MODEL_PRESETS[hermesProviderValue(provider)] ?? []
}

export function hermesDefaultModel(provider: unknown): string {
  return HERMES_PROVIDER_PRESETS.find(preset => preset.value === hermesProviderValue(provider))?.defaultModel ?? ''
}

export function isHermesPresetModel(provider: unknown, model: unknown): boolean {
  const value = typeof model === 'string' ? model.trim() : ''
  if (!value) return false
  return hermesModelPresets(provider).some(preset => preset.value === value)
}

export function hermesModelSelectValue(provider: unknown, model: unknown): string {
  const value = typeof model === 'string' ? model.trim() : ''
  if (value) {
    return isHermesPresetModel(provider, value) ? value : HERMES_CUSTOM_MODEL_VALUE
  }
  return hermesDefaultModel(provider) || HERMES_CUSTOM_MODEL_VALUE
}

export function ensureHermesManagedDefaults(managed: Record<string, string>): void {
  const provider = hermesProviderValue(managed.provider)
  managed.provider = provider
  if (!String(managed.model ?? '').trim()) {
    managed.model = hermesDefaultModel(provider)
  }
  if (isHermesProviderPreset(provider) && !isHermesCustomProvider(provider)) {
    managed.base_url = ''
  }
}

export function hermesAPIKeyPlaceholder(provider: unknown, fallback?: string): string | undefined {
  const value = hermesProviderValue(provider)
  if (value === 'gemini') return 'AIza...'
  if (value === 'openrouter') return 'sk-or-v1-...'
  if (value === 'openai-api') return 'sk-...'
  return fallback
}
