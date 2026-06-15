<template>
  <!-- A single item renders as a bare row (no group header). -->
  <ToolCallInline
    v-if="items.length === 1 && single && single.type === 'tool'"
    :block="(single as ToolCallBlockType)"
  />
  <ThinkingBlock
    v-else-if="items.length === 1 && single && single.type === 'reasoning'"
    :block="(single as ThinkingBlockType)"
    :streaming="active === true"
  />

  <!-- Multiple items collapse into one process block. -->
  <div
    v-else
    class="text-[0.8125rem] leading-[1.125rem] font-[400]"
  >
    <button
      class="group/h flex items-center gap-1.5 w-full text-left transition-colors duration-75 cursor-pointer py-px text-muted-foreground hover:text-foreground select-none"
      @click="toggle"
    >
      <span
        class="min-w-0 truncate font-[400]"
        :class="running ? 'tool-shimmer-text' : ''"
      >{{ headerLabel }}</span>
      <ChevronDown
        v-if="open"
        class="size-3.5 shrink-0 ml-0.5 opacity-50 group-hover/h:opacity-100"
      />
      <ChevronRight
        v-else
        class="size-3.5 shrink-0 ml-0.5 opacity-50 group-hover/h:opacity-100"
      />
    </button>

    <CollapseSection :open="open">
      <div class="mt-1 rounded-md bg-muted px-2.5 py-1.5 space-y-0.5">
        <template
          v-for="(item, i) in items"
          :key="item.id"
        >
          <ToolCallInline
            v-if="item.type === 'tool'"
            :block="(item as ToolCallBlockType)"
            in-group
          />
          <ThinkingBlock
            v-else-if="item.type === 'reasoning'"
            :block="(item as ThinkingBlockType)"
            :streaming="active === true && i === items.length - 1"
          />
        </template>
      </div>
    </CollapseSection>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { ContentBlock, ThinkingBlock as ThinkingBlockType, ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import { getToolDisplay } from './tool-call-registry'
import ToolCallInline from './tool-call-inline.vue'
import ThinkingBlock from './thinking-block.vue'
import CollapseSection from './collapse-section.vue'
import { getCollapseOpen, groupCollapseKey, setCollapseOpen } from './process-collapse'

const props = defineProps<{
  // Ordered run of tool + reasoning blocks belonging to one process segment.
  items: ContentBlock[]
  // True when this segment is the last block of a still-streaming assistant turn.
  active?: boolean
}>()

const { t } = useI18n()

const single = computed(() => props.items[0])
const toolItems = computed(() => props.items.filter((b): b is ToolCallBlockType => b.type === 'tool'))

// Open state is purely user-driven and persisted across the post-turn refetch:
// a process is collapsed until the user opens it, then stays as they left it
// (no auto-open while streaming, no auto-close on completion). The header still
// acts as a live ticker via `running`/`headerLabel`, so the user can follow
// progress without the body being forced open.
const collapseKey = computed(() => groupCollapseKey(props.items))
const open = ref(getCollapseOpen(collapseKey.value))
watch(collapseKey, (key) => {
  open.value = getCollapseOpen(key)
})
function toggle() {
  open.value = !open.value
  setCollapseOpen(collapseKey.value, open.value)
}

const anyToolRunning = computed(() => toolItems.value.some(tool => tool.running))
const running = computed(() => props.active === true || anyToolRunning.value)

function basename(path: string): string {
  if (!path) return ''
  const parts = path.split('/').filter(Boolean)
  return parts[parts.length - 1] ?? path
}

const FILE_PATH_TOOLS = new Set(['read', 'write', 'edit', 'list'])

// Subject of a single tool call: a short, human target (filename / query /
// command) rather than a bare count — "Read chat-pane.vue", not "Read 1".
function subjectOf(tool: ToolCallBlockType): string {
  const display = getToolDisplay(tool)
  if (FILE_PATH_TOOLS.has(tool.toolName)) return basename(display.target) || display.target
  return display.target
}

function verbOf(tool: ToolCallBlockType): string {
  const display = getToolDisplay(tool)
  return t(`chat.tools.${display.actionKey}`, display.actionParams ?? {})
}

function labelFor(tool: ToolCallBlockType): string {
  const subject = subjectOf(tool)
  const verb = verbOf(tool)
  return subject ? `${verb} ${subject}` : verb
}

// Collapsed summary: a single tool keeps its subject; multiple tools fall back
// to category counts ("Read 3 files · Edited 2 files").
const BROWSE_TOOLS = new Set([
  'read', 'list', 'web_search', 'web_fetch', 'search_memory', 'search_messages',
  'get_contacts', 'list_sessions', 'list_email', 'read_email', 'list_email_accounts',
  'list_schedule', 'get_schedule', 'list_skills',
])
const RUN_TOOLS = new Set(['exec'])
const EDIT_TOOLS = new Set(['write', 'edit'])

function bucket(name: string): 'browse' | 'edit' | 'run' | 'other' {
  if (BROWSE_TOOLS.has(name)) return 'browse'
  if (EDIT_TOOLS.has(name)) return 'edit'
  if (RUN_TOOLS.has(name)) return 'run'
  return 'other'
}

const aggregateLabel = computed(() => {
  const tools = toolItems.value
  if (tools.length === 0) return t('chat.process.thought')
  if (tools.length === 1) return labelFor(tools[0]!)
  const acc = { browse: 0, edit: 0, run: 0 }
  for (const tool of tools) {
    const b = bucket(tool.toolName)
    if (b !== 'other') acc[b] += 1
  }
  const segments: string[] = []
  if (acc.browse) segments.push(t('chat.process.browse', { count: acc.browse }))
  if (acc.edit) segments.push(t('chat.process.edit', { count: acc.edit }))
  if (acc.run) segments.push(t('chat.process.run', { count: acc.run }))
  return segments.length ? segments.join(' · ') : t('chat.process.steps', { count: tools.length })
})

// Streaming header acts as a ticker for the current (last) item.
const tickerLabel = computed(() => {
  const current = props.items[props.items.length - 1]
  if (!current) return ''
  if (current.type === 'reasoning') return t('chat.thinkingInProgress')
  if (current.type === 'tool') return labelFor(current as ToolCallBlockType)
  return aggregateLabel.value
})

const headerLabel = computed(() => (running.value ? tickerLabel.value : aggregateLabel.value))
</script>
