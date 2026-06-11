<template>
  <div
    class="group/tab relative flex h-full min-w-0 items-center pl-2.5 pr-2"
    :style="{ '--tab-fade': isActive ? 'var(--background)' : 'var(--ui-hover)' }"
    @auxclick.middle.prevent="close"
  >
    <span
      class="min-w-0 flex-1 truncate text-xs leading-none transition-colors"
      :class="isActive
        ? 'font-semibold text-foreground dark:text-[color:oklch(0.97_0_0)]'
        : 'font-normal text-muted-foreground'"
    >{{ title }}</span>
    <!-- Close is a floating overlay pinned to the right, revealed only on hover,
         so it never reserves layout: the title always uses the full tab width
         and simply truncates. A short gradient fades the title out beneath the
         button, keyed to the tab's own background (--background when active,
         ui-hover when an inactive tab is hovered) so a long title never collides
         with the X. pointer-events stay off the fade so clicking it still selects
         the tab; only the button itself is interactive. -->
    <span
      class="pointer-events-none absolute inset-y-0 right-0 flex items-center pl-8 pr-1 opacity-0 transition-opacity duration-150 [background:linear-gradient(to_right,transparent,var(--tab-fade)_1.5rem)] group-hover/tab:opacity-100 focus-within:opacity-100"
    >
      <Button
        variant="ghost"
        class="pointer-events-auto size-6 shrink-0 rounded-sm p-0 text-muted-foreground hover:text-foreground"
        :aria-label="t('chat.tabMenu.close')"
        @pointerdown.stop
        @mousedown.stop
        @click.stop.prevent="close"
      >
        <X class="size-3.5" />
      </Button>
    </span>
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
// hover/active — ghost close button on the tab chip.
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
