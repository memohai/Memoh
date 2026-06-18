<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { appKeyboardCommands } from '@/lib/keyboard-commands'
import { useKeyboardCommand } from '@/composables/useKeyboardCommand'
import { isFileSaveEligible } from './file-save-command'
import { detectSaveConflict, deriveChipContext } from './file-conflict'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { File, Download, RefreshCw, GitCompare, X } from 'lucide-vue-next'
import { Button, Spinner } from '@memohai/ui'
import {
  getBotsByBotIdContainerFsRead,
  postBotsByBotIdContainerFsWrite,
  getBotsByBotIdContainerFsDownload,
} from '@memohai/sdk'
import type { HandlersFsFileInfo } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import MonacoDiff from '@/components/monaco-editor/diff.vue'
import { sdkApiUrl, sdkAuthQuery } from '@/lib/api-client'
import { formatRelativeTime } from '@/utils/date-time'
import { isTextFile, isImageFile } from './utils'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'

const props = defineProps<{
  botId: string
  file: HandlersFsFileInfo
  readonly?: boolean
}>()

const emit = defineEmits<{
  saved: []
  'update:dirty': [dirty: boolean]
}>()

const { t } = useI18n()

const content = ref('')
const originalContent = ref('')
const loading = ref(false)
const saving = ref(false)
const imageUrl = ref('')
// True once the file has been read at least once for the current path. Used
// to suppress the full-area spinner on subsequent reloads — the editor / image
// stays mounted and the content swaps in place, the way VS Code handles
// external file changes.
const loaded = ref(false)
// Timestamp (ms) of the last successful read. Feeds the save baseline guard —
// any fs bump newer than this for our path means the user would overwrite a
// fresher disk version. Resets on path change.
const lastLoadedAt = ref(0)
// Conflict state machine. 'chip' = inline banner above the editor with the
// usual choice (Compare / Reload / Save anyway). 'compare' = diff editor
// replaces the main pane. 'none' = clean.
const conflictState = ref<'none' | 'chip' | 'compare'>('none')
// Right side of the diff editor. Snapshot at the moment Compare opens — if
// the agent writes again while Compare is up, the snapshot stays put (user
// closes Compare and re-opens to refresh).
const compareDiskContent = ref('')
let compareController: AbortController | null = null
// Drives "5s ago" / "2m ago" relative-time labels in the chip. Ticks once a
// minute; agent writes are usually fresh enough that "just now" wins, but a
// chip left up across coffee should age gracefully.
const nowTick = ref(Date.now())
let nowTickInterval: ReturnType<typeof setInterval> | null = null

const filename = computed(() => props.file.name ?? '')
const filePath = computed(() => props.file.path ?? '')
const isText = computed(() => isTextFile(filename.value))
const isImage = computed(() => isImageFile(filename.value))
const isDirty = computed(() => content.value !== originalContent.value)

const chatStore = useChatStore()
const { fsChangedAt, currentBotId, bots } = storeToRefs(chatStore)
const botName = computed(() => {
  const bot = bots.value.find(b => b.id === props.botId)
  return bot?.display_name || bot?.name || null
})
const fsEvent = computed(() => chatStore.fsEventForPath(filePath.value))
const chipContext = computed(() => deriveChipContext(fsEvent.value, botName.value))
const fallbackAgent = computed(() => t('bots.files.externalChange.fallbackAgent'))
const chipMessage = computed(() => {
  const ctx = chipContext.value
  const agent = ctx.agentName ?? fallbackAgent.value
  // formatRelativeTime depends on Date.now(); reading the tick here makes the
  // label re-evaluate as it ages.
  void nowTick.value
  const time = ctx.occurredAt ? formatRelativeTime(new Date(ctx.occurredAt)) : ''
  if (ctx.kind === 'write' && ctx.newLineCount != null) {
    return t('bots.files.externalChange.wrote', { agent, lines: ctx.newLineCount, time })
  }
  if (ctx.kind === 'edit' && (ctx.addedLines != null || ctx.removedLines != null)) {
    return t('bots.files.externalChange.edited', {
      agent,
      added: ctx.addedLines ?? 0,
      removed: ctx.removedLines ?? 0,
      time,
    })
  }
  if (ctx.kind === 'exec') return t('bots.files.externalChange.ran', { agent, time })
  if (ctx.kind === 'apply_patch') return t('bots.files.externalChange.patched', { agent, time })
  return t('bots.files.externalChange.generic', { agent, time })
})

