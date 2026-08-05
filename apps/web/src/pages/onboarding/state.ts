import { shallowRef } from 'vue'
import { normalizeACPAgentID } from '@/utils/acp'
import { safeSessionGet, safeSessionRemove, safeSessionSet } from '@/utils/safe-storage'
import { ONBOARDING_KEYS } from './constants'

export interface OnboardingACPSelection {
  agentId: string
  setupMode: string
  managed: Record<string, string>
}

export type OnboardingAISelection =
  | { kind: 'none' }
  | { kind: 'provider', providerId: string }
  | { kind: 'acp', selection: OnboardingACPSelection }

export interface OnboardingBotResult {
  botId: string
  selectedModelId?: string
  settingsApplied: boolean
  acpLaunchAgentId?: string
}

export interface OnboardingRuntimeState {
  selection: OnboardingAISelection
  botResult?: OnboardingBotResult
}

const emptyState = (): OnboardingRuntimeState => ({ selection: { kind: 'none' } })

function parseACPSelection(value: unknown): OnboardingACPSelection | null {
  if (!value || typeof value !== 'object') return null
  const candidate = value as Partial<OnboardingACPSelection>
  const agentId = normalizeACPAgentID(candidate.agentId)
  if (!agentId) return null
  const managed: Record<string, string> = {}
  if (candidate.managed && typeof candidate.managed === 'object') {
    for (const [key, fieldValue] of Object.entries(candidate.managed)) {
      managed[key] = String(fieldValue ?? '')
    }
  }
  return {
    agentId,
    setupMode: typeof candidate.setupMode === 'string' && candidate.setupMode
      ? candidate.setupMode
      : 'api_key',
    managed,
  }
}

function parseState(raw: string | null): OnboardingRuntimeState {
  if (!raw) return emptyState()
  try {
    const candidate = JSON.parse(raw) as Partial<OnboardingRuntimeState>
    let selection: OnboardingAISelection = { kind: 'none' }
    if (candidate.selection?.kind === 'provider') {
      const providerId = String(candidate.selection.providerId ?? '').trim()
      if (providerId) selection = { kind: 'provider', providerId }
    } else if (candidate.selection?.kind === 'acp') {
      const acp = parseACPSelection(candidate.selection.selection)
      if (acp) selection = { kind: 'acp', selection: acp }
    }

    const rawResult = candidate.botResult
    const botId = typeof rawResult?.botId === 'string' ? rawResult.botId.trim() : ''
    const selectedModelId = typeof rawResult?.selectedModelId === 'string'
      ? rawResult.selectedModelId.trim()
      : ''
    const acpLaunchAgentId = normalizeACPAgentID(rawResult?.acpLaunchAgentId)
    return {
      selection,
      ...(botId && {
        botResult: {
          botId,
          settingsApplied: rawResult?.settingsApplied === true,
          ...(selectedModelId && { selectedModelId }),
          ...(acpLaunchAgentId && { acpLaunchAgentId }),
        },
      }),
    }
  } catch {
    return emptyState()
  }
}

export const onboardingRuntimeState = shallowRef(
  parseState(safeSessionGet(ONBOARDING_KEYS.runtimeState)),
)

function commit(state: OnboardingRuntimeState) {
  onboardingRuntimeState.value = state
  safeSessionSet(ONBOARDING_KEYS.runtimeState, JSON.stringify(state))
}

export function commitOnboardingProvider(providerId: string) {
  commit({ selection: { kind: 'provider', providerId }, botResult: undefined })
}

export function commitOnboardingACP(selection: OnboardingACPSelection) {
  const normalized = parseACPSelection(selection)
  if (!normalized) return
  commit({ selection: { kind: 'acp', selection: normalized }, botResult: undefined })
}

export function beginOnboardingBotCreation() {
  commit({ selection: onboardingRuntimeState.value.selection })
}

export function commitOnboardingBotResult(result: OnboardingBotResult) {
  const selectedModelId = result.settingsApplied ? result.selectedModelId?.trim() : ''
  commit({
    selection: onboardingRuntimeState.value.selection,
    botResult: {
      botId: result.botId,
      settingsApplied: result.settingsApplied,
      ...(selectedModelId && { selectedModelId }),
      ...(result.acpLaunchAgentId && { acpLaunchAgentId: result.acpLaunchAgentId }),
    },
  })
}

export function disableOnboardingACPLaunch() {
  const result = onboardingRuntimeState.value.botResult
  if (!result?.acpLaunchAgentId) return
  commit({
    selection: onboardingRuntimeState.value.selection,
    botResult: {
      botId: result.botId,
      settingsApplied: result.settingsApplied,
      ...(result.selectedModelId && { selectedModelId: result.selectedModelId }),
    },
  })
}

export function onboardingOAuthResumeBotId(): string {
  const { selection, botResult } = onboardingRuntimeState.value
  if (selection.kind !== 'acp' || selection.selection.setupMode !== 'oauth') return ''
  if (botResult?.acpLaunchAgentId !== selection.selection.agentId) return ''
  return botResult.botId
}

export function resetOnboardingRuntimeState() {
  onboardingRuntimeState.value = emptyState()
  safeSessionRemove(ONBOARDING_KEYS.runtimeState)
}
