<template>
  <TooltipProvider v-if="normalizedDescription">
    <Tooltip>
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

const normalizedDescription = computed(() => props.description?.trim() || '')
</script>
