<script setup lang="ts">
import { Button, Input } from '@memoh/ui'

export interface KeyValuePair {
  key: string
  value: string
}

const props = withDefaults(defineProps<{
  modelValue: KeyValuePair[]
  keyPlaceholder?: string
  valuePlaceholder?: string
  readonly?: boolean
}>(), {
  keyPlaceholder: 'Key',
  valuePlaceholder: 'Value',
  readonly: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: KeyValuePair[]]
}>()

function updateRow(index: number, field: 'key' | 'value', val: string) {
  const list = [...props.modelValue]
  list[index] = { ...list[index], [field]: val }
  emit('update:modelValue', list)
}

function addRow() {
  emit('update:modelValue', [...props.modelValue, { key: '', value: '' }])
}

function removeRow(index: number) {
  const list = [...props.modelValue]
  list.splice(index, 1)
  emit('update:modelValue', list)
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div
      v-for="(pair, index) in modelValue"
      :key="index"
      class="flex items-center gap-2"
    >
      <Input
        :model-value="pair.key"
        :placeholder="keyPlaceholder"
        :readonly="readonly"
        class="flex-1 font-mono text-xs"
        @update:model-value="(val) => updateRow(index, 'key', String(val))"
      />
      <Input
        :model-value="pair.value"
        :placeholder="valuePlaceholder"
        :readonly="readonly"
        class="flex-1 font-mono text-xs"
        @update:model-value="(val) => updateRow(index, 'value', String(val))"
      />
      <Button
        v-if="!readonly"
        type="button"
        variant="ghost"
        size="icon"
        class="shrink-0 size-8 text-muted-foreground hover:text-destructive"
        @click="removeRow(index)"
      >
        <FontAwesomeIcon :icon="['fas', 'xmark']" />
      </Button>
    </div>
    <Button
      v-if="!readonly"
      type="button"
      variant="outline"
      size="sm"
      class="w-fit"
      @click="addRow"
    >
      <FontAwesomeIcon
        :icon="['fas', 'plus']"
        class="mr-1.5"
      />
      {{ $t('common.add') }}
    </Button>
  </div>
</template>
