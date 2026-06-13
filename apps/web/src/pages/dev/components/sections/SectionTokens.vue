<script setup lang="ts">
// Design-token swatch wall: every semantic color token, side by side, in the
// current theme + color scheme. Edit a token in packages/ui/src/style.css and
// HMR repaints these instantly.
import SectionShell from '../components/SectionShell.vue'
import TokenSwatchGrid from '../components/TokenSwatchGrid.vue'
import { tokenGroups } from '../lib/token-catalog'
</script>

<template>
  <SectionShell
    id="tokens"
    label="Design tokens"
    description="Semantic color tokens from style.css. Filled blocks are surface/fill colors; “Ag” samples are text/foreground colors. Values resolve for the current theme + scheme."
  >
    <div class="flex flex-col gap-6">
      <TokenSwatchGrid
        v-for="group in tokenGroups"
        :key="group.id"
        :group="group"
      />
    </div>
  </SectionShell>
</template>
