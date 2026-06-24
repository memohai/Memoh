<template>
  <div
    class="group/tab relative z-[1] flex h-full min-w-0 items-center pl-4 pr-4"
    @auxclick.middle.prevent="close"
  >
    <!-- Active state is signalled by text color (and the CSS top-accent on the
         tab chip), NOT by weight or size — so selecting a tab never changes the
         label's metrics or baseline. The chip hugs its title with symmetric
         padding: no slot is reserved for the close button on ANY tab (selected or
         not); it overlays the text on hover instead. -->
    <!-- leading-[1.4], not leading-none: with truncate (overflow:hidden) a line box
         equal to the font size clips descenders (g/y) and CJK bottoms; a slightly
         taller line box gives them room without affecting the centered baseline. -->
    <!-- Sized by its own content (no flex-grow): the tab is natural width (see
         dockview-theme.css), so a flex-basis:0 grow span made the tab's intrinsic
         width ambiguous and it jumped on title changes. min-w-0 still lets it shrink
         + truncate once the tab hits its max-width cap. Centred in the chip (not
         lifted to the bar centre), so it rides a touch below the top headroom. -->
    <span
      class="min-w-0 truncate text-[12.5px] leading-[1.4] transition-colors"
      :class="isActive ? 'text-foreground' : 'text-muted-foreground'"
    >{{ title }}</span>
    <!-- Unsaved-changes dot: sits in the close slot AT REST so the affordance never
         shifts; hovering fades it out as the close button fades in (VS Code's tab).
         Painted over the same fade as the button so a long title dissolves behind
         it instead of colliding with the glyph. -->
    <div
      v-if="isDirty"
      class="close-fade pointer-events-none absolute right-0 flex items-center pl-6 pr-2 opacity-100 group-hover/tab:opacity-0"
    >
      <span class="flex size-5 items-center justify-center">
        <span
          class="size-[7px] rounded-full"
          :class="isActive ? 'bg-foreground' : 'bg-muted-foreground'"
        />
      </span>
    </div>
    <!-- Close affordance: hover-only, absolutely positioned so it never reserves a
         slot or resizes the chip (geometry is identical hovered or not). It paints
         the chip's own OPAQUE hover colour (--tab-hover-bg) as a left→right fade, so
         the title dissolves into the chip and nothing stays legible under the
         button. The fade layer is click-through; only the button takes pointer
         events. Keyboard focus reveals it for a11y; middle-click closes without it. -->
    <div
      class="close-fade pointer-events-none absolute right-0 flex items-center pl-6 pr-2 opacity-0 group-hover/tab:opacity-100 focus-within:opacity-100"
    >
      <!-- No own hover fill: the close affordance is read through the left→right
           fade (which already paints the chip's hover surface) plus the icon
           darkening on hover. A second darker square behind the glyph just
           double-stacks chrome, so the ghost hover background is suppressed. -->
      <Button
        variant="ghost"
        class="pointer-events-auto size-5 shrink-0 rounded-full p-0 text-muted-foreground [--btn-ghost-hover:transparent] hover:text-foreground"
        :aria-label="t('chat.tabMenu.close')"
        @pointerdown.stop
        @mousedown.stop
        @click.stop.prevent="close"
      >
        <X class="size-3.5" />
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

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
const workspaceTabs = useWorkspaceTabsStore()

const panelId = props.params.api.id
const title = ref(props.params.api.title ?? '')
const isActive = ref(props.params.api.isActive)
// Unsaved-changes flag for file panels — read from the store's reactive map, so
// the dot, the sidebar badge and the close dialog never drift apart.
const isDirty = computed(() => !!workspaceTabs.fileDirty[panelId])
// Ephemeral (VS Code "preview") tabs still get replaced in place when another
// preview-eligible tab opens into the same group (see workspace-tabs store), but
// the state is no longer surfaced visually — there is no italic or other marker.

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

// Route through the store guard: a dirty file opens the save-confirm dialog
// instead of closing straight away; clean tabs close immediately.
function close() {
  workspaceTabs.requestCloseTab(panelId)
}

onBeforeUnmount(() => {
  for (const d of disposables) d.dispose()
})
</script>

<style scoped>
/* The close affordance blots out the title with the chip's own opaque hover colour:
 * transparent on the left so the text dissolves into the chip, fully opaque by the
 * button so NOTHING is legible underneath. --tab-hover-bg is inherited from .dv-tab
 * (the editor surface for the active tab, the hover tint otherwise), so the fade is
 * seamless with whatever the chip is wearing. Absolutely positioned, so painting it
 * never reserves a slot or resizes the chip. */
/* Two stacked fades so the blot is ALWAYS opaque. --tab-hover-bg is the surface a
 * tab wears (opaque --surface-editor when active; the TRANSLUCENT hover overlay
 * otherwise), so painting it alone left the title legible under the close button on
 * a hovered tab. Layering the opaque --surface-chrome base UNDER that overlay
 * composites to exactly what the tab shows — chrome+overlay on hover, plain editor
 * when active (the opaque top layer hides the base) — but never see-through. */
.close-fade {
  top: var(--tab-hover-fill-top, 0);
  bottom: 0;
  /* Instant hide when hover ends — avoids white/grey close-fade lingering on a tab
   * you just switched away from. Fade-in only while the tab group is hovered. */
  transition: none;
  background:
    linear-gradient(to right, transparent, var(--tab-hover-bg, var(--surface-editor)) 1rem),
    linear-gradient(to right, transparent, var(--surface-chrome) 1rem);
  /* Full-height. Only the TOP-right corner is rounded, to the crown radius. */
  border-top-right-radius: var(--radius-sm);
}

.group\/tab:hover .close-fade,
.group\/tab:focus-within .close-fade {
  transition: opacity var(--tab-hover-duration, 150ms) ease-out;
}
</style>
