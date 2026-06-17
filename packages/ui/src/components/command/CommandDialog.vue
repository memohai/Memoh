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
      menu-shell, card vs popover, shadow-2xl vs dropdown). The backdrop scrim AND the
      open/close motion (100ms fade + 2% zoom) are inherited from DialogContent itself —
      we ONLY strip chrome here, so the palette shares the one modal scrim + motion
      language instead of re-declaring its own. The close X is dropped
      (show-close-button=false): a palette dismisses on Esc / outside-click / selection,
      and a corner X both clashed with the search row and skipped the icon-button
      contract. Width is held to a palette-friendly max-w-md.
    -->
    <DialogContent
      :show-close-button="false"
      class="gap-0 border-0 bg-transparent p-0 shadow-none rounded-none sm:max-w-md"
    >
      <DialogHeader class="sr-only">
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription>{{ description }}</DialogDescription>
      </DialogHeader>
      <!-- Modal elevation, NOT the inline preview's flat dropdown cast: the palette
           floats over a dimmed scrim, so it carries --shadow-modal (shared with the
           card Dialog/Sheet). Edge is the shared --border-menu-elevated: a white
           hairline in dark mode, and NONE in light mode — a white palette already
           separates from the dark scrim by luminance, so a dark hairline there would
           just muddy the edge instead of sharpening it. -->
      <Command class="border-[color:var(--border-menu-elevated)] shadow-[var(--shadow-modal)]">
        <slot v-bind="slotProps" />
      </Command>
    </DialogContent>
  </Dialog>
</template>
