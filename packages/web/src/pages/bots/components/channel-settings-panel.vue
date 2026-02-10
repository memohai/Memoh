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
      <Badge :variant="channelItem.configured ? 'default' : 'secondary'">
        {{ channelItem.configured ? $t('bots.channels.configured') : $t('bots.channels.notConfigured') }}
      </Badge>
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
        <Label>
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
            v-model="form.credentials[key]"
            :type="visibleSecrets[key] ? 'text' : 'password'"
            :placeholder="field.example ? String(field.example) : ''"
          />
          <button
            type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
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
          <SelectTrigger>
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
          v-model="form.credentials[key]"
          type="text"
          :placeholder="field.example ? String(field.example) : ''"
        />
      </div>
    </div>

    <Separator />

    <!-- Status -->
    <div class="flex items-center justify-between">
      <Label>{{ $t('bots.channels.status') }}</Label>
      <Switch
        :model-value="form.status === 'active'"
        @update:model-value="(val) => form.status = val ? 'active' : 'inactive'"
      />
    </div>

    <!-- Save -->
    <div class="flex justify-end">
      <Button
        :disabled="isLoading"
        @click="handleSave"
      >
        <Spinner v-if="isLoading" />
        {{ $t('bots.channels.save') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Badge,
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
import { reactive, watch, computed } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import {
  useUpsertBotChannel,
  type BotChannelItem,
  type FieldSchema,
} from '@/composables/api/useChannels'
import { ApiError } from '@/utils/request'
import type { Ref } from 'vue'

const props = defineProps<{
  botId: string
  channelItem: BotChannelItem
}>()

const emit = defineEmits<{
  saved: []
}>()

const { t } = useI18n()
const botIdRef = computed(() => props.botId) as Ref<string>
const { mutateAsync: upsertChannel, isLoading } = useUpsertBotChannel(botIdRef)

// ---- Form state ----

const form = reactive<{
  credentials: Record<string, unknown>
  status: string
}>({
  credentials: {},
  status: 'active',
})

const visibleSecrets = reactive<Record<string, boolean>>({})

// Schema fields sorted: required first
const orderedFields = computed(() => {
  const fields = props.channelItem.meta.config_schema?.fields ?? {}
  const entries = Object.entries(fields)
  entries.sort(([, a], [, b]) => {
    if (a.required && !b.required) return -1
    if (!a.required && b.required) return 1
    return 0
  })
  return Object.fromEntries(entries) as Record<string, FieldSchema>
})

// 初始化表单
function initForm() {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  const existingCredentials = props.channelItem.config?.credentials ?? {}

  const creds: Record<string, unknown> = {}
  for (const key of Object.keys(schema)) {
    creds[key] = existingCredentials[key] ?? ''
  }
  form.credentials = creds
  form.status = props.channelItem.config?.status ?? 'active'
}

watch(
  () => props.channelItem,
  () => initForm(),
  { immediate: true },
)

// 客户端校验必填字段
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

async function handleSave() {
  if (!validateRequired()) return

  try {
    const credentials: Record<string, unknown> = {}
    for (const [key, val] of Object.entries(form.credentials)) {
      if (val !== '' && val !== undefined && val !== null) {
        credentials[key] = val
      }
    }

    await upsertChannel({
      platform: props.channelItem.meta.type,
      data: {
        credentials,
        status: form.status,
      },
    })
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    let detail = ''
    if (err instanceof ApiError && err.body) {
      const body = err.body as Record<string, unknown>
      detail = String(body.message || body.error || '')
    } else if (err instanceof Error) {
      detail = err.message
    }
    toast.error(detail ? `${t('bots.channels.saveFailed')}: ${detail}` : t('bots.channels.saveFailed'))
  }
}
</script>
