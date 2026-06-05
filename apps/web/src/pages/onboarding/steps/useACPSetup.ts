import { normalizeACPAgentID } from '@/utils/acp'
import { ONBOARDING_KEYS } from '../constants'

export interface OnboardingACPSelection {
  agentId: string
  setupMode: string
  managed: Record<string, string>
}

export function readACPSelection(): OnboardingACPSelection | null {
  try {
    const raw = sessionStorage.getItem(ONBOARDING_KEYS.acpSelection)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<OnboardingACPSelection>
    const agentId = normalizeACPAgentID(parsed.agentId)
    if (!agentId) return null
    const managed: Record<string, string> = {}
    if (parsed.managed && typeof parsed.managed === 'object') {
      for (const [key, value] of Object.entries(parsed.managed)) {
        managed[key] = String(value ?? '')
      }
    }
    return {
      agentId,
      setupMode: typeof parsed.setupMode === 'string' && parsed.setupMode ? parsed.setupMode : 'api_key',
      managed,
    }
  } catch {
    return null
  }
}

export function writeACPSelection(selection: OnboardingACPSelection): void {
  sessionStorage.setItem(ONBOARDING_KEYS.acpSelection, JSON.stringify(selection))
}

export function clearACPSelection(): void {
  sessionStorage.removeItem(ONBOARDING_KEYS.acpSelection)
}
