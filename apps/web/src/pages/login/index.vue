<template>
  <main
    class="w-screen h-screen flex flex-col items-center justify-center bg-background relative overflow-hidden p-6 pb-24"
  >
    <div class="login-grid absolute inset-0 pointer-events-none" />

    <div class="absolute top-6 left-6 flex items-center gap-2.5 z-10">
      <img
        src="/logo.svg"
        class="size-7"
        alt=""
        aria-hidden="true"
      >
      <span class="text-control font-semibold text-foreground">Memoh</span>
    </div>

    <section
      class="relative z-10 w-full max-w-xs flex flex-col items-center gap-8 transition-all duration-[175ms] ease-out"
      :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
    >
      <div class="flex flex-col items-center gap-3">
        <img
          src="/logo.svg"
          class="size-12"
          alt=""
          aria-hidden="true"
        >
        <h1 class="text-display font-semibold text-foreground">
          {{ $t('auth.pageTitle') }}
        </h1>
      </div>

      <form
        class="w-full flex flex-col gap-2.5"
        @submit.prevent="login"
      >
        <Input
          id="username"
          v-model="username"
          type="text"
          :placeholder="$t('auth.username')"
          :disabled="isSubmitting"
          autocomplete="username"
          class="bg-background"
        />
        <Input
          id="password"
          v-model="password"
          type="password"
          :placeholder="$t('auth.password')"
          autocomplete="current-password"
          :disabled="isSubmitting"
          class="bg-background"
        />
        <LoadingButton
          class="w-full mt-1"
          type="submit"
          :loading="isSubmitting"
          :loading-delay="0"
          :disabled="!canSubmit"
        >
          {{ $t('auth.continue') }}
        </LoadingButton>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Input } from '@memohai/ui'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/store/user'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { postAuthLogin } from '@memohai/sdk'
import LoadingButton from '@/components/loading-button/index.vue'
import { submitLogin } from './login-submit'
import { safeSessionSet } from '@/utils/safe-storage'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'

const router = useRouter()
const { t } = useI18n()

const username = ref('')
const password = ref('')
const exiting = ref(false)

const canSubmit = computed(() =>
  !!username.value.trim() && !!password.value.trim(),
)

const { login: loginHandle } = useUserStore()
const isSubmitting = ref(false)

const login = async () => {
  if (!canSubmit.value || isSubmitting.value) return
  await submitLogin(
    { username: username.value.trim(), password: password.value },
    isSubmitting,
    {
      authenticate: (body) => postAuthLogin({ body }),
      applyLogin: loginHandle,
      navigateHome: async () => {
        exiting.value = true
        safeSessionSet(ONBOARDING_KEYS.entryAnimation, '1')
        await new Promise<void>(resolve => setTimeout(resolve, 175))
        await router.replace({ path: '/' })
      },
      notifyInvalidCredentials: () => {
        toast.error(t('auth.invalidCredentials'), {
          description: t('auth.retryHint'),
        })
      },
    },
  )
}
</script>

<style scoped>
.login-grid {
  background-image:
    linear-gradient(var(--border) 1px, transparent 1px),
    linear-gradient(90deg, var(--border) 1px, transparent 1px);
  background-size: 28px 28px;
  opacity: 0.3;
}


form :deep([data-button]:active::before) {
  scale: 1 !important;
  transition: none !important;
}
</style>
