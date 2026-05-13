<template>
  <div
    class="mx-auto w-full px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12"
    :class="widthClass"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type SettingsWidth = 'narrow' | 'standard' | 'wide' | 'full'

const props = withDefaults(defineProps<{
  width?: SettingsWidth
}>(), {
  width: 'standard',
})

const widthClass = computed(() => {
  switch (props.width) {
    case 'narrow':
      return 'max-w-3xl'
    case 'wide':
      return 'max-w-6xl'
    case 'full':
      return ''
    case 'standard':
    default:
      return 'max-w-4xl'
  }
})
</script>
