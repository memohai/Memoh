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
            <Search />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </template>

    <template #sidebar-content>
      <div
        v-if="loading && items.length === 0"
        class="flex items-center gap-2 text-xs text-muted-foreground p-4"
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
                  :class="statusDotClass(item)"
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
          <Plus
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
        <SettingsShell
          width="wide"
          class="space-y-6"
        >
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold">
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
            class="grid gap-4 md:grid-cols-2"
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
              <div class="space-y-1.5 md:col-span-2">
                <Label>{{ $t('mcp.command') }}</Label>
                <Input
                  v-model="formData.command"
                  :placeholder="$t('mcp.commandPlaceholder')"
                />
              </div>
              <div class="space-y-1.5 md:col-span-2">
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
              <div class="space-y-1.5 md:col-span-2">
                <Label>{{ $t('mcp.env') }}</Label>
                <KeyValueEditor
                  v-model="envPairs"
                  key-placeholder="KEY"
                  value-placeholder="VALUE"
                />
              </div>
              <div class="space-y-1.5 md:col-span-2">
                <Label>{{ $t('mcp.cwd') }}</Label>
                <Input
                  v-model="formData.cwd"
                  :placeholder="$t('mcp.cwdPlaceholder')"
                />
              </div>
            </template>

            <template v-else>
              <div class="space-y-1.5 md:col-span-2">
                <Label>URL</Label>
                <Input
                  v-model="formData.url"
                  placeholder="https://example.com/mcp"
                />
              </div>
              <div class="space-y-1.5 md:col-span-2">
                <Label>Headers</Label>
                <KeyValueEditor
                  v-model="headerPairs"
                  key-placeholder="Header-Name"
                  value-placeholder="Header-Value"
                />
              </div>
              <div class="space-y-1.5">
                <Label>Transport</Label>
                <Select
                  v-model="formData.transport"
                >
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

            <div class="flex items-center justify-between pt-2 border-t md:col-span-2">
              <div class="flex items-center gap-2">
                <Label class="text-xs font-normal">{{ $t('mcp.active') }}</Label>
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

          <!-- Probe status & tools -->
          <div
            v-if="selectedItem.id"
            class="border-t pt-4 space-y-4"
          >
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <h4 class="text-xs font-medium">
                  {{ $t('mcp.tools') }}
                </h4>
                <Badge
                  v-if="selectedItem.status === 'connected'"
                  variant="outline"
                  class="text-[10px] text-green-600"
                >
                  {{ $t('mcp.statusConnected') }}
                </Badge>
                <Badge
                  v-else-if="selectedItem.status === 'error'"
                  variant="outline"
                  class="text-[10px] text-destructive"
                >
                  {{ $t('mcp.statusError') }}
                </Badge>
                <Badge
                  v-else
                  variant="outline"
                  class="text-[10px] text-muted-foreground"
                >
                  {{ $t('mcp.statusUnknown') }}
                </Badge>
              </div>
              <Button
                variant="outline"
                size="sm"
                :disabled="probing"
                @click="handleProbe(selectedItem!)"
              >
                <Spinner
                  v-if="probing"
                  class="mr-1.5"
                />
                <RefreshCw
                  v-else
                  class="mr-1.5"
                />
                {{ probing ? $t('mcp.probing') : $t('mcp.probe') }}
              </Button>
            </div>

            <div
              v-if="selectedItem.status_message && selectedItem.status === 'error'"
              class="text-xs text-destructive bg-destructive/5 rounded-md p-3"
            >
              {{ selectedItem.status_message }}
            </div>

            <div
              v-if="probeAuthRequired"
              class="text-xs text-amber-600 bg-amber-50 dark:bg-amber-900/20 rounded-md p-3 flex items-center gap-2"
            >
              <Lock />
              {{ $t('mcp.authRequired') }}
            </div>

            <!-- OAuth section (remote connections only) -->
            <div
              v-if="selectedItem.type !== 'stdio' && (probeAuthRequired || selectedItem.auth_type === 'oauth' || oauthStatus)"
              class="border rounded-md p-4 space-y-3"
            >
              <div class="flex items-center justify-between">
                <h4 class="text-xs font-medium">
                  {{ $t('mcp.oauth.title') }}
                </h4>
                <Badge
                  v-if="oauthStatus?.has_token && !oauthStatus?.expired"
                  variant="outline"
                  class="text-[10px] text-green-600"
                >
                  {{ $t('mcp.oauth.authorized') }}
                </Badge>
                <Badge
                  v-else-if="oauthStatus?.has_token && oauthStatus?.expired"
                  variant="outline"
                  class="text-[10px] text-amber-600"
                >
                  {{ $t('mcp.oauth.expired') }}
                </Badge>
                <Badge
                  v-else-if="oauthStatus?.configured"
                  variant="outline"
                  class="text-[10px] text-muted-foreground"
                >
                  {{ $t('mcp.oauth.notConfigured') }}
                </Badge>
              </div>

              <div
                v-if="oauthStatus?.auth_server"
                class="text-xs text-muted-foreground"
              >
                {{ $t('mcp.oauth.authServer') }}: {{ oauthStatus.auth_server }}
              </div>

              <!-- Client ID input (shown when server doesn't support DCR) -->
              <div
                v-if="oauthNeedsClientId && (!oauthStatus?.has_token || oauthStatus?.expired)"
                class="space-y-2"
              >
                <p class="text-xs text-muted-foreground">
                  {{ $t('mcp.oauth.clientIdHint') }}
                </p>
                <div class="space-y-1.5">
                  <Label class="text-xs">
                    {{ $t('mcp.oauth.clientId') }}
                  </Label>
                  <Input
                    v-model="oauthClientId"
                    :placeholder="$t('mcp.oauth.clientIdPlaceholder')"
                    class="font-mono text-xs"
                  />
                </div>
                <div class="space-y-1.5">
                  <Label class="text-xs">
                    {{ $t('mcp.oauth.clientSecret') }}
                  </Label>
                  <Input
                    v-model="oauthClientSecret"
                    type="password"
                    :placeholder="$t('mcp.oauth.clientSecretPlaceholder')"
                    class="font-mono text-xs"
                  />
                </div>
                <div
                  v-if="oauthCallbackUrl"
                  class="space-y-1"
                >
                  <Label class="text-xs">
                    {{ $t('mcp.oauth.callbackUrl') }}
                  </Label>
                  <div class="flex items-center gap-1.5">
                    <code class="text-xs bg-muted px-2 py-1 rounded flex-1 break-all select-all">{{ oauthCallbackUrl }}</code>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      @click="copyText(oauthCallbackUrl); toast.success($t('common.copied'))"
                    >
                      <Copy />
                    </Button>
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ $t('mcp.oauth.callbackUrlHint') }}
                  </p>
                </div>
              </div>

              <div class="flex gap-2">
                <Button
                  v-if="!oauthStatus?.has_token || oauthStatus?.expired"
                  variant="outline"
                  size="sm"
                  :disabled="oauthDiscovering || oauthAuthorizing || (oauthNeedsClientId && !oauthClientId.trim())"
                  @click="handleOAuthFlow"
                >
                  <Spinner
                    v-if="oauthDiscovering || oauthAuthorizing"
                    class="mr-1.5"
                  />
                  <KeyRound
                    v-else
                    class="mr-1.5"
                  />
                  {{ oauthDiscovering ? $t('mcp.oauth.discovering') : oauthAuthorizing ? $t('mcp.oauth.authorizing') : $t('mcp.oauth.authorize') }}
                </Button>
                <Button
                  v-if="oauthStatus?.has_token"
                  variant="ghost"
                  size="sm"
                  @click="handleOAuthRevoke"
                >
                  {{ $t('mcp.oauth.revoke') }}
                </Button>
              </div>
            </div>

            <div
              v-if="displayTools.length > 0"
              class="space-y-1"
            >
              <p class="text-xs text-muted-foreground mb-2">
                {{ $t('mcp.toolsCount', { count: displayTools.length }) }}
              </p>
              <div
                v-for="tool in displayTools"
                :key="tool.name"
                class="flex items-start gap-2 py-1.5 px-2 rounded text-xs hover:bg-accent/50"
              >
                <Wrench
                  class="mt-1 text-muted-foreground shrink-0 text-xs"
                />
                <div class="min-w-0">
                  <span class="font-mono text-xs font-medium">{{ tool.name }}</span>
                  <p
                    v-if="tool.description"
                    class="text-xs text-muted-foreground truncate"
                  >
                    {{ tool.description }}
                  </p>
                </div>
              </div>
            </div>
            <p
              v-else-if="selectedItem.status === 'connected'"
              class="text-xs text-muted-foreground"
            >
              {{ $t('mcp.toolsEmpty') }}
            </p>

            <p
              v-if="selectedItem.last_probed_at"
              class="text-xs text-muted-foreground"
            >
              {{ $t('mcp.lastProbed') }}: {{ formatDate(selectedItem.last_probed_at) }}
            </p>
          </div>
        </SettingsShell>
      </ScrollArea>

      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Plug />
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
      <p class="text-xs text-muted-foreground mt-2">
        {{ $t('mcp.importHint') }}
      </p>
      <div class="h-87.5 rounded-md border overflow-hidden mt-3">
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
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{{ $t('common.export') }} mcpServers</DialogTitle>
      </DialogHeader>
      <div class="h-87.5 rounded-md border overflow-hidden mt-4">
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
import { Search, Plus, RefreshCw, Lock, Copy, KeyRound, Wrench, Plug } from 'lucide-vue-next'
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
} from '@memohai/ui'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'
import SettingsShell from '@/components/settings-shell/index.vue'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import KeyValueEditor from '@/components/key-value-editor/index.vue'
import type { KeyValuePair } from '@/components/key-value-editor/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import {
  getBotsByBotIdMcp,
  postBotsByBotIdMcp,
  putBotsByBotIdMcpById,
  deleteBotsByBotIdMcpById,
  postBotsByBotIdMcpByIdProbe,
  putBotsByBotIdMcpImport,
  getBotsByBotIdMcpByIdOauthStatus,
  postBotsByBotIdMcpByIdOauthDiscover,
  postBotsByBotIdMcpByIdOauthAuthorize,
  deleteBotsByBotIdMcpByIdOauthToken,
} from '@memohai/sdk'
import type {
  McpUpsertRequest,
  McpImportRequest,
  McpToolDescriptor,
  McpMcpServerEntry,
  McpOAuthStatus,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useClipboard } from '@/composables/useClipboard'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useSupermarketMcpDraft } from '@/stores/supermarket-mcp-draft'

