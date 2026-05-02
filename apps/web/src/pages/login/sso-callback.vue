<template>
  <main class="w-screen h-screen flex items-center justify-center bg-background p-4">
    <Spinner />
  </main>
</template>

<script setup lang="ts">
import { Spinner } from '@memohai/ui'
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useUserStore } from '@/store/user'
import { postAuthSsoExchange } from '@memohai/sdk'

const router = useRouter()
const { login } = useUserStore()

onMounted(async () => {
  const code = new URLSearchParams(window.location.search).get('code')?.trim()
  if (!code) {
    toast.error('SSO login failed')
    await router.replace({ name: 'Login' })
    return
  }

  const { data } = await postAuthSsoExchange({ body: { code } })
  if (!data?.access_token || !data.user_id) {
    toast.error('SSO login failed')
    await router.replace({ name: 'Login' })
    return
  }
  login({
    id: data.user_id,
    username: data.username ?? '',
    displayName: data.display_name ?? '',
    avatarUrl: data.avatar_url ?? '',
    timezone: data.timezone ?? 'UTC',
  }, data.access_token)
  await router.replace({ path: '/' })
})
</script>
