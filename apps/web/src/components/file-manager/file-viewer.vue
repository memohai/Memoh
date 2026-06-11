<script setup lang="ts">
import { ref, watch, computed, onBeforeUnmount, defineAsyncComponent, h } from 'vue'
import { appKeyboardCommands } from '@/lib/keyboard-commands'
import { useKeyboardCommand } from '@/composables/useKeyboardCommand'
import { isFileSaveEligible } from './file-save-command'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { File, Download, Save } from 'lucide-vue-next'
import { Button, Spinner } from '@memohai/ui'
import {
  getBotsByBotIdContainerFsRead,
  postBotsByBotIdContainerFsWrite,
  getBotsByBotIdContainerFsDownload,
} from '@memohai/sdk'
import type { HandlersFsFileInfo } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import PreviewModeToggle, { type PreviewMode } from '@/components/preview-mode-toggle/index.vue'
import { sdkApiUrl, sdkAuthQuery } from '@/lib/api-client'
import { isTextFile, isImageFile, isMarkdownFile, isHtmlFile } from './utils'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'

const AsyncLoading = {
  render: () =>
    h(
      'div',
      { class: 'flex h-full items-center justify-center text-muted-foreground' },
      [h(Spinner, { class: 'mr-2' })],
    ),
}

const MarkdownPreview = defineAsyncComponent({
  loader: () => import('@/components/markdown-preview/index.vue'),
  loadingComponent: AsyncLoading,
  delay: 120,
})

const HtmlPreview = defineAsyncComponent({
  loader: () => import('@/components/html-preview/index.vue'),
  loadingComponent: AsyncLoading,
  delay: 120,
})

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

const filename = computed(() => props.file.name ?? '')
const filePath = computed(() => props.file.path ?? '')
const isText = computed(() => isTextFile(filename.value))
const isImage = computed(() => isImageFile(filename.value))
const isMd = computed(() => isMarkdownFile(filename.value))
const isHtml = computed(() => isHtmlFile(filename.value))
const hasPreviewToggle = computed(() => isText.value && (isMd.value || isHtml.value))
const isDirty = computed(() => content.value !== originalContent.value)

const mode = ref<PreviewMode>('raw')

watch(isDirty, (dirty) => {
  emit('update:dirty', dirty)
}, { immediate: true })

async function loadTextContent() {
  loading.value = true
  try {
    const { data } = await getBotsByBotIdContainerFsRead({
      path: { bot_id: props.botId },
      query: { path: filePath.value },
      throwOnError: true,
    })
    content.value = data.content ?? ''
    originalContent.value = content.value
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.readFailed')))
  } finally {
    loading.value = false
  }
}

async function loadImageBlob() {
  loading.value = true
  try {
    const response = await getBotsByBotIdContainerFsDownload({
      path: { bot_id: props.botId },
      query: { path: filePath.value },
      parseAs: 'blob',
      throwOnError: true,
    })
    const blob = response.data as unknown as Blob
    imageUrl.value = URL.createObjectURL(blob)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.readFailed')))
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  if (props.readonly) return
  if (!isDirty.value || saving.value) return
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
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.saveFailed')))
  } finally {
    saving.value = false
  }
}

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
  // Default mode: rich preview for markdown, raw source for HTML/other text.
  if (isMd.value) mode.value = 'preview'
  else mode.value = 'raw'
  if (isText.value) {
    void loadTextContent()
  } else if (isImage.value) {
    void loadImageBlob()
  }
}, { immediate: true })

// Reload the file when the chat agent runs a fs-mutating tool (write/edit/exec)
// against the same bot. Skip if the user has unsaved changes — we don't want to
// silently overwrite their edits.
const chatStore = useChatStore()
const { fsChangedAt, currentBotId } = storeToRefs(chatStore)
watch(fsChangedAt, () => {
  if (!props.botId || props.botId !== currentBotId.value) return
  if (isDirty.value) return
  if (isText.value) {
    void loadTextContent()
  } else if (isImage.value) {
    cleanupImageUrl()
    void loadImageBlob()
  }
})

// Save on Cmd/Ctrl+S via the shared keyboard layer. The handler is scoped to this
// component's lifetime: it returns true only for an editable, dirty text file, so
// the browser keeps its native save behavior in every other state. The
// cross-platform mod key is handled by the binding table.
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
  cleanupImageUrl()
})
</script>

<template>
  <div class="relative flex h-full flex-col overflow-hidden">
    <div
      v-if="hasPreviewToggle || isText"
      class="absolute top-2 right-2 z-10 flex items-center gap-2"
    >
      <PreviewModeToggle
        v-if="hasPreviewToggle"
        v-model="mode"
      />
      <Button
        v-if="!readonly && isText && (!hasPreviewToggle || mode === 'raw')"
        type="button"
        size="sm"
        class="gap-1.5 bg-primary text-primary-foreground shadow-md hover:bg-brand-hover disabled:bg-primary/40 disabled:text-primary-foreground/80"
        :disabled="!isDirty || saving"
        :title="t('bots.files.save')"
        @click="handleSave"
      >
        <Spinner v-if="saving" />
        <Save
          v-else
          class="size-3.5"
        />
        {{ t('bots.files.save') }}
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

      <MarkdownPreview
        v-else-if="isText && isMd && mode === 'preview'"
        :content="content"
        class="h-full"
      />

      <HtmlPreview
        v-else-if="isText && isHtml && mode === 'preview'"
        :content="content"
        class="h-full"
      />

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
