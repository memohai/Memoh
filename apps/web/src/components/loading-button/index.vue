<template>
  <Button
    v-bind="$attrs"
    :class="{ 'pointer-events-none': loading && !spinnerVisible }"
    :loading="spinnerVisible"
    loading-mode="leading"
    :disabled="disabled"
    :variant="variant"
    :size="size"
  >
    <slot />
  </Button>
</template>

<script setup lang="ts">
// Thin convenience over @felinic/ui Button: the spinner visuals (the leading
// slot that grows in, the busy color hold, the layout-stable swap) all live in
// Button now. This wrapper only adds the one thing that is an app concern rather
// than a component concern: holding the spinner back until a request actually
// feels slow, so a local round trip resolves without a flash. While we hold it
// we still block clicks (pointer-events, not `disabled`, so the button keeps
// full color instead of fading) to swallow double submits in that window.
import { Button } from '@felinic/ui'
import type { ButtonVariants } from '@felinic/ui'
import { onBeforeUnmount, ref, watch } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  loading?: boolean
  disabled?: boolean
  loadingDelay?: number
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
}>(), {
  loading: false,
  disabled: false,
  loadingDelay: 200,
  variant: 'default',
  size: 'default',
})

const spinnerVisible = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

function clearTimer() {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
}

watch(() => props.loading, (loading) => {
  clearTimer()
  if (!loading) {
    spinnerVisible.value = false
    return
  }
  if (props.loadingDelay > 0) {
    timer = setTimeout(() => {
      spinnerVisible.value = true
      timer = null
    }, props.loadingDelay)
  }
  else {
    spinnerVisible.value = true
  }
}, { immediate: true })

onBeforeUnmount(clearTimer)
</script>
