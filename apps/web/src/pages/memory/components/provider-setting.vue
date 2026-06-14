<template>
  <SettingsShell
    v-if="curProvider"
    width="narrow"
    class="space-y-6"
  >
    <div class="flex items-center justify-between gap-3">
      <div class="min-w-0">
        <h3 class="truncate text-sm font-semibold">
          {{ curProvider.name }}
        </h3>
        <p class="mt-0.5 text-xs text-muted-foreground">
          {{ $t(`memory.providerNames.${curProvider.provider}`, curProvider.provider ?? '') }}
        </p>
      </div>
      <ConfirmPopover
        :message="$t('memory.deleteConfirm')"
        @confirm="handleDelete"
      >
        <template #trigger>
          <Button
            variant="outline"
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

    <div class="space-y-2">
      <Label>{{ $t('memory.name') }}</Label>
      <Input
        v-model="form.name"
        :placeholder="$t('memory.namePlaceholder')"
      />
    </div>

    <div
      v-if="providerSchema"
      class="grid gap-4 md:grid-cols-2"
    >
      <div
        v-for="(fieldSchema, fieldKey) in providerSchema.fields"
        :key="fieldKey"
        class="space-y-2"
        :class="isWideField(fieldKey, fieldSchema) ? 'md:col-span-2' : ''"
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
    </div>

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
  </SettingsShell>
</template>

<script setup lang="ts">
import { inject, reactive, watch, computed, ref, type Ref } from 'vue'
import {
  Button,
  Input,
  Label,
  Separator,
  Spinner,
} from '@memohai/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getMemoryProvidersMeta, putMemoryProvidersById, deleteMemoryProvidersById } from '@memohai/sdk'
import type { AdaptersProviderGetResponse, AdaptersProviderMeta } from '@memohai/sdk'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import SettingsShell from '@/components/settings-shell/index.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const curProvider = inject<Ref<AdaptersProviderGetResponse | null>>('curMemoryProvider')

const form = reactive({ name: '' })
const configForm = reactive<Record<string, string>>({})

const saveLoading = ref(false)
const deleteLoading = ref(false)

const { data: metaData } = useQuery({
  key: ['memory-providers-meta'],
  query: async () => {
    const { data } = await getMemoryProvidersMeta({ throwOnError: true })
    return data
  },
})

const providerSchema = computed(() => {
  if (!curProvider?.value || !metaData.value) return null
  const meta = (metaData.value as AdaptersProviderMeta[])?.find(
    (m) => m.provider === curProvider.value?.provider,
  )
  return meta?.config_schema ?? null
})

// Heuristic: URL / endpoint / api-key / secret / long descriptions get full width.
function isWideField(key: string | number, schema: { secret?: boolean; type?: string; description?: string }) {
  const keyStr = String(key).toLowerCase()
  if (schema.secret) return true
  if (keyStr.includes('url') || keyStr.includes('endpoint') || keyStr.includes('key') || keyStr.includes('token') || keyStr.includes('path') || keyStr.includes('uri')) return true
  if ((schema.description ?? '').length > 80) return true
  return false
}

watch(curProvider!, (val) => {
  if (val) {
    form.name = val.name ?? ''
    Object.keys(configForm).forEach((k) => delete configForm[k])
    if (val.config) {
      Object.entries(val.config).forEach(([k, v]) => {
        configForm[k] = (v as string) ?? ''
      })
    }
  }
}, { immediate: true })

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
    toast.success(t('memory.saveSuccess'))
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
    toast.success(t('memory.deleteSuccess'))
    queryCache.invalidateQueries({ key: ['memory-providers'] })
  } catch (error) {
    console.error('Failed to delete:', error)
    toast.error(t('memory.deleteFailed'))
  } finally {
    deleteLoading.value = false
  }
}
</script>
