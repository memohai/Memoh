<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button, Label, Spinner } from '@memohai/ui'
import { Brain, Check } from 'lucide-vue-next'
import {
  getBotsByBotIdMemoryStatus,
  getBotsByBotIdSettings,
  getMemoryProvidersById,
  getModels,
  getProviders,
  postBotsByBotIdMemoryRebuild,
  putMemoryProvidersById,
  type AdaptersMemoryStatusResponse,
  type AdaptersRebuildResult,
  type ModelsGetResponse,
  type ProvidersGetResponse,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import ModelSelect from './model-select.vue'

// Post-import finalize step: a restored bot's file-backed memory (built-in
// sparse/dense Qdrant, or Mem0 SaaS) arrives as files but its derived index is
// stale. This guides the user to rebuild it, and — for dense memory missing a
// usable embedding model — lets them pick one before rebuilding.
const props = defineProps<{ botId: string }>()
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()

const loading = ref(true)
const status = ref<AdaptersMemoryStatusResponse | null>(null)
const memoryProviderId = ref('')
const providerConfig = ref<Record<string, unknown>>({})
const embeddingModelId = ref('')
const models = ref<ModelsGetResponse[]>([])
const providers = ref<ProvidersGetResponse[]>([])
const rebuilding = ref(false)
const savingModel = ref(false)
const result = ref<AdaptersRebuildResult | null>(null)

const providerType = computed(() => status.value?.provider_type ?? '')
const memoryMode = computed(() => status.value?.memory_mode ?? '')
const sourceCount = computed(() => status.value?.source_count ?? 0)
const needsEmbedding = computed(() => providerType.value === 'builtin' && memoryMode.value === 'dense')

const variantKey = computed<'dense' | 'sparse' | 'mem0' | 'generic'>(() => {
  if (providerType.value === 'mem0') return 'mem0'
  if (providerType.value === 'builtin' && memoryMode.value === 'dense') return 'dense'
  if (providerType.value === 'builtin' && memoryMode.value === 'sparse') return 'sparse'
  return 'generic'
})

const canRebuild = computed(() => {
  if (rebuilding.value || savingModel.value || loading.value) return false
  if (needsEmbedding.value) return !!embeddingModelId.value.trim()
  return true
})

onMounted(load)

async function load() {
  loading.value = true
  try {
    const [{ data: st }, { data: settings }] = await Promise.all([
      getBotsByBotIdMemoryStatus({ path: { bot_id: props.botId }, throwOnError: true }),
      getBotsByBotIdSettings({ path: { bot_id: props.botId }, throwOnError: true }),
    ])
    status.value = st ?? null
    memoryProviderId.value = settings?.memory_provider_id ?? ''
    if (needsEmbedding.value) await loadEmbeddingOptions()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.backup.memoryRebuild.statusFailed')))
  } finally {
    loading.value = false
  }
}

async function loadEmbeddingOptions() {
  const tasks: Promise<unknown>[] = [
    getModels({ throwOnError: true }).then(({ data }) => { models.value = data ?? [] }),
    getProviders({ throwOnError: true }).then(({ data }) => { providers.value = data ?? [] }),
  ]
  if (memoryProviderId.value) {
    tasks.push(
      getMemoryProvidersById({ path: { id: memoryProviderId.value }, throwOnError: true }).then(({ data }) => {
        providerConfig.value = (data?.config as Record<string, unknown>) ?? {}
        const current = providerConfig.value.embedding_model_id
        embeddingModelId.value = typeof current === 'string' ? current : ''
      }),
    )
  }
  await Promise.all(tasks)
}

async function handleRebuild() {
  if (!canRebuild.value) return
  try {
    // A changed embedding model is persisted on the shared memory provider
    // before the rebuild so the dense encoder can come up.
    if (needsEmbedding.value && memoryProviderId.value) {
      const current = typeof providerConfig.value.embedding_model_id === 'string' ? providerConfig.value.embedding_model_id : ''
      if (embeddingModelId.value && embeddingModelId.value !== current) {
        savingModel.value = true
        await putMemoryProvidersById({
          path: { id: memoryProviderId.value },
          body: { config: { ...providerConfig.value, memory_mode: 'dense', embedding_model_id: embeddingModelId.value } },
          throwOnError: true,
        })
        providerConfig.value = { ...providerConfig.value, embedding_model_id: embeddingModelId.value }
        savingModel.value = false
      }
    }
    rebuilding.value = true
    const { data } = await postBotsByBotIdMemoryRebuild({ path: { bot_id: props.botId }, throwOnError: true })
    result.value = data ?? null
    toast.success(t('bots.backup.memoryRebuild.success', { count: data?.restored_count ?? 0 }))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.backup.memoryRebuild.failed')))
  } finally {
    rebuilding.value = false
    savingModel.value = false
  }
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3">
    <div class="flex-1 space-y-3 overflow-y-auto px-0.5">
      <div class="flex items-start gap-3 rounded-md border border-border/60 bg-background p-3">
        <div class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Brain class="size-4" />
        </div>
        <div class="min-w-0 flex-1 space-y-0.5">
          <p class="text-xs font-medium">
            {{ t('bots.backup.memoryRebuild.title') }}
          </p>
          <p class="text-[11px] text-muted-foreground">
            {{ t(`bots.backup.memoryRebuild.variant.${variantKey}`) }}
          </p>
        </div>
      </div>

      <div
        v-if="loading"
        class="flex items-center gap-2 rounded-md border border-border/60 bg-background p-3 text-xs text-muted-foreground"
      >
        <Spinner />
        {{ t('bots.backup.memoryRebuild.loading') }}
      </div>

      <!-- Rebuild result -->
      <div
        v-else-if="result"
        class="space-y-1 rounded-md border border-success-border bg-success-soft p-3 text-xs text-success-foreground"
      >
        <div class="flex items-center gap-1.5 font-medium">
          <Check class="size-3.5" />
          {{ t('bots.backup.memoryRebuild.resultTitle') }}
        </div>
        <p class="text-[11px]">
          {{ t('bots.backup.memoryRebuild.resultDetail', { restored: result.restored_count ?? 0, total: result.fs_count ?? 0 }) }}
        </p>
      </div>

      <template v-else>
        <!-- Dense: embedding model picker -->
        <div
          v-if="needsEmbedding"
          class="space-y-2 rounded-md border border-border/60 bg-background p-3"
        >
          <Label class="text-xs">{{ t('bots.backup.memoryRebuild.selectModel') }}</Label>
          <p class="text-[11px] text-muted-foreground">
            {{ t('bots.backup.memoryRebuild.selectModelHint') }}
          </p>
          <ModelSelect
            v-model="embeddingModelId"
            :models="models"
            :providers="providers"
            model-type="embedding"
            :placeholder="t('bots.backup.memoryRebuild.selectModelPlaceholder')"
          />
          <p
            v-if="!embeddingModelId"
            class="text-[11px] text-warning-foreground"
          >
            {{ t('bots.backup.memoryRebuild.selectModelRequired') }}
          </p>
        </div>

        <p
          v-if="sourceCount > 0"
          class="px-0.5 text-[11px] text-muted-foreground"
        >
          {{ t('bots.backup.memoryRebuild.sourceInfo', { count: sourceCount }) }}
        </p>
      </template>
    </div>

    <!-- Actions -->
    <div class="shrink-0 space-y-2 border-t pt-3">
      <div class="flex justify-end gap-2">
        <Button
          variant="ghost"
          size="sm"
          :disabled="rebuilding || savingModel"
          @click="emit('done')"
        >
          {{ result ? t('common.done') : t('bots.backup.memoryRebuild.skip') }}
        </Button>
        <Button
          v-if="!result"
          size="sm"
          :disabled="!canRebuild"
          @click="handleRebuild"
        >
          <Spinner
            v-if="rebuilding || savingModel"
            class="mr-1.5"
          />
          {{ t(`bots.backup.memoryRebuild.action.${variantKey}`) }}
        </Button>
      </div>
    </div>
  </div>
</template>
