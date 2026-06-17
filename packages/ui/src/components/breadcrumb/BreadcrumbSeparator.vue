<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <li
    data-slot="breadcrumb-separator"
    role="presentation"
    aria-hidden="true"
    :class="cn('inline-flex items-center select-none [&>svg]:size-3 text-muted-foreground/70 leading-none [&>*]:leading-none', props.class)"
  >
    <slot>
      /
    </slot>
  </li>
</template>
