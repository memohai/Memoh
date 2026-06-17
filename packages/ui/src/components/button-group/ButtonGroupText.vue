<script setup lang="ts">
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonGroupVariants } from '.'
import { Primitive } from 'reka-ui'
import { cn } from '#/lib/utils'

interface Props extends PrimitiveProps {
  class?: HTMLAttributes['class']
  orientation?: ButtonGroupVariants['orientation']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'div',
})
</script>

<template>
  <Primitive
    data-slot="button-group-text"
    :data-orientation="props.orientation"
    :as="as"
    :as-child="asChild"
    :class="cn('flex select-none items-center justify-center gap-1.5 whitespace-nowrap px-3 text-control font-medium text-muted-foreground [&_svg]:pointer-events-none [&_svg:not([class*=size-])]:size-4', props.class)"
  >
    <slot />
  </Primitive>
</template>
