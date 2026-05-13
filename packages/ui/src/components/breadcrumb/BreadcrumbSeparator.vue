<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { ChevronRight } from 'lucide-vue-next'
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
    :class="cn('inline-flex items-center [&>svg]:size-3.5 text-muted-foreground leading-none [&>*]:leading-none', props.class)"
  >
    <slot>
      <ChevronRight />
    </slot>
  </li>
</template>
