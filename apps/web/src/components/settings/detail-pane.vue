<template>
  <div>
    <!-- Back row, width-matched to the reused detail body below (which brings its
         own SettingsShell) so the arrow lines up with the content's left edge. -->
    <div
      class="mx-auto w-full px-4 pt-4 md:px-6 md:pt-6"
      :class="widthClass"
    >
      <Button
        variant="ghost"
        class="text-foreground/85"
        @click="emit('back')"
      >
        <ChevronLeft class="size-4" />
        {{ backLabel }}
      </Button>
    </div>

    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@memohai/ui'
import { ChevronLeft } from 'lucide-vue-next'

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
