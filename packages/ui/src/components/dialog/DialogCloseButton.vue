<script setup lang="ts">
// Shared corner dismiss for every modal surface (Dialog, DialogScrollContent, Sheet).
// One source of truth so the close affordance can't drift per component: it reuses the
// ghost icon Button (icon-button contract — size-8 hit area, 16px lucide glyph, ghost
// hover/press chrome from style.css) wrapped in reka DialogClose. as-child keeps
// Button's data-button anchor so the ghost chrome survives the DialogClose data-slot
// merge. Sheet rides the same reka Dialog primitive, so DialogClose dismisses it too.
import type { HTMLAttributes } from 'vue'
import { DialogClose } from 'reka-ui'
import { X } from 'lucide-vue-next'
import { Button } from '#/components/button'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <DialogClose as-child>
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="Close"
      :class="cn('absolute top-3 right-3', props.class)"
    >
      <X />
    </Button>
  </DialogClose>
</template>
