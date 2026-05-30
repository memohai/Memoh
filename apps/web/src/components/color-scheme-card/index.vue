<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check } from 'lucide-vue-next'
import type { ColorSchemeOption } from '@/constants/color-schemes'

withDefaults(defineProps<{
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
</script>

<template>
  <button
    type="button"
    class="rounded-lg border bg-background p-2 text-left transition-colors"
    :class="selected ? 'border-foreground' : 'border-border hover:border-muted-foreground/50'"
    @click="$emit('select')"
  >
    <div class="rounded-md border border-border bg-muted p-2">
      <div class="h-1.5 w-3/5 rounded-full bg-muted-foreground/40" />
      <div class="mt-1 h-1.5 w-4/5 rounded-full bg-muted-foreground/20" />
      <div class="mt-2 flex items-center gap-1">
        <div
          class="h-2 w-1/2 rounded-full"
          :style="{ backgroundColor: scheme.swatches[4] }"
        />
        <div
          class="size-2 shrink-0 rounded-full"
          :style="{ backgroundColor: scheme.swatches[5] }"
        />
        <div
          class="size-2 shrink-0 rounded-full"
          :style="{ backgroundColor: scheme.swatches[6] }"
        />
        <div
          class="size-2 shrink-0 rounded-full"
          :style="{ backgroundColor: scheme.swatches[7] }"
        />
      </div>
    </div>
    <div class="mt-2 flex items-center justify-between gap-2 px-0.5">
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
        class="size-3.5 shrink-0"
      />
    </div>
  </button>
</template>
