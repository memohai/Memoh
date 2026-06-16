<script setup lang="ts">
import { ref, watch, computed, onBeforeUnmount } from 'vue'
import { appKeyboardCommands } from '@/lib/keyboard-commands'
import { useKeyboardCommand } from '@/composables/useKeyboardCommand'
import { isFileSaveEligible } from './file-save-command'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { File, Download, RefreshCw } from 'lucide-vue-next'
import { Button, Spinner } from '@memohai/ui'
import {
  getBotsByBotIdContainerFsRead,
  postBotsByBotIdContainerFsWrite,
  getBotsByBotIdContainerFsDownload,
} from '@memohai/sdk'
import type { HandlersFsFileInfo } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import { sdkApiUrl, sdkAuthQuery } from '@/lib/api-client'
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
// Set when the agent rewrites this file while the user has unsaved edits. We
// don't silently reload (that would lose their work); the template shows an
// inline notice with a Reload action instead.
const externalChangePending = ref(false)

const filename = computed(() => props.file.name ?? '')
const filePath = computed(() => props.file.path ?? '')
const isText = computed(() => isTextFile(filename.value))
const isImage = computed(() => isImageFile(filename.value))
const isDirty = computed(() => content.value !== originalContent.value)

watch(isDirty, (dirty) => {
  emit('update:dirty', dirty)
  // User resolved the conflict by saving or reverting their changes.
  if (!dirty) externalChangePending.value = false
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
async function handleSave(): Promise<boolean> {
  if (props.readonly) return true
  if (!isDirty.value) return true
  if (saving.value) return false
  saving.value = true
  try {
    await postBotsByBotIdContainerFsWrite({
      path: { bot_id: props.botId },
      body: { path: filePath.value, content: content.value },
      throwOnError: true,
    })
    originalContent.value = content.value
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

defineExpose({ save: handleSave })

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
  cleanupImageUrl()
  content.value = ''
  originalContent.value = ''
  externalChangePending.value = false
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
const chatStore = useChatStore()
const { fsChangedAt, currentBotId } = storeToRefs(chatStore)
watch(fsChangedAt, () => {
  if (!props.botId || props.botId !== currentBotId.value) return
  if (!chatStore.affectsPath(filePath.value)) return
  if (saving.value) return
  if (isDirty.value) {
    externalChangePending.value = true
    return
  }
  if (isText.value) {
    void loadTextContent()
  } else if (isImage.value) {
    void loadImageBlob()
  }
})

async function acceptExternalChange() {
  externalChangePending.value = false
  if (isText.value) {
    await loadTextContent()
  } else if (isImage.value) {
    await loadImageBlob()
  }
}

// Save on Cmd/Ctrl+S via the shared keyboard layer. The handler is scoped to this
// component's lifetime: it returns true only for an editable, dirty text file, so
// the browser keeps its native save behavior in every other state.
useKeyboardCommand(appKeyboardCommands.saveActiveFile, () => {
  if (!isFileSaveEligible({
    readonly: props.readonly ?? false,
    isText: isText.value,
    isDirty: isDirty.value,
    saving: saving.value,
  })) {
    return false
  }
  void handleSave()
  return true
})

onBeforeUnmount(() => {
  activeReadController?.abort()
  cleanupImageUrl()
})
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden bg-surface-editor">
    <div
      v-if="externalChangePending"
      class="flex shrink-0 items-center gap-2 border-b border-border bg-accent/40 px-3 py-1.5 text-caption text-foreground"
    >
      <RefreshCw class="size-3.5 shrink-0 text-muted-foreground" />
      <span class="min-w-0 flex-1 truncate">{{ t('bots.files.externalChangeNotice') }}</span>
      <Button
        variant="ghost"
        size="sm"
        class="h-6 shrink-0 px-2 text-xs"
        @click="acceptExternalChange"
      >
        {{ t('bots.files.externalChangeReload') }}
      </Button>
    </div>
    <div class="flex-1 min-h-0 overflow-hidden">
      <div
        v-if="loading"
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
