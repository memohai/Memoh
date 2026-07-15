<template>
  <div>
    <!-- Back row, width-matched to the reused detail body below (which brings
         its own SettingsShell) so the header shares the content's rails. The
         button itself (geometry / rail alignment) is SettingsBackButton —
         one shared spec, tuned there only. -->
    <div
      class="mx-auto w-full px-4 pt-4 md:px-6 md:pt-6"
      :class="widthClass"
    >
      <SettingsBackButton
        :label="backLabel"
        @click="emit('back')"
      />
    </div>

    <!-- Loading hold: reuse SettingsShell so rails / top gap match the real
         detail body (model-setting etc.). Header card mirrors the provider
         identity row 1:1 — round avatar, flex-1 title, trailing icon + switch
         — so the frame does not jump when content swaps in. -->
    <SettingsShell
      v-if="loading"
      :width="width"
      data-testid="detail-pane-skeleton"
    >
      <div class="space-y-6">
        <section class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3">
          <!-- Real header uses size-9 rounded-full avatar; force full round so the
               Skeleton base rounded-lg cannot leave a square chip. -->
          <Skeleton class="size-9 shrink-0 !rounded-full" />
          <div class="min-w-0 flex-1">
            <Skeleton class="h-4 w-36 max-w-full !rounded-md" />
          </div>
          <div class="ml-auto flex shrink-0 items-center gap-2">
            <!-- icon-sm trash affordance — use solid Tailwind radius tokens.
                 Arbitrary rounded-[var(--radius-control)] was not applying, so
                 the block rendered as a sharp square. -->
            <Skeleton class="size-8 shrink-0 !rounded-md" />
            <!-- Switch track (h-5 w-9 rounded-full), not a square block -->
            <Skeleton class="h-5 w-9 shrink-0 !rounded-full" />
          </div>
        </section>

        <!-- Form card: SettingsRow geometry (mx-4 / py-3.5 / inset hairlines) +
             h-9 field controls matching real Input / Select height. -->
        <div class="overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card">
          <div
            v-for="i in 4"
            :key="i"
            class="mx-4 flex items-center justify-between border-b border-border py-3.5 last:border-b-0"
          >
            <Skeleton class="h-4 w-24 shrink-0 !rounded-md" />
            <Skeleton class="h-9 w-80 max-w-[55%] !rounded-md" />
          </div>
        </div>
      </div>
    </SettingsShell>
    <slot v-else />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Skeleton } from '@felinic/ui'
import SettingsBackButton from '@/components/settings/back-button.vue'
import SettingsShell from '@/components/settings-shell/index.vue'

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
