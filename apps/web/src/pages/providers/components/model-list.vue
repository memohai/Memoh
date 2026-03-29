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
          <Search
            class="size-3.5 text-muted-foreground"
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
        v-if="totalPages > 1"
        class="flex items-center justify-between pt-4"
      >
        <span class="text-xs text-muted-foreground">
          {{ $t('models.showingCount', { count: `${pageStart}-${pageEnd}`, total: filteredModels.length }) }}
        </span>
        <Pagination
          :total="filteredModels.length"
          :items-per-page="PAGE_SIZE"
          :sibling-count="1"
          :page="currentPage"
          show-edges
          @update:page="currentPage = $event"
        >
          <PaginationContent v-slot="{ items }">
            <PaginationFirst />
            <PaginationPrevious />
            <template
              v-for="(item, index) in items"
              :key="index"
            >
              <PaginationEllipsis
                v-if="item.type === 'ellipsis'"
                :index="index"
              />
              <PaginationItem
                v-else
                :value="item.value"
              />
            </template>
            <PaginationNext />
            <PaginationLast />
          </PaginationContent>
        </Pagination>
      </div>

      <Empty
        v-if="filteredModels.length === 0"
        class="flex justify-center items-center py-8"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Search />
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
          <List />
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
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationFirst,
  PaginationItem,
  PaginationLast,
  PaginationNext,
  PaginationPrevious,
} from '@memohai/ui'
import { Search, List } from 'lucide-vue-next'
import CreateModel from '@/components/create-model/index.vue'
import ImportModelsDialog from '@/components/import-models-dialog/index.vue'
import ModelItem from './model-item.vue'
import type { ModelsGetResponse } from '@memohai/sdk'

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
const currentPage = ref(1)

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

const totalPages = computed(() => Math.ceil(filteredModels.value.length / PAGE_SIZE))
const pageStart = computed(() => (currentPage.value - 1) * PAGE_SIZE + 1)
const pageEnd = computed(() => Math.min(currentPage.value * PAGE_SIZE, filteredModels.value.length))
const displayedModels = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return filteredModels.value.slice(start, start + PAGE_SIZE)
})

watch(searchQuery, () => {
  currentPage.value = 1
})
</script>
