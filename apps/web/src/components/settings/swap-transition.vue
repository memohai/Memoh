<template>
  <!-- Directional push-pop for the list <-> detail swap. No `mode`, so the
       leaving and entering panes animate at the SAME time and cross-slide; the
       leaving pane is pulled out of flow (position:absolute, via the keyframes
       in style.css) so it doesn't shove the entering pane around. No `appear`:
       the first paint (landing from the sidebar) is a plain cut, like
       Appearance / Profile — only the swap moves. -->
  <!-- overflow-x-clip: the slide pushes panes ±24px horizontally; clip (not
       hidden) keeps that from flashing a horizontal scrollbar WITHOUT turning
       this into a vertical scroll container — vertical scrolling stays owned by
       the ancestor scroll area. -->
  <div class="relative overflow-x-clip">
    <Transition :name="direction === 'back' ? 'swap-back' : 'swap-forward'">
      <slot />
    </Transition>
  </div>
</template>

<script setup lang="ts">
import type { SwapDirection } from '@/composables/useViewSwap'

defineProps<{ direction: SwapDirection }>()
</script>
