<template>
  <!-- Vertical Label-over-control field. Distinct from SettingsRow, whose label
       sits BESIDE the control; a FieldStack stacks them so a run of them reads as
       a form column. space-y-1.5 is the label→control→help rhythm the house form
       uses everywhere a field is stacked. -->
  <div class="space-y-1.5">
    <!-- The label line. A plain #label slot lets a caller pair the label text
         with an inline toggle/meta on the same row (a name field + its enable
         switch); when it's filled it replaces the bound Label entirely, so the
         caller owns both the copy and the `for` wiring in that case. Otherwise
         the Label binds to the control via `for` so a click focuses it. -->
    <slot name="label">
      <Label
        v-if="label"
        :for="props.for"
      >
        {{ label }}
      </Label>
    </slot>

    <slot />

    <p
      v-if="help"
      class="text-xs text-muted-foreground"
    >
      {{ help }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { Label } from '@memohai/ui'

const props = withDefaults(defineProps<{
  label?: string
  // Bound to the control's id so clicking the label focuses it. Left to the
  // caller because only the caller knows the control's id.
  for?: string
  help?: string
}>(), {
  label: '',
  for: undefined,
  help: '',
})
</script>
