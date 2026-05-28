<script setup lang="ts">
import { ref, computed, onMounted, watch, provide, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import {
  postProviders,
  postProvidersByIdImportModels,
  deleteModelsById,
  getProvidersByIdModels,
  getProvidersNameByName,
  putProvidersById,
  type ProvidersCreateRequest,
  type ModelsGetResponse,
} from '@memohai/sdk'
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
} from '@memohai/ui'
import { ArrowLeft, Plus, AlertCircle } from 'lucide-vue-next'
import { useOnboarding } from '@/composables/useOnboarding'
import ProviderIcon from '@/components/provider-icon/index.vue'
import CreateModel from '@/components/create-model/index.vue'
import ModelItem from '@/pages/providers/components/model-item.vue'
import { providerPresets, type ProviderPreset } from '@/constants/provider-presets'
import { LLM_CLIENT_TYPE_LIST } from '@/constants/client-types'

const ADDED_COUNT_KEY = 'onboarding.provider.addedCount'

const { t } = useI18n()
const { nextStep, prevStep } = useOnboarding()

const visible = ref(false)
const listVisible = ref(false)
const exiting = ref(false)
const mode = ref<'list' | 'form'>('list')
const formVisible = ref(false)
const formContentVisible = ref(false)
const selectedPreset = ref<ProviderPreset | null>(null)
const formError = ref('')
const addedCount = ref(0)

const formValues = ref({
  name: '',
  api_key: '',
  base_url: '',
  client_type: 'openai-completions',
})

const createdProviderId = ref<string | null>(null)
const errorState = ref<'http' | 'noModels' | null>(null)
const manualMode = ref(false)
const suppressDirtyReset = ref(false)

const openModelState = ref(false)
const openModelTitle = ref<'edit' | 'title'>('title')
const openModelEdit = ref<ModelsGetResponse | null>(null)

provide('openModel', openModelState)
provide('openModelTitle', openModelTitle)
provide('openModelState', openModelEdit)

const { state: providerModelsState, refresh: refreshProviderModels } = useQuery({
  key: () => ['provider-models', createdProviderId.value ?? 'none'],
  query: async () => {
    if (!createdProviderId.value) return [] as ModelsGetResponse[]
    const { data } = await getProvidersByIdModels({
      path: { id: createdProviderId.value },
      throwOnError: true,
    })
    return data ?? []
  },
  enabled: () => !!createdProviderId.value && manualMode.value,
})

const providerModels = computed<ModelsGetResponse[]>(() => providerModelsState.value.data ?? [])

const availableClientTypes = computed(() =>
  LLM_CLIENT_TYPE_LIST.filter(ct => !['openai-codex', 'github-copilot'].includes(ct.value)),
)

const baseUrlPlaceholder = computed(() => {
  switch (formValues.value.client_type) {
    case 'anthropic-messages':
      return 'https://api.anthropic.com'
    case 'google-generative-ai':
      return 'https://generativelanguage.googleapis.com/v1beta'
    default:
      return 'https://api.example.com/v1'
  }
})

const queryCache = useQueryCache()

const { mutateAsync: createProvider } = useMutation({
  mutation: async (payload: ProvidersCreateRequest) => {
    const { data } = await postProviders({ body: payload, throwOnError: true })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['providers'] })
  },
})

const { mutateAsync: importModels, isLoading: importing } = useMutation({
  mutation: async (providerId: string) => {
    const { data } = await postProvidersByIdImportModels({
      path: { id: providerId },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['models'] })
    queryCache.invalidateQueries({ key: ['provider-models'] })
  },
})

const { mutateAsync: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: async (id: string) => {
    await deleteModelsById({ path: { id }, throwOnError: true })
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['models'] })
    queryCache.invalidateQueries({ key: ['provider-models'] })
  },
})

const submitting = computed(() => importing.value)

const ctaLabel = computed(() => addedCount.value > 0 ? t('onboarding.next') : t('onboarding.skip'))

