<script setup lang="ts">
import type { ContextMenuSubContentEmits, ContextMenuSubContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  ContextMenuSubContent,
  useForwardPropsEmits,
} from 'reka-ui'
import { menuContentClass, menuSlideClass, menuViewportClass } from '#/lib/menu'
import { cn } from '#/lib/utils'

const props = defineProps<ContextMenuSubContentProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<ContextMenuSubContentEmits>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <ContextMenuSubContent
    data-slot="context-menu-sub-content"
    v-bind="forwarded"
    :class="
      cn(
        menuContentClass,
        menuSlideClass,
        menuViewportClass,
        'min-w-[8rem] origin-(--reka-context-menu-content-transform-origin)',
        props.class,
      )
    "
  >
    <slot />
  </ContextMenuSubContent>
</template>
