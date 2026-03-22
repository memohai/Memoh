<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <div class="flex items-center gap-2">
        <FontAwesomeIcon
          :icon="['fas', 'window-maximize']"
          class="size-5"
        />
        <div>
          <h2 class="text-base font-semibold">
            {{ curContext?.name || $t('browserContext.title') }}
          </h2>
          <p class="text-xs text-muted-foreground">
            {{ curContext?.id }}
          </p>
        </div>
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form @submit="handleSave">
      <div class="space-y-4">
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <Label>{{ $t('browserContext.name') }}</Label>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('browserContext.namePlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>

        <Separator class="my-4" />
        <h3 class="text-sm font-medium text-foreground">
          {{ $t('browserContext.config') }}
        </h3>

        <FormField
          v-slot="{ value, handleChange }"
          name="core"
        >
          <FormItem>
            <Label>{{ $t('browserContext.core') }}</Label>
            <FormControl>
              <div class="flex gap-3">
                <button
                  v-for="c in availableCores"
                  :key="c"
                  type="button"
                  class="flex items-center gap-2 px-3 py-1.5 rounded-md border text-sm transition-colors"
                  :class="value === c
                    ? 'border-primary bg-primary/10 text-primary font-medium'
                    : 'border-border bg-card text-muted-foreground hover:bg-accent'"
                  @click="handleChange(c)"
                >
                  {{ $t(`browserContext.${c}`) }}
                </button>
              </div>
            </FormControl>
          </FormItem>
        </FormField>

        <div class="grid grid-cols-2 gap-4">
          <FormField
            v-slot="{ componentField }"
            name="viewportWidth"
          >
            <FormItem>
              <Label>{{ $t('browserContext.viewportWidth') }}</Label>
              <FormControl>
                <Input
                  type="number"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <FormField
            v-slot="{ componentField }"
            name="viewportHeight"
          >
            <FormItem>
              <Label>{{ $t('browserContext.viewportHeight') }}</Label>
              <FormControl>
                <Input
                  type="number"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </div>

        <FormField
          v-slot="{ componentField }"
          name="userAgent"
        >
          <FormItem>
            <Label>{{ $t('browserContext.userAgent') }}</Label>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('browserContext.userAgentPlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>

        <div class="grid grid-cols-2 gap-4">
          <FormField
            v-slot="{ componentField }"
            name="deviceScaleFactor"
          >
            <FormItem>
              <Label>{{ $t('browserContext.deviceScaleFactor') }}</Label>
              <FormControl>
                <Input
                  type="number"
                  step="0.1"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <FormField
            v-slot="{ componentField }"
            name="locale"
          >
            <FormItem>
              <Label>{{ $t('browserContext.locale') }}</Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('browserContext.localePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </div>

        <FormField
          v-slot="{ componentField }"
          name="timezoneId"
        >
          <FormItem>
            <Label>{{ $t('browserContext.timezoneId') }}</Label>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('browserContext.timezonePlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>

        <div class="flex items-center gap-4">
          <FormField
            v-slot="{ value, handleChange }"
            name="isMobile"
          >
            <FormItem class="flex items-center gap-2">
              <FormControl>
                <Switch
                  :model-value="value"
                  @update:model-value="handleChange"
                />
              </FormControl>
              <Label class="mt-0!">{{ $t('browserContext.isMobile') }}</Label>
            </FormItem>
          </FormField>

          <FormField
            v-slot="{ value, handleChange }"
            name="ignoreHTTPSErrors"
          >
            <FormItem class="flex items-center gap-2">
              <FormControl>
                <Switch
                  :model-value="value"
                  @update:model-value="handleChange"
                />
              </FormControl>
              <Label class="mt-0!">{{ $t('browserContext.ignoreHTTPSErrors') }}</Label>
            </FormItem>
          </FormField>
        </div>
      </div>

      <Separator class="my-6" />

      <div class="flex gap-2 items-center justify-between">
        <ConfirmPopover
          :title="$t('browserContext.deleteConfirm')"
          :confirm-text="$t('common.delete')"
          @confirm="handleDelete"
        >
          <Button
            variant="destructive"
            type="button"
          >
            <FontAwesomeIcon :icon="['fas', 'trash']" />
          </Button>
        </ConfirmPopover>
        <Button
          type="submit"
          :disabled="isSaving"
        >
          {{ $t('common.save') }}
        </Button>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import {
  Input,
  FormField,
  FormControl,
  FormItem,
  Label,
  Separator,
  Button,
  Switch,
} from '@memoh/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { putBrowserContextsById, deleteBrowserContextsById } from '@memoh/sdk'
import { getBrowserContextsCoresQuery } from '@memoh/sdk/colada'
import type { BrowsercontextsBrowserContext, BrowsercontextsUpdateRequest } from '@memoh/sdk'
import { inject, watch, computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useDialogMutation } from '@/composables/useDialogMutation'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const { t } = useI18n()
const { run } = useDialogMutation()
const queryCache = useQueryCache()

const curContext = inject<Ref<BrowsercontextsBrowserContext | undefined>>('curBrowserContext')

const { data: coresData } = useQuery(getBrowserContextsCoresQuery())
const availableCores = computed(() => coresData.value?.cores ?? ['chromium'])

interface ConfigShape {
  core?: string
  viewport?: { width?: number; height?: number }
  userAgent?: string
  deviceScaleFactor?: number
  isMobile?: boolean
  locale?: string
  timezoneId?: string
  ignoreHTTPSErrors?: boolean
}

function parseConfig(ctx: BrowsercontextsBrowserContext | undefined): ConfigShape {
  if (!ctx?.config) return {}
  if (typeof ctx.config === 'string') {
    try { return JSON.parse(ctx.config) } catch { return {} }
  }
  if (typeof ctx.config === 'object' && !Array.isArray(ctx.config)) {
    return ctx.config as unknown as ConfigShape
  }
  return {}
}

const schema = toTypedSchema(z.object({
  name: z.string().min(1),
  core: z.enum(['chromium', 'firefox']).optional(),
  viewportWidth: z.coerce.number().optional(),
  viewportHeight: z.coerce.number().optional(),
  userAgent: z.string().optional(),
  deviceScaleFactor: z.coerce.number().optional(),
  isMobile: z.boolean().optional(),
  locale: z.string().optional(),
  timezoneId: z.string().optional(),
  ignoreHTTPSErrors: z.boolean().optional(),
}))

const form = useForm({ validationSchema: schema })

watch(() => curContext?.value, (ctx) => {
  if (!ctx) return
  const cfg = parseConfig(ctx)
  form.resetForm({
    values: {
      name: ctx.name || '',
      core: (cfg.core as 'chromium' | 'firefox') ?? 'chromium',
      viewportWidth: cfg.viewport?.width ?? 1280,
      viewportHeight: cfg.viewport?.height ?? 720,
      userAgent: cfg.userAgent ?? '',
      deviceScaleFactor: cfg.deviceScaleFactor ?? 1,
      isMobile: cfg.isMobile ?? false,
      locale: cfg.locale ?? '',
      timezoneId: cfg.timezoneId ?? '',
      ignoreHTTPSErrors: cfg.ignoreHTTPSErrors ?? false,
    },
  })
}, { immediate: true })

const { mutateAsync: updateMutation, isLoading: isSaving } = useMutation({
  mutation: async (data: { id: string; name: string; config: Record<string, unknown> }) => {
    const { data: result } = await putBrowserContextsById({
      path: { id: data.id },
      body: { name: data.name } as BrowsercontextsUpdateRequest,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['browser-contexts'] }),
})

const { mutateAsync: deleteMutation } = useMutation({
  mutation: async (id: string) => {
    await deleteBrowserContextsById({
      path: { id },
      throwOnError: true,
    })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['browser-contexts'] }),
})

const handleSave = form.handleSubmit(async (values) => {
  const id = curContext?.value?.id
  if (!id) return

  const config: Record<string, unknown> = {
    core: values.core ?? 'chromium',
  }
  if (values.viewportWidth || values.viewportHeight) {
    config.viewport = {
      width: values.viewportWidth || 1280,
      height: values.viewportHeight || 720,
    }
  }
  if (values.userAgent) config.userAgent = values.userAgent
  if (values.deviceScaleFactor) config.deviceScaleFactor = values.deviceScaleFactor
  if (values.isMobile) config.isMobile = values.isMobile
  if (values.locale) config.locale = values.locale
  if (values.timezoneId) config.timezoneId = values.timezoneId
  if (values.ignoreHTTPSErrors) config.ignoreHTTPSErrors = values.ignoreHTTPSErrors

  await run(
    () => updateMutation({ id, name: values.name, config }),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => toast.success(t('browserContext.saveSuccess')),
    },
  )
})

async function handleDelete() {
  const id = curContext?.value?.id
  if (!id) return
  try {
    await deleteMutation(id)
    toast.success(t('browserContext.deleteSuccess'))
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('common.deleteFailed')))
  }
}
</script>
