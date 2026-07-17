<template>
  <div class="flex items-start gap-2 px-1.5 py-1 text-destructive">
    <CircleAlert class="mt-0.5 size-3.5 shrink-0" />
    <span class="min-w-0 break-words text-body">{{ message }}</span>
  </div>
</template>

<script setup lang="ts">
// One error line inside the composer panel (send / model-switch / agent-switch
// failures). No shell of its own — the destructive tone is carried only by the
// icon + text color, since the panel's capsule already provides the surface.
import { CircleAlert } from 'lucide-vue-next'

defineProps<{
  message: string
}>()
</script>
