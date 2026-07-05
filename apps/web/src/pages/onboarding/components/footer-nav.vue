<template>
  <!-- 之前 13 个裸 button 手写三套宽度、disabled 透明度不一致;归一到这里。 -->
  <div
    class="mt-auto pt-12 flex items-center justify-end gap-3 transition-all duration-[350ms] ease-out"
    :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
  >
    <slot name="prev">
      <button
        v-if="prevLabel"
        type="button"
        :class="prevClass"
        @click="$emit('prev')"
      >
        {{ prevLabel }}
      </button>
    </slot>
    <slot name="next">
      <button
        v-if="nextLabel"
        type="button"
        :class="nextClass"
        :disabled="nextDisabled || nextLoading"
        @click="$emit('next')"
      >
        <Spinner
          v-if="nextLoading"
          class="mr-2"
        />
        {{ nextLabel }}
      </button>
    </slot>
  </div>
</template>

<script setup lang="ts">
import { Spinner } from '@memohai/ui'

// hover/disabled 是 owner 级刻意交互反馈,不属于页面注入
const prevClass = 'inline-flex h-[2.625rem] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring' /* ui-allow-style */
const nextClass = 'inline-flex h-[2.625rem] w-[180px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2' /* ui-allow-style */

withDefaults(defineProps<{
  prevLabel?: string
  nextLabel?: string
  nextDisabled?: boolean
  nextLoading?: boolean
  visible?: boolean
}>(), {
  prevLabel: '',
  nextLabel: '',
  nextDisabled: false,
  nextLoading: false,
  visible: false,
})

defineEmits<{
  prev: []
  next: []
}>()
</script>
