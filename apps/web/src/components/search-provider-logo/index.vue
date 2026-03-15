<template>
  <component
    :is="iconComponent"
    class="shrink-0"
    :class="sizeClass"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { searchProviderIconMap } from '@memoh/icon'
import { GlobeIcon } from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  provider: string
  size?: 'xs' | 'sm' | 'md' | 'lg'
}>(), {
  size: 'sm',
})

const sizeClass = computed(() => {
  switch (props.size) {
    case 'xs': return 'size-3'
    case 'sm': return 'size-3.5'
    case 'md': return 'size-4'
    case 'lg': return 'size-5'
    default: return 'size-3.5'
  }
})

const iconComponent = computed(() => {
  const key = (props.provider ?? '').trim().toLowerCase()
  return searchProviderIconMap[key] ?? GlobeIcon
})
</script>
