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
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@memohai/ui'
import { SquarePen, CircleHelp, Bot } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getMemoryProviders, putBotsByBotIdSettings } from '@memohai/sdk'
import { postBotsMutation, getBotsQueryKey } from '@memohai/sdk/colada'
import { useOnboarding } from '@/composables/useOnboarding'
import { useCapabilitiesStore } from '@/store/capabilities'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { defaultAclPreset } from '@/constants/acl-presets'
import AvatarEditDialog from '@/pages/bots/components/avatar-edit-dialog.vue'

const { t } = useI18n()
const { nextStep, prevStep } = useOnboarding()
const queryCache = useQueryCache()
const capabilities = useCapabilitiesStore()

const visible = ref(false)
const exiting = ref(false)
const workspaceVisible = ref(false)
const submitting = ref(false)

onMounted(() => {
  void capabilities.load()
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      visible.value = true
    })
  })
})

function go(action: () => void) {
  exiting.value = true
  setTimeout(action, 175)
}

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)

const form = reactive({
  display_name: '',
  avatar_url: '',
  memory_provider_id: '',
  workspace_backend: 'container',
})

watch(localWorkspaceEnabled, (enabled) => {
  if (!enabled) {
    workspaceVisible.value = false
    return
  }
  form.workspace_backend = 'local'
  workspaceVisible.value = false
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      workspaceVisible.value = true
    })
  })
}, { immediate: true })

const avatarDialogOpen = ref(false)
const avatarFallback = useAvatarInitials(() => form.display_name || '')

const { data: memoryProviderData } = useQuery({
  key: ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const memoryProviders = computed(() => memoryProviderData.value ?? [])

watch(memoryProviders, (list) => {
  if (form.memory_provider_id) return
  const builtin = list.find(p => p.provider === 'builtin')
  if (builtin?.id) {
    form.memory_provider_id = builtin.id
  }
}, { immediate: true })

const canSubmit = computed(() => {
  return !!form.display_name.trim()
})

const { mutateAsync: createBot } = useMutation({
  ...postBotsMutation(),
  onSettled: () => queryCache.invalidateQueries({ key: getBotsQueryKey() }),
})

async function handleSubmit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true

  const metadata = localWorkspaceEnabled.value && form.workspace_backend === 'local'
    ? {
        workspace: {
          backend: 'local' as const,
        },
      }
    : undefined

  const tz = undefined

  try {
    const bot = await createBot({
      body: {
        display_name: form.display_name.trim(),
        avatar_url: form.avatar_url.trim() || undefined,
        timezone: tz,
        is_active: true,
        acl_preset: defaultAclPreset,
        metadata,
        wait_for_ready: true,
      },
    })

    const botId = bot?.id
    if (botId) {
      sessionStorage.setItem('memoh:onboarding-created-bot-id', botId)
    }
    if (botId && form.memory_provider_id) {
      try {
        await putBotsByBotIdSettings({
          path: { bot_id: botId },
          body: { memory_provider_id: form.memory_provider_id },
          throwOnError: true,
        })
      } catch {
        // Bot created successfully, settings save failed — non-fatal
      }
    }

    go(nextStep)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <TooltipProvider :delay-duration="0">
    <div
      class="transition-all duration-[175ms] ease-out"
      :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
    >
      <div class="text-left min-h-[542px] max-h-[calc(100vh-7rem)] flex flex-col pt-32">
        <h2
          class="text-3xl font-semibold mb-6 transition-all duration-[350ms] ease-out"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          {{ t('onboarding.bot.title') }}
        </h2>

        <div class="min-h-0 flex-1 overflow-y-auto -mx-2 px-2 -my-1 py-1">
          <form
            @submit.prevent="handleSubmit"
          >
            <div
              class="transition-all duration-[350ms] ease-out delay-[60ms]"
              :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
            >
              <div class="flex items-center gap-4">
                <div class="group/avatar relative size-16 shrink-0 rounded-full overflow-hidden cursor-pointer border border-border">
                  <Avatar class="size-16 rounded-full">
                    <AvatarImage
                      v-if="form.avatar_url?.trim()"
                      :src="form.avatar_url.trim()"
                      :alt="form.display_name"
                    />
                    <AvatarFallback class="text-xl text-muted-foreground">
                      <Bot
                        v-if="!form.display_name.trim()"
                        class="size-7"
                      />
                      <template v-else>
                        {{ avatarFallback }}
                      </template>
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
                    <span
                      v-if="!form.display_name.trim()"
                      class="text-destructive"
                    >*</span>
                  </Label>
                  <Input
                    v-model="form.display_name"
                    type="text"
                    :placeholder="$t('bots.displayNamePlaceholder')"
                  />
                </div>
              </div>
            </div>

            <div
              class="transition-all duration-[350ms] ease-out delay-[100ms]"
              :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
            >
              <Separator class="my-6" />
            </div>

            <template v-if="localWorkspaceEnabled">
              <div
                class="transition-all duration-[350ms] ease-out delay-[140ms]"
                :class="workspaceVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
              >
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

                  <div
                    v-if="form.workspace_backend === 'local'"
                    class="rounded-md border border-warning-border bg-warning-soft px-3 py-2 text-xs text-warning-foreground"
                  >
                    {{ $t('bots.localWorkspaceWarning') }}
                  </div>
                </div>
              </div>
            </template>

            <div
              v-if="form.workspace_backend !== 'local'"
              class="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground mt-6 transition-all duration-[350ms] ease-out delay-[200ms]"
              :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
            >
              {{ $t('bots.createBotWaitHint') }}
            </div>
          </form>
        </div>

        <div
          class="mt-auto pt-12 flex items-center justify-end gap-3 transition-all duration-[350ms] ease-out delay-[220ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          <button
            class="inline-flex h-[42px] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed"
            @click="go(prevStep)"
          >
            {{ t('onboarding.prev') }}
          </button>
          <button
            class="inline-flex h-[42px] min-w-[180px] items-center justify-center gap-2 rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-60 disabled:cursor-not-allowed"
            :disabled="!canSubmit || submitting"
            @click="handleSubmit"
          >
            {{ t('onboarding.next') }}
          </button>
        </div>

        <AvatarEditDialog
          v-model:open="avatarDialogOpen"
          v-model:avatar-url="form.avatar_url"
          :fallback-text="avatarFallback"
        />
      </div>
    </div>
  </TooltipProvider>
</template>
