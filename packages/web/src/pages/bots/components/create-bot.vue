<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <slot name="trigger">
        <Button variant="default">
          <FontAwesomeIcon
            :icon="['fas', 'plus']"
            class="mr-1.5"
          />
          {{ $t('bots.createBot') }}
        </Button>
      </slot>
    </DialogTrigger>
    <DialogContent class="sm:max-w-md">
      <form @submit="handleSubmit">
        <DialogHeader>
          <DialogTitle>{{ isEdit ? $t('bots.editBot') : $t('bots.createBot') }}</DialogTitle>
          <DialogDescription>
            <Separator class="my-4" />
          </DialogDescription>
        </DialogHeader>

        <div class="flex flex-col gap-4">
          <!-- Display Name -->
          <FormField
            v-slot="{ componentField }"
            name="display_name"
          >
            <FormItem>
              <Label class="mb-2">{{ $t('bots.displayName') }}</Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('bots.displayNamePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Avatar URL -->
          <FormField
            v-slot="{ componentField }"
            name="avatar_url"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('bots.avatarUrl') }}
                <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
              </Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('bots.avatarUrlPlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Type (only for create) -->
          <FormField
            v-if="!isEdit"
            v-slot="{ componentField }"
            name="type"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('bots.type') }}
                <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
              </Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('bots.typePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Active (only for edit) -->
          <FormField
            v-if="isEdit"
            v-slot="{ componentField }"
            name="is_active"
          >
            <FormItem class="flex items-center justify-between">
              <Label>{{ $t('bots.active') }}</Label>
              <Switch
                v-model="componentField.modelValue"
                @update:model-value="componentField['onUpdate:modelValue']"
              />
            </FormItem>
          </FormField>
        </div>

        <DialogFooter class="mt-6">
          <DialogClose as-child>
            <Button variant="outline">
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            type="submit"
            :disabled="!form.meta.value.valid || submitLoading"
          >
            <Spinner v-if="submitLoading" />
            {{ isEdit ? $t('common.save') : $t('bots.createBot') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
  Button,
  FormField,
  FormControl,
  FormItem,
  Separator,
  Label,
  Spinner,
  Switch,
} from '@memoh/ui'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { computed, watch } from 'vue'
import { useCreateBot, useUpdateBot, type BotInfo } from '@/composables/api/useBots'

const open = defineModel<boolean>('open', { default: false })
const editBot = defineModel<BotInfo | null>('editBot', { default: null })

const isEdit = computed(() => !!editBot.value)

const formSchema = toTypedSchema(z.object({
  display_name: z.string().min(1),
  avatar_url: z.string().optional(),
  type: z.string().optional(),
  is_active: z.coerce.boolean().optional(),
}))

const form = useForm({
  validationSchema: formSchema,
})

const { mutate: createBot, isLoading: createLoading } = useCreateBot()
const { mutate: updateBot, isLoading: updateLoading } = useUpdateBot()

const submitLoading = computed(() => createLoading.value || updateLoading.value)

// 打开弹窗时，如果是编辑模式则填入数据，否则重置
watch(open, (val) => {
  if (val && editBot.value) {
    form.resetForm({
      values: {
        display_name: editBot.value.display_name,
        avatar_url: editBot.value.avatar_url || '',
        is_active: editBot.value.is_active,
      },
    })
  } else if (!val) {
    form.resetForm()
    editBot.value = null
  }
})

const handleSubmit = form.handleSubmit(async (values) => {
  try {
    if (isEdit.value && editBot.value) {
      await updateBot({
        id: editBot.value.id,
        display_name: values.display_name,
        avatar_url: values.avatar_url || undefined,
        is_active: values.is_active,
      })
    } else {
      await createBot({
        display_name: values.display_name,
        avatar_url: values.avatar_url || undefined,
        type: values.type || undefined,
        is_active: true,
      })
    }
    open.value = false
  } catch {
    return
  }
})
</script>
