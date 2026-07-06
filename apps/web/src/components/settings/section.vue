<template>
  <section class="space-y-2.5">
    <div
      v-if="title || $slots.actions"
      class="flex min-h-7 items-center justify-between gap-4 px-2"
    >
      <h2
        v-if="title"
        class="text-[13px] font-medium text-muted-foreground"
      >
        {{ title }}
      </h2>
      <slot name="actions" />
    </div>
    <!-- When a footer is present it becomes the card's REAL last child, so the
         row above it loses its :last-child border-b-0 escape and its inset
         hairline stacks against the footer's full-bleed border-t — two lines
         fighting. The nth-last-child(2) rule hands the "I'm last" treatment to
         whatever element sits directly above the footer. -->
    <div
      class="overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card"
      :class="$slots.footer ? '[&>:nth-last-child(2)]:border-b-0' : ''"
    >
      <slot />
      <!-- Footer: a right-aligned action bar (Save/Cancel) or a pagination strip.
           Lives INSIDE the card, after the rows, so its top hairline meets both
           card edges — the same inset logic as a row divider, but full-bleed
           because it splits the card body from its action band. Only rendered
           when a caller fills it, so a plain section is untouched. -->
      <div
        v-if="$slots.footer"
        class="flex items-center justify-end gap-2 border-t border-border px-4 py-3"
      >
        <slot name="footer" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  title?: string
}>(), {
  title: '',
})
</script>
