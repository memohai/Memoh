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
  TooltipProvider,
  TooltipTrigger,
} from '@memohai/ui'
import { SquarePen, CircleHelp, Bot } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getModels, getProviders, getMemoryProviders } from '@memohai/sdk'
import type { BotsCreateBotRequest } from '@memohai/sdk'
import { getBotsQueryKey } from '@memohai/sdk/colada'
import { storeToRefs } from 'pinia'
import { useOnboarding } from '@/composables/useOnboarding'
import { useCapabilitiesStore } from '@/store/capabilities'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { defaultAclPreset } from '@/constants/acl-presets'
import { useBotCreateProgressStore } from '@/store/bot-create-progress'
import AvatarEditDialog from '@/pages/bots/components/avatar-edit-dialog.vue'
import BotCreateTerminal from '@/pages/bots/components/bot-create-terminal.vue'
import ModelSelect from '@/pages/bots/components/model-select.vue'
import { useStepTransition, nextFrame } from '../useStepTransition'
import { ONBOARDING_KEYS } from '../constants'

const { t } = useI18n()
const { nextStep, prevStep } = useOnboarding()
const queryCache = useQueryCache()
const capabilities = useCapabilitiesStore()
const { visible, exiting, leave } = useStepTransition()

const workspaceVisible = ref(false)
const submitting = ref(false)

const store = useBotCreateProgressStore()
const { lines: terminalLines, status: createStatus } = storeToRefs(store)

onMounted(() => {
  void capabilities.load()
})

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)

const form = reactive({
  display_name: '',
  avatar_url: '',
  chat_model_id: '',
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
  nextFrame(() => {
    workspaceVisible.value = true
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

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])

const canSubmit = computed(() => {
  return !!form.display_name.trim()
})

const isContainerSubmitting = computed(() => submitting.value && form.workspace_backend !== 'local')

const ctaLabel = computed(() => {
  if (isContainerSubmitting.value) return t('onboarding.bot.preparingEnvironment')
  return t('onboarding.next')
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

  const payload: BotsCreateBotRequest = {
    display_name: form.display_name.trim(),
    avatar_url: form.avatar_url.trim() || undefined,
    timezone: undefined,
    is_active: true,
    acl_preset: defaultAclPreset,
    metadata,
    wait_for_ready: true,
  }

  // The store drives the inline terminal reactively while we await completion.
  await store.start(payload, {
    display: {
      display_name: form.display_name.trim(),
      avatar_url: form.avatar_url.trim() || undefined,
    },
    settings: {
      chat_model_id: form.chat_model_id || undefined,
      memory_provider_id: form.memory_provider_id || undefined,
    },
  })
  submitting.value = false

  if (store.status === 'error') {
    toast.error(store.setupError ?? t('common.saveFailed'))
    store.reset()
    return
  }

  const botId = store.bot?.id
  if (botId) {
    sessionStorage.setItem(ONBOARDING_KEYS.createdBotId, botId)
  }
  if (store.setupError) {
    toast.error(store.setupError)
  }
  void queryCache.invalidateQueries({ key: getBotsQueryKey() })
  leave(nextStep)
  store.reset()
}
</script>

<template>
  <TooltipProvider :delay-duration="0">
    <div
      class="transition-all duration-[175ms] ease-out"
      :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
    >
      <div class="text-left pt-16 h-[560px] max-h-[calc(100vh-7rem)] flex flex-col">
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

            <div
              class="transition-all duration-[350ms] ease-out delay-[120ms]"
              :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
            >
              <div class="mb-2 flex items-center gap-2">
                <Label>{{ $t('bots.settings.chatModel') }}</Label>
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
                    {{ $t('onboarding.bot.model.hint') }}
                  </TooltipContent>
                </Tooltip>
              </div>
              <ModelSelect
                v-model="form.chat_model_id"
                :models="models"
                :providers="providers"
                model-type="chat"
                :placeholder="$t('onboarding.bot.model.selectPlaceholder')"
              />
            </div>

            <template v-if="localWorkspaceEnabled">
              <div
                class="transition-all duration-[350ms] ease-out delay-[140ms] mt-6"
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
            <div
              v-if="form.workspace_backend !== 'local' && (createStatus === 'creating' || createStatus === 'error') && terminalLines.length"
              class="mt-3 transition-all duration-[350ms] ease-out delay-[220ms]"
              :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
            >
              <BotCreateTerminal :lines="terminalLines" />
            </div>
          </form>
        </div>

        <div
          class="mt-auto pt-12 flex items-center justify-end gap-3 transition-all duration-[350ms] ease-out delay-[220ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          <button
            class="inline-flex h-[42px] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed"
            @click="leave(prevStep)"
          >
            {{ t('onboarding.prev') }}
          </button>
          <button
            class="inline-flex h-[42px] min-w-[180px] items-center justify-center gap-2 rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-60 disabled:cursor-not-allowed"
            :disabled="!canSubmit || submitting"
            @click="handleSubmit"
          >
            <Transition
              mode="out-in"
              enter-active-class="transition-all duration-[160ms] ease-out"
              enter-from-class="opacity-0 translate-y-1"
              enter-to-class="opacity-100 translate-y-0"
              leave-active-class="transition-all duration-[140ms] ease-in"
              leave-from-class="opacity-100 translate-y-0"
              leave-to-class="opacity-0 -translate-y-1"
            >
              <span
                :key="ctaLabel"
                class="inline-flex items-center gap-2"
              >
                <Spinner v-if="isContainerSubmitting" />
                {{ ctaLabel }}
              </span>
            </Transition>
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
