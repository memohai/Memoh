<template>
  <div
    class="flex flex-col gap-0.5 p-1"
    role="listbox"
  >
    <button
      type="button"
      role="option"
      :aria-selected="modelValue === 'off'"
      class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-accent hover:text-accent-foreground"
      :class="{ 'bg-accent': modelValue === 'off' }"
      @click="$emit('update:modelValue', 'off')"
    >
      <Lightbulb class="size-3.5 shrink-0 opacity-10" />
      {{ $t('chat.reasoningOff') }}
    </button>
    <button
      v-for="effort in efforts"
      :key="effort"
      type="button"
      role="option"
      :aria-selected="modelValue === effort"
      class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-accent hover:text-accent-foreground"
      :class="{ 'bg-accent': modelValue === effort }"
      @click="$emit('update:modelValue', effort)"
    >
      <Lightbulb
        class="size-3.5 shrink-0"
        :style="{ opacity: EFFORT_OPACITY[effort] ?? 0.5 }"
      />
      {{ $t(EFFORT_LABELS[effort] ?? effort) }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { Lightbulb } from 'lucide-vue-next'
import { EFFORT_LABELS, EFFORT_OPACITY } from './reasoning-effort'

defineProps<{
  efforts: string[]
}>()

defineEmits<{
  'update:modelValue': [value: string]
}>()

const modelValue = defineModel<string>({ default: '' })
</script>
