<template>
  <SwapTransition :direction="direction">
    <!-- List: one server per card, two-up. The card opens its setup; a dashed
         tile beside real servers is the secondary "add" affordance. -->
    <PageShell
      v-if="view === 'list'"
      variant="tab"
      :title="$t('bots.tabs.mcp')"
    >
      <template #actions>
        <div
          v-if="showSearch"
          class="w-40 sm:w-56"
        >
          <InputGroup class="w-full">
            <InputGroupAddon align="inline-start">
              <Search class="size-3.5 text-muted-foreground" />
            </InputGroupAddon>
            <InputGroupInput
              v-model="searchText"
              :placeholder="$t('mcp.searchServers')"
            />
          </InputGroup>
        </div>
        <TooltipProvider>
          <Tooltip :delay-duration="300">
            <TooltipTrigger as-child>
              <Button
                variant="outline"
                size="icon"
                :aria-label="$t('common.import')"
                @click="startImport"
              >
                <Upload class="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {{ $t('common.import') }}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
        <Button @click="openCreate">
          <Plus class="size-4" />
          {{ $t('mcp.addNew') }}
        </Button>
      </template>

      <div
        v-if="loading && items.length === 0"
        class="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <Skeleton
          v-for="n in 4"
          :key="n"
          class="h-[4.5rem] w-full rounded-[var(--radius-menu-shell)]"
        />
      </div>

      <div
        v-else-if="items.length > 0"
        class="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <BackendCard
          v-for="item in filteredItems"
          :key="item.id"
          :name="item.name || $t('mcp.unnamedServer')"
          :subtitle="cardSubtitle(item)"
          :enabled="item.is_active"
          @click="openServer(item)"
        >
          <template #leading>
            <span class="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
              <component
                :is="item.type === 'stdio' ? Terminal : Globe"
                class="size-5"
              />
            </span>
          </template>
          <template #trailing>
            <div class="flex items-center gap-2">
              <Badge
                v-if="item.is_active && item.status === 'error'"
                variant="outline"
                size="sm"
                class="border-destructive/30 text-destructive"
              >
                {{ $t('mcp.statusError') }}
              </Badge>
              <ChevronRight class="size-4 shrink-0 text-muted-foreground/60" />
            </div>
          </template>
        </BackendCard>

        <button
          type="button"
          class="flex min-h-[4.5rem] items-center justify-center gap-2 rounded-[var(--radius-menu-shell)] border border-dashed border-border bg-background text-sm text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          @click="openCreate"
        >
          <Plus class="size-4" />
          {{ $t('mcp.addNew') }}
        </button>
      </div>

      <Empty
        v-else
        class="rounded-[var(--radius-menu-shell)] border border-border py-16"
      >
        <EmptyTitle>{{ $t('mcp.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('mcp.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openCreate"
          >
            <Plus class="size-4" />
            {{ $t('mcp.addNew') }}
          </Button>
        </EmptyContent>
      </Empty>
    </PageShell>

    <!-- Setup: the selected server only. Padding mirrors the list's PageShell tab
         variant so the back arrow lands at the list title's height. -->
    <section
      v-else
      class="mx-auto max-w-3xl pt-6 pb-8"
    >
      <Button
        variant="ghost"
        class="mb-6 text-foreground/85"
        @click="backToList()"
      >
        <ChevronLeft class="size-4" />
        {{ $t('bots.tabs.mcp') }}
      </Button>

      <McpServerDetail
        :key="detailKey"
        :bot-id="botId"
        :server="selectedServer"
        @created="onDetailCreated"
        @changed="loadList"
        @deleted="onDetailDeleted"
      />
    </section>
  </SwapTransition>

  <!-- Import: paste a standard mcpServers JSON; same-name servers are updated. -->
  <Dialog v-model:open="importOpen">
    <DialogContent class="flex max-h-[calc(100vh-2rem)] flex-col overflow-hidden p-0 sm:h-[70vh] sm:max-w-3xl">
      <DialogHeader class="shrink-0 border-b border-border p-4">
        <DialogTitle>{{ $t('mcp.importSandbox') }}</DialogTitle>
        <DialogDescription>{{ $t('mcp.importHint') }}</DialogDescription>
      </DialogHeader>

      <div class="min-h-0 flex-1 p-4">
        <div class="flex h-full min-h-0 flex-col overflow-hidden rounded-[var(--radius-menu-shell)] border border-border">
          <MonacoEditor
            v-model="importJson"
            language="json"
            :readonly="importSubmitting"
            class="min-h-0 flex-1"
            :options="{
              automaticLayout: true,
              fixedOverflowWidgets: true,
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
            }"
          />
        </div>
        <p
          v-if="importError"
          class="mt-2 text-xs text-destructive"
        >
          {{ importError }}
        </p>
      </div>

      <DialogFooter class="shrink-0 items-center gap-2 border-t border-border p-4 sm:justify-between">
        <Button
          variant="outline"
          :disabled="importSubmitting"
          @click="formatImportJson"
        >
          {{ $t('common.format') }}
        </Button>
        <div class="flex items-center gap-2">
          <DialogClose as-child>
            <Button
              variant="ghost"
              :disabled="importSubmitting"
            >
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            class="min-w-24"
            :disabled="importSubmitting || !importJson.trim()"
            @click="executeImport"
          >
            <Spinner
              v-if="importSubmitting"
              class="size-4"
            />
            {{ $t('mcp.blindImport') }}
          </Button>
        </div>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Badge, Button, Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, Empty, EmptyContent, EmptyDescription, EmptyTitle,
  InputGroup, InputGroupAddon, InputGroupInput, Skeleton, Spinner, toast, Tooltip,
  TooltipContent, TooltipProvider, TooltipTrigger,
} from '@memohai/ui'
import { ChevronLeft, ChevronRight, Globe, Plus, Search, Terminal, Upload } from 'lucide-vue-next'
import { getBotsByBotIdMcp, putBotsByBotIdMcpImport } from '@memohai/sdk'
import type { McpImportRequest, McpToolDescriptor } from '@memohai/sdk'
import McpServerDetail from './mcp-server-detail.vue'
import type { McpItem } from './mcp-types'
import PageShell from '@/components/page-shell/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import SwapTransition from '@/components/settings/swap-transition.vue'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{ botId: string }>()
const { t } = useI18n()

const { view, direction, openDetail, backToList } = useViewSwap()
const loading = ref(false)
const items = ref<McpItem[]>([])
const searchText = ref('')
const selectedId = ref('')
const selectedMcpId = useSyncedQueryParam('mcpId', '')

// A stable key for the open detail session: it only changes when the user opens
// a *different* server (or a new one), so the create→edit transition never
// remounts the form and discards its live probe state.
const detailKey = ref('')
let keySeq = 0

const importOpen = ref(false)
const importJson = ref('{\n  "mcpServers": {\n    \n  }\n}')
const importSubmitting = ref(false)
const importError = ref('')

const showSearch = computed(() => items.value.length > 0)

const filteredItems = computed(() => {
  const kw = searchText.value.trim().toLowerCase()
  if (!kw) return items.value
  return items.value.filter(i => (i.name || '').toLowerCase().includes(kw))
})

const selectedServer = computed(() => items.value.find(i => i.id === selectedId.value) ?? null)

function cardSubtitle(item: McpItem): string {
  const cfg = item.config ?? {}
  if (item.type === 'stdio') {
    const command = typeof cfg.command === 'string' ? cfg.command : ''
    return command
  }
  const url = typeof cfg.url === 'string' ? cfg.url : ''
  try {
    return url ? new URL(url).host : ''
  } catch {
    return url
  }
}

function openServer(item: McpItem) {
  selectedId.value = item.id
  detailKey.value = `s${++keySeq}`
  openDetail()
}

function openCreate() {
  selectedId.value = ''
  detailKey.value = `s${++keySeq}`
  openDetail()
}

function onDetailCreated(id: string) {
  selectedId.value = id
}

function onDetailDeleted() {
  selectedId.value = ''
  backToList()
  void loadList()
}

async function loadList() {
  loading.value = true
  try {
    const { data } = await getBotsByBotIdMcp({ path: { bot_id: props.botId } as unknown as { bot_id: string }, throwOnError: true })
    items.value = (data.items ?? []).map((item: Record<string, unknown>) => ({
      ...(item as unknown as McpItem),
      status: (item.status as string) ?? 'unknown',
      tools_cache: (item.tools_cache as McpToolDescriptor[]) ?? [],
      last_probed_at: (item.last_probed_at as string) ?? null,
      status_message: (item.status_message as string) ?? '',
      auth_type: (item.auth_type as string) ?? 'none',
    }))

    // Deep link: open the server named in ?mcpId= on first load.
    if (view.value === 'list' && selectedMcpId.value) {
      const target = items.value.find(i => i.id === selectedMcpId.value)
      if (target) openServer(target)
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.loadFailed')))
  } finally {
    loading.value = false
  }
}

function startImport() {
  importJson.value = '{\n  "mcpServers": {\n    \n  }\n}'
  importError.value = ''
  importOpen.value = true
}

function formatImportJson() {
  try {
    importJson.value = JSON.stringify(JSON.parse(importJson.value), null, 2)
    importError.value = ''
  } catch {
    importError.value = t('mcp.importErrorJson')
  }
}

async function executeImport() {
  importSubmitting.value = true
  importError.value = ''
  try {
    let parsed: McpImportRequest = JSON.parse(importJson.value)
    if (!parsed.mcpServers && typeof parsed === 'object') {
      parsed = { mcpServers: parsed as McpImportRequest['mcpServers'] }
    }
    await putBotsByBotIdMcpImport({ path: { bot_id: props.botId } as unknown as { bot_id: string }, body: parsed, throwOnError: true })
    importOpen.value = false
    await loadList()
    toast.success(t('mcp.importSuccess'))
  } catch (error) {
    importError.value = error instanceof SyntaxError
      ? t('mcp.importErrorJson')
      : resolveApiErrorMessage(error, t('mcp.importErrorFormat'))
  } finally {
    importSubmitting.value = false
  }
}

watch(() => props.botId, () => { if (props.botId) void loadList() }, { immediate: true })

watch([view, selectedId], () => {
  selectedMcpId.value = view.value === 'detail' && selectedId.value ? selectedId.value : ''
})
</script>
