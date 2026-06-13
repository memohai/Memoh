export const REASONING_EFFORT_DISABLE = 'disable'
// Legacy override value. "adaptive" is no longer an effort tier the UI offers —
// it is a thinking mode handled server-side. The constant is
// kept so previously-stored values still render gracefully.
export const REASONING_EFFORT_ADAPTIVE = 'adaptive'

export type ThinkingMode = 'toggle' | 'adaptive' | 'only_adaptive' | 'none'

// Effort tiers the UI understands, ordered weakest → strongest.
export const KNOWN_EFFORTS = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

// Keep in sync with isOpenAIReasoningWire in internal/conversation/flow/resolver.go
// and the Twilight SDK's openai provider package.
const OPENAI_FORMAT_CLIENT_TYPES = new Set(['openai-completions', 'openai-responses', 'openai-codex'])

export const EFFORT_LABELS: Record<string, string> = {
  [REASONING_EFFORT_DISABLE]: 'chat.reasoningOff',
  [REASONING_EFFORT_ADAPTIVE]: 'chat.reasoningAdaptive',
  none: 'chat.reasoningNone',
  minimal: 'chat.reasoningMinimal',
  low: 'chat.reasoningLow',
  medium: 'chat.reasoningMedium',
  high: 'chat.reasoningHigh',
  xhigh: 'chat.reasoningXHigh',
  max: 'chat.reasoningMax',
}

export const EFFORT_OPACITY: Record<string, number> = {
  [REASONING_EFFORT_DISABLE]: 0.1,
  [REASONING_EFFORT_ADAPTIVE]: 0.25,
  none: 0.15,
  minimal: 0.25,
  low: 0.4,
  medium: 0.6,
  high: 0.8,
  xhigh: 0.92,
  max: 1,
}

interface ModelConfigLike {
  thinking_mode?: string
  reasoning_efforts?: string[]
  compatibilities?: string[]
}

// resolveThinkingMode derives the effective thinking mode from a model config,
// with a legacy fallback for models imported before thinking_mode existed:
// the old "reasoning" compatibility maps to toggle, its absence to none.
export function resolveThinkingMode(config?: ModelConfigLike | null): ThinkingMode {
  const mode = config?.thinking_mode
  if (mode === 'toggle' || mode === 'adaptive' || mode === 'none') {
    return mode
  }
  if (mode === 'only_adaptive') return 'adaptive'
  return config?.compatibilities?.includes('reasoning') ? 'toggle' : 'none'
}

// resolveEffortLevels returns the model's supported effort tiers (filtered to
// known ones), falling back to the common low/medium/high subset. OpenAI-format
// clients use xhigh as the highest effort tier, even when the underlying model
// exposes an Anthropic-style max tier.
export function resolveEffortLevels(config?: ModelConfigLike | null, clientType?: string | null): string[] {
  const efforts = (config?.reasoning_efforts ?? []).filter((e) =>
    (KNOWN_EFFORTS as readonly string[]).includes(e),
  )
  const levels = efforts.length > 0 ? efforts : ['low', 'medium', 'high']
  if (OPENAI_FORMAT_CLIENT_TYPES.has(clientType ?? '')) {
    return levels.filter((e) => e !== 'max')
  }
  return levels
}

// availableEffortsForMode builds the selectable list for a thinking mode:
//   - none:          nothing
//   - adaptive/toggle: an explicit "off" plus the effort tiers
export function availableEffortsForMode(mode: ThinkingMode, levels: string[]): string[] {
  if (mode === 'none') return []
  return [REASONING_EFFORT_DISABLE, ...levels]
}
