<script setup lang="ts">
// DialogViewHeader — the header row for a dialog that swaps views (a list and
// a drill-in form) or otherwise needs back / title / close on ONE centerline.
// Extracted from the Access → Advanced-rules dialog; the two traps it fixed
// are baked in here so no caller re-trips them:
//
// - The built-in corner close (DialogContent's absolute top-3 right-3) sits
//   ~12px off the title's centerline once the title is a grid row below the
//   p-6 padding. Callers MUST pass :show-close-button="false" on
//   DialogContent — this header renders the close inline instead.
//   The close keeps `relative` (NEVER `static`): the ghost hover wash is an
//   absolute ::before{inset:0} that needs the button as its containing block;
//   `static` lets it escape to the fixed DialogContent and tint the whole
//   dialog on hover.
// - A centered title needs BOTH flanks anchored, never decorative centering:
//   centered is earned by a back chevron (form view) or body weight below
//   (populated list); a bare empty state flushes left. The component doesn't
//   guess from data — the caller drives `centered` with its own predicate
//   (e.g. `formVisible || items.length > 0`) and `showBack` for the chevron.
//
// Grid: centered → [2rem_1fr_2rem] (equal side columns keep the title on the
// optical center; a <span> holds the left cell when there's no chevron);
// left → [1fr_2rem] (workbench form: title flush-left, close right).
import { ChevronLeft } from 'lucide-vue-next'
import { Button } from '#/components/button'
import DialogCloseButton from './DialogCloseButton.vue'
import DialogHeader from './DialogHeader.vue'
import DialogTitle from './DialogTitle.vue'

withDefaults(defineProps<{
  /** Center the title (both flanks anchored) or flush it left. */
  centered?: boolean
  /** Show the back chevron in the left cell (implies a centered layout use). */
  showBack?: boolean
  /** aria-label for the back chevron — pass the caller's localized "Back". */
  backLabel?: string
}>(), {
  centered: false,
  showBack: false,
  backLabel: 'Back',
})

defineEmits<{
  back: []
}>()
</script>

<template>
  <DialogHeader>
    <div
      data-slot="dialog-view-header"
      class="grid items-center"
      :class="centered ? 'grid-cols-[2rem_1fr_2rem]' : 'grid-cols-[1fr_2rem]'"
    >
      <template v-if="centered">
        <Button
          v-if="showBack"
          variant="ghost"
          size="icon-sm"
          class="text-muted-foreground"
          :aria-label="backLabel"
          @click="$emit('back')"
        >
          <ChevronLeft class="size-4" />
        </Button>
        <span v-else />
      </template>
      <DialogTitle :class="centered ? 'text-center' : ''">
        <slot />
      </DialogTitle>
      <DialogCloseButton class="relative top-auto right-auto" />
    </div>
  </DialogHeader>
</template>
