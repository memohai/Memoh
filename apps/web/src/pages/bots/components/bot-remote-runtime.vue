<template>
  <PageShell
    variant="tab"
    :title="t('bots.remoteRuntime.title')"
    :description="t('bots.remoteRuntime.description')"
  >
    <template #actions>
      <Button
        :loading="saving"
        :disabled="!canSave"
        @click="save"
      >
        {{ t('common.save') }}
      </Button>
    </template>

    <SettingsSection :title="t('bots.remoteRuntime.workspaceTitle')">
      <InlineLoadingRow
        v-if="initialLoading"
        surface="card-row"
      >
        {{ t('bots.remoteRuntime.loading') }}
      </InlineLoadingRow>

      <SettingsRow
        v-else-if="loadFailed"
        :label="t('bots.remoteRuntime.loadFailed')"
        :description="t('bots.remoteRuntime.loadFailedDescription')"
      >
        <Button
          variant="outline"
          size="sm"
          @click="retry"
        >
          {{ t('runtimes.retry') }}
        </Button>
      </SettingsRow>

      <template v-else>
        <SettingsRow
          :label="t('bots.remoteRuntime.source')"
          :description="t('bots.remoteRuntime.sourceDescription')"
        >
          <Select
            v-model="selectedSource"
            :disabled="saving"
          >
            <SelectTrigger class="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectItem :value="nativeSource">
                {{ t('bots.remoteRuntime.nativeWorkspace') }}
              </SelectItem>
              <SelectItem
                v-for="runtime in runtimeItems"
                :key="runtime.id"
                :value="runtime.id!"
              >
                {{ runtime.name }} · {{ runtime.online ? t('runtimes.status.online') : t('runtimes.status.offline') }}
              </SelectItem>
            </SelectContent>
          </Select>
        </SettingsRow>

        <SettingsRow
          v-if="usesRemoteRuntime"
          stack="sm"
          :label="t('bots.remoteRuntime.path')"
          :description="t('bots.remoteRuntime.pathDescription', { path: defaultWorkspacePath })"
        >
          <Input
            v-model="workspacePath"
            class="w-full sm:w-72"
            :placeholder="defaultWorkspacePath"
            :disabled="saving"
            autocomplete="off"
            spellcheck="false"
          />
        </SettingsRow>

        <SettingsRow
          v-if="usesRemoteRuntime"
          :label="t('bots.remoteRuntime.status')"
          :description="selectedRuntime?.name || binding?.runtime_name || t('bots.remoteRuntime.unknownComputer')"
        >
          <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span
              class="size-1.5 rounded-full"
              :class="effectiveStatus === 'online' ? 'bg-success' : effectiveStatus === 'offline' ? 'bg-accent-gray-border' : 'bg-destructive'"
            />
            {{ statusLabel(effectiveStatus) }}
          </span>
        </SettingsRow>

        <SettingsRow
          v-if="runtimeItems.length === 0"
          :label="t('bots.remoteRuntime.noComputers')"
          :description="t('bots.remoteRuntime.noComputersDescription')"
        >
          <Button
            variant="outline"
            size="sm"
            @click="openComputers"
          >
            {{ t('runtimes.connect') }}
          </Button>
        </SettingsRow>
      </template>
    </SettingsSection>
  </PageShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useMutation, useQuery } from '@pinia/colada'
import {
  deleteBotsByBotIdRemoteRuntime,
  getBotsByBotIdRemoteRuntime,
  getUsersMeRuntimes,
  putBotsByBotIdRemoteRuntime,
  type WorkspaceRemoteWorkspaceBinding,
} from '@memohai/sdk'
import {
  Button,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  toast,
} from '@felinic/ui'
import PageShell from '@/components/page-shell/index.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import InlineLoadingRow from '@/components/inline-loading-row/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const nativeSource = '__native_workspace__'
const { t } = useI18n()
const router = useRouter()

const {
  data: runtimes,
  error: runtimesError,
  isLoading: runtimesLoading,
  refetch: refetchRuntimes,
} = useQuery({
  key: () => ['remote-runtimes'],
  query: async () => {
    const { data } = await getUsersMeRuntimes({ throwOnError: true })
    return data
  },
  refetchOnWindowFocus: true,
})

const runtimeItems = computed(() => runtimes.value ?? [])
const binding = ref<WorkspaceRemoteWorkspaceBinding | null>(null)
const bindingLoading = ref(false)
const bindingLoadFailed = ref(false)
const selectedSource = ref(nativeSource)
const workspacePath = ref('')
let bindingRequestVersion = 0

