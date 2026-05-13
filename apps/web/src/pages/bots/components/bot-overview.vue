<template>
  <div class="max-w-2xl mx-auto pb-12 space-y-5">
    <!-- Top Action Bar -->
    <div class="flex items-start justify-between pb-4 border-b border-border/50">
      <div class="space-y-1">
        <h3 class="text-sm font-semibold text-foreground">
          {{ $t('bots.tabs.overview') }}
        </h3>
        <p class="text-[11px] text-muted-foreground">
          {{ $t('bots.tabs.overviewSubtitle') }}
        </p>
      </div>
      <div class="flex items-center gap-3">
        <p class="text-[11px] text-muted-foreground/60 hidden sm:block mt-1 tracking-wide">
          {{ $t('bots.checks.lastSync', { time: lastSyncTime }) }}
        </p>
        <Button
          size="sm"
          class="h-8 text-xs font-medium min-w-24 shadow-none mt-1"
          :disabled="checksLoading"
          @click="handleRefreshChecks"
        >
          <RotateCcw
            v-if="!checksLoading"
            class="size-3.5 shrink-0 mr-1.5"
          />
          <Spinner
            v-else
            class="size-3.5 shrink-0 mr-1.5"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>
    </div>

    <!-- Hero Section -->
    <div class="space-y-4 rounded-md border p-4">
      <!-- Section Header -->
      <div class="space-y-1">
        <h4 class="text-xs font-medium">
          {{ $t('bots.checks.title') }}
        </h4>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.checks.subtitle') }}
        </p>
      </div>

      <!-- Metrics Grid -->
      <div class="grid gap-3 sm:grid-cols-2">
        <!-- Health Status Card -->
        <div class="rounded-md border bg-background/70 p-3 flex flex-col justify-between">
          <div class="space-y-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.checks.statusTitle') }}
            </p>
            <div class="flex items-center gap-2 mt-2">
              <component 
                :is="hasIssue ? AlertCircle : CheckCircle2" 
                :class="[
                  'w-5 h-5 shrink-0 transition-colors',
                  hasIssue ? 'text-destructive' : 'text-foreground'
                ]"
              />
              <p class="text-2xl font-semibold">
                {{ hasIssue ? $t('bots.checks.status.issue') : $t('bots.checks.status.stable') }}
              </p>
            </div>
          </div>
          <div class="mt-3 min-h-[32px] flex items-center">
            <p class="text-[11px] leading-4 text-muted-foreground break-words">
              {{ checksSummaryText }}
            </p>
          </div>
        </div>

        <!-- System Vitality Card -->
        <div class="rounded-md border bg-background/70 p-3 flex flex-col justify-between">
          <div class="space-y-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.checks.vitalityTitle') }}
            </p>
            <div class="mt-2">
              <span class="text-2xl font-semibold">{{ okCount }}</span>
              <span class="text-xs font-medium text-muted-foreground ml-1">/ {{ totalCount }} {{ $t('bots.checks.vitalityUnit') }}</span>
            </div>
          </div>
          
          <!-- Segmented Progress Bar -->
          <div class="mt-3 min-h-[32px] flex items-center w-full">
            <div class="flex gap-1 h-1.5 w-full">
              <div 
                v-for="(check, index) in checks" 
                :key="index"
                class="h-full rounded-full transition-all duration-500"
                :class="[
                  check.status === 'ok' ? 'bg-brand' : 
                  check.status === 'error' ? 'bg-destructive' : 'bg-muted-foreground/40',
                  'flex-1'
                ]"
              />
              <div
                v-if="checks.length === 0"
                class="h-full w-full bg-muted rounded-full"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Diagnostic Log Section -->
    <div class="space-y-4 rounded-md border p-4">
      <!-- Diagnostic Header and Controls -->
      <div class="flex items-center justify-between">
        <div class="space-y-1">
          <h4 class="text-xs font-medium">
            {{ $t('bots.checks.diagnosticTitle') }}
          </h4>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.checks.diagnosticSubtitle') }}
          </p>
        </div>
        <div
          v-if="checks.length > 0"
          class="flex gap-2"
        >
          <Button 
            variant="ghost" 
            size="sm" 
            class="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
            @click="toggleAll(true)"
          >
            {{ $t('bots.checks.expandAll') }}
          </Button>
          <Button 
            variant="ghost" 
            size="sm" 
            class="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
            @click="toggleAll(false)"
          >
            {{ $t('bots.checks.collapse') }}
          </Button>
        </div>
      </div>

      <!-- Empty State -->
      <div 
        v-if="!checksLoading && checks.length === 0" 
        class="rounded-md border border-dashed p-4 text-center py-10"
      >
        <Activity class="w-5 h-5 text-muted-foreground/50 mx-auto mb-2" />
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.checks.empty') }}
        </p>
      </div>

      <!-- Loading State -->
      <div
        v-else-if="checksLoading && checks.length === 0"
        class="space-y-2"
      >
        <div
          v-for="i in 5"
          :key="i"
          class="rounded-md border transition-opacity opacity-60"
        >
          <div class="flex w-full items-center justify-between gap-3 px-3 py-2">
            <div class="flex items-center gap-3 min-w-0 w-full">
              <div class="w-4 h-4 shrink-0 rounded-full bg-muted/40 animate-pulse" />
              <div class="min-w-0 space-y-0.5 w-full">
                <div class="h-3 w-32 rounded bg-muted/40 animate-pulse" />
              </div>
            </div>
            <div class="size-4 shrink-0 rounded-sm bg-muted/40 animate-pulse" />
          </div>
        </div>
      </div>

      <!-- Smart Collapsible List -->
      <div
        v-else
        class="space-y-2"
      >
        <Collapsible
          v-for="item in checks"
          :key="item.id"
          :open="expandedIds.has(item.id)"
          class="rounded-md border transition-opacity"
          :class="item.status === 'ok' ? 'opacity-60 hover:opacity-100' : 'opacity-100'"
        >
          <CollapsibleTrigger 
            class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-accent/40"
            @click="toggleItem(item.id)"
          >
            <div class="flex items-center gap-3 min-w-0">
              <component 
                :is="getStatusIcon(item.status)" 
                :class="['w-4 h-4 shrink-0', getStatusColor(item.status)]"
              />
              <div class="min-w-0 space-y-0.5">
                <p class="text-xs font-medium leading-none truncate">
                  {{ checkTitleLabel(item) }}
                </p>
                <p
                  v-if="item.subtitle"
                  class="text-[10px] text-muted-foreground truncate"
                >
                  {{ item.subtitle }}
                </p>
              </div>
            </div>
            <ChevronRight 
              class="size-4 shrink-0 text-muted-foreground transition-transform duration-200"
              :class="{ 'rotate-90': expandedIds.has(item.id) }"
            />
          </CollapsibleTrigger>
          
          <CollapsibleContent>
            <div class="space-y-3 border-t px-3 py-3">
              <p class="text-xs text-foreground leading-relaxed">
                {{ item.summary }}
              </p>
              
              <!-- Code Block with Syntax Highlighting -->
              <div
                v-if="item.detail"
                class="group/code relative rounded border bg-muted/30"
              >
                <div class="absolute right-1.5 top-1.5 opacity-0 group-hover/code:opacity-100 transition-opacity">
                  <Button 
                    variant="ghost" 
                    size="icon" 
                    class="h-6 w-6" 
                    @click.stop="copyToClipboard(item.detail)"
                  >
                    <Copy class="w-3 h-3" />
                  </Button>
                </div>
                <!-- eslint-disable-next-line vue/no-v-html -->
                <pre class="p-3 font-mono text-[11px] leading-relaxed overflow-x-auto max-h-[240px] overflow-y-auto select-text whitespace-pre-wrap"><code v-html="highlightCode(item.detail)" /></pre>
              </div>
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { 
  getBotsByIdChecks, 
  getBotsById, 
  type BotsBotCheck 
} from '@memohai/sdk'
import { useRoute } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { 
  Button, 
  Spinner, 
  Collapsible, 
  CollapsibleTrigger, 
  CollapsibleContent 
} from '@memohai/ui'
import { 
  CheckCircle2, 
  AlertCircle, 
  AlertTriangle, 
  XCircle, 
  ChevronRight, 
  RotateCcw, 
  Activity,
  Copy,
  HelpCircle
} from 'lucide-vue-next'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'

