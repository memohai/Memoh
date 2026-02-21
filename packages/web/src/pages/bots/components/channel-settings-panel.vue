<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-semibold">
          {{ channelItem.meta.display_name }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ channelItem.meta.type }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <template v-if="isEditMode">
          <Button
            variant="outline"
            size="sm"
            :disabled="isBusy"
            @click="handleToggleDisabled"
          >
            <Spinner
              v-if="action === 'toggle'"
              class="mr-1.5"
            />
            {{ form.disabled ? $t('bots.channels.actionEnable') : $t('bots.channels.actionDisable') }}
          </Button>
          <ConfirmPopover
            :message="$t('bots.channels.deleteConfirm')"
            :loading="action === 'delete'"
            @confirm="handleDelete"
          >
            <template #trigger>
              <Button
                variant="destructive"
                size="sm"
                :disabled="isBusy"
              >
                <Spinner
                  v-if="action === 'delete'"
                  class="mr-1.5"
                />
                {{ $t('common.delete') }}
              </Button>
            </template>
          </ConfirmPopover>
        </template>
      </div>
    </div>

    <Separator />

    <!-- Credentials form (dynamic from config_schema) -->
    <div class="space-y-4">
      <h4 class="text-sm font-medium">
        {{ $t('bots.channels.credentials') }}
      </h4>

      <div
        v-for="(field, key) in orderedFields"
        :key="key"
        class="space-y-2"
      >
        <Label :for="field.type === 'bool' || field.type === 'enum' ? undefined : `channel-field-${key}`">
          {{ field.title || key }}
          <span
            v-if="!field.required"
            class="text-xs text-muted-foreground ml-1"
          >({{ $t('common.optional') }})</span>
        </Label>
        <p
          v-if="field.description"
          class="text-xs text-muted-foreground"
        >
          {{ field.description }}
        </p>

        <!-- Secret field -->
        <div
          v-if="field.type === 'secret'"
          class="relative"
        >
          <Input
            :id="`channel-field-${key}`"
            v-model="form.credentials[key]"
            :type="visibleSecrets[key] ? 'text' : 'password'"
            :placeholder="field.example ? String(field.example) : ''"
          />
          <button
            type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            :aria-label="`${visibleSecrets[key] ? 'Hide' : 'Show'} ${field.title || key}`"
            :aria-pressed="!!visibleSecrets[key]"
            @click="visibleSecrets[key] = !visibleSecrets[key]"
          >
            <FontAwesomeIcon
              :icon="['fas', visibleSecrets[key] ? 'eye-slash' : 'eye']"
              class="size-3.5"
            />
          </button>
        </div>

        <!-- Boolean field -->
        <Switch
          v-else-if="field.type === 'bool'"
          :model-value="!!form.credentials[key]"
          @update:model-value="(val) => form.credentials[key] = !!val"
        />

        <!-- Number field -->
        <Input
          v-else-if="field.type === 'number'"
          :id="`channel-field-${key}`"
          v-model.number="form.credentials[key]"
          type="number"
          :placeholder="field.example ? String(field.example) : ''"
        />

        <!-- Enum field -->
        <Select
          v-else-if="field.type === 'enum' && field.enum"
          :model-value="String(form.credentials[key] || '')"
          @update:model-value="(val) => form.credentials[key] = val"
        >
          <SelectTrigger :aria-label="field.title || key">
            <SelectValue :placeholder="field.title" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="opt in field.enum"
              :key="opt"
              :value="opt"
            >
              {{ opt }}
            </SelectItem>
          </SelectContent>
        </Select>

        <!-- String field (default) -->
        <Input
          v-else
          :id="`channel-field-${key}`"
          v-model="form.credentials[key]"
          type="text"
          :placeholder="field.example ? String(field.example) : ''"
        />
      </div>
    </div>

    <Separator />

    <div class="flex justify-end gap-2">
      <template v-if="isEditMode">
        <Button
          :disabled="isBusy"
          @click="handleEditSave"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5"
          />
          {{ $t('common.save') }}
        </Button>
      </template>
      <template v-else>
        <Button
          variant="outline"
          :disabled="isBusy"
          @click="handleCreateSaveOnly"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5"
          />
          {{ $t('bots.channels.saveOnly') }}
        </Button>
        <Button
          :disabled="isBusy"
          @click="handleCreateSaveAndEnable"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5"
          />
          {{ $t('bots.channels.saveAndEnable') }}
        </Button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Label,
  Separator,
  Switch,
  Spinner,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@memoh/ui'
import { reactive, watch, computed, ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation, useQueryCache } from '@pinia/colada'
import { putBotsByIdChannelByPlatform } from '@memoh/sdk'
import { client } from '@memoh/sdk/client'
import type { HandlersChannelMeta, ChannelChannelConfig, ChannelFieldSchema } from '@memoh/sdk'
import ConfirmPopover from '@/components/confirm-popover/index.vue'

interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const props = defineProps<{
  botId: string
  channelItem: BotChannelItem
}>()

const emit = defineEmits<{
  saved: []
}>()

const { t } = useI18n()
const botIdRef = computed(() => props.botId)
const queryCache = useQueryCache()
const { mutateAsync: upsertChannel, isLoading } = useMutation({
  mutation: async ({ platform, data }: { platform: string; data: Record<string, unknown> }) => {
    const { data: result } = await putBotsByIdChannelByPlatform({
      path: { id: botIdRef.value, platform },
      body: data as any,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-channels', botIdRef.value] }),
})
const { mutateAsync: updateChannelStatus, isLoading: isStatusLoading } = useMutation({
  mutation: async ({ platform, disabled }: { platform: string; disabled: boolean }) => {
    const { data } = await client.patch({
      url: `/bots/${botIdRef.value}/channel/${platform}/status`,
      body: { disabled },
      throwOnError: true,
    })
    return data as ChannelChannelConfig
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-channels', botIdRef.value] }),
})
const action = ref<'save' | 'toggle' | 'delete' | ''>('')
const isBusy = computed(() => isLoading.value || isStatusLoading.value || action.value !== '')
const isEditMode = computed(() => props.channelItem.configured)

// ---- Form state ----

const form = reactive<{
  credentials: Record<string, unknown>
  disabled: boolean
}>({
  credentials: {},
  disabled: false,
})

const visibleSecrets = reactive<Record<string, boolean>>({})

// Schema fields sorted: required first. Exclude "status"/"disabled" from credential form.
const orderedFields = computed(() => {
  const fields = props.channelItem.meta.config_schema?.fields ?? {}
  const entries = Object.entries(fields).filter(([key]) => key !== 'status' && key !== 'disabled')
  entries.sort(([, a], [, b]) => {
    if (a.required && !b.required) return -1
    if (!a.required && b.required) return 1
    return 0
  })
  return Object.fromEntries(entries) as Record<string, ChannelFieldSchema>
})

function initForm() {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  const existingCredentials = props.channelItem.config?.credentials ?? {}

  const creds: Record<string, unknown> = {}
  for (const key of Object.keys(schema)) {
    creds[key] = existingCredentials[key] ?? ''
  }
  form.credentials = creds
  form.disabled = props.channelItem.config?.disabled ?? false
}

watch(
  () => props.channelItem,
  () => initForm(),
  { immediate: true },
)

function validateRequired(): boolean {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  for (const [key, field] of Object.entries(schema)) {
    if (field.required) {
      const val = form.credentials[key]
      if (!val || (typeof val === 'string' && val.trim() === '')) {
        toast.error(t('bots.channels.requiredField', { field: field.title || key }))
        return false
      }
    }
  }
  return true
}

function buildCredentials(): Record<string, unknown> {
  const credentials: Record<string, unknown> = {}
  for (const [key, val] of Object.entries(form.credentials)) {
    if (key === 'status' || key === 'disabled') continue
    if (val === '' || val === undefined || val === null) continue
    credentials[key] = val
  }
  return credentials
}

async function saveChannel(disabled: boolean, nextAction: 'save' | 'toggle') {
  if (!validateRequired()) return
  action.value = nextAction
  try {
    await upsertChannel({
      platform: props.channelItem.meta.type,
      data: {
        credentials: buildCredentials(),
        disabled,
      },
    })
    form.disabled = disabled
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    let detail = ''
    if (err instanceof Error) {
      detail = err.message
    }
    toast.error(detail ? `${t('bots.channels.saveFailed')}: ${detail}` : t('bots.channels.saveFailed'))
  } finally {
    action.value = ''
  }
}

async function handleCreateSaveOnly() {
  await saveChannel(true, 'save')
}

async function handleCreateSaveAndEnable() {
  await saveChannel(false, 'save')
}

async function handleEditSave() {
  await saveChannel(form.disabled, 'save')
}

async function handleToggleDisabled() {
  action.value = 'toggle'
  try {
    const nextDisabled = !form.disabled
    const result = await updateChannelStatus({
      platform: props.channelItem.meta.type,
      disabled: nextDisabled,
    })
    form.disabled = !!result?.disabled
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    const detail = err instanceof Error ? err.message : ''
    toast.error(detail ? `${t('bots.channels.saveFailed')}: ${detail}` : t('bots.channels.saveFailed'))
  } finally {
    action.value = ''
  }
}

async function handleDelete() {
  action.value = 'delete'
  try {
    await client.delete({
      url: `/bots/${botIdRef.value}/channel/${props.channelItem.meta.type}`,
      throwOnError: true,
    })
    toast.success(t('bots.channels.deleteSuccess'))
    emit('saved')
  } catch (err) {
    const detail = err instanceof Error ? err.message : ''
    toast.error(detail ? `${t('bots.channels.deleteFailed')}: ${detail}` : t('bots.channels.deleteFailed'))
  } finally {
    action.value = ''
  }
}
</script>
