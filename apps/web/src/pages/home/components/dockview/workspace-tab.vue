<template>
  <div
    class="group/tab relative flex h-full min-w-0 items-center pl-3 pr-7"
    @auxclick.middle.prevent="close"
  >
    <!-- Active state is signalled by text color (and the CSS top-accent on the
         tab chip), NOT by weight or size — so selecting a tab never changes the
         label's metrics or baseline. -->
    <span
      class="min-w-0 flex-1 truncate text-xs leading-none transition-colors"
      :class="isActive ? 'text-foreground' : 'text-muted-foreground'"
    >{{ title }}</span>
    <!-- Close: a fixed slot is always reserved on the right (pr-7 above), so the
         title simply truncates before it and the button never overlaps text or
         reflows the chip. Shown on hover (any tab) and always on the active tab,
         dimmed otherwise — VS Code's behaviour. -->
    <Button
      variant="ghost"
      class="absolute right-1 top-1/2 size-5 -translate-y-1/2 shrink-0 rounded-sm p-0 text-muted-foreground opacity-0 transition-opacity duration-100 hover:bg-[color:var(--ui-hover)] hover:text-foreground group-hover/tab:opacity-100 focus-visible:opacity-100"
      :class="{ 'opacity-100': isActive }"
      :aria-label="t('chat.tabMenu.close')"
      @pointerdown.stop
      @mousedown.stop
      @click.stop.prevent="close"
    >
      <X class="size-3.5" />
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
// block) with a design-system label + a ghost close button on a fixed slot.
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
