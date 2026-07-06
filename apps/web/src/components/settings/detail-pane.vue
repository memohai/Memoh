<template>
  <div>
    <!-- Back row, width-matched to the reused detail body below (which brings
         its own SettingsShell) so the header shares the content's rails. The
         button itself (geometry, alignment, hover-chip overhang) is
         SettingsBackButton — one shared spec, tuned there only. -->
    <div
      class="mx-auto w-full px-4 pt-4 md:px-6 md:pt-6"
      :class="widthClass"
    >
      <SettingsBackButton
        :label="backLabel"
        @click="emit('back')"
      />
    </div>

    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import SettingsBackButton from '@/components/settings/back-button.vue'

const props = withDefaults(defineProps<{
  backLabel: string
  width?: 'narrow' | 'standard' | 'wide'
}>(), {
  width: 'standard',
})

const emit = defineEmits<{ back: [] }>()

const widthClass = computed(() => {
  switch (props.width) {
    case 'narrow':
      return 'max-w-3xl'
    case 'wide':
      return 'max-w-6xl'
    case 'standard':
    default:
      return 'max-w-4xl'
  }
})
</script>