watch(isDirty, (dirty) => {
  emit('update:dirty', dirty)
}, { immediate: true })

// One in-flight read at a time per viewer; a new load aborts the old one so a
// slower stale response can't overwrite newer content when fsChangedAt fires
// in bursts.
let activeReadController: AbortController | null = null

async function loadTextContent() {
  activeReadController?.abort()
  const controller = new AbortController()
  activeReadController = controller
  loading.value = true
  try {
    const { data } = await getBotsByBotIdContainerFsRead({
      path: { bot_id: props.botId },
      query: { path: filePath.value },
      signal: controller.signal,
      throwOnError: true,
    })
    if (controller.signal.aborted) return
    content.value = data.content ?? ''
    originalContent.value = content.value
    lastLoadedAt.value = Date.now()
    loaded.value = true
    conflictState.value = 'none'
  } catch (error) {
    if (controller.signal.aborted) return
    toast.error(resolveApiErrorMessage(error, t('bots.files.readFailed')))
  } finally {
    if (activeReadController === controller) {
      activeReadController = null
      loading.value = false
    }
  }
}

async function loadImageBlob() {
  activeReadController?.abort()
  const controller = new AbortController()
  activeReadController = controller
  loading.value = true
  let url = ''
  try {
    const response = await getBotsByBotIdContainerFsDownload({
      path: { bot_id: props.botId },
      query: { path: filePath.value },
      parseAs: 'blob',
      signal: controller.signal,
      throwOnError: true,
    })
    if (controller.signal.aborted) return
    const blob = response.data as unknown as Blob
    url = URL.createObjectURL(blob)
    cleanupImageUrl()
    imageUrl.value = url
    url = ''
    lastLoadedAt.value = Date.now()
    loaded.value = true
    conflictState.value = 'none'
  } catch (error) {
    if (controller.signal.aborted) return
    toast.error(resolveApiErrorMessage(error, t('bots.files.readFailed')))
  } finally {
    if (url) URL.revokeObjectURL(url)
    if (activeReadController === controller) {
      activeReadController = null
      loading.value = false
    }
  }
}

// Returns whether the file is in a saved state afterwards, so an external caller
// (the tab close-confirm flow) can decide whether to proceed with closing. A
// read-only or already-clean file reports success without a network round-trip.
async function handleSave(force = false): Promise<boolean> {
  if (props.readonly) return true
  if (!isDirty.value) return true
  if (saving.value) return false
  // VS Code-style two-gate save: even if the chip wasn't surfaced yet (e.g.
  // user pressed Cmd+S in the brief window between bump and consumer reaction),
  // re-check the baseline before we POST. force=true skips this — that's the
  // "Save anyway" path the user explicitly chose after seeing the conflict.
  if (!force) {
    const hasConflict = detectSaveConflict({
      lastFsChangeAt: fsChangedAt.value,
      lastLoadedAt: lastLoadedAt.value,
      affects: chatStore.affectsPath(filePath.value),
    })
    if (hasConflict) {
      // Don't drop the user back to compare if they were already comparing.
      if (conflictState.value !== 'compare') conflictState.value = 'chip'
      return false
    }
  }
  saving.value = true
  try {
    await postBotsByBotIdContainerFsWrite({
      path: { bot_id: props.botId },
      body: { path: filePath.value, content: content.value },
      throwOnError: true,
    })
    originalContent.value = content.value
    // The save's POST is the new baseline; subsequent fs bumps from agent
    // writes after this moment should re-trigger the save guard.
    lastLoadedAt.value = Date.now()
    conflictState.value = 'none'
    toast.success(t('bots.files.saveSuccess'))
    emit('saved')
    return true
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.saveFailed')))
    return false
  } finally {
    saving.value = false
  }
}

