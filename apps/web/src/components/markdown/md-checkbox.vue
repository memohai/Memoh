<script setup lang="ts">
import { Checkbox } from '@felinic/ui'

// A rendered `- [x]` / `- [ ]` task marker. We swap markstream's bespoke
// checkbox glyph for the real library Checkbox so a task list in chat matches
// the checkbox used everywhere else in the app. It is display-only: kept
// non-interactive via pointer-events / tabindex rather than `disabled`, so it
// holds full color instead of the 40% disabled fade. The `checkbox-node` class
// is preserved so the list-marker rules in style.css still key off it.
defineProps<{
  node: { type: 'checkbox', checked: boolean, raw: string }
}>()
</script>

<template>
  <Checkbox
    :model-value="node.checked"
    tabindex="-1"
    aria-hidden="true"
    class="checkbox-node pointer-events-none inline-grid"
  />
</template>
