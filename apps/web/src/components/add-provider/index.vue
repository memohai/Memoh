<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('provider.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('provider.add')"
      :submit-disabled="(form.meta.value.valid === false) || isLoading"
      :loading="isLoading"
      @submit="createProvider"
    >
      <template #trigger>
        <Button
          class="w-full shadow-none! text-muted-foreground mb-4"
          variant="outline"
        >
          <Plus
            class="mr-1"
          /> {{ $t('provider.addBtn') }}
        </Button>
      </template>
      <template #body>
        <div
          class="flex-col gap-3 flex mt-4"
        >
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label
                class="mb-2"
                for="provider-create-name"
              >
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  id="provider-create-name"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('common.name')"
                />
              </FormControl>
            </FormItem>
          </FormField>
          <FormField
            v-if="!['openai-codex', 'github-copilot'].includes(form.values.client_type)"
            v-slot="{ componentField }"
            name="api_key"
          >
            <FormItem>
              <Label
                class="mb-2"
                for="provider-create-api-key"
              >
                {{ $t('provider.apiKey') }}
              </Label>
              <FormControl>
                <Input
                  id="provider-create-api-key"
                  type="text"
                  :placeholder="$t('provider.apiKeyPlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('provider.apiKey')"
                />
              </FormControl>
            </FormItem>
          </FormField>
          <div
            v-else-if="['openai-codex', 'github-copilot'].includes(form.values.client_type)"
            class="rounded-lg border p-3 text-xs text-muted-foreground"
          >
            {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.githubCreateHint' : 'provider.oauth.openaiCreateHint') }}
          </div>
          <FormField
            v-if="form.values.client_type !== 'github-copilot'"
            v-slot="{ componentField }"
            name="base_url"
          >
            <FormItem>
              <Label
                class="mb-2"
                for="provider-create-base-url"
              >
                {{ $t('provider.url') }}
              </Label>
              <FormControl>
                <Input
                  id="provider-create-base-url"
                  type="text"
                  :placeholder="$t('provider.urlPlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('provider.url')"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <FormField
            v-slot="{ value, handleChange }"
            name="client_type"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('provider.clientType') }}
              </Label>
              <FormControl>
                <SearchableSelectPopover
                  :model-value="value"
                  :options="clientTypeOptions"
                  :placeholder="$t('models.clientTypePlaceholder')"
                  @update:model-value="handleChange"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <Separator />

          <FormField
            v-slot="{ value, handleChange }"
            name="auto_import"
          >
            <FormItem class="flex flex-row items-center justify-between rounded-lg border p-3 shadow-sm">
              <div class="space-y-0.5">
                <Label class="text-sm">
                  {{ $t('provider.autoImport') }}
                </Label>
                <p class="text-[0.8rem] text-muted-foreground">
                  {{ $t('provider.autoImportHint') }}
                </p>
              </div>
              <FormControl>
                <Switch
                  :model-value="value"
                  @update:model-value="handleChange"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </div>
      </template>
    </FormDialogShell>
  </section>
</template>
<script setup lang="ts">
import {
  Button,
  Input,
  FormField,
  FormControl,
  FormItem,
  Label,
  Switch,
  Separator,
} from '@memohai/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProviders, postProvidersByIdImportModels } from '@memohai/sdk'
import type { ProvidersCreateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import { CLIENT_TYPE_LIST, CLIENT_TYPE_META } from '@/constants/client-types'
import { toast } from 'vue-sonner'
import { computed, watch } from 'vue'

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const clientTypeOptions = computed(() =>
  CLIENT_TYPE_LIST.map((ct) => ({
    value: ct.value,
    label: ct.label,
    description: ct.hint,
    keywords: [ct.label, ct.hint, CLIENT_TYPE_META[ct.value]?.value ?? ct.value],
  })),
)

const queryCache = useQueryCache()
const { mutateAsync: createProviderMutation, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const config: Record<string, unknown> = {}
    if (data.base_url) config.base_url = data.base_url
    if (typeof data.api_key === 'string' && data.api_key.trim() !== '' && data.client_type !== 'github-copilot') {
      config.api_key = data.api_key.trim()
    }
    const payload = {
      name: data.name,
      client_type: data.client_type,
      config,
    }
    const { data: result } = await postProviders({ body: payload as ProvidersCreateRequest, throwOnError: true })
    if (data.auto_import && result?.id) {
      try {
        const { data: importResult } = await postProvidersByIdImportModels({
          path: { id: result.id },
          throwOnError: true,
        })
        if (importResult) {
          toast.success(t('models.importSuccess', {
            created: importResult.created,
            skipped: importResult.skipped,
          }))
        }
      }
      catch (e) {
        console.error('Auto import failed:', e)
        toast.error(t('models.importFailed'))
      }
    }
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  api_key: z.string().optional(),
  base_url: z.string().optional(),
  name: z.string().min(1),
  client_type: z.string().min(1),
  auto_import: z.boolean().optional(),
}).superRefine((value, ctx) => {
  if (!['openai-codex', 'github-copilot'].includes(value.client_type) && !value.api_key?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['api_key'],
      message: 'API key is required',
    })
  }
  if (value.client_type !== 'github-copilot' && !value.base_url?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['base_url'],
      message: 'Base URL is required',
    })
  }
}))

const form = useForm({
  validationSchema: providerSchema,
  initialValues: {
    auto_import: false,
    client_type: 'openai-completions',
  }
})

watch(() => form.values.client_type, (clientType) => {
  if (clientType === 'openai-codex' && !form.values.base_url) {
    form.setFieldValue('base_url', 'https://chatgpt.com/backend-api')
  }
  if (clientType === 'github-copilot') {
    form.setFieldValue('base_url', '')
  }
})

const createProvider = form.handleSubmit(async (value) => {
  await run(
    () => createProviderMutation(value),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {
        open.value = false
        form.resetForm()
      },
    },
  )
})
</script>
