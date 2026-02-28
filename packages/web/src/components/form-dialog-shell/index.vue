<template>
  <Dialog
    v-model:open="open"
  >
    <DialogTrigger as-child>
      <slot name="trigger" />
    </DialogTrigger>
    <DialogContent         
      :class="maxWidthClass"
    >
      <form        
        @submit.prevent="handleSubmit"
      >
        <DialogHeader>
          <DialogTitle>{{ title }}</DialogTitle>
          <DialogDescription v-show="description">
            {{ description }}
          </DialogDescription>
        </DialogHeader>
        <slot name="body" />
        <DialogFooter class="mt-4">
          <DialogClose as-child>
            <Button
              variant="outline"
            >
              {{ cancelText }}
            </Button>
          </DialogClose>
          <Button
            type="submit"
            :disabled="submitDisabled || loading"
          >
            <Spinner
              v-if="loading"
              class="mr-1"
            />
            {{ submitText }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import {
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Spinner
} from '@memoh/ui'
import { Form, Field } from 'vee-validate'

withDefaults(defineProps<{
  title: string
  description?: string
  cancelText: string
  submitText: string
  submitDisabled?: boolean
  loading?: boolean
  maxWidthClass?: string
}>(), {
  description: undefined,
  submitDisabled: false,
  loading: false,
  maxWidthClass: 'sm:max-w-106.25',
})

const open = defineModel<boolean>('open', { default: false })

const emit = defineEmits<{
  submit: []
}>()

function handleSubmit() {
  emit('submit')
}
</script>
