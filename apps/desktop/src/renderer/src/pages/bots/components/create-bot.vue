<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <slot name="trigger">
        <Button variant="default">
          <Plus class="mr-1.5" />
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
                  @update:model-value="(val: string) => handleChange(val === emptyTimezoneValue ? '' : val)"
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
                  @update:model-value="(nextValue: string) => handleChange(nextValue)"
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

          <FormField
            v-if="localWorkspaceEnabled"
            v-slot="{ value, handleChange }"
            name="workspace_backend"
          >
            <FormItem>
              <Label class="mb-2">{{ $t('bots.workspaceBackend') }}</Label>
              <FormControl>
                <Select
                  :model-value="value || 'container'"
                  @update:model-value="(nextValue: string) => handleChange(nextValue)"
                >
                  <SelectTrigger class="w-full">
                    <SelectValue :placeholder="$t('bots.workspaceBackend')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="container">
                      {{ $t('bots.workspaceBackends.container') }}
                    </SelectItem>
                    <SelectItem value="local">
                      {{ $t('bots.workspaceBackends.local') }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormControl>
              <p class="text-xs text-muted-foreground">
                {{ $t('bots.workspaceBackendHint') }}
              </p>
            </FormItem>
          </FormField>

          <FormField
            v-if="localWorkspaceEnabled && form.values.workspace_backend === 'local'"
            v-slot="{ componentField }"
            name="local_workspace_path"
          >
            <FormItem>
              <Label class="mb-2">{{ $t('bots.localWorkspacePath') }}</Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('bots.localWorkspacePathPlaceholder')"
                  v-bind="componentField"
                  @input="localPathTouched = true"
                />
              </FormControl>
              <p class="text-xs text-muted-foreground">
                {{ $t('bots.localWorkspaceWarning') }}
              </p>
            </FormItem>
          </FormField>

          <div class="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            {{ form.values.workspace_backend === 'local' ? $t('bots.createBotLocalHint') : $t('bots.createBotWaitHint') }}
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
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  FormControl,
  FormField,
  FormItem,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Spinner,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@memohai/ui'
import { CircleHelp, Plus } from 'lucide-vue-next'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postBotsMutation, getBotsQueryKey } from '@memohai/sdk/colada'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { computed, watch, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import z from 'zod'
import { useDialogMutation } from '@memohai/web/composables/useDialogMutation'
import { aclPresetOptions, defaultAclPreset } from '@memohai/web/constants/acl-presets'
import TimezoneSelect from '@memohai/web/components/timezone-select/index.vue'
import { emptyTimezoneValue } from '@memohai/web/utils/timezones'
import { useCapabilitiesStore } from '@memohai/web/store/capabilities'

const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const { run } = useDialogMutation()
const capabilities = useCapabilitiesStore()
void capabilities.load()

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)
const localPathTouched = ref(false)

const formSchema = toTypedSchema(z.object({
  display_name: z.string().min(1),
  avatar_url: z.string().optional(),
  timezone: z.string().optional(),
  acl_preset: z.string().min(1),
  workspace_backend: z.enum(['container', 'local']),
  local_workspace_path: z.string().optional(),
}))

const form = useForm({
  validationSchema: formSchema,
  initialValues: {
    display_name: '',
    avatar_url: '',
    timezone: '',
    acl_preset: defaultAclPreset,
    workspace_backend: 'container',
    local_workspace_path: '',
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

async function refreshDefaultWorkspacePath() {
  if (!localWorkspaceEnabled.value || form.values.workspace_backend !== 'local' || localPathTouched.value) return
  const displayName = form.values.display_name?.trim()
  if (!displayName) return
  const path = await window.api.desktop.defaultWorkspacePath(displayName)
  form.setFieldValue('local_workspace_path', path)
}

watch(() => [form.values.display_name, form.values.workspace_backend] as const, () => {
  void refreshDefaultWorkspacePath()
})

watch(open, (val) => {
  if (val) {
    localPathTouched.value = false
    form.resetForm({
      values: {
        display_name: '',
        avatar_url: '',
        timezone: '',
        acl_preset: defaultAclPreset,
        workspace_backend: localWorkspaceEnabled.value ? 'local' : 'container',
        local_workspace_path: '',
      },
    })
  } else {
    form.resetForm()
  }
})

const handleSubmit = form.handleSubmit(async (values) => {
  const metadata = values.workspace_backend === 'local'
    ? {
        workspace: {
          backend: 'local',
          local_workspace_path: values.local_workspace_path,
        },
      }
    : undefined

  await run(
    () => createBot({
      body: {
        display_name: values.display_name,
        avatar_url: values.avatar_url || undefined,
        timezone: values.timezone || undefined,
        is_active: true,
        acl_preset: values.acl_preset,
        metadata,
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
