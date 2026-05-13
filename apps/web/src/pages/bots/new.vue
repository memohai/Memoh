<template>
  <section class="px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12 mx-auto max-w-2xl">
    <form @submit.prevent="handleSubmit">
      <!-- Basic Info -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.basicInfo') }}
        </h3>
        <div class="flex items-start gap-4">
          <div class="group/avatar relative size-16 shrink-0 rounded-full overflow-hidden cursor-pointer">
            <Avatar class="size-16 rounded-full">
              <AvatarImage
                v-if="form.avatar_url?.trim()"
                :src="form.avatar_url.trim()"
                :alt="form.display_name"
              />
              <AvatarFallback class="text-xl">
                {{ avatarFallback }}
              </AvatarFallback>
            </Avatar>
            <button
              type="button"
              class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
              :title="$t('common.edit')"
              :aria-label="$t('common.edit')"
              @click="avatarDialogOpen = true"
            >
              <SquarePen class="size-6 text-white" />
            </button>
          </div>
          <div class="flex-1 min-w-0">
            <Label class="mb-2">
              {{ $t('bots.displayName') }}
              <span class="text-destructive">*</span>
            </Label>
            <Input
              v-model="form.display_name"
              type="text"
              :placeholder="$t('bots.displayNamePlaceholder')"
            />
          </div>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Workspace (conditional) -->
      <template v-if="localWorkspaceEnabled">
        <div>
          <h3 class="text-sm font-medium mb-4">
            {{ $t('bots.steps.workspace') }}
          </h3>
          <div class="flex flex-col gap-4">
            <div>
              <div class="mb-2 flex items-center gap-2">
                <Label>{{ $t('bots.workspaceBackend') }}</Label>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      class="size-5 text-muted-foreground hover:text-foreground"
                    >
                      <CircleHelp class="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-80 text-left leading-relaxed">
                    {{ $t('bots.workspaceBackendHint') }}
                  </TooltipContent>
                </Tooltip>
              </div>
              <Select v-model="form.workspace_backend">
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
            </div>

            <template v-if="form.workspace_backend === 'local'">
              <div>
                <Label class="mb-2">
                  {{ $t('bots.localWorkspacePath') }}
                  <span class="text-destructive">*</span>
                </Label>
                <Input
                  v-model="form.local_workspace_path"
                  type="text"
                  :placeholder="$t('bots.localWorkspacePathPlaceholder')"
                />
              </div>
              <div class="rounded-md border border-warning-border bg-warning-soft px-3 py-2 text-xs text-warning-foreground">
                {{ $t('bots.localWorkspaceWarning') }}
              </div>
            </template>
          </div>
        </div>

        <Separator class="my-6" />
      </template>

      <!-- Security Policy -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.security') }}
        </h3>
        <div class="flex flex-col gap-3">
          <div class="mb-2 flex items-center gap-2">
            <Label>
              {{ $t('bots.aclPreset') }}
              <span class="text-destructive">*</span>
            </Label>
            <Tooltip>
              <TooltipTrigger as-child>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  class="size-5 text-muted-foreground hover:text-foreground"
                >
                  <CircleHelp class="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent class="max-w-80 text-left leading-relaxed">
                {{ $t('bots.aclPresetHelp') }}
              </TooltipContent>
            </Tooltip>
          </div>
          <Select v-model="form.acl_preset">
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
          <p
            v-if="aclDescription"
            class="text-xs text-muted-foreground"
          >
            {{ aclDescription }}
          </p>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Model -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.model') }}
        </h3>
        <p class="text-xs text-muted-foreground mb-3">
          {{ $t('bots.steps.modelDesc') }}
        </p>
        <Label class="mb-2">{{ $t('bots.settings.chatModel') }}</Label>
        <ModelSelect
          v-model="form.chat_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('common.none')"
        />
      </div>

      <Separator class="my-6" />

      <!-- Memory -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.memory') }}
        </h3>
        <p class="text-xs text-muted-foreground mb-3">
          {{ $t('bots.steps.memoryDesc') }}
        </p>
        <Label class="mb-2">{{ $t('bots.settings.memoryProvider') }}</Label>
        <MemoryProviderSelect
          v-model="form.memory_provider_id"
          :providers="memoryProviders"
          :placeholder="$t('common.none')"
        />
      </div>

      <Separator class="my-6" />

      <!-- Settings -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.settings') }}
        </h3>
        <Label class="mb-2">
          {{ $t('bots.timezone') }}
          <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
        </Label>
        <TimezoneSelect
          v-model="form.timezone"
          :placeholder="$t('bots.timezonePlaceholder')"
          allow-empty
          :empty-label="$t('bots.timezoneInherited')"
        />
      </div>

      <!-- Hint -->
      <div class="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground mt-6">
        {{ $t('bots.createBotWaitHint') }}
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 mt-6 pb-4">
        <Button
          type="button"
          variant="outline"
          @click="router.back()"
        >
          {{ $t('common.cancel') }}
        </Button>
        <Button
          type="submit"
          :disabled="!canSubmit || submitLoading"
        >
          <Spinner v-if="submitLoading" />
          {{ $t('bots.createBot') }}
        </Button>
      </div>
    </form>

    <AvatarEditDialog
      v-model:open="avatarDialogOpen"
      v-model:avatar-url="form.avatar_url"
      :fallback-text="avatarFallback"
    />
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Button,
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
import { SquarePen, CircleHelp } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getModels, getProviders, getMemoryProviders, putBotsByBotIdSettings } from '@memohai/sdk'
import { postBotsMutation, getBotsQueryKey } from '@memohai/sdk/colada'
import { useCapabilitiesStore } from '@/store/capabilities'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { aclPresetOptions, defaultAclPreset } from '@/constants/acl-presets'
import { emptyTimezoneValue } from '@/utils/timezones'
import TimezoneSelect from '@/components/timezone-select/index.vue'
import ModelSelect from './components/model-select.vue'
import MemoryProviderSelect from './components/memory-provider-select.vue'
import AvatarEditDialog from './components/avatar-edit-dialog.vue'

