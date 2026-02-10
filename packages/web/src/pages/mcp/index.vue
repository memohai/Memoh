<template>
  <section class="[&_td:last-child]:w-40">
    <CreateMCP />
    <DataTable
      :columns="columns"
      :data="mcpFormatData"
    />
  </section>
</template>

<script setup lang="ts">
import { h, provide, ref, computed } from 'vue'
import DataTable from '@/components/data-table/index.vue'
import CreateMCP from '@/components/create-mcp/index.vue'
import { type ColumnDef } from '@tanstack/vue-table'
import {
  Badge,
  Button
} from '@memoh/ui'
import { type MCPListItem as MCPType } from '@memoh/shared'
import { i18nRef } from '@/i18n'
import { useMcpList, useDeleteMcp } from '@/composables/api/useMcp'

const open = ref(false)
const editMCPData = ref<{
  name: string
  config: MCPType['config']
  active: boolean
  id: string
} | null>(null)
provide('open', open)
provide('mcpEditData', editMCPData)

const { mutate: DeleteMCP } = useDeleteMcp()
const columns:ColumnDef<MCPType>[] = [
  {
    accessorKey: 'name',
    header: () => h('div', { class: 'text-left py-4' }, 'Name'),
   
  },
  {
    accessorKey: 'type',
    header: () => h('div', { class: 'text-left' }, 'Type'),
  },
  {
    accessorKey: 'config.command',
    header: () => h('div', { class: 'text-left' }, 'Command'),
  },
  {
    accessorKey: 'config.cwd',
    header: () => h('div', { class: 'text-left' }, 'Cwd'),
  },
  {
    accessorKey: 'config.args',
    header: () => h('div', { class: 'text-left' }, 'Arguments'),
    cell: ({ row }) => h('div', {class:'flex gap-4'}, row.original.config.args.map((argTxt) => {
      return h(Badge, {
        variant:'default'
      },()=>argTxt)
    }))
  },
  {
    accessorKey: 'config.env',
    header: () => h('div', { class: 'text-left' }, 'Env'),
    cell: ({ row }) => h('div', { class: 'flex gap-4' }, Object.entries(row.original.config.env).map(([key,value]) => {
      return h(Badge, {
        variant: 'outline'
      }, ()=>`${key}:${value}`)
    }))
  },
  {
    accessorKey: 'control',
    header: () => h('div', { class: 'text-center' }, i18nRef('common.operation').value),
    cell: ({ row }) => h('div', {class:'flex gap-2'}, [
      h(Button, {
        onClick() {
          editMCPData.value = {
            name: row.original.name,
            config: {...row.original.config},
            active: row.original.active,
            id:row.original.id
          }       
          open.value=true
        }
      }, ()=>i18nRef('common.edit').value),
      h(Button, {
        variant: 'destructive',
        async onClick() {        
          try {
            await DeleteMCP(row.original.id)
          } catch {
            return
          }
        }
      },()=>i18nRef('common.delete').value)
    ])
  }
]

const { data: mcpData } = useMcpList()

const mcpFormatData = computed(() => mcpData.value ?? [])

</script>