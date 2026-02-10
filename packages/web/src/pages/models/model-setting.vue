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
import { type ProviderInfo, type ModelInfo } from '@memoh/shared'
import {
  useUpdateProvider,
  useDeleteProvider,
} from '@/composables/api/useProviders'
import {
  useModelList,
  useDeleteModel,
} from '@/composables/api/useModels'

// ---- Model 编辑状态（provide 给 CreateModel） ----
const openModel = reactive<{
  state: boolean
  title: 'title' | 'edit'
  curState: ModelInfo | null
}>({
  state: false,
  title: 'title',
  curState: null,
})

provide('openModel', toRef(openModel, 'state'))
provide('openModelTitle', toRef(openModel, 'title'))
provide('openModelState', toRef(openModel, 'curState'))

function handleEditModel(model: ModelInfo) {
  openModel.state = true
  openModel.title = 'edit'
  openModel.curState = { ...model }
}

// ---- 当前 Provider ----
const curProvider = inject('curProvider', ref<Partial<ProviderInfo & { id: string }>>())
const curProviderId = computed(() => curProvider.value?.id)

// ---- API Hooks ----
const { mutate: deleteProvider, isLoading: deleteLoading } = useDeleteProvider(curProviderId)
const { mutate: changeProvider, isLoading: editLoading } = useUpdateProvider(curProviderId)
const { mutate: deleteModel, isLoading: deleteModelLoading } = useDeleteModel()
const { data: modelDataList, invalidate: invalidateModels } = useModelList(curProviderId)

watch(curProvider, () => {
  invalidateModels()
}, { immediate: true })
</script>
