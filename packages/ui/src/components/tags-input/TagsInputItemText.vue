<script setup lang="ts">
import type { TagsInputItemTextProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TagsInputItemText, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<TagsInputItemTextProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TagsInputItemText
    v-bind="forwardedProps"
    :class="cn('text-caption leading-none bg-transparent', props.class)"
  />
</template>