type BotCheck = BotsBotCheck

const checksLoading = ref(false)
const checks = ref<BotCheck[]>([])
const expandedIds = ref<Set<string>>(new Set())
const lastSyncTime = ref('--:--')

const route = useRoute()
const botId = computed(() => route.params.botId as string)
const activeTab = useSyncedQueryParam('tab', 'overview')
const { t } = useI18n()

// Data Fetching
const { data: bot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

const { hasIssue } = useBotStatusMeta(bot, t)

const okCount = computed(() => checks.value.filter(c => c.status === 'ok').length)
const totalCount = computed(() => checks.value.length)

// Watchers
watch([activeTab, botId], ([tab]) => {
  if (botId.value && tab === 'overview') {
    void loadChecks(true)
  }
}, { immediate: true })

async function loadChecks(showToast: boolean) {
  checksLoading.value = true
  try {
    const { data } = await getBotsByIdChecks({ path: { id: botId.value }, throwOnError: true })
    checks.value = data?.items ?? []
    lastSyncTime.value = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
    
    // Smart collapse logic: expand abnormal items by default
    expandedIds.value = new Set(
      checks.value.filter(c => c.status !== 'ok').map(c => c.id)
    )
  } catch (error) {
    if (showToast) {
      toast.error(resolveApiErrorMessage(error, t('bots.checks.loadFailed')))
    }
  } finally {
    checksLoading.value = false
  }
}

// Interactivity
function toggleItem(id: string) {
  if (expandedIds.value.has(id)) {
    expandedIds.value.delete(id)
  } else {
    expandedIds.value.add(id)
  }
}

function toggleAll(expand: boolean) {
  if (expand) {
    expandedIds.value = new Set(checks.value.map(c => c.id))
  } else {
    expandedIds.value.clear()
  }
}

async function handleRefreshChecks() {
  await loadChecks(true)
  toast.info(t('common.refresh')) 
}

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text)
  toast.success(t('common.copied'))
}

