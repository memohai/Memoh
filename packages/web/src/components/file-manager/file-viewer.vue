<script setup lang="ts">
import { ref, watch, computed, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { Button, Spinner } from '@memoh/ui'
import {
  getBotsByBotIdContainerFsRead,
  postBotsByBotIdContainerFsWrite,
  getBotsByBotIdContainerFsDownload,
} from '@memoh/sdk'
import type { HandlersFsFileInfo } from '@memoh/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import { isTextFile, isImageFile, formatFileSize } from './utils'

const props = defineProps<{
  botId: string
  file: HandlersFsFileInfo
}>()

const emit = defineEmits<{
  close: []
  saved: []
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
const isDirty = computed(() => content.value !== originalContent.value)

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
  const url = `/api/bots/${props.botId}/container/fs/download?path=${encodeURIComponent(filePath.value)}`
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
  if (isText.value) {
    void loadTextContent()
  } else if (isImage.value) {
    void loadImageBlob()
  }
}, { immediate: true })

onBeforeUnmount(() => {
  cleanupImageUrl()
})
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-border px-4 py-2">
      <div class="flex items-center gap-2 min-w-0">
        <FontAwesomeIcon
          :icon="['fas', 'file']"
          class="size-3.5 shrink-0 text-muted-foreground"
        />
        <span class="truncate text-sm font-medium">{{ filename }}</span>
        <span class="shrink-0 text-xs text-muted-foreground">{{ formatFileSize(file.size) }}</span>
        <span
          v-if="isDirty"
          class="shrink-0 text-xs text-orange-500"
        >{{ t('bots.files.unsaved') }}</span>
      </div>
      <div class="flex items-center gap-1.5">
        <Button
          v-if="isText && isDirty"
          size="sm"
          :disabled="saving"
          @click="handleSave"
        >
          <Spinner
            v-if="saving"
            class="mr-1"
          />
          {{ t('bots.files.save') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          @click="handleDownload"
        >
          <FontAwesomeIcon
            :icon="['fas', 'download']"
            class="mr-1 size-3"
          />
          {{ t('bots.files.download') }}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          class="size-7 p-0"
          @click="emit('close')"
        >
          <FontAwesomeIcon
            :icon="['fas', 'xmark']"
            class="size-4"
          />
        </Button>
      </div>
    </div>

    <!-- Content -->
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
        <FontAwesomeIcon
          :icon="['fas', 'file']"
          class="size-12 opacity-30"
        />
        <p class="text-sm">
          {{ t('bots.files.previewNotAvailable') }}
        </p>
        <Button
          variant="outline"
          size="sm"
          @click="handleDownload"
        >
          <FontAwesomeIcon
            :icon="['fas', 'download']"
            class="mr-1.5 size-3"
          />
          {{ t('bots.files.download') }}
        </Button>
      </div>
    </div>
  </div>
</template>
