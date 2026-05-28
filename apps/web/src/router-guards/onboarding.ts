import { useUserStore } from '@/store/user'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'

function shouldForceOnboarding(): boolean {
  return localStorage.getItem(ONBOARDING_KEYS.forceOnboarding)?.trim() === '1'
}

export async function ensureOnboarding(): Promise<boolean> {
  if (shouldForceOnboarding()) return false
  const store = useUserStore()
  if (store.onboardingCompleted) return true
  await store.fetchMe()
  return store.onboardingCompleted
}
