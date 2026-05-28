import { ref, onMounted } from 'vue'

export function nextFrame(fn: () => void) {
  requestAnimationFrame(() => {
    requestAnimationFrame(fn)
  })
}

export function useStepTransition() {
  const visible = ref(false)
  const exiting = ref(false)

  onMounted(() => {
    nextFrame(() => {
      visible.value = true
    })
  })

  function leave(action: () => void) {
    exiting.value = true
    setTimeout(action, 175)
  }

  return { visible, exiting, leave }
}