const formCtaLabel = computed(() => {
  if (importing.value) return t('onboarding.provider.form.importing')
  return t('onboarding.next')
})

const formCtaDisabled = computed(() => {
  if (importing.value) return true
  if (manualMode.value) return providerModels.value.length === 0
  if (errorState.value) return true
  return false
})

function resetFormState() {
  createdProviderId.value = null
  errorState.value = null
  manualMode.value = false
  openModelState.value = false
  openModelTitle.value = 'title'
  openModelEdit.value = null
}

function openForm(preset: ProviderPreset | null) {
  selectedPreset.value = preset
  suppressDirtyReset.value = true
  formValues.value = preset
    ? { name: preset.name, api_key: '', base_url: preset.baseUrl, client_type: preset.clientType }
    : { name: '', api_key: '', base_url: '', client_type: 'openai-completions' }
  formError.value = ''
  resetFormState()
  listVisible.value = false
  setTimeout(() => {
    mode.value = 'form'
    formVisible.value = false
    formContentVisible.value = false
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        formVisible.value = true
        formContentVisible.value = true
        suppressDirtyReset.value = false
      })
    })
  }, 175)
}

function backToList() {
  formVisible.value = false
  formContentVisible.value = false
  setTimeout(() => {
    mode.value = 'list'
    selectedPreset.value = null
    resetFormState()
    listVisible.value = false
    visible.value = false
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        listVisible.value = true
        visible.value = true
      })
    })
  }, 175)
}

async function ensureProviderCreated(): Promise<string | null> {
  if (createdProviderId.value) return createdProviderId.value

  const name = formValues.value.name.trim()
  const apiKey = formValues.value.api_key.trim()
  const baseUrl = formValues.value.base_url.trim()
  if (!name || !apiKey || !baseUrl) {
    formError.value = t('onboarding.provider.form.requiredError')
    return null
  }
  formError.value = ''

  try {
    const lookupName = selectedPreset.value?.registryName ?? name
    const { data: existing } = await getProvidersNameByName({
      path: { name: lookupName },
    })

    if (existing?.id) {
      await putProvidersById({
        path: { id: existing.id },
        body: { config: { base_url: baseUrl, api_key: apiKey }, enable: true },
        throwOnError: true,
      })
      createdProviderId.value = existing.id
      return existing.id
    }

    const result = await createProvider({
      name,
      client_type: formValues.value.client_type,
      config: { base_url: baseUrl, api_key: apiKey },
    } as ProvidersCreateRequest)
    if (!result?.id) {
      errorState.value = 'http'
      return null
    }
    createdProviderId.value = result.id
    return result.id
  }
  catch (e) {
    formError.value = (e as Error).message || t('onboarding.provider.form.saveFailed')
    return null
  }
}

async function runImport(providerId: string) {
  errorState.value = null
  let importFailed = false
  try {
    await importModels(providerId)
  } catch {
    importFailed = true
  }

  try {
    const { data: models } = await getProvidersByIdModels({
      path: { id: providerId },
    })
    if (models && models.length > 0) {
      addedCount.value++
      sessionStorage.setItem(ADDED_COUNT_KEY, String(addedCount.value))
      go(nextStep)
      return
    }
  } catch {
    errorState.value = 'http'
    return
  }

  errorState.value = importFailed ? 'http' : 'noModels'
}

async function saveAndNext() {
  if (manualMode.value) {
    if (providerModels.value.length === 0) return
    addedCount.value++
    sessionStorage.setItem(ADDED_COUNT_KEY, String(addedCount.value))
    go(nextStep)
    return
  }

  const providerId = await ensureProviderCreated()
  if (!providerId) return
  await runImport(providerId)
}

async function onRetry() {
  if (!createdProviderId.value) {
    await saveAndNext()
    return
  }
  await runImport(createdProviderId.value)
}

async function onEnterManual() {
  if (!createdProviderId.value) return
  errorState.value = null
  manualMode.value = true
  await refreshProviderModels()
  await nextTick()
  openModelEdit.value = null
  openModelTitle.value = 'title'
  openModelState.value = true
}

