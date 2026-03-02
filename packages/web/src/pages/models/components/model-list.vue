<template>
  <section>
    <section class="flex justify-between items-center mb-4">
      <h4 class="scroll-m-20 font-semibold tracking-tight">
        {{ $t('models.title') }}
      </h4>
      <div
        v-if="providerId"
        class="flex items-center gap-2 ml-auto"
      >
        <ImportModelsDialog :provider-id="providerId" />
        <CreateModel :id="providerId" />
      </div>
    </section>

    <section
      v-if="models && models.length > 0"
      class="flex flex-col gap-4"
    >
      <ModelItem
        v-for="model in models"
        :key="model.id || `${model.llm_provider_id}:${model.model_id}`"
        :model="model"
        :delete-loading="deleteModelLoading"
        @edit="(model) => $emit('edit', model)"
        @delete="(id) => $emit('delete', id)"
      />
    </section>

    <Empty
      v-else
      class="h-full flex justify-center items-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <FontAwesomeIcon :icon="['far', 'rectangle-list']" />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ $t('models.emptyTitle') }}</EmptyTitle>
      <EmptyDescription>{{ $t('models.emptyDescription') }}</EmptyDescription>
      <EmptyContent />
    </Empty>
  </section>
</template>

<script setup lang="ts">
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@memoh/ui'
import CreateModel from '@/components/create-model/index.vue'
import ImportModelsDialog from '@/components/import-models-dialog/index.vue'
import ModelItem from './model-item.vue'
import type { ModelsGetResponse } from '@memoh/sdk'

defineProps<{
  providerId: string | undefined
  models: ModelsGetResponse[] | undefined
  deleteModelLoading: boolean
}>()

defineEmits<{
  edit: [model: ModelsGetResponse]
  delete: [id: string]
}>()
</script>
