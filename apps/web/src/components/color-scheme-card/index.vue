<script setup lang="ts">
import { computed } from 'vue'
import { useDark } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Check } from 'lucide-vue-next'
import type { ColorSchemeOption } from '@/constants/color-schemes'

const props = withDefaults(defineProps<{
  scheme: ColorSchemeOption
  selected?: boolean
  showDescription?: boolean
}>(), {
  selected: false,
  showDescription: false,
})

defineEmits<{
  select: []
}>()

const { t } = useI18n()
const isDark = useDark()
const previewSwatches = computed(() => isDark.value ? props.scheme.darkSwatches : props.scheme.swatches)
</script>

<template>
  <button
    type="button"
    class="group overflow-hidden rounded-[var(--radius-menu-shell)] border bg-card text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    :class="selected ? 'border-foreground' : 'border-border hover:border-muted-foreground/50'"
    :aria-pressed="selected"
    @click="$emit('select')"
  >
    <div
      class="relative h-14 overflow-hidden border-b border-border"
      :style="{ backgroundColor: previewSwatches[0] }"
    >
      <div
        class="absolute inset-y-0 left-0 w-[16%]"
        :style="{ backgroundColor: previewSwatches[1], opacity: 0.16 }"
      />

      <div class="relative ml-[16%] flex h-full flex-col justify-center px-2.5">
        <div
          class="h-1.5 w-20 rounded-full opacity-55"
          :style="{ backgroundColor: previewSwatches[2] }"
        />
        <div
          class="mt-1 h-1.5 w-14 rounded-full opacity-38"
          :style="{ backgroundColor: previewSwatches[2] }"
        />
        <div
          class="mt-1 h-1.5 w-16 rounded-full opacity-30"
          :style="{ backgroundColor: previewSwatches[2] }"
        />
        <div
          class="mt-2.5 h-2 w-14 rounded-full"
          :style="{ backgroundColor: previewSwatches[4] }"
        />
      </div>

      <div
        v-if="selected"
        class="absolute right-1.5 top-1.5 flex size-4 items-center justify-center rounded-full text-primary-foreground"
        :style="{ backgroundColor: previewSwatches[4] }"
      >
        <Check class="size-2.5" />
      </div>
    </div>

    <div class="flex items-center justify-between gap-2 px-2.5 py-1.5">
      <div class="min-w-0">
        <p class="text-xs font-medium">
          {{ t(scheme.labelKey) }}
        </p>
        <p
          v-if="showDescription"
          class="mt-0.5 text-[11px] text-muted-foreground"
        >
          {{ t(scheme.descriptionKey) }}
        </p>
      </div>
      <Check
        v-if="selected"
        class="size-3.5 shrink-0 text-foreground"
      />
    </div>
  </button>
</template>
