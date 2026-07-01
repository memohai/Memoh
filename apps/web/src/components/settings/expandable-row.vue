<template>
  <!-- A settings row whose whole header toggles a collapsible body. The border
       and inset wrap the header AND the expanded panel as one unit (mx-4, one
       hairline at the bottom), so an open row reads as a single block, not two
       stacked rows. The header reuses SettingsRow's spacing skeleton
       (min-h-[3.75rem] py-3) but is a real <button> — SettingsRow itself is not
       interactive, so composing it here would nest a button inside a plain row;
       reusing the skeleton keeps the rhythm without that trap. -->
  <div class="mx-4 border-b border-border py-3 last:border-b-0">
    <button
      type="button"
      class="flex min-h-[3.75rem] w-full items-center py-0 text-left"
      :aria-expanded="open"
      @click="open = !open"
    >
      <!-- Leading media, same gap-3 hug as SettingsRow. -->
      <div
        v-if="$slots.leading"
        class="mr-3 flex shrink-0 items-center"
      >
        <slot name="leading" />
      </div>

      <!-- Body: a custom block (#content) or the label/description pair. -->
      <div class="min-w-0 flex-1">
        <slot name="content">
          <div class="truncate text-sm font-medium text-foreground">
            {{ label }}
          </div>
          <p
            v-if="description"
            class="mt-0.5 text-xs text-muted-foreground"
          >
            {{ description }}
          </p>
        </slot>
      </div>

      <!-- Trailing: a caller cluster (#trailing) plus the disclosure chevron,
           which rotates a half-turn on open. Default is just the chevron. -->
      <div class="ml-4 flex shrink-0 items-center gap-3">
        <slot name="trailing" />
        <ChevronDown
          class="size-4 text-muted-foreground transition-transform"
          :class="{ 'rotate-180': open }"
        />
      </div>
    </button>

    <!-- Expanded body: a grid-rows 0fr→1fr transition animates height without
         measuring the content, so a variable-height panel opens smoothly. The
         inner wrapper needs overflow-hidden for the collapse to clip cleanly. -->
    <div
      class="grid transition-[grid-template-rows] duration-200 ease-out"
      :class="open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'"
    >
      <div class="overflow-hidden">
        <div class="mt-3">
          <slot name="expanded" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ChevronDown } from 'lucide-vue-next'

withDefaults(defineProps<{
  label?: string
  description?: string
}>(), {
  label: '',
  description: '',
})

// Open state is two-way so a parent can also collapse rows programmatically
// (e.g. collapse-all), while the header click toggles it locally.
const open = defineModel<boolean>('open', { default: false })
</script>
