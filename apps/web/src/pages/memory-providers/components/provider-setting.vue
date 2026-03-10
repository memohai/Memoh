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
        <Label>{{ $t('memoryProvider.memoryModel') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('memoryProvider.memoryModelDescription') }}
        </p>
        <ModelSelect
          v-model="configForm.memory_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('memoryProvider.memoryModel')"
        />
      </div>
      <div class="space-y-2">
        <Label>{{ $t('memoryProvider.embeddingModel') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('memoryProvider.embeddingModelDescription') }}
        </p>
        <ModelSelect
          v-model="configForm.embedding_model_id"
          :models="models"
          :providers="providers"
          model-type="embedding"
          :placeholder="$t('memoryProvider.embeddingModel')"
        />
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
import { Button, Input, Label, Separator, Spinner } from '@memoh/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getModels, getProviders, getMemoryProvidersMeta, putMemoryProvidersById, deleteMemoryProvidersById } from '@memoh/sdk'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelSelect from '@/pages/bots/components/model-select.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const curProvider = inject<Ref<any>>('curMemoryProvider')

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

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])

const providerSchema = computed(() => {
  if (!curProvider?.value || !metaData.value) return null
  const meta = (metaData.value as any[])?.find(
    (m: any) => m.provider === curProvider.value.provider,
  )
  return meta?.config_schema ?? null
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
      if (!configForm.memory_model_id) configForm.memory_model_id = ''
      if (!configForm.embedding_model_id) configForm.embedding_model_id = ''
    }
  }
}, { immediate: true })

async function handleSave() {
  if (!curProvider?.value) return
  saveLoading.value = true
  try {
    const config: Record<string, any> = {}
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
