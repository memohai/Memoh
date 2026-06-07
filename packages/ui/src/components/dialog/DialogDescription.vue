<script setup lang="ts">
// DESIGN NOTE — use sparingly. A one-line "short description" under the title is
// almost always noise: it restates the title or pads the header without telling the
// user anything actionable. Prefer NO description and let the title + body carry the
// dialog. Reach for it ONLY when the action is genuinely complex/risky and needs a
// framing sentence the title can't hold; otherwise put any real explanation in the
// body content, not here. (It still satisfies the a11y aria-describedby contract, so
// when omitted visually, keep a sr-only description — see CommandDialog.)
import type { DialogDescriptionProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { DialogDescription, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<DialogDescriptionProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <DialogDescription
    data-slot="dialog-description"
    v-bind="forwardedProps"
    :class="cn('text-muted-foreground text-body', props.class)"
  >
    <slot />
  </DialogDescription>
</template>
