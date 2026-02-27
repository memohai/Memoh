<template>
  <div class="max-w-4xl mx-auto space-y-5">
    <div class="flex items-start justify-between gap-3">
      <div class="space-y-1 min-w-0">
        <h3 class="text-lg font-semibold">
          {{ $t('mcp.addTitle') }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ $t('mcp.addDescription') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2 shrink-0 justify-end">
        <template v-if="selectedIds.length === 0">
          <Button
            variant="outline"
            size="sm"
            :disabled="loading"
            @click="loadList"
          >
            <Spinner
              v-if="loading"
              class="mr-1.5"
            />
            {{ $t('common.refresh') }}
          </Button>
          <Button
            size="sm"
            @click="openCreateDialog"
          >
            {{ $t('common.add') }}
          </Button>
        </template>
        <template v-else>
          <span class="text-sm text-muted-foreground mr-1">
            {{ $t('common.batchSelected', { count: selectedIds.length }) }}
          </span>
          <Button
            variant="ghost"
            size="sm"
            @click="clearSelection"
          >
            {{ $t('common.cancelSelection') }}
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="handleBatchExport"
          >
            {{ $t('common.export') }}
          </Button>
          <ConfirmPopover
            :message="$t('common.batchDeleteConfirm', { count: selectedIds.length })"
            @confirm="handleBatchDelete"
          >
            <template #trigger>
              <Button
                variant="destructive"
                size="sm"
              >
                {{ $t('common.delete') }}
              </Button>
            </template>
          </ConfirmPopover>
        </template>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="loading && items.length === 0"
      class="flex items-center gap-2 text-sm text-muted-foreground"
    >
      <Spinner />
      <span>{{ $t('common.loading') }}</span>
    </div>

    <!-- Empty -->
    <div
      v-else-if="items.length === 0"
      class="rounded-md border p-4"
    >
      <p class="text-sm text-muted-foreground">
        {{ $t('mcp.empty') }}
      </p>
    </div>

    <!-- Table -->
    <DataTable
      v-else
      :columns="columns"
      :data="items"
    />

    <!-- Add dialog: tabs (single | import). Edit dialog: two columns (form | json) with sync -->
    <Dialog v-model:open="formDialogOpen">
      <DialogContent :class="editingItem ? 'sm:max-w-4xl max-h-[90vh] flex flex-col w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] sm:w-auto' : 'sm:max-w-md w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] sm:w-auto'">
        <DialogHeader>
          <DialogTitle>{{ editingItem ? $t('common.edit') : $t('common.add') }} MCP Server</DialogTitle>
        </DialogHeader>

        <!-- Edit: two columns on desktop, stacked on mobile -->
        <template v-if="editingItem">
          <div class="mt-3 flex flex-col md:grid md:grid-cols-2 gap-4 flex-1 min-h-0 overflow-y-auto">
            <form
              class="flex flex-col gap-3 min-h-0 rounded-lg border border-border bg-card p-3 md:bg-transparent md:border-0 md:p-0 md:rounded-none md:overflow-y-auto md:pr-2"
              @submit.prevent="handleSubmit"
            >
              <div class="space-y-1.5">
                <Label>{{ $t('common.name') }}</Label>
                <Input
                  v-model="formData.name"
                  :placeholder="$t('common.namePlaceholder')"
                  @update:model-value="syncFormToEditJson"
                />
              </div>
              <Tabs
                v-model="connectionMode"
                class="w-full"
              >
                <TabsList class="w-full">
                  <TabsTrigger value="stdio">
                    {{ $t('mcp.types.stdio') }}
                  </TabsTrigger>
                  <TabsTrigger value="remote">
                    {{ $t('mcp.types.remote') }}
                  </TabsTrigger>
                </TabsList>
                <TabsContent
                  value="stdio"
                  class="mt-3 flex flex-col gap-3"
                >
                  <div class="space-y-1.5">
                    <Label>{{ $t('mcp.command') }}</Label>
                    <Input
                      v-model="formData.command"
                      :placeholder="$t('mcp.commandPlaceholder')"
                      @update:model-value="syncFormToEditJson"
                    />
                  </div>
                  <div class="space-y-1.5">
                    <Label>{{ $t('mcp.arguments') }}</Label>
                    <TagsInput
                      v-model="argsTags"
                      :add-on-blur="true"
                      :duplicate="true"
                      @update:model-value="syncFormToEditJson"
                    >
                      <TagsInputItem
                        v-for="item in argsTags"
                        :key="item"
                        :value="item"
                      >
                        <TagsInputItemText />
                        <TagsInputItemDelete />
                      </TagsInputItem>
                      <TagsInputInput
                        :placeholder="$t('mcp.argumentsPlaceholder')"
                        class="w-full py-1"
                      />
                    </TagsInput>
                  </div>
                  <div class="space-y-1.5">
                    <Label>{{ $t('mcp.env') }}</Label>
                    <TagsInput
                      :model-value="envTags.tagList.value"
                      :add-on-blur="true"
                      :convert-value="envTags.convertValue"
                      @update:model-value="(tags) => { envTags.handleUpdate(tags.map(String)); syncFormToEditJson() }"
                    >
                      <TagsInputItem
                        v-for="(value, index) in envTags.tagList.value"
                        :key="index"
                        :value="value"
                      >
                        <TagsInputItemText />
                        <TagsInputItemDelete />
                      </TagsInputItem>
                      <TagsInputInput
                        :placeholder="$t('mcp.envPlaceholder')"
                        class="w-full py-1"
                      />
                    </TagsInput>
                  </div>
                  <div class="space-y-1.5">
                    <Label>{{ $t('mcp.cwd') }}</Label>
                    <Input
                      v-model="formData.cwd"
                      :placeholder="$t('mcp.cwdPlaceholder')"
                      @update:model-value="syncFormToEditJson"
                    />
                  </div>
                </TabsContent>
                <TabsContent
                  value="remote"
                  class="mt-3 flex flex-col gap-3"
                >
                  <div class="space-y-1.5">
                    <Label>URL</Label>
                    <Input
                      v-model="formData.url"
                      placeholder="https://example.com/mcp"
                      @update:model-value="syncFormToEditJson"
                    />
                  </div>
                  <div class="space-y-1.5">
                    <Label>Headers</Label>
                    <TagsInput
                      :model-value="headerTags.tagList.value"
                      :add-on-blur="true"
                      :convert-value="headerTags.convertValue"
                      @update:model-value="(tags) => { headerTags.handleUpdate(tags.map(String)); syncFormToEditJson() }"
                    >
                      <TagsInputItem
                        v-for="(value, index) in headerTags.tagList.value"
                        :key="index"
                        :value="value"
                      >
                        <TagsInputItemText />
                        <TagsInputItemDelete />
                      </TagsInputItem>
                      <TagsInputInput
                        placeholder="Key:Value"
                        class="w-full py-1"
                      />
                    </TagsInput>
                  </div>
                  <div class="space-y-1.5">
                    <Label>Transport</Label>
                    <Select
                      v-model="formData.transport"
                      @update:model-value="syncFormToEditJson"
                    >
                      <SelectTrigger
                        class="w-full"
                        aria-label="Transport"
                      >
                        <SelectValue placeholder="http" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="http">
                            HTTP (Streamable)
                          </SelectItem>
                          <SelectItem value="sse">
                            SSE
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </div>
                </TabsContent>
              </Tabs>
            </form>
            <div class="flex flex-col min-h-0 rounded-lg border border-border bg-card p-3 md:bg-transparent md:border-0 md:p-0 md:rounded-none">
              <Label class="text-sm mb-1">JSON</Label>
              <Textarea
                v-model="editJson"
                class="font-mono text-xs flex-1 min-h-[180px] md:min-h-[200px]"
                @update:model-value="syncEditJsonToForm"
              />
            </div>
          </div>
          <DialogFooter class="mt-4 shrink-0 flex-row flex-wrap items-center gap-2 sm:justify-between">
            <div class="flex items-center gap-2">
              <Label class="text-sm font-normal">{{ $t('mcp.active') }}</Label>
              <Switch
                :model-value="formData.active"
                @update:model-value="(val) => (formData.active = !!val)"
              />
            </div>
            <div class="flex gap-2">
              <DialogClose as-child>
                <Button variant="outline">
                  {{ $t('common.cancel') }}
                </Button>
              </DialogClose>
              <Button
                :disabled="submitting || !formData.name.trim() || (connectionMode === 'stdio' ? !formData.command.trim() : !formData.url.trim())"
                @click="handleSubmit"
              >
                <Spinner
                  v-if="submitting"
                  class="mr-1.5"
                />
                {{ $t('common.confirm') }}
              </Button>
            </div>
          </DialogFooter>
        </template>

        <!-- Add: tabs single | import -->
        <template v-else>
          <Tabs
            v-model="addDialogTab"
            class="mt-4 w-full"
          >
            <TabsList class="w-full">
              <TabsTrigger value="single">
                {{ $t('common.tabAddSingle') }}
              </TabsTrigger>
              <TabsTrigger value="import">
                {{ $t('common.tabImportJson') }}
              </TabsTrigger>
            </TabsList>

            <TabsContent
              value="single"
              class="mt-3"
            >
              <form
                class="flex flex-col gap-3"
                @submit.prevent="handleSubmit"
              >
                <div class="space-y-1.5">
                  <Label>{{ $t('common.name') }}</Label>
                  <Input
                    v-model="formData.name"
                    :placeholder="$t('common.namePlaceholder')"
                  />
                </div>
                <Tabs
                  v-model="connectionMode"
                  class="w-full"
                >
                  <TabsList class="w-full">
                    <TabsTrigger value="stdio">
                      {{ $t('mcp.types.stdio') }}
                    </TabsTrigger>
                    <TabsTrigger value="remote">
                      {{ $t('mcp.types.remote') }}
                    </TabsTrigger>
                  </TabsList>
                  <TabsContent
                    value="stdio"
                    class="mt-3 flex flex-col gap-3"
                  >
                    <div class="space-y-1.5">
                      <Label>{{ $t('mcp.command') }}</Label>
                      <Input
                        v-model="formData.command"
                        :placeholder="$t('mcp.commandPlaceholder')"
                      />
                    </div>
                    <div class="space-y-1.5">
                      <Label>{{ $t('mcp.arguments') }}</Label>
                      <TagsInput
                        v-model="argsTags"
                        :add-on-blur="true"
                        :duplicate="true"
                      >
                        <TagsInputItem
                          v-for="item in argsTags"
                          :key="item"
                          :value="item"
                        >
                          <TagsInputItemText />
                          <TagsInputItemDelete />
                        </TagsInputItem>
                        <TagsInputInput
                          :placeholder="$t('mcp.argumentsPlaceholder')"
                          class="w-full py-1"
                        />
                      </TagsInput>
                    </div>
                    <div class="space-y-1.5">
                      <Label>{{ $t('mcp.env') }}</Label>
                      <TagsInput
                        :model-value="envTags.tagList.value"
                        :add-on-blur="true"
                        :convert-value="envTags.convertValue"
                        @update:model-value="(tags) => envTags.handleUpdate(tags.map(String))"
                      >
                        <TagsInputItem
                          v-for="(value, index) in envTags.tagList.value"
                          :key="index"
                          :value="value"
                        >
                          <TagsInputItemText />
                          <TagsInputItemDelete />
                        </TagsInputItem>
                        <TagsInputInput
                          :placeholder="$t('mcp.envPlaceholder')"
                          class="w-full py-1"
                        />
                      </TagsInput>
                    </div>
                    <div class="space-y-1.5">
                      <Label>{{ $t('mcp.cwd') }}</Label>
                      <Input
                        v-model="formData.cwd"
                        :placeholder="$t('mcp.cwdPlaceholder')"
                      />
                    </div>
                  </TabsContent>
                  <TabsContent
                    value="remote"
                    class="mt-3 flex flex-col gap-3"
                  >
                    <div class="space-y-1.5">
                      <Label>URL</Label>
                      <Input
                        v-model="formData.url"
                        placeholder="https://example.com/mcp"
                      />
                    </div>
                    <div class="space-y-1.5">
                      <Label>Headers</Label>
                      <TagsInput
                        :model-value="headerTags.tagList.value"
                        :add-on-blur="true"
                        :convert-value="headerTags.convertValue"
                        @update:model-value="(tags) => headerTags.handleUpdate(tags.map(String))"
                      >
                        <TagsInputItem
                          v-for="(value, index) in headerTags.tagList.value"
                          :key="index"
                          :value="value"
                        >
                          <TagsInputItemText />
                          <TagsInputItemDelete />
                        </TagsInputItem>
                        <TagsInputInput
                          placeholder="Key:Value"
                          class="w-full py-1"
                        />
                      </TagsInput>
                    </div>
                    <div class="space-y-1.5">
                      <Label>Transport</Label>
                      <Select v-model="formData.transport">
                        <SelectTrigger
                          class="w-full"
                          aria-label="Transport"
                        >
                          <SelectValue placeholder="http" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            <SelectItem value="http">
                              HTTP (Streamable)
                            </SelectItem>
                            <SelectItem value="sse">
                              SSE
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </div>
                  </TabsContent>
                </Tabs>
                <DialogFooter class="mt-4 flex-row flex-wrap items-center gap-2 sm:justify-between">
                  <div class="flex items-center gap-2">
                    <Label class="text-sm font-normal">{{ $t('mcp.active') }}</Label>
                    <Switch
                      :model-value="formData.active"
                      @update:model-value="(val) => (formData.active = !!val)"
                    />
                  </div>
                  <div class="flex gap-2">
                    <DialogClose as-child>
                      <Button variant="outline">
                        {{ $t('common.cancel') }}
                      </Button>
                    </DialogClose>
                    <Button
                      type="submit"
                      :disabled="submitting || !formData.name.trim() || (connectionMode === 'stdio' ? !formData.command.trim() : !formData.url.trim())"
                    >
                      <Spinner
                        v-if="submitting"
                        class="mr-1.5"
                      />
                      {{ $t('common.confirm') }}
                    </Button>
                  </div>
                </DialogFooter>
              </form>
            </TabsContent>

            <TabsContent
              value="import"
              class="mt-3 space-y-3"
            >
              <p class="text-sm text-muted-foreground">
                {{ $t('mcp.importHint') }}
              </p>
              <Textarea
                v-model="importJson"
                rows="10"
                class="font-mono text-xs"
                :placeholder="importJsonPlaceholder"
              />
              <DialogFooter class="mt-4">
                <DialogClose as-child>
                  <Button variant="outline">
                    {{ $t('common.cancel') }}
                  </Button>
                </DialogClose>
                <Button
                  :disabled="importSubmitting || !importJson.trim()"
                  @click="handleImport"
                >
                  <Spinner
                    v-if="importSubmitting"
                    class="mr-1.5"
                  />
                  {{ $t('common.import') }}
                </Button>
              </DialogFooter>
            </TabsContent>
          </Tabs>
        </template>
      </DialogContent>
    </Dialog>

    <!-- Export dialog -->
    <Dialog v-model:open="exportDialogOpen">
      <DialogContent class="sm:max-w-lg w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] sm:w-auto">
        <DialogHeader>
          <DialogTitle>{{ $t('common.export') }} mcpServers</DialogTitle>
        </DialogHeader>
        <div class="mt-4">
          <Textarea
            :model-value="exportJson"
            rows="10"
            class="font-mono text-xs"
            readonly
          />
        </div>
        <DialogFooter class="mt-4">
          <Button
            variant="outline"
            @click="handleCopyExport"
          >
            {{ $t('common.copy') }}
          </Button>
          <DialogClose as-child>
            <Button>
              {{ $t('common.confirm') }}
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { type ColumnDef } from '@tanstack/vue-table'
import {
  Badge,
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  Switch,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
  Textarea,
} from '@memoh/ui'
import DataTable from '@/components/data-table/index.vue'
import { useKeyValueTags } from '@/composables/useKeyValueTags'
import {
  getBotsByBotIdMcp,
  postBotsByBotIdMcp,
  putBotsByBotIdMcpById,
  deleteBotsByBotIdMcpById,
  postBotsByBotIdMcpOpsBatchDelete,
} from '@memoh/sdk'
import type { McpUpsertRequest, McpImportRequest } from '@memoh/sdk'
import { client } from '@memoh/sdk/client'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { tagsToRecord } from '@/utils/key-value-tags'
import { useClipboard } from '@/composables/useClipboard'

interface McpItem {
  id: string
  name: string
  type: string
  config: Record<string, unknown>
  is_active: boolean
}

interface McpServerEntry {
  command?: string
  args?: string[]
  env?: Record<string, string>
  cwd?: string
  url?: string
  headers?: Record<string, string>
  transport?: string
}

const props = defineProps<{ botId: string }>()
const { t } = useI18n()
const { copyText } = useClipboard()

const loading = ref(false)
const items = ref<McpItem[]>([])
const formDialogOpen = ref(false)
const editingItem = ref<McpItem | null>(null)
const submitting = ref(false)
const addDialogTab = ref<'single' | 'import'>('single')
const importJsonPlaceholder = `{
  "mcpServers": {
    "hello": {
      "command": "npx",
      "args": ["-y", "mcp-hello-world"]
    }
  }
}`
const importJson = ref('')
const importSubmitting = ref(false)
const exportDialogOpen = ref(false)
const exportJson = ref('')
const selectedIds = ref<string[]>([])

const connectionMode = ref<'stdio' | 'remote'>('stdio')

const formData = ref({
  name: '',
  command: '',
  url: '',
  cwd: '',
  transport: 'http',
  active: true,
})

// Edit dialog: JSON panel synced with formData
const editJson = ref('')
let editSyncFromJson = false

watch(connectionMode, (mode) => {
  if (mode === 'stdio') {
    formData.value.url = ''
    formData.value.transport = 'http'
    headerTags.initFromObject(null)
  } else {
    formData.value.command = ''
    formData.value.cwd = ''
    argsTags.value = []
    envTags.initFromObject(null)
  }
})
const argsTags = ref<string[]>([])
const envTags = useKeyValueTags()
const headerTags = useKeyValueTags()

function configValue(config: Record<string, unknown>, key: string): string {
  const val = config?.[key]
  return typeof val === 'string' ? val : ''
}

function configArray(config: Record<string, unknown>, key: string): string[] {
  const val = config?.[key]
  if (Array.isArray(val)) return val.map(String)
  return []
}

function configMap(config: Record<string, unknown>, key: string): Record<string, string> {
  const val = config?.[key]
  if (val && typeof val === 'object' && !Array.isArray(val)) {
    const out: Record<string, string> = {}
    for (const [k, v] of Object.entries(val)) {
      out[k] = String(v)
    }
    return out
  }
  return {}
}

function toggleSelection(id: string, checked: boolean) {
  const set = new Set(selectedIds.value)
  if (checked) set.add(id)
  else set.delete(id)
  selectedIds.value = Array.from(set)
}

function toggleSelectAll(checked: boolean) {
  selectedIds.value = checked ? items.value.map((i) => i.id) : []
}

const isAllSelected = computed(() =>
  items.value.length > 0 && selectedIds.value.length === items.value.length,
)

function clearSelection() {
  selectedIds.value = []
}

function itemToExportEntry(item: McpItem): McpServerEntry {
  const cfg = item.config ?? {}
  if (item.type === 'stdio') {
    const entry: McpServerEntry = {
      command: configValue(cfg, 'command') || undefined,
      args: configArray(cfg, 'args').length ? configArray(cfg, 'args') : undefined,
      cwd: configValue(cfg, 'cwd') || undefined,
      env: Object.keys(configMap(cfg, 'env')).length ? configMap(cfg, 'env') : undefined,
    }
    return entry
  }
  const entry: McpServerEntry = {
    url: configValue(cfg, 'url') || undefined,
    headers: Object.keys(configMap(cfg, 'headers')).length ? configMap(cfg, 'headers') : undefined,
    transport: item.type === 'sse' ? 'sse' : undefined,
  }
  return entry
}

const columns = computed<ColumnDef<McpItem>[]>(() => [
  {
    id: 'select',
    header: () =>
      h('div', { class: 'flex items-center justify-center py-4' }, [
        h('input', {
          type: 'checkbox',
          class: 'size-4 cursor-pointer rounded border border-input',
          'aria-label': 'Select all MCP servers',
          checked: isAllSelected.value,
          onChange: (e: Event) => {
            toggleSelectAll((e.target as HTMLInputElement).checked)
          },
        }),
      ]),
    cell: ({ row }) => {
      const id = row.original.id
      return h('div', { class: 'flex justify-center' }, [
        h('input', {
          type: 'checkbox',
          class: 'size-4 cursor-pointer rounded border border-input',
          'aria-label': `Select MCP server ${row.original.name}`,
          checked: selectedIds.value.includes(id),
          onChange: (e: Event) => {
            toggleSelection(id, (e.target as HTMLInputElement).checked)
          },
        }),
      ])
    },
  },
  {
    accessorKey: 'name',
    header: () => h('div', { class: 'text-left py-4' }, t('common.name')),
  },
  {
    accessorKey: 'type',
    header: () => h('div', { class: 'text-left' }, t('common.type')),
    cell: ({ row }) => h(Badge, { variant: 'outline' }, () => row.original.type),
  },
  {
    id: 'target',
    header: () => h('div', { class: 'text-left' }, 'Command / URL'),
    cell: ({ row }) => {
      const cfg = row.original.config ?? {}
      const cmd = configValue(cfg, 'command')
      const url = configValue(cfg, 'url')
      const args = configArray(cfg, 'args')
      const full =
        cmd
          ? (args.length ? `${cmd} ${args.join(' ')}` : cmd)
          : (url || '-')
      return h('span', {
        class: 'font-mono text-xs block max-w-[280px] truncate',
        title: full,
      }, full)
    },
  },
  {
    id: 'status',
    header: () => h('div', { class: 'text-center' }, t('mcp.active')),
    cell: ({ row }) => h('div', { class: 'text-center' },
      h(Badge, { variant: row.original.is_active ? 'default' : 'secondary' },
        () => row.original.is_active ? 'ON' : 'OFF'),
    ),
  },
  {
    id: 'actions',
    header: () => h('div', { class: 'text-center' }, t('common.operation')),
    cell: ({ row }) => h('div', { class: 'flex gap-2 justify-center' }, [
      h(Button, {
        size: 'sm',
        variant: 'outline',
        onClick: () => openEditDialog(row.original),
      }, () => t('common.edit')),
      h(ConfirmPopover, {
        message: t('mcp.deleteConfirm'),
        onConfirm: () => handleDelete(row.original.id),
      }, {
        trigger: () => h(Button, {
          size: 'sm',
          variant: 'destructive',
        }, () => t('common.delete')),
      }),
    ]),
  },
])

async function loadList() {
  loading.value = true
  try {
    const { data } = await getBotsByBotIdMcp({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    items.value = data.items ?? []
  } catch (error) {
    toast.error(resolveError(error, t('common.loadFailed')))
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  editingItem.value = null
  addDialogTab.value = 'single'
  importJson.value = ''
  connectionMode.value = 'stdio'
  formData.value = { name: '', command: '', url: '', cwd: '', transport: 'http', active: true }
  argsTags.value = []
  envTags.initFromObject(null)
  headerTags.initFromObject(null)
  formDialogOpen.value = true
}

function openEditDialog(item: McpItem) {
  editingItem.value = item
  const cfg = item.config ?? {}
  connectionMode.value = item.type === 'stdio' ? 'stdio' : 'remote'
  formData.value = {
    name: item.name,
    command: configValue(cfg, 'command'),
    url: configValue(cfg, 'url'),
    cwd: configValue(cfg, 'cwd'),
    transport: item.type === 'sse' ? 'sse' : 'http',
    active: !!item.is_active,
  }
  argsTags.value = configArray(cfg, 'args')
  envTags.initFromObject(configMap(cfg, 'env'))
  headerTags.initFromObject(configMap(cfg, 'headers'))
  editSyncFromJson = false
  syncFormToEditJson()
  formDialogOpen.value = true
}

function buildFormToEntry(): McpServerEntry | null {
  const d = formData.value
  const name = d.name.trim()
  if (!name) return null
  if (d.command.trim()) {
    const entry: McpServerEntry = {
      command: d.command.trim(),
      args: argsTags.value.length ? argsTags.value : undefined,
      cwd: d.cwd.trim() || undefined,
    }
    const env = tagsToRecord(envTags.tagList.value)
    if (Object.keys(env).length > 0) entry.env = env
    return entry
  }
  if (d.url.trim()) {
    const entry: McpServerEntry = {
      url: d.url.trim(),
      transport: d.transport === 'sse' ? 'sse' : undefined,
    }
    const headers = tagsToRecord(headerTags.tagList.value)
    if (Object.keys(headers).length > 0) entry.headers = headers
    return entry
  }
  return null
}

function syncFormToEditJson() {
  if (editSyncFromJson) return
  const entry = buildFormToEntry()
  if (!entry) {
    editJson.value = ''
    return
  }
  const name = formData.value.name.trim()
  const mcpServers: Record<string, McpServerEntry> = { [name]: entry }
  editJson.value = JSON.stringify({ mcpServers }, null, 2)
}

function syncEditJsonToForm() {
  const raw = editJson.value.trim()
  if (!raw) return
  editSyncFromJson = true
  try {
    let parsed: { mcpServers?: Record<string, McpServerEntry> } = JSON.parse(raw)
    if (!parsed.mcpServers && typeof parsed === 'object' && !Array.isArray(parsed)) {
      parsed = { mcpServers: parsed as Record<string, McpServerEntry> }
    }
    const servers = parsed.mcpServers
    if (!servers || typeof servers !== 'object') {
      editSyncFromJson = false
      return
    }
    const entries = Object.entries(servers)
    const single = entries.length === 1 ? entries[0] : null
    if (!single) {
      editSyncFromJson = false
      return
    }
    const [name, e] = single
    if (e.command) {
      connectionMode.value = 'stdio'
      formData.value = {
        name,
        command: e.command ?? '',
        url: '',
        cwd: e.cwd ?? '',
        transport: 'http',
        active: formData.value.active,
      }
      argsTags.value = e.args ?? []
      envTags.initFromObject(e.env ?? null)
      headerTags.initFromObject(null)
    } else if (e.url) {
      connectionMode.value = 'remote'
      formData.value = {
        name,
        command: '',
        url: e.url ?? '',
        cwd: '',
        transport: e.transport === 'sse' ? 'sse' : 'http',
        active: formData.value.active,
      }
      argsTags.value = []
      envTags.initFromObject(null)
      headerTags.initFromObject(e.headers ?? null)
    }
  } catch {
    // ignore parse error
  }
  editSyncFromJson = false
}

function buildRequestBody(): McpUpsertRequest {
  const body: McpUpsertRequest = {
    name: formData.value.name.trim(),
    is_active: formData.value.active,
  }
  if (formData.value.command.trim()) {
    body.command = formData.value.command.trim()
    if (argsTags.value.length > 0) body.args = argsTags.value
    const env = tagsToRecord(envTags.tagList.value)
    if (Object.keys(env).length > 0) body.env = env
    if (formData.value.cwd.trim()) body.cwd = formData.value.cwd.trim()
  } else if (formData.value.url.trim()) {
    body.url = formData.value.url.trim()
    const headers = tagsToRecord(headerTags.tagList.value)
    if (Object.keys(headers).length > 0) body.headers = headers
    if (formData.value.transport === 'sse') body.transport = 'sse'
  }
  return body
}

async function handleSubmit() {
  submitting.value = true
  try {
    const body = buildRequestBody()
    if (editingItem.value) {
      await putBotsByBotIdMcpById({
        path: { bot_id: props.botId, id: editingItem.value.id },
        body,
        throwOnError: true,
      })
    } else {
      await postBotsByBotIdMcp({
        path: { bot_id: props.botId },
        body,
        throwOnError: true,
      })
    }
    formDialogOpen.value = false
    await loadList()
    toast.success(editingItem.value ? t('mcp.updateSuccess') : t('mcp.createSuccess'))
  } catch (error) {
    toast.error(resolveError(error, t('common.saveFailed')))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(id: string) {
  try {
    await deleteBotsByBotIdMcpById({
      path: { bot_id: props.botId, id },
      throwOnError: true,
    })
    selectedIds.value = selectedIds.value.filter((x) => x !== id)
    await loadList()
    toast.success(t('mcp.deleteSuccess'))
  } catch (error) {
    toast.error(resolveError(error, t('mcp.deleteFailed')))
  }
}

async function handleBatchDelete() {
  if (selectedIds.value.length === 0) return
  try {
    await postBotsByBotIdMcpOpsBatchDelete({
      path: { bot_id: props.botId },
      body: { ids: selectedIds.value },
      throwOnError: true,
    })
    selectedIds.value = []
    await loadList()
    toast.success(t('mcp.deleteSuccess'))
  } catch (error) {
    toast.error(resolveError(error, t('mcp.deleteFailed')))
  }
}

function handleBatchExport() {
  const selected = items.value.filter((i) => selectedIds.value.includes(i.id))
  if (selected.length === 0) return
  const mcpServers: Record<string, McpServerEntry> = {}
  selected.forEach((item) => {
    mcpServers[item.name] = itemToExportEntry(item)
  })
  exportJson.value = JSON.stringify({ mcpServers }, null, 2)
  exportDialogOpen.value = true
}

async function handleImport() {
  importSubmitting.value = true
  try {
    let parsed: McpImportRequest = JSON.parse(importJson.value)
    if (!parsed.mcpServers && typeof parsed === 'object') {
      parsed = { mcpServers: parsed as McpImportRequest['mcpServers'] }
    }
    await client.put({
      url: '/bots/{bot_id}/mcp-ops/import',
      path: { bot_id: props.botId },
      body: parsed,
      throwOnError: true,
    })
    formDialogOpen.value = false
    importJson.value = ''
    await loadList()
    toast.success(t('mcp.importSuccess'))
  } catch (error) {
    toast.error(resolveError(error, t('mcp.importFailed')))
  } finally {
    importSubmitting.value = false
  }
}

function handleCopyExport() {
  void copyText(exportJson.value)
  toast.success(t('common.copied'))
}

function resolveError(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

watch(() => props.botId, () => {
  if (props.botId) loadList()
}, { immediate: true })
</script>