const router = useRouter()
const { t } = useI18n()
const queryCache = useQueryCache()
const capabilities = useCapabilitiesStore()

onMounted(() => {
  void capabilities.load()
})

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)

const form = reactive({
  display_name: '',
  avatar_url: '',
  acl_preset: defaultAclPreset as string,
  chat_model_id: '',
  memory_provider_id: '',
  timezone: emptyTimezoneValue,
  workspace_backend: 'container',
  local_workspace_path: '',
})

watch(localWorkspaceEnabled, (enabled) => {
  if (enabled) {
    form.workspace_backend = 'local'
  }
}, { immediate: true })

const localPathTouched = ref(false)

watch([() => form.display_name, () => form.workspace_backend], async ([displayName, backend]) => {
  if (backend !== 'local' || !displayName?.trim()) return
  try {
    const api = (window as Record<string, unknown>).api as
      | { desktop?: { defaultWorkspacePath?: (name: string) => Promise<string> } }
      | undefined
    const path = await api?.desktop?.defaultWorkspacePath?.(displayName.trim())
    if (path && !localPathTouched.value) {
      form.local_workspace_path = path
    }
  } catch {
    // Not in Electron or IPC unavailable
  }
})

watch(() => form.local_workspace_path, () => {
  localPathTouched.value = true
})

const avatarDialogOpen = ref(false)
const avatarFallback = useAvatarInitials(() => form.display_name || '')

// Data queries
const { data: modelData } = useQuery({
  key: ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const { data: memoryProviderData } = useQuery({
  key: ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])
const memoryProviders = computed(() => memoryProviderData.value ?? [])

watch(memoryProviders, (list) => {
  if (form.memory_provider_id) return
  const builtin = list.find(p => p.provider === 'builtin')
  if (builtin?.id) {
    form.memory_provider_id = builtin.id
  }
}, { immediate: true })

// ACL description
const aclDescription = computed(() => {
  const opt = aclPresetOptions.find(o => o.value === form.acl_preset)
  return opt ? t(opt.descriptionKey) : ''
})

// Validation
const canSubmit = computed(() => {
  if (!form.display_name.trim()) return false
  if (!form.acl_preset) return false
  if (localWorkspaceEnabled.value && form.workspace_backend === 'local' && !form.local_workspace_path.trim()) return false
  return true
})

// Submit
const { mutateAsync: createBot, isLoading: submitLoading } = useMutation({
  ...postBotsMutation(),
  onSettled: () => queryCache.invalidateQueries({ key: getBotsQueryKey() }),
})

async function handleSubmit() {
  if (!canSubmit.value || submitLoading.value) return

  const metadata = localWorkspaceEnabled.value && form.workspace_backend === 'local'
    ? {
        workspace: {
          backend: 'local',
          local_workspace_path: form.local_workspace_path,
        },
      }
    : undefined

  const tz = form.timezone === emptyTimezoneValue ? undefined : form.timezone || undefined

  try {
    const bot = await createBot({
      body: {
        display_name: form.display_name.trim(),
        avatar_url: form.avatar_url.trim() || undefined,
        timezone: tz,
        is_active: true,
        acl_preset: form.acl_preset,
        metadata,
      },
    })

    const botId = bot?.id
    if (botId && (form.chat_model_id || form.memory_provider_id)) {
      try {
        await putBotsByBotIdSettings({
          path: { bot_id: botId },
          body: {
            ...(form.chat_model_id ? { chat_model_id: form.chat_model_id } : {}),
            ...(form.memory_provider_id ? { memory_provider_id: form.memory_provider_id } : {}),
          },
          throwOnError: true,
        })
      } catch {
        // Bot created successfully, settings save failed — non-fatal
      }
    }

    toast.success(t('bots.createBotSuccess'))
    if (botId) {
      router.push({ name: 'bot-detail', params: { botId } })
    } else {
      router.push({ name: 'bots' })
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  }
}
</script>
