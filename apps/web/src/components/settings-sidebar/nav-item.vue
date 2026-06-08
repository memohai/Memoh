<template>
  <Button
    variant="ghost"
    block
    class="justify-start gap-2.5 pl-3.5! pr-3! font-[450] text-foreground/80 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:px-0!"
    :class="active && 'bg-sidebar-accent text-foreground!'"
  >
    <slot />
  </Button>
</template>

<script setup lang="ts">
import { Button } from '@memohai/ui'

withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})
</script>
