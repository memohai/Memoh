<template>
  <PageShell
    variant="tab"
    :title="$t('bots.tabs.acp')"
    :description="$t('bots.settings.blocks.acpDescription')"
  >
    <SettingsAcpCard
      :bot-id="botId"
      :profiles="profiles"
      :form="form"
      :loading="profilesLoading"
      @commit="persistACPForm"
    />
  </PageShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getAcpProfiles, getBotsById, putBotsById } from '@memohai/sdk'
import type { AcpprofilePublicProfile, BotsUpdateBotRequest } from '@memohai/sdk'
import type { Ref } from 'vue'
import SettingsAcpCard from './settings-acp-card.vue'
import PageShell from '@/components/page-shell/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
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
const lastPersistedSnapshot = ref('')
const persistRunning = ref(false)
const persistQueued = ref(false)

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

const { mutateAsync: updateBot } = useMutation({
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

async function persistACPForm() {
  if (!bot.value) return
  if (persistRunning.value) {
    persistQueued.value = true
    return
  }
  const normalized = normalizeACPForm(form, profiles.value)
  const snapshot = JSON.stringify(normalized)
  if (snapshot === lastPersistedSnapshot.value) return
  const validationError = validateForm(normalized, profiles.value)
  if (validationError) {
    toast.error(validationError)
    return
  }
  persistRunning.value = true
  try {
    await updateBot({
      metadata: withACPMetadata(
        bot.value?.metadata as Record<string, unknown> | undefined,
        normalized,
        profiles.value,
      ),
    })
    lastPersistedSnapshot.value = snapshot
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  } finally {
    persistRunning.value = false
    if (persistQueued.value) {
      persistQueued.value = false
      void persistACPForm()
    }
  }
}

function applyMetadataToForm(metadata: Record<string, unknown> | undefined, list: AcpprofilePublicProfile[]) {
  const next = readACPConfig(metadata, list)
  const nextSnapshot = JSON.stringify(next)
  const currentSnapshot = JSON.stringify(normalizeACPForm(form, list))
  if ((persistRunning.value || persistQueued.value || currentSnapshot !== lastPersistedSnapshot.value) && nextSnapshot === lastPersistedSnapshot.value) {
    return
  }
  for (const key of Object.keys(form.agents)) {
    if (!next.agents[key]) delete form.agents[key]
  }
  for (const profile of list) {
    const id = normalizeACPAgentID(profile.id)
    if (!id) continue
    form.agents[id] = next.agents[id] ?? emptyACPAgentForm(profile)
  }
  lastPersistedSnapshot.value = nextSnapshot
}

function validateForm(value: ACPForm, list: AcpprofilePublicProfile[]): string {
  const missing = findMissingRequiredACPField(value, list)
  if (!missing) return ''
  return t('bots.settings.acpRequiredField', {
    agent: missing.profile.display_name || missing.profile.id,
    field: missing.field.label || missing.field.id,
  })
}
</script>
