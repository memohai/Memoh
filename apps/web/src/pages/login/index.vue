<template>
  <main
    class="w-screen h-screen flex flex-col items-center justify-center bg-background relative p-6 pb-24"
  >
    <section
      class="w-full max-w-[20.5rem] flex flex-col items-center gap-6 transition-all duration-[175ms] ease-out"
      :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
    >
      <div class="flex flex-col items-center gap-3 text-center">
        <img
          src="/logo.svg"
          class="size-14"
          alt=""
          aria-hidden="true"
        >
        <h1 class="text-display font-semibold text-foreground">
          {{ $t('auth.pageTitle') }}
        </h1>
        <p class="text-sm text-muted-foreground">
          {{ $t('auth.pageSubtitle') }}
        </p>
      </div>

      <!-- 表单元素只有三个,不值得再包一层卡;直接摆在页面底色上 -->
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
        />
        <PasswordInput
          id="password"
          v-model="password"
          :placeholder="$t('auth.password')"
          autocomplete="current-password"
          :disabled="isSubmitting"
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
import PasswordInput from '@/components/password-input/index.vue'
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
