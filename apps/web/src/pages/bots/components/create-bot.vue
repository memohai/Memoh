<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <slot name="trigger">
        <Button variant="default">
          <Plus
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

          <FormField
            v-slot="{ value, handleChange }"
            name="timezone"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('bots.timezone') }}
                <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
              </Label>
              <FormControl>
                <TimezoneSelect
                  :model-value="value || emptyTimezoneValue"
                  :placeholder="$t('bots.timezonePlaceholder')"
                  allow-empty
                  :empty-label="$t('bots.timezoneInherited')"
                  @update:model-value="(val) => handleChange(val === emptyTimezoneValue ? '' : val)"
                />
              </FormControl>
            </FormItem>
          </FormField>
          <FormField
            v-slot="{ value, handleChange }"
            name="acl_preset"
          >
            <FormItem>
              <div class="mb-2 flex items-center gap-2">
                <Label>{{ $t('bots.aclPreset') }}</Label>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      class="size-5 text-muted-foreground hover:text-foreground"
                      :aria-label="$t('bots.aclPresetHelp')"
                    >
                      <CircleHelp class="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-80 text-left leading-relaxed">
                    {{ $t('bots.aclPresetHelp') }}
                  </TooltipContent>
                </Tooltip>
              </div>
              <FormControl>
                <Select
                  :model-value="value || defaultAclPreset"
                  @update:model-value="(nextValue) => handleChange(nextValue)"
                >
                  <SelectTrigger class="w-full">
                    <SelectValue :placeholder="$t('bots.aclPreset')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="preset in aclPresetOptions"
                      :key="preset.value"
                      :value="preset.value"
                    >
                      {{ $t(preset.titleKey) }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormControl>
              <p
                v-if="getAclPresetDescription(value || defaultAclPreset)"
                class="text-xs text-muted-foreground"
              >
                {{ getAclPresetDescription(value || defaultAclPreset) }}
              </p>
            </FormItem>
          </FormField>
          <div class="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            {{ $t('bots.createBotWaitHint') }}
          </div>
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Label,
  Spinner,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@memohai/ui'
import { CircleHelp, Plus } from 'lucide-vue-next'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { watch } from 'vue'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postBotsMutation, getBotsQueryKey } from '@memohai/sdk/colada'
import { useI18n } from 'vue-i18n'
import { useDialogMutation } from '@/composables/useDialogMutation'
import { aclPresetOptions, defaultAclPreset } from '@/constants/acl-presets'
import { emptyTimezoneValue } from '@/utils/timezones'
import TimezoneSelect from '@/components/timezone-select/index.vue'

const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const { run } = useDialogMutation()

const formSchema = toTypedSchema(z.object({
  display_name: z.string().min(1),
  avatar_url: z.string().optional(),
  timezone: z.string().optional(),
  acl_preset: z.string().min(1),
}))

const form = useForm({
  validationSchema: formSchema,
  initialValues: {
    display_name: '',
    avatar_url: '',
    timezone: '',
    acl_preset: defaultAclPreset,
  },
})

const queryCache = useQueryCache()
const { mutateAsync: createBot, isLoading: submitLoading } = useMutation({
  ...postBotsMutation(),
  onSettled: () => queryCache.invalidateQueries({ key: getBotsQueryKey() }),
})

function getAclPresetOption(value?: string) {
  const presetValue = value || defaultAclPreset
  return aclPresetOptions.find(option => option.value === presetValue)
}

function getAclPresetDescriptionKey(value?: string) {
  return getAclPresetOption(value)?.descriptionKey
}

function getAclPresetDescription(value?: string) {
  const descriptionKey = getAclPresetDescriptionKey(value)
  return descriptionKey ? t(descriptionKey) : ''
}

watch(open, (val) => {
  if (val) {
    form.resetForm({
      values: {
        display_name: '',
        avatar_url: '',
        timezone: '',
        acl_preset: defaultAclPreset,
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
        timezone: values.timezone || undefined,
        is_active: true,
        acl_preset: values.acl_preset,
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
