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
        <span
          v-if="hideTrigger"
          class="hidden"
        />
        <Button
          v-else
          class="w-full shadow-none! text-muted-foreground h-9 px-3 rounded-md border-border bg-background hover:bg-accent"
          variant="outline"
        >
          <Plus
            class="mr-1 size-4"
          /> {{ $t('provider.addBtn') }}
        </Button>
      </template>
      <template #body>
        <div
          class="flex-col gap-3 flex mt-4"
        >
          <FieldStack :label="$t('provider.preset')">
            <SearchableSelectPopover
              :model-value="selectedPresetId"
              :options="providerPresetOptions"
              :placeholder="$t('provider.presetPlaceholder')"
              :search-placeholder="$t('provider.presetSearchPlaceholder')"
              :empty-text="$t('provider.presetNoResults')"
              @update:model-value="applyPreset"
            >
              <template #option-icon="{ option }">
                <ProviderIcon
                  v-if="getPresetById(option.value)?.icon"
                  :icon="getPresetById(option.value)?.icon ?? ''"
                  size="1em"
                  class="size-4 shrink-0"
                />
                <span
                  v-else-if="getPresetById(option.value)"
                  class="flex size-4 shrink-0 items-center justify-center text-xs font-medium text-muted-foreground"
                >
                  {{ getPresetById(option.value)?.name?.slice(0, 1) }}
                </span>
                <Plus
                  v-else
                  class="size-4 shrink-0 text-muted-foreground"
                />
              </template>
            </SearchableSelectPopover>
          </FieldStack>

          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FieldStack
              :label="$t('common.name')"
              for="provider-create-name"
            >
              <FormControl>
                <Input
                  id="provider-create-name"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('common.name')"
                />
              </FormControl>
            </FieldStack>
          </FormField>
          <FormField
            v-if="apiKeyRequired"
            v-slot="{ componentField }"
            name="api_key"
          >
            <FieldStack
              :label="$t('provider.apiKey')"
              for="provider-create-api-key"
            >
              <FormControl>
                <Input
                  id="provider-create-api-key"
                  type="text"
                  :placeholder="$t('provider.apiKeyPlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('provider.apiKey')"
                />
              </FormControl>
            </FieldStack>
          </FormField>
          <div
            v-else-if="isManagedOAuthClientType(form.values.client_type)"
            class="rounded-lg border p-3 text-xs text-muted-foreground"
          >
            {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.githubCreateHint' : 'provider.oauth.openaiCreateHint') }}
          </div>
          <FormField
            v-if="form.values.client_type !== 'github-copilot'"
            v-slot="{ componentField }"
            name="base_url"
          >
            <FieldStack
              :label="$t('provider.url')"
              for="provider-create-base-url"
            >
              <FormControl>
                <Input
                  id="provider-create-base-url"
                  type="text"
                  :placeholder="$t('provider.urlPlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('provider.url')"
                />
              </FormControl>
            </FieldStack>
          </FormField>

          <FormField
            v-if="!selectedPreset"
            v-slot="{ value, handleChange }"
            name="client_type"
            keep-value
          >
            <FieldStack :label="$t('provider.clientType')">
              <FormControl>
                <SearchableSelectPopover
                  :model-value="value"
                  :options="clientTypeOptions"
                  :placeholder="$t('models.clientTypePlaceholder')"
                  @update:model-value="handleChange"
                />
              </FormControl>
            </FieldStack>
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
} from '@felinic/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProviders, postProvidersByIdImportModels } from '@memohai/sdk'
import type { ProvidersCreateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import FieldStack from '@/components/settings/field-stack.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import {
  CLIENT_TYPE_META,
  isManagedOAuthClientType,
  MANUAL_LLM_CLIENT_TYPE_LIST,
} from '@/constants/client-types'
import { toast } from '@felinic/ui'
import { computed, ref, watch } from 'vue'
import { providerPresets } from '@/constants/provider-presets'
import type { ProviderPreset } from '@/constants/provider-presets'
import ProviderIcon from '@/components/provider-icon/index.vue'
import { suggestProviderName } from './provider-presets'

const open = defineModel<boolean>('open')
const props = withDefaults(defineProps<{
  providers?: Array<{ name?: string }>
  hideTrigger?: boolean
  presetDomain?: 'llm' | 'video' | 'speech' | 'transcription'
  importModels?: (providerId: string) => Promise<{ created?: number, skipped?: number } | null | undefined>
}>(), {
  providers: () => [],
  hideTrigger: false,
  presetDomain: 'llm',
  importModels: undefined,
})
const { t } = useI18n()
const { run } = useDialogMutation()

