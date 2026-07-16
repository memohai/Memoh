<template>
  <TooltipProvider v-if="normalizedDescription">
    <Tooltip v-model:open="open">
      <TooltipTrigger as-child>
        <slot />
      </TooltipTrigger>
      <TooltipContent class="max-w-80 whitespace-pre-wrap text-left leading-relaxed">
        {{ normalizedDescription }}
      </TooltipContent>
    </Tooltip>
  </TooltipProvider>
  <slot v-else />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@felinic/ui'

const props = defineProps<{
  description?: string | null
}>()

const open = defineModel<boolean>('open', { default: false })
const normalizedDescription = computed(() => props.description?.trim() || '')
</script>
