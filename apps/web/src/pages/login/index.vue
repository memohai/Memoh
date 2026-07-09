<template>
  <!-- 几何居中 + 光学上移(与 SaaS 登录同语言);无 OAuth / 多步,仅 username + password。 -->
  <main
    class="relative flex h-screen w-screen flex-col items-center justify-center overflow-hidden bg-background p-6"
  >
    <DotMatrixBg class="pointer-events-none absolute inset-0" />

    <section
      class="login-scope relative flex w-full max-w-[22rem] -translate-y-8 flex-col items-center gap-6 transition-[opacity,transform] duration-[175ms] ease-out"
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
        <p class="max-w-[18rem] text-sm text-muted-foreground">
          {{ $t('auth.pageSubtitle') }}
        </p>
      </div>

      <!-- 无卡、无描边:两字段 + Continue 直接浮在点阵上(SaaS filled-control 语言)。
           placeholder 用 Enter …,不与 label 裸词重复(P0 label≠placeholder)。 -->
      <form
        class="flex w-full flex-col"
        @submit.prevent="login"
      >
        <Input
          id="username"
          v-model="username"
          type="text"
          :placeholder="$t('auth.usernamePlaceholder')"
          :disabled="isSubmitting"
          autocomplete="username"
          autofocus
        />
        <PasswordInput
          id="password"
          v-model="password"
          class="mt-2.5"
          :placeholder="$t('auth.passwordPlaceholder')"
          autocomplete="current-password"
          :disabled="isSubmitting"
        />
        <LoadingButton
          type="submit"
          class="login-entry-submit"
          block
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
import DotMatrixBg from '@/pages/login/components/dot-matrix-bg.vue'
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
        // 可显隐密码框:失败不清空,用户多半只改账号或笔误。
        toast.error(t('auth.invalidCredentials'), {
          description: t('auth.retryHint'),
        })
      },
    },
  )
}
</script>

<style scoped>
/* placeholder 是主要可见文案,与全局 placeholder(360)相比提半档。 */
.login-scope :deep(input)::placeholder {
  font-weight: 450;
}
</style>

<!-- 非 scoped:scoped 内 :global(.dark) 组合选择器编译不可靠;用 .login-scope 限定。 -->
<style>
/* ── 登录页专属控件语言(与 SaaS 登录同源,2026-07)──────────────
 * 无卡、无描边:控件 = 比页面底色高一阶的填充块,浮在点阵动画上。
 * rest 用不透明实色;hover 用透明度叠在实色上。与产品内 field-edge
 * 是有意分叉(登录=营销面 / 产品内=工具面)。 */
.login-scope {
  --login-control: #ececec;
  --login-control-hover: color-mix(in oklab, #ececec, rgb(0 0 0 / 0.05));
}
.dark .login-scope {
  --login-control: #242424;
  --login-control-hover: color-mix(in oklab, #242424, rgb(255 255 255 / 0.07));
}

/* 防双击选中系统蓝框 */
.login-scope [data-button] {
  user-select: none;
  -webkit-user-select: none;
}

/* 禁用纱层:背景色叠在控件上,不引入第三色;loading 豁免 */
.login-scope [data-button]:not([data-variant="quiet"])::after {
  content: "";
  position: absolute;
  inset: 0;
  z-index: 1;
  border-radius: inherit;
  background-color: var(--background);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease-out;
}
.login-scope [data-button]:disabled:not([data-loading])::after {
  opacity: 0.55;
}
.login-scope [data-button]:disabled {
  opacity: 1;
  pointer-events: auto;
  cursor: not-allowed;
}

/* 输入框:实心填充,无 field-edge;聚焦不变色 */
.login-scope [data-slot="input"],
.login-scope [data-slot="input-group"] {
  background-color: var(--login-control);
  box-shadow: none;
  transition: background-color 0.2s ease-out;
}
.login-scope [data-slot="input"]:focus,
.login-scope [data-slot="input-group"]:focus-within {
  box-shadow: none;
}
.login-scope [data-slot="input"]:disabled,
.login-scope [data-slot="input-group"]:has(input:disabled) {
  background-color: color-mix(in oklab, var(--background) 55%, var(--login-control));
  opacity: 1;
  cursor: not-allowed;
}

/* 主按钮到字段:密码框→Continue 20px(与 SaaS 密码步同档) */
.login-scope .login-entry-submit {
  margin-top: 1.25rem;
}
</style>