interface McpItem {
  id: string
  name: string
  type: string
  config: Record<string, unknown>
  is_active: boolean
  status: string
  tools_cache: McpToolDescriptor[]
  last_probed_at: string | null
  status_message: string
  auth_type: string
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
const selectedMcpId = useSyncedQueryParam('mcpId', '')
const searchText = ref('')
const submitting = ref(false)
const probing = ref(false)
const probeAuthRequired = ref(false)
const oauthDiscovering = ref(false)
const oauthAuthorizing = ref(false)

const oauthStatus = ref<McpOAuthStatus | null>(null)
const oauthClientId = ref('')
const oauthClientSecret = ref('')
const oauthNeedsClientId = ref(false)
const oauthCallbackUrl = ref('')
const oauthDiscovered = ref(false)

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

const displayTools = computed<McpToolDescriptor[]>(() => {
  if (!selectedItem.value) return []
  return selectedItem.value.tools_cache ?? []
})

function statusDotClass(item: McpItem): string {
  if (!item.id) return 'bg-muted-foreground/40'
  if (!item.is_active) return 'bg-muted-foreground/40'
  switch (item.status) {
    case 'connected': return 'bg-green-500'
    case 'error': return 'bg-destructive'
    default: return 'bg-amber-400'
  }
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return ''
  try {
    return new Date(dateStr).toLocaleString()
  } catch {
    return dateStr
  }
}

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
  probeAuthRequired.value = false
  oauthStatus.value = null
  oauthClientId.value = ''
  oauthClientSecret.value = ''
  oauthNeedsClientId.value = false
  oauthCallbackUrl.value = ''
  oauthDiscovered.value = false
  if (item.id && item.type !== 'stdio') {
    loadOAuthStatus(item)
  }
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
    status: 'unknown',
    tools_cache: [],
    last_probed_at: null,
    status_message: '',
    auth_type: 'none',
  }
  items.value = [draft, ...items.value]
  selectItem(draft)
  createDialogOpen.value = false
}

