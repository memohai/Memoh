export const REASONING_EFFORT_DISABLE = 'disable'
// Legacy override value. "adaptive" is no longer an effort tier the UI offers —
// it is a thinking mode handled server-side (see only_adaptive). The constant is
// kept so previously-stored values still render gracefully.
export const REASONING_EFFORT_ADAPTIVE = 'adaptive'

export type ThinkingMode = 'toggle' | 'only_adaptive' | 'none'

// Effort tiers the UI understands, ordered weakest → strongest.
export const KNOWN_EFFORTS = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

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
  if (mode === 'toggle' || mode === 'only_adaptive' || mode === 'none') {
    return mode
  }
  return config?.compatibilities?.includes('reasoning') ? 'toggle' : 'none'
}

// resolveEffortLevels returns the model's supported effort tiers (filtered to
// known ones), falling back to the common low/medium/high subset.
export function resolveEffortLevels(config?: ModelConfigLike | null): string[] {
  const efforts = (config?.reasoning_efforts ?? []).filter((e) =>
    (KNOWN_EFFORTS as readonly string[]).includes(e),
  )
  return efforts.length > 0 ? efforts : ['low', 'medium', 'high']
}

// availableEffortsForMode builds the selectable list for a thinking mode:
//   - none:          nothing
//   - only_adaptive: effort tiers only (thinking is forced on, no off switch)
//   - toggle:        an explicit "off" plus the effort tiers
export function availableEffortsForMode(mode: ThinkingMode, levels: string[]): string[] {
  if (mode === 'none') return []
  if (mode === 'only_adaptive') return [...levels]
  return [REASONING_EFFORT_DISABLE, ...levels]
}
