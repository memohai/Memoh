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

    <template v-if="models && models.length > 0">
      <InputGroup
        v-if="models.length > 5"
        class="shadow-none mb-4"
      >
        <InputGroupAddon align="inline-start">
          <FontAwesomeIcon
            :icon="['fas', 'magnifying-glass']"
            class="text-muted-foreground"
          />
        </InputGroupAddon>
        <InputGroupInput
          v-model="searchQuery"
          :placeholder="$t('models.searchModelPlaceholder')"
        />
      </InputGroup>

      <section class="flex flex-col gap-4">
        <ModelItem
          v-for="model in displayedModels"
          :key="model.id || `${model.llm_provider_id}:${model.model_id}`"
          :model="model"
          :delete-loading="deleteModelLoading"
          @edit="(model) => $emit('edit', model)"
          @delete="(id) => $emit('delete', id)"
        />
      </section>

      <div
        v-if="hasMore"
        class="flex flex-col items-center gap-2 pt-4"
      >
        <span class="text-xs text-muted-foreground">
          {{ $t('models.showingCount', { count: displayLimit, total: filteredModels.length }) }}
        </span>
        <Button
          variant="outline"
          size="sm"
          class="cursor-pointer"
          @click="showMore"
        >
          {{ $t('models.showMore') }}
        </Button>
      </div>

      <Empty
        v-if="filteredModels.length === 0"
        class="flex justify-center items-center py-8"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('models.searchNoResults') }}</EmptyTitle>
      </Empty>
    </template>

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
import { ref, computed, watch } from 'vue'
import {
  Button,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@memoh/ui'
import CreateModel from '@/components/create-model/index.vue'
import ImportModelsDialog from '@/components/import-models-dialog/index.vue'
import ModelItem from './model-item.vue'
import type { ModelsGetResponse } from '@memoh/sdk'

const PAGE_SIZE = 30

const props = defineProps<{
  providerId: string | undefined
  models: ModelsGetResponse[] | undefined
  deleteModelLoading: boolean
}>()

defineEmits<{
  edit: [model: ModelsGetResponse]
  delete: [id: string]
}>()

const searchQuery = ref('')
const displayLimit = ref(PAGE_SIZE)

const filteredModels = computed(() => {
  if (!props.models) return []
  if (!searchQuery.value) return props.models
  const keyword = searchQuery.value.toLowerCase()
  return props.models.filter((model) => {
    const name = (model.name ?? '').toLowerCase()
    const modelId = (model.model_id ?? '').toLowerCase()
    return name.includes(keyword) || modelId.includes(keyword)
  })
})

const displayedModels = computed(() => filteredModels.value.slice(0, displayLimit.value))
const hasMore = computed(() => displayLimit.value < filteredModels.value.length)

function showMore() {
  displayLimit.value += PAGE_SIZE
}

watch(searchQuery, () => {
  displayLimit.value = PAGE_SIZE
})
</script>
