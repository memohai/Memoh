<script setup lang="ts">
// import type { Payment } from '@/components/columns'
import { h, computed, ref, provide, watch } from 'vue'
import CreateModel from '@/components/CreateModel/index.vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import {
  Button,
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationNext,
  PaginationPrevious

} from '@memoh/ui'
import DataTable from '@/components/DataTable/index.vue'
import request from '@/utils/request'
import { type ColumnDef } from '@tanstack/vue-table'




interface ModelType {
  apiKey: string,
  baseUrl: string,
  clientType: 'OpenAI' | 'Anthropic' | 'Google',
  modelId: string,
  name: string,
  type: 'chat' | 'embedding',
  id: string
}

const openDialogModel = ref(false)
const editModelInfo = ref<ModelType & { id: string } | null>(null)
provide('open', openDialogModel)
provide('editModelInfo', editModelInfo)

watch(openDialogModel, () => {
  if (!openDialogModel.value) {
    editModelInfo.value = null
  }
}, {
  immediate: true
})


const cacheQuery = useQueryCache()
const {
  mutate: deleteModel,
} = useMutation({
  mutation: (id: string) =>
    request({
      url: `model/${id}`,
      method: 'DELETE'
    }),
  onSettled: () => {
    cacheQuery.invalidateQueries({
      key: ['models']
    })
  }
})

const columns: ColumnDef<ModelType>[] = [
  {
    accessorKey: 'modelId',
    header: () => h('div', { class: 'text-left py-4' }, 'Name'),
    cell({ row }) {
      return h('div', { class: 'text-left py-4' }, row.getValue('modelId'))
    }
  },
  {
    accessorKey: 'baseUrl',
    header: () => h('div', { class: 'text-left' }, 'Base Url'),
  },
  {
    accessorKey: 'apiKey',
    header: () => h('div', { class: 'text-left' }, 'Api Key'),
  },
  {
    accessorKey: 'clientType',
    header: () => h('div', { class: 'text-left' }, 'Client Type'),
  },
  {
    accessorKey: 'name',
    header: () => h('div', { class: 'text-left' }, 'Name'),
  },
  {
    accessorKey: 'type',
    header: () => h('div', { class: 'text-left' }, 'Type'),
  },
  {
    accessorKey: 'control',
    header: () => h('div', { class: 'text-center' }, '操作'),
    cell: ({ row }) => h('div', { class: ' w-full flex justify-around' }, [h(Button, {
      'onClick': () => {
        editModelInfo.value = row.original
        openDialogModel.value = true
      }
    }, () => '编辑'), h(Button, {
      variant: 'destructive', onClick() {
        deleteModel(row.original.id)

      }
    }, () => '删除')])
  }
]

const { data: modelData } = useQuery({
  key: ['models'],
  query() {
    return request({
      url: '/model'
    })
  }
})


const displayFormat = computed(() => {
  return modelData.value?.data?.items?.map((currentModel: { model: Omit<ModelType, 'id'>, id: 'string' }) => ({ id: currentModel.id, ...currentModel.model })) ?? []
})

const pagination = computed(() => {
  return modelData.value?.data.pagination ?? {}
})

</script>

<template>
  <div class="w-full py-10 mx-auto">
    <div class="flex mb-4">
      <CreateModel />
    </div>
    <div class="[&_td:last-child]:w-45">
      <DataTable
        :columns="columns"
        :data="displayFormat"
      />
    </div>
    <div class="flex flex-col mt-4">
      <Pagination
        v-slot="{ page }"
        :total="pagination.value?.total ?? 0"
        :items-per-page="10"
        show-edges
      >
        <PaginationContent v-slot="{ items }">
          <PaginationPrevious />
          <template
            v-for="(item, index) in items"
            :key="index"
          >
            <PaginationItem
              v-if="item.type === 'page'"
              :key="index"
              :value="item.value"
              :is-active="item.value === page"
            >
              {{ item.value }}
            </PaginationItem>
            <PaginationEllipsis
              v-else
              :key="item.type"
              :index="index"
              class="w-9 h-9 flex items-center justify-center"
            >
              &#8230;
            </PaginationEllipsis>
          </template>

          <PaginationNext />
        </PaginationContent>
      </Pagination>
    </div>
  </div>
</template>