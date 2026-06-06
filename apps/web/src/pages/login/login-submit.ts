import type { Ref } from 'vue'
import type { UserInfo } from '@/store/user'

export interface LoginCredentials {
  username: string
  password: string
}

interface LoginResponseData {
  access_token?: string
  user_id?: string
  username?: string
  display_name?: string | null
  role?: string | null
  avatar_url?: string | null
  timezone?: string | null
}

export interface SubmitLoginDependencies {
  authenticate: (values: LoginCredentials) => Promise<{ data?: LoginResponseData | null }>
  applyLogin: (userData: UserInfo, token: string) => void
  navigateHome: () => Promise<unknown> | unknown
  notifyInvalidCredentials: () => void
}

export async function submitLogin(
  values: LoginCredentials,
  isSubmitting: Ref<boolean>,
  dependencies: SubmitLoginDependencies,
): Promise<boolean> {
  if (isSubmitting.value) return false

  isSubmitting.value = true
  try {
    let data: LoginResponseData | null | undefined
    try {
      const response = await dependencies.authenticate(values)
      data = response.data
    } catch {
      dependencies.notifyInvalidCredentials()
      return false
    }

    if (!data?.access_token || !data.user_id) {
      dependencies.notifyInvalidCredentials()
      return false
    }

    dependencies.applyLogin({
      id: data.user_id,
      username: data.username ?? '',
      displayName: data.display_name ?? '',
      role: data.role ?? '',
      avatarUrl: data.avatar_url ?? '',
      timezone: data.timezone ?? 'UTC',
    }, data.access_token)

    await dependencies.navigateHome()
    return true
  } finally {
    isSubmitting.value = false
  }
}
