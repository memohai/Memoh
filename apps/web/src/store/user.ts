import { defineStore } from 'pinia'
import { reactive, ref, watch } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { useRouter } from 'vue-router'
import { useQueryCache } from '@pinia/colada'
import { getUsersMe } from '@memohai/sdk'
import { notifyAuthSessionCleared, onAuthSessionCleared, type AuthSessionClearReason } from '@/lib/auth-session'
import { resetOnboardingState } from '@/composables/useOnboarding'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'

export interface UserInfo {
  id: string;
  username: string;
  role: string;
  displayName: string;
  avatarUrl: string;
  timezone: string;
}

export const useUserStore = defineStore(
  'user',
  () => {
    const userInfo = reactive<UserInfo>({
      id: '',
      username: '',
      role: '',
      displayName: '',
      avatarUrl: '',
      timezone: 'UTC',
    })

    const localToken = useLocalStorage('token', '')
    const onboardingCompleted = ref(false)

    let _meChecked = false
    let _pendingFetch: Promise<void> | null = null

    async function fetchMe() {
      if (_meChecked) return
      if (onboardingCompleted.value) { _meChecked = true; return }
      if (_pendingFetch) { await _pendingFetch; return }
      _pendingFetch = (async () => {
        try {
          const { data } = await getUsersMe({ throwOnError: true })
          if (data) {
            userInfo.id = data.id ?? ''
            userInfo.username = data.username ?? ''
            userInfo.role = data.role ?? ''
            userInfo.displayName = data.display_name ?? ''
            userInfo.avatarUrl = data.avatar_url ?? ''
            userInfo.timezone = data.timezone || 'UTC'
            onboardingCompleted.value = data.metadata?.onboarding_completed === true
          }
          _meChecked = true
        } catch {
          onboardingCompleted.value = true
        } finally {
          _pendingFetch = null
        }
      })()
      await _pendingFetch
    }

    const resetUserInfo = () => {
      for (const key of Object.keys(userInfo) as (keyof UserInfo)[]) {
        userInfo[key] = key === 'timezone' ? 'UTC' : ''
      }
    }

    const clearQueryCache = () => {
      try {
        const queryCache = useQueryCache()
        queryCache.cancelQueries({}, new Error('auth session changed'))
        for (const entry of queryCache.getEntries()) {
          queryCache.remove(entry)
        }
      } catch (error) {
        console.warn('Failed to clear query cache after auth session change:', error)
      }
    }

    const clearFrontendSessionState = (reason: AuthSessionClearReason) => {
      clearQueryCache()
      notifyAuthSessionCleared(reason)
    }

    const login = (userData: UserInfo, token: string) => {
      clearFrontendSessionState('login')
      localToken.value = token
      for (const key of Object.keys(userData) as (keyof UserInfo)[]) {
        userInfo[key] = userData[key]
      }
    }

    const patchUserInfo = (patch: Partial<UserInfo>) => {
      for (const key of Object.keys(patch) as (keyof UserInfo)[]) {
        const value = patch[key]
        if (value !== undefined) {
          userInfo[key] = value
        }
      }
    }

    const resetOnboarding = () => {
      onboardingCompleted.value = false
      _meChecked = false
      _pendingFetch = null
      localStorage.removeItem(ONBOARDING_KEYS.introSeen)
      resetOnboardingState()
    }

    const exitLogin = () => {
      clearFrontendSessionState('logout')
      localToken.value = ''
      resetOnboarding()
      resetUserInfo()
    }

    const router = useRouter()
    watch(
      localToken,
      () => {
        if (!localToken.value) {
          clearFrontendSessionState('token-cleared')
          resetOnboarding()
          resetUserInfo()
          if (router.currentRoute.value.name !== 'Login') {
            void router.replace({ name: 'Login' })
          }
        }
      },
      {
        immediate: true,
      },
    )
    onAuthSessionCleared(({ reason }) => {
      if (reason !== 'unauthorized') return
      clearQueryCache()
      localToken.value = ''
      resetOnboarding()
      resetUserInfo()
    })
    return {
      userInfo,
      onboardingCompleted,
      fetchMe,
      login,
      patchUserInfo,
      exitLogin,
    }
  },
  {
    persist: {
      pick: ['userInfo', 'onboardingCompleted'],
    },
  },
)
