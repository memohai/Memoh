<template>
  <div ref="anchor">
    <LivePeekLine
      v-if="isActive"
      :text="task.outputTail"
      mono
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, useTemplateRef, watch } from 'vue'
import { useElementVisibility } from '@vueuse/core'
import type { BackgroundTask } from '@/store/chat-list'
import { latestOutputLine } from '@/store/chat-list.utils'
import { useBgTaskBeacon } from '../composables/useBgTaskBeacons'
import LivePeekLine from './live-peek-line.vue'

const props = defineProps<{ task: BackgroundTask }>()

function normalizedStatus(): string {
  return (props.task.status || '').trim().toLowerCase()
}

const isActive = computed(() => {
  const status = normalizedStatus()
  return status === 'running' || status === 'stalled'
})

const isDone = computed(() => {
  const status = normalizedStatus()
  return status === 'completed' || status === 'failed' || status === 'killed'
})

const anchor = useTemplateRef<HTMLElement>('anchor')
const visible = useElementVisibility(anchor)
const beacon = useBgTaskBeacon()

// Mirror this row's task into the pane-level beacon registry so a floating pill
// can surface a running task once its row scrolls off screen, and briefly note
// its completion. The registry decides what (if anything) to show.
watch(
  () => ({
    taskId: props.task.taskId,
    active: isActive.value,
    done: isDone.value,
    visible: visible.value,
    line: latestOutputLine(props.task.outputTail) || props.task.command || '',
  }),
  (state) => {
    if (!beacon) return
    if (state.active || state.done) {
      beacon.upsert({
        taskId: state.taskId,
        phase: state.active ? 'active' : 'done',
        visible: state.visible,
        latestLine: state.line,
        scrollIntoView: () => anchor.value?.scrollIntoView({ block: 'center', behavior: 'smooth' }),
      })
    } else {
      beacon.remove(state.taskId)
    }
  },
  { immediate: true, deep: true },
)

onUnmounted(() => {
  beacon?.remove(props.task.taskId)
})
</script>
