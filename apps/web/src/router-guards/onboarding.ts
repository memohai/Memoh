import { getUsersMe } from '@memohai/sdk'

let checkDone = false
let completed = false
let pendingPromise: Promise<void> | null = null

function shouldForce(): boolean {
  return localStorage.getItem('memoh:dev:force-onboarding')?.trim() === '1'
}

export async function checkOnboarding(): Promise<boolean> {
  if (shouldForce()) return false

  // Fast path: completed in this session or via localStorage cache
  if (completed) return true
  if (localStorage.getItem('memoh:onboarding:completed') === '1') {
    completed = true
    checkDone = true
    return true
  }

  if (checkDone) return completed

  if (!pendingPromise) {
    pendingPromise = getUsersMe({ throwOnError: true })
      .then(({ data }) => {
        const meta = data?.metadata
        completed = meta?.onboarding_completed === true
        if (completed) {
          localStorage.setItem('memoh:onboarding:completed', '1')
        }
      })
      .catch((err) => {
        console.warn('[onboarding-guard] failed to check onboarding status, allowing navigation', err)
        completed = true
      })
      .finally(() => {
        checkDone = true
        pendingPromise = null
      })
  }

  await pendingPromise
  return completed
}
