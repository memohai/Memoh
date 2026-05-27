import { defineStore } from 'pinia'
import { reactive, watch } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { useRouter } from 'vue-router'
import { useQueryCache } from '@pinia/colada'
import { notifyAuthSessionCleared, onAuthSessionCleared, type AuthSessionClearReason } from '@/lib/auth-session'

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

    const exitLogin = () => {
      clearFrontendSessionState('logout')
      localToken.value = ''
      resetUserInfo()
    }
    const router = useRouter()
    watch(
      localToken,
      () => {
        if (!localToken.value) {
          clearFrontendSessionState('token-cleared')
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
      resetUserInfo()
    })
    return {
      userInfo,
      login,
      patchUserInfo,
      exitLogin,
    }
  },
  {
    persist: true,
  },
)
