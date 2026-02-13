<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <h4 class="scroll-m-20 tracking-tight">
        {{ curProvider?.name }}
      </h4>
    </section>
    <Separator class="mt-4 mb-6" />

    <ProviderForm
      :provider="curProvider"
      :edit-loading="editLoading"
      :delete-loading="deleteLoading"
      @submit="changeProvider"
      @delete="deleteProvider"
    />

    <Separator class="mt-4 mb-6" />

    <ModelList
      :provider-id="curProvider?.id"
      :models="modelDataList"
      :delete-model-loading="deleteModelLoading"
      @edit="handleEditModel"
      @delete="deleteModel"
    />
  </div>
</template>

<script setup lang="ts">
import { Separator } from '@memoh/ui'
import ProviderForm from './components/provider-form.vue'
import ModelList from './components/model-list.vue'
import { computed, inject, provide, reactive, ref, toRef, watch } from 'vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { putProvidersById, deleteProvidersById, getProvidersByIdModels, deleteModelsModelByModelId } from '@memoh/sdk'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memoh/sdk'

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

// ---- API Hooks ----
const queryCache = useQueryCache()

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    if (!curProviderId.value) return
    const { data: result } = await putProvidersById({
      path: { id: curProviderId.value },
      body: data as any,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const { mutate: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: async (modelName: string) => {
    await deleteModelsModelByModelId({ path: { modelId: modelName }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['provider-models'] }),
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
