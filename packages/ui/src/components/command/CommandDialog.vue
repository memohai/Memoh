<script setup lang="ts">
import type { DialogRootEmits, DialogRootProps } from 'reka-ui'
import { useForwardPropsEmits } from 'reka-ui'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '#/components/dialog'
import Command from './Command.vue'

const props = withDefaults(defineProps<DialogRootProps & {
  title?: string
  description?: string
}>(), {
  title: 'Command Palette',
  description: 'Search for a command to run...',
})
const emits = defineEmits<DialogRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
</script>

<template>
  <Dialog
    v-slot="slotProps"
    v-bind="forwarded"
  >
    <!--
      The palette IS a menu surface, so it must read EXACTLY like the inline
      <Command> / DropdownMenu — not a generic card dialog. So we strip the shared
      DialogContent chrome to nothing (transparent, borderless, shadowless, no
      padding/radius/gap) and let the <Command> surface — which now owns the menu
      chrome itself — show through. That removes the prior mismatch (rounded-xl vs
      menu-shell, card vs popover, shadow-2xl vs dropdown). The backdrop is lightened
      (overlay-class) because the heavy default scrim swallowed the panel's hairline
      and dropdown shadow, making the floating card look edgeless. The close X is
      dropped (show-close-button=false): a palette dismisses on Esc / outside-click /
      selection, and a corner X both clashed with the search row and skipped the
      icon-button contract. Width is held to a palette-friendly max-w-md.
    -->
    <!--
      will-change-transform promotes the scaling dialog to its own GPU layer for the
      open/close zoom. Without it the surface's deep elevated shadow (a large blur)
      gets re-rasterized on the CPU every frame as the element scales — that paint
      cost (NOT JS) is what made opening feel janky. Promoted, the shadowed surface
      rasterizes once and the scale composites on the GPU, so the heavier shadow is
      free during the animation.
    -->
    <DialogContent
      :show-close-button="false"
      overlay-class="bg-black/40 duration-100"
      class="gap-0 border-0 bg-transparent p-0 shadow-none rounded-none duration-100 data-[state=open]:zoom-in-[0.98] data-[state=closed]:zoom-out-[0.98] will-change-transform sm:max-w-md"
    >
      <DialogHeader class="sr-only">
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription>{{ description }}</DialogDescription>
      </DialogHeader>
      <!-- Heavier elevation + stronger edge than the inline preview: the palette
           floats over a dimmed scrim, so it needs a deeper cast and a crisper
           outline to separate cleanly from the darkened background. -->
      <Command class="border-[color:var(--border-menu-elevated)] shadow-[var(--shadow-dropdown-elevated)]">
        <slot v-bind="slotProps" />
      </Command>
    </DialogContent>
  </Dialog>
</template>