function onSkipStep() {
  if (createdProviderId.value) {
    addedCount.value++
    sessionStorage.setItem(ADDED_COUNT_KEY, String(addedCount.value))
  }
  go(nextStep)
}

function handleEditModel(model: ModelsGetResponse) {
  openModelEdit.value = { ...model }
  openModelTitle.value = 'edit'
  openModelState.value = true
}

async function handleDeleteModel(id: string) {
  if (!id) return
  await deleteModel(id)
  await refreshProviderModels()
}

watch(
  () => [formValues.value.name, formValues.value.api_key, formValues.value.base_url, formValues.value.client_type],
  () => {
    if (suppressDirtyReset.value) return
    if (manualMode.value) return
    if (createdProviderId.value) {
      createdProviderId.value = null
    }
    if (errorState.value) {
      errorState.value = null
    }
  },
)

onMounted(() => {
  const stored = sessionStorage.getItem(ADDED_COUNT_KEY)
  if (stored !== null) {
    const parsed = Number.parseInt(stored, 10)
    if (Number.isFinite(parsed) && parsed >= 0) addedCount.value = parsed
  }
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      listVisible.value = true
      visible.value = true
    })
  })

  if (import.meta.env.DEV) {
    ;(window as unknown as Record<string, unknown>).__step3 = {
      showError(kind: 'http' | 'noModels' = 'noModels') {
        createdProviderId.value = 'mock-provider-id'
        errorState.value = kind
        manualMode.value = false
        console.info(`[step3] error state -> ${kind}`)
      },
      showManual() {
        createdProviderId.value = 'mock-provider-id'
        errorState.value = null
        manualMode.value = true
        console.info('[step3] manual mode (use real API for adds, models won\'t persist with mock id)')
      },
      openAddDialog() {
        openModelEdit.value = null
        openModelTitle.value = 'title'
        openModelState.value = true
      },
      reset() {
        resetFormState()
        console.info('[step3] reset')
      },
      state() {
        return {
          mode: mode.value,
          createdProviderId: createdProviderId.value,
          errorState: errorState.value,
          manualMode: manualMode.value,
          providerModels: providerModels.value,
          importing: importing.value,
        }
      },
    }
    console.info('[step3] dev helpers: __step3.showError("http"|"noModels"), __step3.showManual(), __step3.openAddDialog(), __step3.reset()')
  }
})

function go(action: () => void) {
  exiting.value = true
  setTimeout(action, 175)
}
</script>

