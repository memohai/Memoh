<script setup lang="ts">
// A titled cell that wraps one component instance. The monospace header shows
// the markup that produced the sample, so the wall doubles as a usage cheat
// sheet. The default slot holds the live component.
defineProps<{
  /** Code-ish caption, e.g. `<Button variant="ghost">`. */
  label: string
  /** Optional secondary note under the label. */
  note?: string
}>()
</script>

<template>
  <div class="flex flex-col gap-2 rounded-lg border border-border bg-card p-3">
    <div class="flex flex-col gap-0.5">
      <code class="text-[11px] font-mono text-muted-foreground break-words">{{ label }}</code>
      <span
        v-if="note"
        class="text-[10px] text-muted-foreground/70"
      >{{ note }}</span>
    </div>
    <div class="flex min-h-9 flex-wrap items-center gap-3">
      <slot />
    </div>
  </div>
</template>
