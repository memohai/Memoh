<script setup lang="ts" generic="T extends string | number">
import type { HTMLAttributes } from 'vue'
import type { SegmentedItem } from '.'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  /** Selected value (v-model). Falls back to the first item when unset. */
  modelValue?: T
  items: SegmentedItem<T>[]
  ariaLabel?: string
  class?: HTMLAttributes['class']
}>()

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()

const root = ref<HTMLElement>()
// The sliding thumb is positioned by MEASURING the active item (offset box) — it
// shares the item's exact footprint so the slide can never mismatch widths.
const indicator = ref({ left: 0, top: 0, width: 0, height: 0 })
// press-shrink fires ONLY when the already-active item is pressed (non-active
// items shrink via their own ::before:active in style.css).
const pressed = ref(false)

const active = computed<T | undefined>(() => props.modelValue ?? props.items[0]?.value)

function sync() {
  const el = root.value?.querySelector<HTMLElement>('[data-active="true"]')
  if (!el)
    return
  indicator.value = {
    left: el.offsetLeft,
    top: el.offsetTop,
    width: el.offsetWidth,
    height: el.offsetHeight,
  }
}

watch(() => [active.value, props.items] as const, () => nextTick(sync), { deep: true })

let ro: ResizeObserver | undefined
onMounted(() => {
  nextTick(sync)
  ro = new ResizeObserver(() => sync())
  if (root.value)
    ro.observe(root.value)
})
onBeforeUnmount(() => ro?.disconnect())

const thumbStyle = computed(() => ({
  translate: `${indicator.value.left}px ${indicator.value.top}px`,
  width: `${indicator.value.width}px`,
  height: `${indicator.value.height}px`,
  scale: pressed.value ? '0.97' : '1',
  // hide until measured so it never flashes a 0-width chip at the origin
  opacity: indicator.value.width > 0 ? '1' : '0',
}))

function select(item: SegmentedItem<T>) {
  if (item.disabled || item.value === active.value)
    return
  emit('update:modelValue', item.value)
}
function onDown(item: SegmentedItem<T>) {
  pressed.value = item.value === active.value
}
function clearPress() {
  pressed.value = false
}
</script>

<template>
  <div
    ref="root"
    data-slot="segmented"
    role="radiogroup"
    :aria-label="ariaLabel"
    :class="cn(props.class)"
  >
    <div
      data-segment-thumb
      :style="thumbStyle"
    />
    <button
      v-for="item in items"
      :key="String(item.value)"
      type="button"
      role="radio"
      data-segment-item
      :data-active="active === item.value"
      :aria-checked="active === item.value"
      :disabled="item.disabled"
      @pointerdown="onDown(item)"
      @pointerup="clearPress"
      @pointerleave="clearPress"
      @pointercancel="clearPress"
      @click="select(item)"
    >
      <slot
        name="item"
        :item="item"
      >
        {{ item.label ?? item.value }}
      </slot>
    </button>
  </div>
</template>
