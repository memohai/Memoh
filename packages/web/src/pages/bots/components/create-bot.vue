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
          <DialogTitle>{{ $t('bots.createBot') }}</DialogTitle>
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

          <!-- Type -->
          <FormField
            v-slot="{ componentField }"
            name="type"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('common.type') }}
              </Label>
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    class="w-full"
                    :aria-label="$t('common.type')"
                  >
                    <SelectValue :placeholder="$t('bots.typePlaceholder')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="personal">
                        {{ $t('bots.types.personal') }}
                      </SelectItem>
                      <SelectItem value="public">
                        {{ $t('bots.types.public') }}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormControl>
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
            {{ $t('bots.createBot') }}
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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@memoh/ui'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { watch } from 'vue'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postBotsMutation, getBotsQueryKey } from '@memoh/sdk/colada'
import { useI18n } from 'vue-i18n'
import { useDialogMutation } from '@/composables/useDialogMutation'

const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const { run } = useDialogMutation()

const formSchema = toTypedSchema(z.object({
  display_name: z.string().min(1),
  avatar_url: z.string().optional(),
  type: z.string(),
}))

const form = useForm({
  validationSchema: formSchema,
  initialValues: {
    display_name: '',
    avatar_url: '',
    type: 'personal',
  },
})

const queryCache = useQueryCache()
const { mutateAsync: createBot, isLoading: submitLoading } = useMutation({
  ...postBotsMutation(),
  onSettled: () => queryCache.invalidateQueries({ key: getBotsQueryKey() }),
})

watch(open, (val) => {
  if (val) {
    form.resetForm({
      values: {
        display_name: '',
        avatar_url: '',
        type: 'personal',
      },
    })
  } else {
    form.resetForm()
  }
})

const handleSubmit = form.handleSubmit(async (values) => {
  await run(
    () => createBot({
      body: {
        display_name: values.display_name,
        avatar_url: values.avatar_url || undefined,
        type: values.type,
        is_active: true,
      },
    }),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {
        open.value = false
      },
    },
  )
})
</script>