function itemToExportEntry(item: McpItem): McpMcpServerEntry {
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
    const serverItems: McpItem[] = (data.items ?? []).map((item: Record<string, unknown>) => ({
      ...item,
      status: item.status ?? 'unknown',
      tools_cache: item.tools_cache ?? [],
      last_probed_at: item.last_probed_at ?? null,
      status_message: item.status_message ?? '',
      auth_type: item.auth_type ?? 'none',
    }))
    const draft = items.value.find((i) => i.id === DRAFT_ID)
    items.value = draft ? [draft, ...serverItems] : serverItems

    if (selectedItem.value && selectedItem.value.id !== DRAFT_ID) {
      const still = serverItems.find((i) => i.id === selectedItem.value!.id)
      if (still) selectItem(still)
      else selectedItem.value = null
    }
    if (!selectedItem.value && items.value.length > 0) {
      const fromUrl = selectedMcpId.value
        ? items.value.find((i) => i.id === selectedMcpId.value)
        : null
      selectItem(fromUrl ?? items.value[0])
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function handleProbe(item: McpItem) {
  if (!item.id) return
  probing.value = true
  probeAuthRequired.value = false
  try {
    const { data } = await postBotsByBotIdMcpByIdProbe({
      path: { bot_id: props.botId, id: item.id },
      throwOnError: true,
    })
    if (data) {
      item.status = data.status ?? item.status
      item.tools_cache = data.tools ?? []
      item.status_message = data.error ?? ''
      item.last_probed_at = new Date().toISOString()
      probeAuthRequired.value = !!data.auth_required
      if (data.status === 'connected') {
        toast.success(t('mcp.probeSuccess'))
      } else if (data.auth_required) {
        toast.warning(t('mcp.authRequired'))
      } else {
        toast.error(data.error || t('mcp.probeFailed'))
      }
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.probeFailed')))
  } finally {
    probing.value = false
  }
}

async function handleSubmit() {
  if (!selectedItem.value) return
  submitting.value = true
  try {
    const body = buildRequestBody(formData.value, connectionType.value, argsTags.value, envPairs.value, headerPairs.value)
    let savedId: string | undefined
    if (isDraft.value) {
      const { data } = await postBotsByBotIdMcp({
        path: { bot_id: props.botId },
        body,
        throwOnError: true,
      })
      savedId = data?.id
      removeDraft()
      await loadList()
      const created = items.value.find((i) => i.id === savedId) ?? items.value.find((i) => i.name === body.name)
      if (created) selectItem(created)
      toast.success(t('mcp.createSuccess'))
    } else {
      savedId = selectedItem.value.id
      await putBotsByBotIdMcpById({
        path: { bot_id: props.botId, id: selectedItem.value.id },
        body,
        throwOnError: true,
      })
      await loadList()
      toast.success(t('mcp.updateSuccess'))
    }
    if (savedId && selectedItem.value) {
      handleProbe(selectedItem.value)
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
    await putBotsByBotIdMcpImport({
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
  const mcpServers: Record<string, McpMcpServerEntry> = {
    [selectedItem.value.name]: itemToExportEntry(selectedItem.value),
  }
  exportJson.value = JSON.stringify({ mcpServers }, null, 2)
  exportDialogOpen.value = true
}

function handleCopyExport() {
  void copyText(exportJson.value)
  toast.success(t('common.copied'))
}

async function loadOAuthStatus(item: McpItem) {
  if (!item.id || item.type === 'stdio') {
    oauthStatus.value = null
    return
  }
  try {
    const { data } = await getBotsByBotIdMcpByIdOauthStatus({
      path: { bot_id: props.botId, id: item.id },
      throwOnError: true,
    })
    oauthStatus.value = data ?? null
    oauthCallbackUrl.value = `${window.location.origin}/oauth/mcp/callback`
  } catch {
    oauthStatus.value = null
  }
}

async function handleOAuthDiscover() {
  if (!selectedItem.value?.id) return
  const item = selectedItem.value

  oauthDiscovering.value = true
  oauthNeedsClientId.value = false
  try {
    const { data } = await postBotsByBotIdMcpByIdOauthDiscover({
      path: { bot_id: props.botId, id: item.id },
      throwOnError: true,
    })
    toast.success(t('mcp.oauth.discoverSuccess'))
    if (!data?.registration_endpoint) {
      oauthNeedsClientId.value = true
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.oauth.discoverFailed')))
    oauthDiscovering.value = false
    return false
  }
  oauthDiscovering.value = false
  return true
}

async function handleOAuthFlow() {
  if (!selectedItem.value?.id) return
  const item = selectedItem.value

  if (!oauthDiscovered.value) {
    const discovered = await handleOAuthDiscover()
    if (!discovered) return
    oauthDiscovered.value = true
    if (oauthNeedsClientId.value && !oauthClientId.value.trim()) {
      return
    }
  }

  oauthAuthorizing.value = true
  try {
    const { data } = await postBotsByBotIdMcpByIdOauthAuthorize({
      path: { bot_id: props.botId, id: item.id },
      body: {
        client_id: oauthClientId.value.trim() || undefined,
        client_secret: oauthClientSecret.value.trim() || undefined,
        callback_url: `${window.location.origin}/oauth/mcp/callback`,
      },
      throwOnError: true,
    })
    if (!data?.authorization_url) {
      toast.error(t('mcp.oauth.authFailed'))
      oauthAuthorizing.value = false
      return
    }

    const popup = window.open(data.authorization_url, 'mcp-oauth', 'width=600,height=700')

    const onMessage = async (event: MessageEvent) => {
      if (event.data?.type === 'mcp-oauth-callback') {
        window.removeEventListener('message', onMessage)
        oauthAuthorizing.value = false
        if (event.data.status === 'success') {
          toast.success(t('mcp.oauth.authSuccess'))
          await loadOAuthStatus(item)
          handleProbe(item)
        } else {
          toast.error(event.data.error || t('mcp.oauth.authFailed'))
        }
      }
    }
    window.addEventListener('message', onMessage)

    const pollTimer = setInterval(() => {
      if (popup && popup.closed) {
        clearInterval(pollTimer)
        window.removeEventListener('message', onMessage)
        oauthAuthorizing.value = false
        loadOAuthStatus(item)
      }
    }, 500)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.oauth.authFailed')))
    oauthAuthorizing.value = false
  }
}

async function handleOAuthRevoke() {
  if (!selectedItem.value?.id) return
  try {
    await deleteBotsByBotIdMcpByIdOauthToken({
      path: { bot_id: props.botId, id: selectedItem.value.id },
      throwOnError: true,
    })
    toast.success(t('mcp.oauth.revokeSuccess'))
    oauthDiscovered.value = false
    oauthNeedsClientId.value = false
    oauthClientId.value = ''
    oauthClientSecret.value = ''
    await loadOAuthStatus(selectedItem.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.oauth.revokeFailed')))
  }
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

const { consumePendingDraft } = useSupermarketMcpDraft()

function applyPendingDraft() {
  const entry = consumePendingDraft()
  if (!entry) return

  removeDraft()
  const isStdio = entry.transport === 'stdio'
  const cfg: Record<string, unknown> = {}

  if (isStdio) {
    if (entry.command) cfg.command = entry.command
    if (entry.args?.length) cfg.args = entry.args
    const envMap: Record<string, string> = {}
    if (entry.env) {
      for (const e of entry.env) {
        if (e.key) envMap[e.key] = e.defaultValue ?? ''
      }
    }
    if (Object.keys(envMap).length) cfg.env = envMap
  } else {
    if (entry.url) cfg.url = entry.url
    const headerMap: Record<string, string> = {}
    if (entry.headers) {
      for (const h of entry.headers) {
        if (h.key) headerMap[h.key] = h.defaultValue ?? ''
      }
    }
    if (Object.keys(headerMap).length) cfg.headers = headerMap
  }

  const draft: McpItem = {
    id: DRAFT_ID,
    name: entry.name ?? '',
    type: isStdio ? 'stdio' : (entry.transport === 'sse' ? 'sse' : 'http'),
    config: cfg,
    is_active: true,
    status: 'unknown',
    tools_cache: [],
    last_probed_at: null,
    status_message: '',
    auth_type: 'none',
  }
  items.value = [draft, ...items.value.filter((i) => i.id !== DRAFT_ID)]
  selectItem(draft)
}

watch(() => props.botId, async () => {
  if (props.botId) {
    selectedItem.value = null
    await loadList()
    applyPendingDraft()
  }
}, { immediate: true })

// Keep ?mcpId in sync with the currently selected connection (drafts ignored).
watch(selectedItem, (item) => {
  const next = item?.id && item.id !== DRAFT_ID ? item.id : ''
  if (selectedMcpId.value !== next) {
    selectedMcpId.value = next
  }
})

// React to URL changes mid-session (e.g. arriving from the chat-home MCP tab).
watch(selectedMcpId, (id) => {
  if (!id) return
  if (selectedItem.value?.id === id) return
  const target = items.value.find((i) => i.id === id)
  if (target) selectItem(target)
})
</script>