const customPresetId = 'custom'
const availablePresets = computed(() => providerPresets.filter(preset => (preset.domain ?? 'llm') === props.presetDomain))
// Only the LLM domain offers a free-form "custom" entry; the others (video,
// speech, transcription) always start on a concrete preset.
const defaultPresetId = computed(() => props.presetDomain === 'llm' ? customPresetId : (availablePresets.value[0]?.id ?? customPresetId))
const selectedPresetId = ref(defaultPresetId.value)

const selectedPreset = computed(() => getPresetById(selectedPresetId.value))

const providerPresetOptions = computed(() => [
  ...(props.presetDomain === 'llm' ? [{
    value: customPresetId,
    label: t('provider.customProvider'),
    group: 'custom',
    groupLabel: t('provider.presetGroupCustom'),
    keywords: ['custom', 'provider'],
  }] : []),
  ...availablePresets.value.map(preset => ({
    value: preset.id,
    label: preset.name,
    description: CLIENT_TYPE_META[preset.clientType]?.label ?? preset.clientType,
    group: 'preset',
    groupLabel: t('provider.presetGroupBuiltIn'),
    keywords: [preset.name, preset.id, preset.clientType, preset.registryName ?? '', preset.source],
  })),
])

function getPresetById(id: string | undefined): ProviderPreset | null {
  if (!id || id === customPresetId) return null
  return providerPresets.find(preset => preset.id === id) ?? null
}

const apiKeyRequired = computed(() => {
  if (isManagedOAuthClientType(form.values.client_type)) return false
  return selectedPreset.value?.requiresApiKey !== false
})

const clientTypeOptions = computed(() =>
  MANUAL_LLM_CLIENT_TYPE_LIST.map((ct) => ({
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
    const payload: ProvidersCreateRequest = {
      name: String(data.name ?? ''),
      client_type: String(data.client_type ?? ''),
      config,
    }
    const preset = selectedPreset.value
    if (preset) {
      if (preset.icon) {
        payload.icon = preset.icon
      }
      payload.metadata = {
        preset: {
          id: preset.id,
          source: preset.source,
        },
      }
    }
    const { data: result } = await postProviders({ body: payload, throwOnError: true })
    if (data.auto_import && result?.id) {
      try {
        const importResult = props.importModels
          ? await props.importModels(result.id)
          : (await postProvidersByIdImportModels({
              path: { id: result.id },
              throwOnError: true,
            })).data
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
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['providers'] })
    queryCache.invalidateQueries({ key: ['models'] })
  },
})

const providerSchema = toTypedSchema(z.object({
  api_key: z.string().optional(),
  base_url: z.string().optional(),
  name: z.string().min(1, t('provider.nameRequired')),
  client_type: z.string().min(1, t('provider.clientTypeRequired')),
  auto_import: z.boolean().optional(),
}).superRefine((value, ctx) => {
  const requiresApiKey = !isManagedOAuthClientType(value.client_type) && selectedPreset.value?.requiresApiKey !== false
  if (requiresApiKey && !value.api_key?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['api_key'],
      message: t('provider.apiKeyRequired'),
    })
  }
  const requiresBaseUrl = value.client_type !== 'github-copilot' && selectedPreset.value?.requiresBaseUrl !== false
  if (requiresBaseUrl && !value.base_url?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['base_url'],
      message: t('provider.urlRequired'),
    })
  }
}))

const defaultFormValues = {
  api_key: '',
  base_url: '',
  name: '',
  client_type: 'openai-completions',
  auto_import: false,
}

function valuesForPreset(preset: ProviderPreset | null) {
  if (!preset) {
    return { ...defaultFormValues }
  }
  return {
    ...defaultFormValues,
    name: suggestProviderName(preset.name, props.providers),
    base_url: preset.baseUrl,
    client_type: preset.clientType,
  }
}

const form = useForm({
  validationSchema: providerSchema,
  initialValues: valuesForPreset(getPresetById(selectedPresetId.value)),
})

function applyPreset(value: string) {
  selectedPresetId.value = value || defaultPresetId.value
  const preset = selectedPreset.value
  form.setValues(valuesForPreset(preset))
}

function resetCreateForm() {
  selectedPresetId.value = defaultPresetId.value
  form.resetForm({ values: valuesForPreset(selectedPreset.value) })
}

watch(() => form.values.client_type, (clientType) => {
  if (clientType === 'openai-codex' && !form.values.base_url) {
    form.setFieldValue('base_url', 'https://chatgpt.com/backend-api')
  }
  if (clientType === 'github-copilot') {
    form.setFieldValue('base_url', '')
  }
})

watch(open, () => resetCreateForm())

const createProvider = form.handleSubmit(async (value) => {
  await run(
    () => createProviderMutation(value),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {
        open.value = false
        resetCreateForm()
      },
    },
  )
})
</script>
