<template>
  <SettingsShell width="narrow">
    <div class="space-y-6">
      <section class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3">
        <span class="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted">
          <ProviderIcon
            v-if="curProvider?.icon"
            :icon="curProvider.icon"
            size="1.5em"
          />
          <span
            v-else
            class="text-xs font-medium text-muted-foreground"
          >
            {{ getInitials(curProvider?.name) }}
          </span>
        </span>
        <div class="min-w-0 flex-1">
          <h4 class="scroll-m-20 tracking-tight truncate">
            {{ curProvider?.name }}
          </h4>
        </div>
        <div class="ml-auto flex items-center gap-2">
          <ConfirmPopover
            :message="$t('provider.deleteConfirm')"
            :loading="deleteLoading"
            @confirm="deleteProvider"
          >
            <template #trigger>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                class="text-muted-foreground hover:text-destructive"
                :aria-label="$t('common.delete')"
              >
                <Trash2 class="size-4" />
              </Button>
            </template>
          </ConfirmPopover>
          <Switch
            :model-value="curProvider?.enable ?? true"
            :disabled="!curProvider?.id || enableLoading"
            :aria-label="$t('provider.enable')"
            @update:model-value="handleToggleEnable"
          />
        </div>
      </section>

      <ProviderForm
        :provider="curProvider"
        :edit-loading="editLoading"
        @submit="changeProvider"
      />

      <ModelList
        :provider-id="curProvider?.id"
        :models="modelDataList"
        :delete-model-loading="deleteModelLoading"
        @edit="handleEditModel"
        @delete="deleteModel"
      />
    </div>
  </SettingsShell>
</template>

<script setup lang="ts">
import { Button, Switch } from '@memohai/ui'
import { Trash2 } from 'lucide-vue-next'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import SettingsShell from '@/components/settings-shell/index.vue'

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}
import ProviderForm from './components/provider-form.vue'
import ModelList from './components/model-list.vue'
import { computed, inject, provide, reactive, ref, toRef, watch } from 'vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { putProvidersById, deleteProvidersById, getProvidersByIdModels, deleteModelsById } from '@memohai/sdk'
import type { ModelsGetResponse, ProvidersGetResponse, ProvidersUpdateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'

// ---- Model 编辑状态（provide 给 CreateModel） ----
const openModel = reactive<{
  state: boolean
  title: 'title' | 'edit'
  curState: ModelsGetResponse | null
}>({
  state: false,
  title: 'title',
  curState: null,
})

provide('openModel', toRef(openModel, 'state'))
provide('openModelTitle', toRef(openModel, 'title'))
provide('openModelState', toRef(openModel, 'curState'))

function handleEditModel(model: ModelsGetResponse) {
  openModel.state = true
  openModel.title = 'edit'
  openModel.curState = { ...model }
}

// ---- 当前 Provider ----
const curProvider = inject('curProvider', ref<ProvidersGetResponse>())
const curProviderId = computed(() => curProvider.value?.id)
const enableLoading = ref(false)
const { t } = useI18n()

// ---- API Hooks ----
const queryCache = useQueryCache()

function invalidateProviderQueries() {
  queryCache.invalidateQueries({ key: ['providers'] })
  queryCache.invalidateQueries({ key: ['models'] })
}

function invalidateModelQueries() {
  queryCache.invalidateQueries({ key: ['provider-models'] })
  queryCache.invalidateQueries({ key: ['models'] })
}

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: invalidateProviderQueries,
})

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    if (!curProviderId.value) return
    const { data: result } = await putProvidersById({
      path: { id: curProviderId.value },
      body: data as ProvidersUpdateRequest,
      throwOnError: true,
    })
    return result
  },
  onSettled: invalidateProviderQueries,
})

async function handleToggleEnable(value: boolean) {
  if (!curProviderId.value || !curProvider.value) return

  const prev = curProvider.value.enable ?? true
  curProvider.value = {
    ...curProvider.value,
    enable: value,
  }

  enableLoading.value = true
  try {
    await putProvidersById({
      path: { id: curProviderId.value },
      body: { enable: value },
      throwOnError: true,
    })
    invalidateProviderQueries()
  } catch {
    curProvider.value = {
      ...curProvider.value,
      enable: prev,
    }
    toast.error(t('common.saveFailed'))
  } finally {
    enableLoading.value = false
  }
}

const { mutate: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: async (modelID: string) => {
    if (!modelID) return
    await deleteModelsById({ path: { id: modelID }, throwOnError: true })
  },
  onSettled: invalidateModelQueries,
})

const { data: modelDataList } = useQuery({
  key: () => ['provider-models', curProviderId.value ?? ''],
  query: async () => {
    if (!curProviderId.value) return []
    const { data } = await getProvidersByIdModels({
      path: { id: curProviderId.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!curProviderId.value,
})

watch(curProvider, () => {
  queryCache.invalidateQueries({ key: ['provider-models'] })
}, { immediate: true })
</script>