// Helper: Syntax Highlighting
function highlightCode(text: string): string {
  return text
    .replace(/(error|fail|failed|denied)/gi, '<span class="text-destructive font-bold">$1</span>')
    .replace(/(warn|warning)/gi, '<span class="text-warning font-bold">$1</span>')
    .replace(/(\/([^\s\/\:]+\/)*[^\s\/\:]+)/g, '<span class="text-foreground underline decoration-muted-foreground/30">$1</span>')
}

// Helper: Icons & Colors
function getStatusIcon(status: BotCheck['status']) {
  if (status === 'error') return XCircle
  if (status === 'warn') return AlertTriangle
  if (status === 'ok') return CheckCircle2
  return HelpCircle
}

function getStatusColor(status: BotCheck['status']) {
  if (status === 'error') return 'text-destructive'
  if (status === 'warn') return 'text-warning'
  if (status === 'ok') return 'text-foreground/40' 
  return 'text-muted-foreground'
}

// Original labels mapping (Locked Business Logic)
function checkTitleLabel(item: BotCheck): string {
  const titleKey = (item.title_key ?? '').trim()
  if (titleKey) {
    const translated = t(titleKey)
    if (translated !== titleKey) return translated
  }
  return (item.type ?? '').trim() || (item.id ?? '').trim() || '-'
}

const checksSummaryText = computed(() => {
  const issueCount = checks.value.filter((item) => item.status === 'warn' || item.status === 'error').length
  if (issueCount > 0) return t('bots.checks.issueCount', { count: issueCount })
  return checks.value.length === 0 ? t('bots.checks.empty') : t('bots.checks.ok')
})
</script>
