<template>
  <div
    class="flex flex-col gap-0.5 p-1"
    role="listbox"
    @pointerleave="hoverIndex = -1"
  >
    <button
      v-for="(effort, index) in efforts"
      :key="effort"
      type="button"
      role="option"
      :aria-selected="modelValue === effort"
      :data-highlighted="hoverIndex === index ? '' : undefined"
      :class="[menuItemClass, 'h-8']"
      @click="$emit('update:modelValue', effort)"
      @pointermove="hoverIndex = index"
    >
      <Lightbulb
        class="size-3.5 shrink-0"
        :style="{ opacity: EFFORT_OPACITY[effort] ?? 0.5 }"
      />
      {{ $t(EFFORT_LABELS[effort] ?? effort) }}
      <Check
        v-if="modelValue === effort"
        class="ml-auto size-4 shrink-0"
      />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Lightbulb, Check } from 'lucide-vue-next'
import { menuItemClass } from '@memohai/ui'
import { EFFORT_LABELS, EFFORT_OPACITY } from './reasoning-effort'

defineProps<{
  efforts: string[]
}>()

defineEmits<{
  'update:modelValue': [value: string]
}>()

const modelValue = defineModel<string>({ default: '' })

const hoverIndex = ref(-1)
</script>