<template>
  <div
    class="transition-all duration-[175ms] ease-out"
    :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
  >
    <div
      v-if="mode === 'list'"
      class="text-left pt-24 min-h-[542px] flex flex-col transition-all duration-[175ms] ease-out"
      :class="listVisible ? 'scale-100 opacity-100' : 'scale-[0.96] opacity-0'"
    >
      <h2
        class="text-3xl font-semibold mb-3 transition-all duration-[350ms] ease-out"
        :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
      >
        {{ t('onboarding.provider.title') }}
      </h2>

      <div>
        <p
          class="text-sm text-muted-foreground leading-relaxed mb-6 transition-all duration-[350ms] ease-out delay-[60ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          {{ t('onboarding.provider.description') }}
        </p>
        <div
          class="grid grid-cols-3 gap-3 transition-all duration-[350ms] ease-out delay-[140ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          <button
            type="button"
            class="h-16 rounded-lg border border-dashed border-border bg-background px-3 flex items-center gap-2.5 text-muted-foreground transition-colors hover:border-foreground/50 hover:text-foreground"
            @click="openForm(null)"
          >
            <Plus class="size-5 shrink-0" />
            <span class="text-sm font-medium truncate">{{ t('onboarding.provider.custom') }}</span>
          </button>
          <button
            v-for="preset in providerPresets"
            :key="preset.id"
            type="button"
            class="h-16 rounded-lg border border-border bg-background px-3 flex items-center gap-2.5 transition-colors hover:border-muted-foreground/50 hover:bg-accent/40"
            @click="openForm(preset)"
          >
            <ProviderIcon
              :icon="preset.icon"
              size="22"
            />
            <span class="text-sm font-medium truncate">
              {{ preset.name }}
            </span>
          </button>
        </div>
      </div>

      <div
        class="mt-auto pt-12 flex items-center justify-end gap-3 transition-all duration-[350ms] ease-out delay-[220ms]"
        :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
      >
        <button
          class="inline-flex h-[42px] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          @click="go(prevStep)"
        >
          {{ t('onboarding.prev') }}
        </button>
        <button
          class="inline-flex h-[42px] w-[180px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          @click="go(nextStep)"
        >
          {{ ctaLabel }}
        </button>
      </div>
    </div>

    <div
      v-else
      class="text-left pt-24 min-h-[542px] flex flex-col transition-all duration-[175ms] ease-out"
      :class="formVisible ? 'scale-100 opacity-100' : 'scale-[0.96] opacity-0'"
    >
      <div
        class="mb-8 flex items-center gap-3 transition-all duration-[200ms] ease-out"
        :class="formContentVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
      >
        <button
          type="button"
          class="-ml-1.5 inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          :disabled="submitting"
          @click="backToList"
        >
          <ArrowLeft class="size-4" />
        </button>
        <ProviderIcon
          v-if="selectedPreset"
          :icon="selectedPreset.icon"
          size="28"
        />
        <h2 class="text-2xl font-semibold">
          {{ selectedPreset ? selectedPreset.name : t('onboarding.provider.custom') }}
        </h2>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto -mx-2 px-2 -my-1 py-1">
        <div class="space-y-4">
          <div
            class="transition-all duration-[200ms] ease-out delay-[20ms]"
            :class="formContentVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
          >
            <Label class="mb-2 block text-sm font-medium">
              {{ t('onboarding.provider.form.name') }}
            </Label>
            <Input
              v-model="formValues.name"
              :placeholder="t('onboarding.provider.form.namePlaceholder')"
            />
          </div>
          <div
            v-if="!selectedPreset"
            class="transition-all duration-[200ms] ease-out delay-[40ms]"
            :class="formContentVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
          >
            <Label class="mb-2 block text-sm font-medium">
              {{ t('onboarding.provider.form.clientType') }}
            </Label>
            <Select v-model="formValues.client_type">
              <SelectTrigger class="w-full">
                <SelectValue :placeholder="t('onboarding.provider.form.clientTypePlaceholder')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="ct in availableClientTypes"
                  :key="ct.value"
                  :value="ct.value"
                >
                  {{ ct.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div
            class="transition-all duration-[200ms] ease-out"
            :class="[
              formContentVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3',
              selectedPreset ? 'delay-[40ms]' : 'delay-[60ms]',
            ]"
          >
            <Label class="mb-2 block text-sm font-medium">
              {{ t('onboarding.provider.form.apiKey') }}
            </Label>
            <Input
              v-model="formValues.api_key"
              type="password"
              autocomplete="off"
              :placeholder="t('onboarding.provider.form.apiKeyPlaceholder')"
            />
          </div>
          <div
            class="transition-all duration-[200ms] ease-out"
            :class="[
              formContentVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3',
              selectedPreset ? 'delay-[60ms]' : 'delay-[80ms]',
            ]"
          >
            <Label class="mb-2 block text-sm font-medium">
              {{ t('onboarding.provider.form.baseUrl') }}
            </Label>
            <Input
              v-model="formValues.base_url"
              :placeholder="baseUrlPlaceholder"
            />
          </div>
        </div>

        <p
          v-if="formError"
          class="mt-3 text-xs text-destructive"
        >
          {{ formError }}
        </p>

        <div
          v-if="errorState"
          class="mt-5 rounded-lg border border-destructive/30 bg-destructive/5 p-4"
        >
          <div class="flex items-start gap-3">
            <AlertCircle class="size-5 shrink-0 text-destructive mt-0.5" />
            <div class="flex-1">
              <p class="text-sm font-medium text-destructive">
                {{ errorState === 'http'
                  ? t('onboarding.provider.form.errorHttpTitle')
                  : t('onboarding.provider.form.errorNoModelsTitle') }}
              </p>
              <p class="mt-1 text-xs text-muted-foreground leading-relaxed">
                {{ errorState === 'http'
                  ? t('onboarding.provider.form.errorHttpDescription')
                  : t('onboarding.provider.form.errorNoModelsDescription') }}
              </p>
              <div class="mt-3 flex items-center gap-2">
                <button
                  type="button"
                  class="inline-flex h-8 items-center justify-center rounded-md border border-border bg-background px-3 text-xs font-medium transition-colors hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed"
                  :disabled="importing"
                  @click="onRetry"
                >
                  {{ t('onboarding.provider.form.retry') }}
                </button>
                <button
                  type="button"
                  class="inline-flex h-8 items-center justify-center rounded-md border border-border bg-background px-3 text-xs font-medium transition-colors hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed"
                  :disabled="importing || !createdProviderId"
                  @click="onEnterManual"
                >
                  {{ t('onboarding.provider.form.manualAdd') }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="errorState && createdProviderId"
          class="mt-2 text-right"
        >
          <button
            type="button"
            class="text-xs text-muted-foreground transition-colors hover:text-foreground underline-offset-2 hover:underline"
            @click="onSkipStep"
          >
            {{ t('onboarding.provider.form.skipStep') }}
          </button>
        </div>

        <div
          v-if="manualMode && createdProviderId"
          class="mt-6"
        >
          <div class="flex items-center justify-between mb-3">
            <h4 class="text-sm font-semibold">
              {{ t('models.title') }}
              <span
                v-if="providerModels.length > 0"
                class="ml-2 text-xs font-normal text-muted-foreground"
              >
                {{ providerModels.length }}
              </span>
            </h4>
            <CreateModel :id="createdProviderId" />
          </div>

          <div
            v-if="providerModels.length === 0"
            class="rounded-lg border border-dashed border-border py-8 text-center"
          >
            <p class="text-sm text-muted-foreground">
              {{ t('models.emptyTitle') }}
            </p>
            <p class="mt-1 text-xs text-muted-foreground">
              {{ t('onboarding.provider.form.manualAddEmpty') }}
            </p>
          </div>

          <div
            v-else
            class="grid gap-3 grid-cols-1 sm:grid-cols-2"
          >
            <ModelItem
              v-for="model in providerModels"
              :key="model.id || `${model.provider_id}:${model.model_id}`"
              :model="model"
              :delete-loading="deleteModelLoading"
              @edit="handleEditModel"
              @delete="handleDeleteModel"
            />
          </div>
        </div>
      </div>

      <div
        class="mt-auto pt-12 flex items-center justify-end gap-3 transition-all duration-[200ms] ease-out"
        :class="[
          formContentVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3',
          selectedPreset ? 'delay-[80ms]' : 'delay-[100ms]',
        ]"
      >
        <button
          class="inline-flex h-[42px] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed"
          :disabled="submitting"
          @click="backToList"
        >
          {{ t('onboarding.provider.form.cancel') }}
        </button>
        <button
          class="inline-flex h-[42px] w-[180px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-60 disabled:cursor-not-allowed"
          :disabled="formCtaDisabled"
          @click="saveAndNext"
        >
          <Transition
            mode="out-in"
            enter-active-class="transition-all duration-[160ms] ease-out"
            enter-from-class="opacity-0 translate-y-1"
            enter-to-class="opacity-100 translate-y-0"
            leave-active-class="transition-all duration-[140ms] ease-in"
            leave-from-class="opacity-100 translate-y-0"
            leave-to-class="opacity-0 -translate-y-1"
          >
            <span :key="formCtaLabel" class="inline-flex items-center gap-2">
              <Spinner v-if="importing" />
              {{ formCtaLabel }}
            </span>
          </Transition>
        </button>
      </div>
    </div>
  </div>
</template>