function forceSave() {
  void handleSave(true)
}

defineExpose({ save: () => handleSave(false) })

function handleDownload() {
  const url = sdkApiUrl({
    url: '/bots/{bot_id}/container/fs/download',
    path: { bot_id: props.botId },
    query: { path: filePath.value, ...sdkAuthQuery() },
  })
  const a = document.createElement('a')
  a.href = url
  a.download = filename.value
  a.click()
}

function cleanupImageUrl() {
  if (imageUrl.value) {
    URL.revokeObjectURL(imageUrl.value)
    imageUrl.value = ''
  }
}

watch(() => props.file.path, () => {
  compareController?.abort()
  compareController = null
  cleanupImageUrl()
  content.value = ''
  originalContent.value = ''
  conflictState.value = 'none'
  compareDiskContent.value = ''
  lastLoadedAt.value = 0
  loaded.value = false
  if (isText.value) {
    void loadTextContent()
  } else if (isImage.value) {
    void loadImageBlob()
  }
}, { immediate: true })

// Reload the file when the chat agent runs a fs-mutating tool (write/edit/apply_patch/exec)
// against the same bot AND the change targets this path. Skip if the user is
// mid-save — the in-flight save owns content/originalContent and we'd race it.
// If the user has unsaved edits, surface an inline notice rather than silently
// reloading or silently dropping the agent's change.
watch(fsChangedAt, () => {
  if (!props.botId || props.botId !== currentBotId.value) return
  if (!chatStore.affectsPath(filePath.value)) return
  if (saving.value) return
  if (isDirty.value) {
    // Don't disrupt an active compare view; user is in the middle of reviewing.
    if (conflictState.value === 'none') conflictState.value = 'chip'
    return
  }
  if (isText.value) void loadTextContent()
  else if (isImage.value) void loadImageBlob()
})

async function acceptExternalChange() {
  if (isText.value) await loadTextContent()
  else if (isImage.value) await loadImageBlob()
}

async function openCompare() {
  if (!isText.value) return
  // Prefer the agent's tool-message content — same bytes the server saw, no
  // extra round trip and immune to "what if disk changed again" races. Fall
  // back to a fresh read for edit / apply_patch / exec where we don't have the
  // whole file.
  const event = fsEvent.value
  if (event?.writeContent != null) {
    compareDiskContent.value = event.writeContent
    conflictState.value = 'compare'
    return
  }
  compareController?.abort()
  const controller = new AbortController()
  compareController = controller
  try {
    const { data } = await getBotsByBotIdContainerFsRead({
      path: { bot_id: props.botId },
      query: { path: filePath.value },
      signal: controller.signal,
      throwOnError: true,
    })
    if (controller.signal.aborted) return
    compareDiskContent.value = data.content ?? ''
    conflictState.value = 'compare'
  } catch (error) {
    if (controller.signal.aborted) return
    toast.error(resolveApiErrorMessage(error, t('bots.files.compareLoadFailed')))
  } finally {
    if (compareController === controller) compareController = null
  }
}

function exitCompare() {
  compareController?.abort()
  compareController = null
  conflictState.value = 'chip'
}

// Save on Cmd/Ctrl+S via the shared keyboard layer. Even if a conflict is
// detected handleSave returns false — but we still report `true` to the
// keyboard layer because we've already shown the conflict UI; falling through
// to browser save would be confusing.
useKeyboardCommand(appKeyboardCommands.saveActiveFile, () => {
  if (!isFileSaveEligible({
    readonly: props.readonly ?? false,
    isText: isText.value,
    isDirty: isDirty.value,
    saving: saving.value,
  })) {
    return false
  }
  void handleSave(false)
  return true
})

onMounted(() => {
  nowTickInterval = setInterval(() => { nowTick.value = Date.now() }, 60_000)
})

