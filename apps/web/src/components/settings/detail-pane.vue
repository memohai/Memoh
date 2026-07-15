<template>
  <div>
    <!-- Back row, width-matched to the reused detail body below (which brings
         its own SettingsShell) so the header shares the content's rails. The
         button itself (geometry, alignment, hover-chip overhang) is
         SettingsBackButton — one shared spec, tuned there only. -->
    <div
      class="mx-auto w-full px-4 pt-4 md:px-6 md:pt-6"
      :class="widthClass"
    >
      <SettingsBackButton
        :label="backLabel"
        @click="emit('back')"
      />
    </div>

    <!-- Loading hold: same shell rails + card shape as a real detail form so a
         slow refresh / cold load does not flash an empty DetailPane. Slot content
         only mounts once the page has resolved its selected resource. -->
    <div
      v-if="loading"
      class="mx-auto w-full px-4 pb-12 pt-2 md:px-6"
      :class="widthClass"
      data-testid="detail-pane-skeleton"
    >
      <div class="space-y-6">
        <div class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3">
          <Skeleton class="size-9 shrink-0 rounded-full" />
          <Skeleton class="h-4 w-32" />
          <Skeleton class="ml-auto size-8 rounded-[var(--radius-control)]" />
        </div>
        <div class="overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card">
          <div
            v-for="i in 4"
            :key="i"
            class="mx-4 flex items-center justify-between border-b border-border py-3.5 last:border-b-0"
          >
            <Skeleton class="h-4 w-24" />
            <Skeleton class="h-8 w-48 max-w-[55%]" />
          </div>
        </div>
      </div>
    </div>
    <slot v-else />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Skeleton } from '@felinic/ui'
import SettingsBackButton from '@/components/settings/back-button.vue'

const props = withDefaults(defineProps<{
  backLabel: string
  width?: 'narrow' | 'standard' | 'wide'
  /** True while the URL asks for detail but the resource is still resolving. */
  loading?: boolean
}>(), {
  width: 'standard',
  loading: false,
})

const emit = defineEmits<{ back: [] }>()

const widthClass = computed(() => {
  switch (props.width) {
    case 'narrow':
      return 'max-w-3xl'
    case 'wide':
      return 'max-w-6xl'
    case 'standard':
    default:
      return 'max-w-4xl'
  }
})
</script>
