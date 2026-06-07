<script setup lang="ts">
// Renders a component across a variant × size grid. Variant/size keys are
// passed in explicitly (see lib/variant-specs.ts) — cva 0.7.1 doesn't expose
// its config for runtime introspection.
//
// The default scoped slot receives `{ variant, size }` (size is undefined when
// no sizes are supplied).
const props = withDefaults(defineProps<{
  variants: string[]
  sizes?: string[]
  /** Axis label shown above each row, e.g. 'variant' or 'orientation'. */
  axisLabel?: string
  /** Sizes a given variant doesn't support, e.g. { outline: ['sm'] } — those
   * cells are dropped entirely (no empty slot, no size label). */
  excludeSizes?: Record<string, string[]>
}>(), {
  axisLabel: 'variant',
})

function cellsFor(variant: string): (string | undefined)[] {
  if (!props.sizes?.length)
    return [undefined]
  const excluded = props.excludeSizes?.[variant] ?? []
  return props.sizes.filter(size => !excluded.includes(size))
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <div
      v-for="variant in variants"
      :key="variant"
      class="flex flex-col gap-1.5"
    >
      <code class="text-[11px] font-mono text-muted-foreground">{{ axisLabel }}="{{ variant }}"</code>
      <div class="flex flex-wrap items-end gap-3">
        <div
          v-for="(size, i) in cellsFor(variant)"
          :key="size ?? i"
          class="flex flex-col items-center gap-1"
        >
          <slot
            :variant="variant"
            :size="size"
          />
          <code
            v-if="size"
            class="text-[10px] font-mono text-muted-foreground/70"
          >{{ size }}</code>
        </div>
      </div>
    </div>
  </div>
</template>
