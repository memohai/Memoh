import { nextTick, ref, watch, type Ref } from 'vue'
import { useElementBounding } from '@vueuse/core'
import { onBeforeRouteLeave } from 'vue-router'

/**
 * 自动滚动逻辑：当内容增长时自动滚动到底部，
 * 用户手动上滑时停止自动滚动，滑到底部时恢复。
 * 路由切换时记住滚动位置。
 */
export function useAutoScroll(
  containerRef: Ref<HTMLElement | undefined>,
  loading: Ref<boolean>,
) {
  const { height, top } = useElementBounding(containerRef)

  let prevScroll = 0
  let curScroll = 0
  let autoScroll = true
  let cachedScroll = 0

  function getScrollParent() {
    return containerRef.value?.parentElement?.parentElement
  }

  // 检测是否滚动到底部 → 恢复自动滚动
  watch(top, () => {
    const container = getScrollParent()
    if (!container) return

    if (height.value === 0) {
      autoScroll = false
      prevScroll = curScroll = 0
    }

    const distanceToBottom = container.scrollHeight - container.clientHeight - container.scrollTop
    if (distanceToBottom < 1) {
      autoScroll = true
      prevScroll = curScroll = container.scrollTop
    }
  })

  // 内容高度变化时决定是否自动滚动
  watch(height, (newVal, oldVal) => {
    const container = getScrollParent()
    if (!container) return

    curScroll = container.scrollTop
    if (curScroll < prevScroll) {
      autoScroll = false
    }
    prevScroll = curScroll

    // 首次加载恢复缓存位置
    if (oldVal === 0 && newVal > container.clientHeight) {
      nextTick(() => {
        container.scrollTo({ top: cachedScroll })
      })
      return
    }

    // 自动滚动到底部
    const distanceToBottom = container.scrollHeight - container.clientHeight - container.scrollTop
    if (distanceToBottom >= 1 && autoScroll && loading.value) {
      container.scrollTo({
        top: container.scrollHeight - container.clientHeight,
        behavior: 'smooth',
      })
    }
  })

  // 离开路由前缓存滚动位置
  onBeforeRouteLeave(() => {
    const container = getScrollParent()
    if (container) {
      cachedScroll = container.scrollTop
    }
  })

  return { containerRef }
}
