import { useUserStore } from '@/store/user'

function shouldForceOnboarding(): boolean {
  return localStorage.getItem('memoh:dev:force-onboarding')?.trim() === '1'
}

export async function ensureOnboarding(): Promise<boolean> {
  if (shouldForceOnboarding()) return false
  const store = useUserStore()
  if (store.onboardingCompleted) return true
  await store.fetchMe()
  return store.onboardingCompleted
}
