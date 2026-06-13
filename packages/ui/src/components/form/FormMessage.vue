<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { CircleAlert } from 'lucide-vue-next'
import { ErrorMessage } from 'vee-validate'
import { toValue } from 'vue'
import { cn } from '#/lib/utils'
import { useFormField } from './useFormField'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const { name, formMessageId } = useFormField()
</script>

<template>
  <ErrorMessage
    :id="formMessageId"
    v-slot="{ message }"
    data-slot="form-message"
    as="p"
    :name="toValue(name)"
    :class="cn(
      'text-destructive flex items-center gap-1.5 text-[13px] leading-snug',
      props.class,
    )"
  >
    <CircleAlert class="size-3.5 shrink-0" />
    <span>{{ message }}</span>
  </ErrorMessage>
</template>
