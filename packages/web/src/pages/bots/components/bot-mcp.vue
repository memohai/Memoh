<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-header>
      <InputGroup class="shadow-none">
        <InputGroupInput
          v-model="searchText"
          :placeholder="$t('mcp.searchPlaceholder')"
          aria-label="Search MCP servers"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            type="button"
            size="icon-xs"
            aria-label="Search"
          >
            <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </template>

    <template #sidebar-content>
      <div
        v-if="loading && items.length === 0"
        class="flex items-center gap-2 text-sm text-muted-foreground p-4"
      >
        <Spinner />
        <span>{{ $t('common.loading') }}</span>
      </div>
      <SidebarMenu
        v-for="item in filteredItems"
        v-else
        :key="item.id || '_draft'"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="['py-4 border w-full text-left', selectedItem?.id === item.id ? 'border-border' : 'border-transparent']"
              :model-value="selectedItem?.id === item.id"
              @update:model-value="(v) => { if (v) selectItem(item) }"
            >
              <div class="flex items-center gap-2 w-full min-w-0">
                <span
                  class="size-2 rounded-full shrink-0"
                  :class="item.is_active ? 'bg-green-500' : 'bg-muted-foreground/40'"
                />
                <span class="truncate flex-1">
                  {{ item.name }}
                  <span
                    v-if="!item.id"
                    class="text-muted-foreground text-xs"
                  >*</span>
                </span>
                <Badge
                  v-if="item.id"
                  variant="outline"
                  class="shrink-0 text-[10px]"
                >
                  {{ item.type }}
                </Badge>
                <Badge
                  v-else
                  variant="secondary"
                  class="shrink-0 text-[10px]"
                >
                  {{ $t('mcp.draft') }}
                </Badge>
              </div>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <div class="flex gap-2 p-2">
        <Button
          class="flex-1"
          size="sm"
          @click="openCreateDialog"
        >
          <FontAwesomeIcon
            :icon="['fas', 'plus']"
            class="mr-1.5"
          />
          {{ $t('common.add') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          @click="openImportDialog"
        >
          {{ $t('common.import') }}
        </Button>
      </div>
    </template>

    <template #detail>
      <ScrollArea
        v-if="selectedItem"
        class="max-h-full h-full"
      >
        <div class="p-6 space-y-6">
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">
              {{ selectedItem.name }}
            </h3>
            <div class="flex items-center gap-2">
              <Button
                v-if="selectedItem.id"
                variant="outline"
                size="sm"
                @click="handleExportSingle"
              >
                {{ $t('common.export') }}
              </Button>
              <ConfirmPopover
                :message="$t('mcp.deleteConfirm')"
                @confirm="handleDelete(selectedItem!)"
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
            </div>
          </div>

          <form
            class="flex flex-col gap-4"
            @submit.prevent="handleSubmit"
          >
            <div class="space-y-1.5">
              <Label>{{ $t('common.name') }}</Label>
              <Input
                v-model="formData.name"
                :placeholder="$t('common.namePlaceholder')"
              />
            </div>

            <div class="space-y-1.5">
              <Label>{{ $t('common.type') }}</Label>
              <Select v-model="connectionType">
                <SelectTrigger class="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="stdio">
                      {{ $t('mcp.types.stdio') }}
                    </SelectItem>
                    <SelectItem value="remote">
                      {{ $t('mcp.types.remote') }}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            <template v-if="connectionType === 'stdio'">
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
                    v-for="tag in argsTags"
                    :key="tag"
                    :value="tag"
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
                <KeyValueEditor
                  v-model="envPairs"
                  key-placeholder="KEY"
                  value-placeholder="VALUE"
                />
              </div>
              <div class="space-y-1.5">
                <Label>{{ $t('mcp.cwd') }}</Label>
                <Input
                  v-model="formData.cwd"
                  :placeholder="$t('mcp.cwdPlaceholder')"
                />
              </div>
            </template>

            <template v-else>
              <div class="space-y-1.5">
                <Label>URL</Label>
                <Input
                  v-model="formData.url"
                  placeholder="https://example.com/mcp"
                />
              </div>
              <div class="space-y-1.5">
                <Label>Headers</Label>
                <KeyValueEditor
                  v-model="headerPairs"
                  key-placeholder="Header-Name"
                  value-placeholder="Header-Value"
                />
              </div>
              <div class="space-y-1.5">
                <Label>Transport</Label>
                <Select v-model="formData.transport">
                  <SelectTrigger class="w-full">
                    <SelectValue />
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
            </template>

            <div class="flex items-center justify-between pt-2 border-t">
              <div class="flex items-center gap-2">
                <Label class="text-sm font-normal">{{ $t('mcp.active') }}</Label>
                <Switch
                  :model-value="formData.active"
                  @update:model-value="(val) => (formData.active = !!val)"
                />
              </div>
              <Button
                type="submit"
                :disabled="submitting || !formData.name.trim()"
              >
                <Spinner
                  v-if="submitting"
                  class="mr-1.5"
                />
                {{ $t('common.save') }}
              </Button>
            </div>
          </form>
        </div>
      </ScrollArea>

      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FontAwesomeIcon :icon="['fas', 'plug']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('mcp.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('mcp.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openCreateDialog"
          >
            {{ $t('common.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>

  <!-- Create dialog -->
  <Dialog v-model:open="createDialogOpen">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ $t('mcp.addTitle') }}</DialogTitle>
      </DialogHeader>
      <form
        class="mt-4 flex flex-col gap-4"
        @submit.prevent="handleCreateDraft"
      >
        <div class="space-y-1.5">
          <Label>{{ $t('common.name') }}</Label>
          <Input
            v-model="createName"
            :placeholder="$t('common.namePlaceholder')"
          />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('common.type') }}</Label>
          <Select v-model="createType">
            <SelectTrigger class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="stdio">
                  {{ $t('mcp.types.stdio') }}
                </SelectItem>
                <SelectItem value="remote">
                  {{ $t('mcp.types.remote') }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <DialogFooter>
          <DialogClose as-child>
            <Button variant="outline">
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            type="submit"
            :disabled="!createName.trim()"
          >
            {{ $t('common.confirm') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>

  <!-- Import dialog -->
  <Dialog v-model:open="importDialogOpen">
    <DialogContent class="sm:max-w-lg w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] sm:w-auto">
      <DialogHeader>
        <DialogTitle>{{ $t('common.import') }} MCP Servers</DialogTitle>
      </DialogHeader>
      <p class="text-sm text-muted-foreground mt-2">
        {{ $t('mcp.importHint') }}
      </p>
      <div class="h-[350px] rounded-md border overflow-hidden mt-3">
        <MonacoEditor
          v-model="importJson"
          language="json"
        />
      </div>
      <DialogFooter class="mt-4">
        <DialogClose as-child>
          <Button variant="outline">
            {{ $t('common.cancel') }}
          </Button>
        </DialogClose>
        <Button
          :disabled="importSubmitting || !importJson.trim()"
          @click="handleImportFromDialog"
        >
          <Spinner
            v-if="importSubmitting"
            class="mr-1.5"
          />
          {{ $t('common.import') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Export dialog -->
  <Dialog v-model:open="exportDialogOpen">
    <DialogContent class="sm:max-w-lg w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] sm:w-auto">
      <DialogHeader>
        <DialogTitle>{{ $t('common.export') }} mcpServers</DialogTitle>
      </DialogHeader>
      <div class="h-[350px] rounded-md border overflow-hidden mt-4">
        <MonacoEditor
          :model-value="exportJson"
          language="json"
          :readonly="true"
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
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Badge,
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Input,
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  Label,
  ScrollArea,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Spinner,
  Switch,
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
  Toggle,
} from '@memoh/ui'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import KeyValueEditor from '@/components/key-value-editor/index.vue'
import type { KeyValuePair } from '@/components/key-value-editor/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import {
  getBotsByBotIdMcp,
  postBotsByBotIdMcp,
  putBotsByBotIdMcpById,
  deleteBotsByBotIdMcpById,
} from '@memoh/sdk'
import type { McpUpsertRequest, McpImportRequest } from '@memoh/sdk'
import { client } from '@memoh/sdk/client'
import { resolveApiErrorMessage } from '@/utils/api-error'
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

const DRAFT_ID = ''

const IMPORT_EXAMPLE = JSON.stringify({
  mcpServers: {
    'example-stdio': {
      command: 'npx',
      args: ['-y', '@example/mcp-server'],
      env: { API_KEY: 'your-api-key' },
    },
    'example-remote': {
      url: 'https://example.com/mcp',
      headers: { Authorization: 'Bearer your-token' },
      transport: 'sse',
    },
  },
}, null, 2)

const props = defineProps<{ botId: string }>()
const { t } = useI18n()
const { copyText } = useClipboard()

const loading = ref(false)
const items = ref<McpItem[]>([])
const selectedItem = ref<McpItem | null>(null)
const searchText = ref('')
const submitting = ref(false)

const createDialogOpen = ref(false)
const createName = ref('')
const importDialogOpen = ref(false)
const exportDialogOpen = ref(false)
const importJson = ref('')
const importSubmitting = ref(false)
const exportJson = ref('')

const createType = ref<'stdio' | 'remote'>('stdio')
const connectionType = ref<'stdio' | 'remote'>('stdio')
const formData = ref({
  name: '',
  command: '',
  url: '',
  cwd: '',
  transport: 'http' as 'http' | 'sse',
  active: true,
})
const argsTags = ref<string[]>([])
const envPairs = ref<KeyValuePair[]>([])
const headerPairs = ref<KeyValuePair[]>([])

const isDraft = computed(() => selectedItem.value?.id === DRAFT_ID)

const filteredItems = computed(() => {
  if (!searchText.value) return items.value
  const kw = searchText.value.toLowerCase()
  return items.value.filter((i) => i.id === DRAFT_ID || i.name.toLowerCase().includes(kw))
})

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

function recordToPairs(record: Record<string, string>): KeyValuePair[] {
  return Object.entries(record).map(([key, value]) => ({ key, value }))
}

function pairsToRecord(pairs: KeyValuePair[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of pairs) {
    if (p.key.trim()) out[p.key.trim()] = p.value
  }
  return out
}

function selectItem(item: McpItem) {
  selectedItem.value = item
  const cfg = item.config ?? {}
  connectionType.value = item.type === 'stdio' ? 'stdio' : 'remote'
  formData.value = {
    name: item.name,
    command: configValue(cfg, 'command'),
    url: configValue(cfg, 'url'),
    cwd: configValue(cfg, 'cwd'),
    transport: item.type === 'sse' ? 'sse' : 'http',
    active: !!item.is_active,
  }
  argsTags.value = configArray(cfg, 'args')
  envPairs.value = recordToPairs(configMap(cfg, 'env'))
  headerPairs.value = recordToPairs(configMap(cfg, 'headers'))
}

function removeDraft() {
  items.value = items.value.filter((i) => i.id !== DRAFT_ID)
}

function openCreateDialog() {
  createName.value = ''
  createType.value = 'stdio'
  createDialogOpen.value = true
}

function openImportDialog() {
  importJson.value = IMPORT_EXAMPLE
  importDialogOpen.value = true
}

function handleCreateDraft() {
  const name = createName.value.trim()
  if (!name) return
  removeDraft()
  const draft: McpItem = {
    id: DRAFT_ID,
    name,
    type: createType.value === 'stdio' ? 'stdio' : 'http',
    config: {},
    is_active: true,
  }
  items.value = [draft, ...items.value]
  selectItem(draft)
  createDialogOpen.value = false
}

function itemToExportEntry(item: McpItem): McpServerEntry {
  const cfg = item.config ?? {}
  if (item.type === 'stdio') {
    return {
      command: configValue(cfg, 'command') || undefined,
      args: configArray(cfg, 'args').length ? configArray(cfg, 'args') : undefined,
      cwd: configValue(cfg, 'cwd') || undefined,
      env: Object.keys(configMap(cfg, 'env')).length ? configMap(cfg, 'env') : undefined,
    }
  }
  return {
    url: configValue(cfg, 'url') || undefined,
    headers: Object.keys(configMap(cfg, 'headers')).length ? configMap(cfg, 'headers') : undefined,
    transport: item.type === 'sse' ? 'sse' : undefined,
  }
}

function buildRequestBody(
  fd: typeof formData.value,
  mode: 'stdio' | 'remote',
  args: string[],
  env: KeyValuePair[],
  headers: KeyValuePair[],
): McpUpsertRequest {
  const body: McpUpsertRequest = {
    name: fd.name.trim(),
    is_active: fd.active,
  }
  if (mode === 'stdio') {
    body.command = fd.command.trim()
    if (args.length > 0) body.args = args
    const envRecord = pairsToRecord(env)
    if (Object.keys(envRecord).length > 0) body.env = envRecord
    if (fd.cwd.trim()) body.cwd = fd.cwd.trim()
  } else {
    body.url = fd.url.trim()
    const headerRecord = pairsToRecord(headers)
    if (Object.keys(headerRecord).length > 0) body.headers = headerRecord
    if (fd.transport === 'sse') body.transport = 'sse'
  }
  return body
}

async function loadList() {
  loading.value = true
  try {
    const { data } = await getBotsByBotIdMcp({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    const serverItems: McpItem[] = data.items ?? []
    const draft = items.value.find((i) => i.id === DRAFT_ID)
    items.value = draft ? [draft, ...serverItems] : serverItems

    if (selectedItem.value && selectedItem.value.id !== DRAFT_ID) {
      const still = serverItems.find((i) => i.id === selectedItem.value!.id)
      if (still) selectItem(still)
      else selectedItem.value = null
    }
    if (!selectedItem.value && items.value.length > 0) {
      selectItem(items.value[0])
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function handleSubmit() {
  if (!selectedItem.value) return
  submitting.value = true
  try {
    const body = buildRequestBody(formData.value, connectionType.value, argsTags.value, envPairs.value, headerPairs.value)
    if (isDraft.value) {
      const { data } = await postBotsByBotIdMcp({
        path: { bot_id: props.botId },
        body,
        throwOnError: true,
      })
      removeDraft()
      await loadList()
      const created = items.value.find((i) => i.id === data?.id) ?? items.value.find((i) => i.name === body.name)
      if (created) selectItem(created)
      toast.success(t('mcp.createSuccess'))
    } else {
      await putBotsByBotIdMcpById({
        path: { bot_id: props.botId, id: selectedItem.value.id },
        body,
        throwOnError: true,
      })
      await loadList()
      toast.success(t('mcp.updateSuccess'))
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(item: McpItem) {
  if (item.id === DRAFT_ID) {
    removeDraft()
    selectedItem.value = items.value.length > 0 ? items.value[0] : null
    if (selectedItem.value) selectItem(selectedItem.value)
    return
  }
  try {
    await deleteBotsByBotIdMcpById({
      path: { bot_id: props.botId, id: item.id },
      throwOnError: true,
    })
    if (selectedItem.value?.id === item.id) selectedItem.value = null
    await loadList()
    toast.success(t('mcp.deleteSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.deleteFailed')))
  }
}

async function handleImportFromDialog() {
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
    importDialogOpen.value = false
    importJson.value = ''
    await loadList()
    toast.success(t('mcp.importSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.importFailed')))
  } finally {
    importSubmitting.value = false
  }
}

function handleExportSingle() {
  if (!selectedItem.value || !selectedItem.value.id) return
  const mcpServers: Record<string, McpServerEntry> = {
    [selectedItem.value.name]: itemToExportEntry(selectedItem.value),
  }
  exportJson.value = JSON.stringify({ mcpServers }, null, 2)
  exportDialogOpen.value = true
}

function handleCopyExport() {
  void copyText(exportJson.value)
  toast.success(t('common.copied'))
}

watch(connectionType, (mode) => {
  if (mode === 'stdio') {
    formData.value.url = ''
    formData.value.transport = 'http'
    headerPairs.value = []
  } else {
    formData.value.command = ''
    formData.value.cwd = ''
    argsTags.value = []
    envPairs.value = []
  }
})

watch(() => props.botId, () => {
  if (props.botId) {
    selectedItem.value = null
    loadList()
  }
}, { immediate: true })
</script>
