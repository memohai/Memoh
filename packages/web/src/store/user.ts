import { defineStore } from 'pinia'
import { reactive, watch } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { useRouter } from 'vue-router'

export interface UserInfo {
  id: string;
  username: string;
  role: string;
  displayName: string;
}

export const useUserStore = defineStore(
  'user',
  () => {
    const userInfo = reactive<UserInfo>({
      id: '',
      username: '',
      role: '',
      displayName: '',
    })

    const localToken = useLocalStorage('token', '')

    const login = (userData: UserInfo, token: string) => {
      localToken.value = token
      for (const key of Object.keys(userData) as (keyof UserInfo)[]) {
        userInfo[key] = userData[key]
      }
    }

    const exitLogin = () => {
      localToken.value = ''
      for (const key of Object.keys(userInfo) as (keyof UserInfo)[]) {
        userInfo[key as keyof UserInfo] = ''
      }
    }
    const router = useRouter()
    watch(
      localToken,
      () => {
        if (!localToken.value) {
          exitLogin()
          router.replace({ name: 'Login' })
        }
      },
      {
        immediate: true,
      },
    )
    return {
      userInfo,
      login,
      exitLogin,
    }
  },
  {
    persist: true,
  },
)
