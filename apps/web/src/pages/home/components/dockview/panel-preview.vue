<template>
  <div class="flex flex-col h-full w-full bg-surface-editor">
    <PanelBreadcrumb :path="filePath" />
    <div class="flex-1 min-h-0">
      <div
        v-if="loading"
        class="flex h-full items-center justify-center text-muted-foreground"
      >
        <Spinner class="mr-2" />
        {{ t('common.loading') }}
      </div>

      <MarkdownPreview
        v-else-if="isMd"
        :content="content"
        class="h-full"
      />
      <HtmlPreview
        v-else-if="isHtml"
        :content="content"
        class="h-full"
      />

      <div
        v-else
        class="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground"
      >
        <FileText class="size-10 opacity-30" />
        <p class="text-xs">
          {{ t('bots.files.previewNotAvailable') }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { FileText } from 'lucide-vue-next'
import { Spinner, toast } from '@memohai/ui'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { getBotsByBotIdContainerFsRead } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { isMarkdownFile, isHtmlFile } from '@/components/file-manager/utils'
import { useChatStore } from '@/store/chat-list'
import { usePanelVisible } from './use-panel-visible'
import PanelBreadcrumb from './panel-breadcrumb.vue'

const MarkdownPreview = defineAsyncComponent(() => import('@/components/markdown-preview/index.vue'))
const HtmlPreview = defineAsyncComponent(() => import('@/components/html-preview/index.vue'))

const props = defineProps<{
  params: {
    params: { filePath?: string }
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const { t } = useI18n()
const chatStore = useChatStore()
const { currentBotId, fsChangedAt } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)
const filePath = computed(() => props.params.params.filePath ?? '')
const filename = computed(() => {
  const path = filePath.value
  const idx = path.lastIndexOf('/')
  return idx >= 0 ? path.slice(idx + 1) : path
})
const isMd = computed(() => isMarkdownFile(filename.value))
const isHtml = computed(() => isHtmlFile(filename.value))
const canPreview = computed(() => isMd.value || isHtml.value)

const content = ref('')
const loading = ref(false)

async function load() {
  const botId = currentBotId.value
  if (!botId || !filePath.value || !canPreview.value) return
  loading.value = true
  try {
    const { data } = await getBotsByBotIdContainerFsRead({
      path: { bot_id: botId },
      query: { path: filePath.value },
      throwOnError: true,
    })
    content.value = data.content ?? ''
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.readFailed')))
  } finally {
    loading.value = false
  }
}

// Load when the panel first becomes visible / its target changes, and refresh
// when the agent mutates the workspace.
watch([visible, filePath], ([isVisible]) => {
  if (isVisible) void load()
}, { immediate: true })

watch(fsChangedAt, () => {
  if (visible.value) void load()
})
</script>
