<template>
  <!-- Label on the left, control on the right; stacks only when the pane is too
       narrow to hold both on one line. -->
  <div class="mx-4 flex flex-col gap-2 border-b border-border py-3 last:border-b-0 sm:min-h-[3.75rem] sm:flex-row sm:items-center sm:justify-between sm:gap-4">
    <div class="min-w-0">
      <Label
        :for="fieldId"
        class="text-sm font-medium text-foreground"
      >
        {{ field.title || fieldKey }}
      </Label>
      <p
        v-if="field.description"
        class="mt-0.5 text-xs text-muted-foreground"
      >
        {{ field.description }}
      </p>
    </div>

    <div class="w-full shrink-0 sm:w-80">
      <InputGroup v-if="field.type === 'secret'">
        <InputGroupInput
          :id="fieldId"
          :model-value="stringValue"
          :type="revealed ? 'text' : 'password'"
          :placeholder="placeholder"
          @update:model-value="(v: string) => emit('update:modelValue', v)"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            size="icon-xs"
            variant="quiet"
            :aria-label="revealed
              ? t('bots.channels.hideSecretField', { field: field.title || fieldKey })
              : t('bots.channels.showSecretField', { field: field.title || fieldKey })"
            @click="revealed = !revealed"
          >
            <component :is="revealed ? EyeOff : Eye" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>

      <div
        v-else-if="field.type === 'bool'"
        class="flex sm:justify-end"
      >
        <Switch
          :model-value="!!modelValue"
          @update:model-value="(v: boolean) => emit('update:modelValue', !!v)"
        />
      </div>

      <Select
        v-else-if="field.type === 'enum' && field.enum"
        :model-value="String(modelValue ?? '')"
        @update:model-value="(v: string) => emit('update:modelValue', v)"
      >
        <SelectTrigger
          size="sm"
          class="w-full"
        >
          <SelectValue :placeholder="field.title || fieldKey" />
        </SelectTrigger>
        <SelectContent class="w-[--reka-select-trigger-width]">
          <SelectItem
            v-for="opt in field.enum"
            :key="opt"
            :value="opt"
          >
            {{ opt }}
          </SelectItem>
        </SelectContent>
      </Select>

      <Input
        v-else-if="field.type === 'number'"
        :id="fieldId"
        :model-value="stringValue"
        type="number"
        :placeholder="placeholder"
        class="w-full tabular-nums"
        @update:model-value="onNumber"
      />

      <Input
        v-else
        :id="fieldId"
        :model-value="stringValue"
        type="text"
        :placeholder="placeholder"
        class="w-full"
        @update:model-value="(v: string) => emit('update:modelValue', v)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Input, Label, Switch,
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
  InputGroup, InputGroupInput, InputGroupAddon, InputGroupButton,
} from '@memohai/ui'
import { Eye, EyeOff } from 'lucide-vue-next'
import type { ChannelFieldSchema } from '@memohai/sdk'

const props = defineProps<{
  field: ChannelFieldSchema
  fieldKey: string
  modelValue: unknown
}>()

const emit = defineEmits<{
  'update:modelValue': [value: unknown]
}>()

const { t } = useI18n()
const revealed = ref(false)

const fieldId = computed(() => `channel-field-${props.fieldKey}`)
const placeholder = computed(() => (props.field.example != null ? String(props.field.example) : ''))
const stringValue = computed(() => {
  const v = props.modelValue
  return typeof v === 'string' || typeof v === 'number' ? String(v) : ''
})

// Mirror the panel's old number handling: empty clears to '', non-numeric is dropped.
function onNumber(v: string | number) {
  if (v === '') {
    emit('update:modelValue', '')
    return
  }
  const n = typeof v === 'number' ? v : Number(v)
  emit('update:modelValue', Number.isNaN(n) ? '' : n)
}
</script>
