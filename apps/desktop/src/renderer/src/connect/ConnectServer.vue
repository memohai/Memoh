<template>
  <main class="relative flex h-screen w-screen flex-col items-center justify-center overflow-hidden bg-background p-6">
    <DotMatrixBg class="pointer-events-none absolute inset-0" />

    <section
      class="relative flex w-full max-w-[22rem] -translate-y-8 flex-col items-center gap-6 transition-[opacity,transform] duration-[175ms] ease-out motion-reduce:transition-none"
      :class="exiting ? 'scale-[0.88] opacity-0 motion-reduce:scale-100 motion-reduce:opacity-100' : 'scale-100 opacity-100'"
    >
      <div class="flex flex-col items-center gap-3 text-center">
        <img
          src="/logo.svg"
          class="size-14"
          alt=""
          aria-hidden="true"
        >
        <h1 class="text-display font-semibold text-foreground">
          {{ t('desktopConnection.title') }}
        </h1>
        <p class="max-w-[18rem] text-sm text-muted-foreground">
          {{ t('desktopConnection.subtitle') }}
        </p>
      </div>

      <form
        class="flex w-full flex-col gap-2.5"
        @submit="onSubmit"
      >
        <Input
          v-bind="baseUrlProps"
          id="server-url"
          v-model="baseUrl"
          type="url"
          inputmode="url"
          autocomplete="url"
          :placeholder="t('desktopConnection.placeholder')"
          :disabled="isSubmitting"
          autofocus
        />
        <LoadingButton
          type="submit"
          block
          :loading="isSubmitting"
          :loading-delay="0"
          :disabled="!meta.valid || isSubmitting"
        >
          {{ t('desktopConnection.connect') }}
        </LoadingButton>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import { z } from 'zod'
import { Input, toast } from '@felinic/ui'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import LoadingButton from '@memohai/web/components/loading-button/index.vue'
import DotMatrixBg from '@memohai/web/pages/login/components/dot-matrix-bg.vue'
import { notifyAuthSessionCleared } from '@memohai/web/lib/auth-session'
import { setupApiClient } from '@memohai/web/api-client'
import { LOGIN_ENTRY_ANIMATION_KEY } from '@memohai/web/pages/login/transition'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const exiting = ref(false)
const exitDuration = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 175

const { defineField, handleSubmit, isSubmitting, meta, setFieldValue } = useForm({
  validationSchema: toTypedSchema(z.object({
    baseUrl: z.string().trim().min(1),
  })),
  initialValues: { baseUrl: '' },
})
const [baseUrl, baseUrlProps] = defineField('baseUrl', { validateOnModelUpdate: true })

onMounted(async () => {
  const status = await window.api.desktop.getServerStatus()
  setFieldValue('baseUrl', status.baseUrl)
})

const onSubmit = handleSubmit(async ({ baseUrl }) => {
  try {
    const result = await window.api.desktop.connectServer(baseUrl)
    if (!result.ok) {
      const description = result.error === 'http-error' && result.status
        ? t('desktopConnection.errors.http', { status: result.status })
        : t(`desktopConnection.errors.${result.error}`)
      toast.error(t('desktopConnection.failed'), { description })
      return
    }

    exiting.value = true
    await new Promise<void>(resolve => setTimeout(resolve, exitDuration))

    if (result.changed) {
      notifyAuthSessionCleared('logout')
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      sessionStorage.clear()
    }
    setupApiClient({
      baseUrl: result.baseUrl,
      onUnauthorized: () => router.replace({ name: 'Login' }),
    })
    if (result.changed || !localStorage.getItem('token')) {
      sessionStorage.setItem(LOGIN_ENTRY_ANIMATION_KEY, '1')
      await router.replace({ name: 'Login' })
      return
    }

    const returnTo = typeof route.query.returnTo === 'string' && route.query.returnTo.startsWith('/')
      ? route.query.returnTo
      : '/'
    await router.replace(returnTo)
  } catch {
    exiting.value = false
    toast.error(t('desktopConnection.failed'), {
      description: t('desktopConnection.errors.unreachable'),
    })
  }
})
</script>
