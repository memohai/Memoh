<script setup lang="ts">
import type { ComboboxContentEmits, ComboboxContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ComboboxContent, ComboboxPortal, useForwardPropsEmits } from 'reka-ui'
import { menuContentClass, menuSlideClass, menuViewportClass } from '#/lib/menu'
import { cn } from '#/lib/utils'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<ComboboxContentProps & { class?: HTMLAttributes['class'] }>(), {
  position: 'popper',
  align: 'center',
  sideOffset: 4,
})
const emits = defineEmits<ComboboxContentEmits>()

const delegatedProps = reactiveOmit(props, 'class')
const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <ComboboxPortal>
    <ComboboxContent
      data-slot="combobox-list"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="cn(
        menuContentClass,
        menuSlideClass,
        menuViewportClass,
        'w-[200px] max-h-[300px] origin-(--reka-combobox-content-transform-origin)',
        props.class
      )"
    >
      <slot />
    </ComboboxContent>
  </ComboboxPortal>
</template>
