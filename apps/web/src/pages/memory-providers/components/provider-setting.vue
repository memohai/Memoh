<template>
  <div
    v-if="curProvider"
    class="p-6 space-y-6"
  >
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-semibold">
          {{ curProvider.name }}
        </h3>
        <p class="text-sm text-muted-foreground mt-0.5">
          {{ $t(`memoryProvider.providerNames.${curProvider.provider}`, curProvider.provider) }}
        </p>
      </div>
      <ConfirmPopover
        :message="$t('memoryProvider.deleteConfirm')"
        @confirm="handleDelete"
      >
        <template #trigger>
          <Button
            variant="destructive"
            size="sm"
            :disabled="deleteLoading"
          >
            <Spinner
              v-if="deleteLoading"
              class="mr-1.5"
            />
            {{ $t('common.delete') }}
          </Button>
        </template>
      </ConfirmPopover>
    </div>

    <Separator />

    <!-- Name -->
    <div class="space-y-2">
      <Label>{{ $t('memoryProvider.name') }}</Label>
      <Input
        v-model="form.name"
        :placeholder="$t('memoryProvider.namePlaceholder')"
      />
    </div>

    <!-- Builtin Config (model selectors) -->
    <template v-if="curProvider.provider === 'builtin'">
      <div class="space-y-2">
        <Label>{{ $t('memoryProvider.builtinMode') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('memoryProvider.builtinModeDescription') }}
        </p>
        <div class="inline-flex rounded-xl border border-border bg-muted/70 p-1">
          <div class="relative grid grid-cols-3">
            <div
              class="absolute inset-y-0 left-0 w-1/3 rounded-lg bg-card shadow-sm ring-1 ring-border/60 transition-transform duration-200 ease-out"
              :class="builtinModeHighlightClass"
            />
            <button
              type="button"
              class="relative z-10 rounded-lg px-4 py-2 text-sm font-medium transition-colors duration-200"
              :class="builtinModeButtonClass('off')"
              @click="handleBuiltinModeChange('off')"
            >
              {{ $t('memoryProvider.modeNames.off') }}
            </button>
            <button
              type="button"
              class="relative z-10 rounded-lg px-4 py-2 text-sm font-medium transition-colors duration-200"
              :class="builtinModeButtonClass('sparse')"
              @click="handleBuiltinModeChange('sparse')"
            >
              {{ $t('memoryProvider.modeNames.sparse') }}
            </button>
            <button
              type="button"
              class="relative z-10 rounded-lg px-4 py-2 text-sm font-medium transition-colors duration-200"
              :class="builtinModeButtonClass('dense')"
              @click="handleBuiltinModeChange('dense')"
            >
              {{ $t('memoryProvider.modeNames.dense') }}
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="builtinMode === 'off'"
        class="rounded-lg border border-border bg-card p-4 space-y-2"
      >
        <h4 class="text-sm font-medium">
          {{ $t('memoryProvider.modeNames.off') }}
        </h4>
        <p class="text-sm text-muted-foreground">
          {{ $t('memoryProvider.modeDescriptions.off') }}
        </p>
      </div>

      <div
        v-if="builtinMode === 'sparse'"
        class="rounded-lg border border-border bg-card p-4 space-y-4"
      >
        <div class="space-y-1">
          <h4 class="text-sm font-medium">
            {{ $t('memoryProvider.sparseSectionTitle') }}
          </h4>
          <p class="text-sm text-muted-foreground">
            {{ $t('memoryProvider.modeDescriptions.sparse') }}
          </p>
        </div>

        <div class="rounded-md border border-border bg-background px-3 py-2 text-sm text-muted-foreground">
          {{ $t('memoryProvider.sparseInstallHint') }}
        </div>
      </div>

      <div
        v-if="builtinMode === 'dense'"
        class="rounded-lg border border-border bg-card p-4 space-y-4"
      >
        <div class="space-y-1">
          <h4 class="text-sm font-medium">
            {{ $t('memoryProvider.denseSectionTitle') }}
          </h4>
          <p class="text-sm text-muted-foreground">
            {{ $t('memoryProvider.modeDescriptions.dense') }}
          </p>
        </div>

        <div class="space-y-2">
          <Label>{{ $t('memoryProvider.denseEmbeddingModel') }}</Label>
          <p class="text-xs text-muted-foreground">
            {{ $t('memoryProvider.denseEmbeddingModelDescription') }}
          </p>
          <ModelSelect
            v-model="configForm.embedding_model_id"
            :models="models"
            :providers="providers"
            model-type="embedding"
            :placeholder="$t('memoryProvider.denseEmbeddingModel')"
          />
        </div>

        <div class="rounded-md border border-border bg-background px-3 py-2 text-sm text-muted-foreground">
          {{ $t('memoryProvider.denseQdrantHint') }}
        </div>
      </div>

      <div
        v-if="builtinCollections.length > 0"
        class="grid gap-3 md:grid-cols-2"
      >
        <div
          v-for="collection in builtinCollections"
          :key="collection.name"
          class="rounded-lg border border-border bg-background/70 p-4 space-y-2"
        >
          <div class="flex items-center justify-between gap-3">
            <p class="text-sm font-medium text-foreground break-all">
              {{ collection.name }}
            </p>
            <span
              class="text-xs"
              :class="collection.qdrant?.ok ? 'text-foreground' : 'text-destructive'"
            >
              {{ collection.qdrant?.ok ? $t('memoryProvider.collectionHealthy') : $t('memoryProvider.collectionUnavailable') }}
            </span>
          </div>
          <p class="text-2xl font-semibold text-foreground">
            {{ collection.points ?? 0 }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ $t('memoryProvider.collectionPoints') }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ collection.exists ? $t('memoryProvider.collectionExists') : $t('memoryProvider.collectionMissing') }}
          </p>
        </div>
      </div>
    </template>

    <!-- Remote provider config (mem0 / openviking) -->
    <template v-if="curProvider.provider !== 'builtin' && providerSchema">
      <div
        v-for="(fieldSchema, fieldKey) in providerSchema.fields"
        :key="fieldKey"
        class="space-y-2"
      >
        <Label>
          {{ fieldSchema.title || fieldKey }}
          <span
            v-if="fieldSchema.required"
            class="text-destructive"
          >*</span>
        </Label>
        <p
          v-if="fieldSchema.description"
          class="text-xs text-muted-foreground"
        >
          {{ fieldSchema.description }}
        </p>
        <Input
          v-model="configForm[fieldKey]"
          :type="fieldSchema.secret ? 'password' : 'text'"
          :placeholder="fieldSchema.example ? String(fieldSchema.example) : ''"
        />
      </div>
    </template>

    <div class="flex justify-end">
      <Button
        :disabled="saveLoading"
        @click="handleSave"
      >
        <Spinner
          v-if="saveLoading"
          class="mr-1.5"
        />
        {{ $t('common.save') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject, ref, reactive, watch, computed, type Ref } from 'vue'
import {
  Button,
  Input,
  Label,
  Separator,
  Spinner,
} from '@memohai/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getModels, getProviders, getMemoryProvidersMeta, getMemoryProvidersByIdStatus, putMemoryProvidersById, deleteMemoryProvidersById } from '@memohai/sdk'
import type { AdaptersProviderGetResponse, AdaptersProviderMeta, AdaptersProviderStatusResponse } from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelSelect from '@/pages/bots/components/model-select.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const curProvider = inject<Ref<AdaptersProviderGetResponse | null>>('curMemoryProvider')

const form = reactive({ name: '' })
const configForm = reactive<Record<string, string>>({})

const saveLoading = ref(false)
const deleteLoading = ref(false)

const { data: modelData } = useQuery({
  key: ['all-models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})
const { data: providerData } = useQuery({
  key: ['all-providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})
const { data: metaData } = useQuery({
  key: ['memory-providers-meta'],
  query: async () => {
    const { data } = await getMemoryProvidersMeta({ throwOnError: true })
    return data
  },
})
const { data: providerStatusData } = useQuery({
  key: () => ['memory-provider-status', curProvider?.value?.id ?? ''],
  query: async () => {
    const providerId = curProvider?.value?.id
    if (!providerId) return null
    const { data } = await getMemoryProvidersByIdStatus({
      path: { id: providerId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!curProvider?.value?.id,
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])

const providerSchema = computed(() => {
  if (!curProvider?.value || !metaData.value) return null
  const meta = (metaData.value as AdaptersProviderMeta[])?.find(
    (m) => m.provider === curProvider.value.provider,
  )
  return meta?.config_schema ?? null
})

const builtinMode = computed(() => {
  if (curProvider?.value?.provider !== 'builtin') return 'off'
  return configForm.memory_mode || 'off'
})
const providerStatus = computed(() => providerStatusData.value as AdaptersProviderStatusResponse | null)
const builtinCollections = computed(() => providerStatus.value?.collections ?? [])

const builtinModeHighlightClass = computed(() => {
  if (builtinMode.value === 'sparse') return 'translate-x-full'
  if (builtinMode.value === 'dense') return 'translate-x-[200%]'
  return 'translate-x-0'
})

watch(curProvider!, (val) => {
  if (val) {
    form.name = val.name ?? ''
    Object.keys(configForm).forEach((k) => delete configForm[k])
    if (val.config) {
      Object.entries(val.config).forEach(([k, v]) => {
        configForm[k] = (v as string) ?? ''
      })
    }
    if (val.provider === 'builtin') {
      if (!configForm.memory_mode) configForm.memory_mode = 'off'
      if (!configForm.embedding_model_id) configForm.embedding_model_id = ''
    }
  }
}, { immediate: true })

function handleBuiltinModeChange(value: string | undefined) {
  configForm.memory_mode = value || 'off'
}

function builtinModeButtonClass(mode: string) {
  return builtinMode.value === mode
    ? 'text-foreground'
    : 'text-muted-foreground hover:text-foreground/90'
}

async function handleSave() {
  if (!curProvider?.value) return
  saveLoading.value = true
  try {
    const config: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(configForm)) {
      if (v) config[k] = v
    }
    const { data } = await putMemoryProvidersById({
      path: { id: curProvider.value.id! },
      body: { name: form.name.trim(), config },
      throwOnError: true,
    })
    if (curProvider?.value && data) {
      Object.assign(curProvider.value, data)
    }
    toast.success(t('memoryProvider.saveSuccess'))
    queryCache.invalidateQueries({ key: ['memory-providers'] })
  } catch (error) {
    console.error('Failed to save:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    saveLoading.value = false
  }
}

async function handleDelete() {
  if (!curProvider?.value) return
  deleteLoading.value = true
  try {
    await deleteMemoryProvidersById({
      path: { id: curProvider.value.id! },
      throwOnError: true,
    })
    toast.success(t('memoryProvider.deleteSuccess'))
    queryCache.invalidateQueries({ key: ['memory-providers'] })
  } catch (error) {
    console.error('Failed to delete:', error)
    toast.error(t('memoryProvider.deleteFailed'))
  } finally {
    deleteLoading.value = false
  }
}
</script>