const { mutateAsync: bindRuntime, isLoading: bindingSaving } = useMutation({
  mutation: async (input: { runtimeID: string, path: string }) => {
    const path = input.path.trim()
    const { data } = await putBotsByBotIdRemoteRuntime({
      path: { bot_id: props.botId },
      body: {
        runtime_id: input.runtimeID,
        ...(path ? { workspace_path: path } : {}),
      },
      throwOnError: true,
    })
    return data
  },
})

const { mutateAsync: unbindRuntime, isLoading: unbinding } = useMutation({
  mutation: async () => {
    await deleteBotsByBotIdRemoteRuntime({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
  },
})

const saving = computed(() => bindingSaving.value || unbinding.value)
const initialLoading = computed(() =>
  bindingLoading.value || (runtimesLoading.value && runtimes.value === undefined),
)
const loadFailed = computed(() => bindingLoadFailed.value || (runtimesError.value && runtimes.value === undefined))
const usesRemoteRuntime = computed(() => selectedSource.value !== nativeSource)
const selectedRuntime = computed(() =>
  runtimeItems.value.find(runtime => runtime.id === selectedSource.value),
)
const defaultWorkspacePath = computed(() => `bots/${props.botId || '<bot-id>'}`)
const persistedSource = computed(() => binding.value?.runtime_id || nativeSource)
const persistedPath = computed(() => binding.value?.workspace_path || defaultWorkspacePath.value)
const effectivePath = computed(() => workspacePath.value.trim() || defaultWorkspacePath.value)
const hasChanges = computed(() => {
  if (selectedSource.value !== persistedSource.value) return true
  return usesRemoteRuntime.value && effectivePath.value !== persistedPath.value
})
const canSave = computed(() =>
  !initialLoading.value
  && !loadFailed.value
  && !saving.value
  && hasChanges.value
  && (selectedSource.value === nativeSource || !!selectedRuntime.value),
)
const effectiveStatus = computed(() => {
  if (!usesRemoteRuntime.value) return 'offline'
  if (binding.value?.runtime_id === selectedSource.value) {
    const status = binding.value.status || 'offline'
    if (status === 'owner_mismatch' || status === 'revoked' || status === 'client_update_required') return status
  }
  return selectedRuntime.value?.online ? 'online' : 'offline'
})

async function loadBinding(): Promise<void> {
  const botID = props.botId
  const version = ++bindingRequestVersion
  bindingLoadFailed.value = false
  if (!botID) {
    bindingLoading.value = false
    return
  }
  bindingLoading.value = true
  try {
    const result = await getBotsByBotIdRemoteRuntime({ path: { bot_id: botID } })
    if (version !== bindingRequestVersion) return
    if (result.error !== undefined) {
      if (result.response?.status === 404) {
        applyBinding(null)
        return
      }
      throw result.error
    }
    applyBinding(result.data ?? null)
  } catch (error) {
    if (version !== bindingRequestVersion) return
    bindingLoadFailed.value = true
    toast.error(resolveApiErrorMessage(error, t('bots.remoteRuntime.loadFailed')))
  } finally {
    if (version === bindingRequestVersion) bindingLoading.value = false
  }
}

function applyBinding(value: WorkspaceRemoteWorkspaceBinding | null): void {
  binding.value = value
  selectedSource.value = value?.runtime_id || nativeSource
  workspacePath.value = value?.workspace_path || ''
}

async function save(): Promise<void> {
  if (!canSave.value) return
  try {
    if (selectedSource.value === nativeSource) {
      await unbindRuntime()
      applyBinding(null)
      toast.success(t('bots.remoteRuntime.nativeWorkspaceSaved'))
      return
    }
    const result = await bindRuntime({
      runtimeID: selectedSource.value,
      path: workspacePath.value,
    })
    applyBinding(result)
    toast.success(t('bots.remoteRuntime.bindSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.remoteRuntime.saveFailed')))
  }
}

async function retry(): Promise<void> {
  await Promise.all([refetchRuntimes(), loadBinding()])
}

function openComputers(): void {
  void router.push({ name: 'runtimes' })
}

function statusLabel(status: string): string {
  const known = new Set(['online', 'offline', 'revoked', 'owner_mismatch', 'client_update_required'])
  return t(`runtimes.status.${known.has(status) ? status : 'offline'}`)
}

watch(() => props.botId, () => {
  applyBinding(null)
  void loadBinding()
}, { immediate: true })
</script>