onBeforeUnmount(() => {
  if (nowTickInterval) clearInterval(nowTickInterval)
  nowTickInterval = null
  activeReadController?.abort()
  compareController?.abort()
  cleanupImageUrl()
})
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden bg-surface-editor">
    <!-- Chip: inline banner above the editor when we have a pending external change -->
    <div
      v-if="conflictState === 'chip'"
      class="flex shrink-0 items-center gap-2 border-b border-border bg-accent/40 px-3 py-1.5 text-caption text-foreground"
    >
      <RefreshCw class="size-3.5 shrink-0 text-muted-foreground" />
      <span class="min-w-0 flex-1 truncate">{{ chipMessage }}</span>
      <Button
        v-if="isText"
        variant="ghost"
        size="sm"
        class="h-6 shrink-0 px-2 text-xs"
        @click="openCompare"
      >
        <GitCompare class="mr-1 size-3" />
        {{ t('bots.files.externalChange.compare') }}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        class="h-6 shrink-0 px-2 text-xs"
        @click="acceptExternalChange"
      >
        {{ t('bots.files.externalChange.reload') }}
      </Button>
      <Button
        v-if="isDirty"
        variant="ghost"
        size="sm"
        class="h-6 shrink-0 px-2 text-xs"
        @click="forceSave"
      >
        {{ t('bots.files.externalChange.saveAnyway') }}
      </Button>
    </div>

    <div class="flex-1 min-h-0 overflow-hidden">
      <!-- Compare view: diff editor replacing the main pane -->
      <div
        v-if="conflictState === 'compare'"
        class="flex h-full flex-col"
      >
        <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border px-3 py-1.5 text-caption text-muted-foreground">
          <span class="min-w-0 truncate">
            {{ t('bots.files.compare.yours') }} ↔ {{ t('bots.files.compare.disk') }}
          </span>
          <div class="flex items-center gap-1.5">
            <Button
              variant="ghost"
              size="sm"
              class="h-6 px-2 text-xs"
              @click="acceptExternalChange"
            >
              {{ t('bots.files.externalChange.reload') }}
            </Button>
            <Button
              v-if="isDirty"
              variant="ghost"
              size="sm"
              class="h-6 px-2 text-xs"
              @click="forceSave"
            >
              {{ t('bots.files.externalChange.saveAnyway') }}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 px-2 text-xs text-muted-foreground"
              @click="exitCompare"
            >
              <X class="mr-1 size-3" />
              {{ t('bots.files.compare.close') }}
            </Button>
          </div>
        </div>
        <MonacoDiff
          :original="compareDiskContent"
          :modified="content"
          :filename="filename"
          :original-title="t('bots.files.compare.disk')"
          :modified-title="t('bots.files.compare.yours')"
          class="min-h-0 flex-1"
        />
      </div>

      <!-- Full-area spinner only on the FIRST load for this path (nothing else
           to show). Subsequent reloads keep the editor/image mounted and let
           the content swap in place, matching VS Code's external-change UX. -->
      <div
        v-else-if="loading && !loaded"
        class="flex h-full items-center justify-center text-muted-foreground"
      >
        <Spinner class="mr-2" />
        {{ t('common.loading') }}
      </div>

      <MonacoEditor
        v-else-if="isText"
        v-model="content"
        :filename="filename"
        :readonly="readonly"
        class="h-full"
      />

      <div
        v-else-if="isImage && imageUrl"
        class="flex h-full items-center justify-center overflow-auto p-4 bg-muted/30"
      >
        <img
          :src="imageUrl"
          :alt="filename"
          class="max-h-full max-w-full object-contain rounded"
        >
      </div>

      <div
        v-else
        class="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground"
      >
        <File class="size-12 opacity-30" />
        <p class="text-xs">
          {{ t('bots.files.previewNotAvailable') }}
        </p>
        <Button
          variant="outline"
          size="sm"
          @click="handleDownload"
        >
          <Download class="mr-1.5 size-3" />
          {{ t('bots.files.download') }}
        </Button>
      </div>
    </div>
  </div>
</template>
