<template>
  <div class="mx-auto max-w-2xl space-y-5 pb-6">
    <div class="flex items-center justify-between border-b border-border/50 pb-4">
      <div class="space-y-1">
        <h3 class="text-sm font-semibold text-foreground">
          {{ $t('bots.tabs.acp') }}
        </h3>
        <p class="text-[11px] text-muted-foreground">
          {{ $t('bots.settings.blocks.acpDescription') }}
        </p>
      </div>

      <div class="flex shrink-0 items-center gap-3">
        <Transition name="fade">
          <div
            v-if="hasChanges"
            class="flex items-center gap-1.5 rounded-full border border-border/50 bg-muted/40 px-2 py-0.5"
          >
            <div class="size-1 rounded-full bg-muted-foreground/40" />
            <span class="whitespace-nowrap text-[10px] font-medium text-muted-foreground">
              {{ $t('common.unsaved') }}
            </span>
          </div>
        </Transition>

        <Button
          size="sm"
          :disabled="!hasChanges || saveLoading"
          class="h-8 min-w-24 text-xs font-medium shadow-none"
          @click="handleSave"
        >
          <Spinner
            v-if="saveLoading"
            class="mr-1.5 size-3"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </div>

    <SettingsAcpCard
      :bot-id="botId"
      :profiles="profiles"
      :form="form"
      :loading="profilesLoading"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { Button, Spinner } from '@memohai/ui'
import { getAcpProfiles, getBotsById, putBotsById } from '@memohai/sdk'
import type { AcpprofilePublicProfile, BotsUpdateBotRequest } from '@memohai/sdk'
import type { Ref } from 'vue'
import SettingsAcpCard from './settings-acp-card.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { isLocalWorkspaceBot } from '@/utils/bot-workspace'
import {
  emptyACPAgentForm,
  findMissingRequiredACPField,
  normalizeACPAgentID,
  normalizeACPForm,
  readACPConfig,
  withACPMetadata,
  type ACPForm,
} from '@/utils/acp'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const botIdRef = computed(() => props.botId) as Ref<string>

const form = reactive<ACPForm>({
  agents: {},
})

const { data: profileData, isLoading: profilesLoading } = useQuery({
  key: () => ['acp-profiles'],
  query: async () => {
    const { data } = await getAcpProfiles({ throwOnError: true })
    return data
  },
})

const profiles = computed<AcpprofilePublicProfile[]>(() => profileData.value?.items ?? [])

const { data: bot } = useQuery({
  key: () => ['bot', botIdRef.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botIdRef.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botIdRef.value,
})

const { mutateAsync: updateBot, isLoading: saveLoading } = useMutation({
  mutation: async (body: BotsUpdateBotRequest) => {
    const { data } = await putBotsById({
      path: { id: botIdRef.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot', botIdRef.value] })
    queryCache.invalidateQueries({ key: ['bots'] })
  },
})

watch([bot, profiles], ([value, list]) => {
  applyMetadataToForm(value?.metadata as Record<string, unknown> | undefined, list)
}, { immediate: true })

const hasChanges = computed(() => {
  if (!bot.value) return false
  return JSON.stringify(normalizeACPForm(form, profiles.value)) !== JSON.stringify(readACPConfig(bot.value.metadata as Record<string, unknown> | undefined, profiles.value))
})

async function handleSave() {
  try {
    const normalized = normalizeACPForm(form, profiles.value)
    const validationError = validateForm(normalized, profiles.value, isLocalWorkspaceBot(bot.value?.metadata))
    if (validationError) {
      toast.error(validationError)
      return
    }
    await updateBot({
      metadata: withACPMetadata(
        bot.value?.metadata as Record<string, unknown> | undefined,
        normalized,
        profiles.value,
      ),
    })
    toast.success(t('bots.settings.saveSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  }
}

function applyMetadataToForm(metadata: Record<string, unknown> | undefined, list: AcpprofilePublicProfile[]) {
  const next = readACPConfig(metadata, list)
  for (const key of Object.keys(form.agents)) {
    if (!next.agents[key]) delete form.agents[key]
  }
  for (const profile of list) {
    const id = normalizeACPAgentID(profile.id)
    if (!id) continue
    form.agents[id] = next.agents[id] ?? emptyACPAgentForm(profile)
  }
}

function validateForm(value: ACPForm, list: AcpprofilePublicProfile[], isLocalWorkspace = false): string {
  const missing = findMissingRequiredACPField(value, list, isLocalWorkspace)
  if (!missing) return ''
  return t('bots.settings.acpRequiredField', {
    agent: missing.profile.display_name || missing.profile.id,
    field: missing.field.label || missing.field.id,
  })
}
</script>
