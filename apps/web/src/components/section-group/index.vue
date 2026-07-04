<template>
  <!-- A titled content group: a foreground section label (+ optional hint) with an
       optional trailing action, heading a BARE body — typically a BackendCard grid.
       Deliberately NOT SettingsSection: that owns a MUTED label and wraps its body in
       a bordered card (the settings-row tier). This is the page-content tier — a
       stronger foreground label heading content that already carries its own borders,
       so the group adds no card of its own and there is no card-in-card. The header
       edges match PageShell's: title inset px-2, actions flush right against the body.
       Use ONLY on pages that stack SEVERAL such groups (voice TTS/STT, web-search
       search/fetch) — a single-group gallery page (providers, video) lets PageShell
       own the title/hint/action directly with no group layer, so wrapping its one
       group here would just duplicate the page title at a second tier. Replaces the
       hand-written `space-y-2.5` header + grid those pages each carried a byte-copy
       of. -->
  <section class="space-y-2.5">
    <div
      v-if="title || description || $slots.actions"
      class="flex items-center justify-between gap-4"
    >
      <div
        v-if="title || description"
        class="min-w-0 px-2"
      >
        <h2
          v-if="title"
          class="text-label font-medium text-foreground"
        >
          {{ title }}
        </h2>
        <p
          v-if="description"
          class="text-xs text-muted-foreground"
        >
          {{ description }}
        </p>
      </div>
      <div
        v-if="$slots.actions"
        class="flex shrink-0 items-center gap-2"
      >
        <slot name="actions" />
      </div>
    </div>
    <slot />
  </section>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
  // An optional muted one-line hint under the section label (e.g. what this group of
  // providers is for). Sits directly under the title, same rhythm as the pages that
  // previously hand-rolled it.
  description?: string
}>()
</script>
