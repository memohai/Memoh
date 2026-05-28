export const REASONING_EFFORT_DISABLE = 'disable'
export const REASONING_EFFORT_ADAPTIVE = 'adaptive'

export const EFFORT_LABELS: Record<string, string> = {
  [REASONING_EFFORT_DISABLE]: 'chat.reasoningOff',
  [REASONING_EFFORT_ADAPTIVE]: 'chat.reasoningAdaptive',
  none: 'chat.reasoningNone',
  low: 'chat.reasoningLow',
  medium: 'chat.reasoningMedium',
  high: 'chat.reasoningHigh',
  xhigh: 'chat.reasoningXHigh',
}

export const EFFORT_OPACITY: Record<string, number> = {
  [REASONING_EFFORT_DISABLE]: 0.1,
  [REASONING_EFFORT_ADAPTIVE]: 0.25,
  none: 0.15,
  low: 0.35,
  medium: 0.6,
  high: 0.85,
  xhigh: 1,
}
