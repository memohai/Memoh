<template>
  <main
    class="w-screen h-screen flex flex-col items-center justify-center bg-background relative overflow-hidden p-6 pb-24"
  >
    <DotMatrixBg class="login-dots absolute inset-0 pointer-events-none" />

    <div class="absolute top-6 left-6 flex items-center gap-2.5 z-(--z-raised)">
      <img
        src="/logo.svg"
        class="size-7"
        alt=""
        aria-hidden="true"
      >
      <span class="text-control font-semibold text-foreground">Memoh</span>
    </div>

    <section
      class="relative z-(--z-raised) w-full max-w-[20.5rem] flex flex-col items-center gap-8 transition-all duration-[175ms] ease-out"
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
        <PasswordInput
          id="password"
          v-model="password"
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
import { Input } from '@felinic/ui'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/store/user'
import { toast } from '@felinic/ui'
import { useI18n } from 'vue-i18n'
import { postAuthLogin } from '@memohai/sdk'
import LoadingButton from '@/components/loading-button/index.vue'
import PasswordInput from '@/components/password-input/index.vue'
import DotMatrixBg from './components/dot-matrix-bg.vue'
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
/* 极轻的四角渐隐:圆心略偏右下,使左上角(有 logo)衰减略多,其余角几乎不遮;
 * 最深也仅到 ~82%,只是收一点点边角,不牺牲可见度 */
.login-dots {
  -webkit-mask-image: radial-gradient(ellipse 118% 118% at 58% 60%, #000 72%, rgba(0, 0, 0, 0.82) 100%);
  mask-image: radial-gradient(ellipse 118% 118% at 58% 60%, #000 72%, rgba(0, 0, 0, 0.82) 100%);
}

form :deep([data-button]:active::before) {
  scale: 1 !important;
  transition: none !important;
}

/* 动画背景上:禁用态 opacity 会让点阵从按钮底下透出来 */
form :deep([data-button]:is([data-variant="default"], [data-variant="primary"])) {
  background-color: var(--btn-primary);
}
form :deep([data-button]:is([data-variant="default"], [data-variant="primary"]):disabled) {
  opacity: 1;
  cursor: not-allowed;
}
form :deep([data-button]:is([data-variant="default"], [data-variant="primary"]):disabled)::before {
  background-color: var(--btn-primary-active);
}

/* 同一道理:禁用态的输入框也不能走 opacity,否则点阵会从框底透出。
 * 保持不透明,只用降阶文字色表达锁定态。 */
form :deep(input:disabled) {
  opacity: 1;
  color: var(--muted-foreground);
}
</style>
