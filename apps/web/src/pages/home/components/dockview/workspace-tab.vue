<template>
  <div
    class="group/tab flex h-full min-w-0 items-center gap-1 pl-2.5 pr-1"
    @auxclick.middle.prevent="close"
  >
    <span class="min-w-0 truncate text-xs leading-none">{{ title }}</span>
    <Button
      variant="ghost"
      class="size-5 p-0 shrink-0 rounded text-muted-foreground hover:text-foreground opacity-0 group-hover/tab:opacity-100 focus-visible:opacity-100"
      :class="isActive && 'opacity-100'"
      :aria-label="t('chat.tabMenu.close')"
      @pointerdown.stop
      @mousedown.stop
      @click.stop.prevent="close"
    >
      <X class="size-3" />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'

// Custom default tab: replaces dockview's built-in tab (square icon-hover
// block) with design-system type and a ghost close button that shows on
// hover/active, VS Code style.
const props = defineProps<{
  params: {
    api: DockviewPanelApi
    containerApi: DockviewApi
    params: Record<string, unknown>
  }
}>()

const { t } = useI18n()

const title = ref(props.params.api.title ?? '')
const isActive = ref(props.params.api.isActive)

const disposables = [
  props.params.api.onDidTitleChange((event) => {
    title.value = event.title
  }),
  props.params.api.onDidActiveChange((event) => {
    isActive.value = event.isActive
  }),
]

// The tab part is initialized before the panel's title is applied (dockview
// sets it right after init), so re-read once the addPanel call stack settled.
onMounted(() => {
  title.value = props.params.api.title ?? title.value
  isActive.value = props.params.api.isActive
})

function close() {
  props.params.api.close()
}

onBeforeUnmount(() => {
  for (const d of disposables) d.dispose()
})
</script>
