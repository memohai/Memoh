<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <span
    data-slot="breadcrumb-page"
    role="link"
    aria-disabled="true"
    aria-current="page"
    :class="cn('text-foreground font-normal inline-flex items-center leading-none [&>*]:leading-none', props.class)"
  >
    <slot />
  </span>
</template>
