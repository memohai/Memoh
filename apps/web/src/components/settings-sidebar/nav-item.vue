<template>
  <Button
    variant="ghost"
    block
    class="justify-start gap-2.5 pl-3.5! pr-3! font-[430] tracking-[-0.1px] text-foreground/85"
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
