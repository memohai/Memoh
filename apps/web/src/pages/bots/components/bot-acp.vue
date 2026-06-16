<template>
  <SwapTransition :direction="direction">
    <!-- List: one agent per row. The card navigates into its setup; the Switch
         is the only enable affordance and stays on this list, never on the
         setup page (so enabling never unfurls a long inline form). -->
    <PageShell
      v-if="view === 'list'"
      variant="tab"
      :title="$t('bots.tabs.acp')"
      :description="$t('bots.settings.blocks.acpDescription')"
    >
      <div
        v-if="profilesLoading && profiles.length === 0"
        class="space-y-3"
      >
        <Skeleton
          v-for="n in 2"
          :key="n"
          class="h-[4.5rem] w-full rounded-[var(--radius-menu-shell)]"
        />
      </div>

      <Empty
        v-else-if="profiles.length === 0"
        class="rounded-[var(--radius-menu-shell)] border border-dashed border-border py-16"
      >
        <EmptyTitle>{{ $t('bots.settings.acpEmptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('bots.settings.acpEmptyDescription') }}</EmptyDescription>
      </Empty>

      <div
        v-else
        class="space-y-3"
      >
        <div
          v-for="profile in profiles"
          :key="profile.id"
          class="relative flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card p-3.5 transition-colors hover:bg-accent/30 dark:hover:bg-accent"
        >
          <!-- Stretched navigate target: fills the card so the whole row opens
               setup, while the Switch above keeps its own click. -->
          <button
            type="button"
            class="absolute inset-0 rounded-[var(--radius-menu-shell)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            :aria-label="profile.display_name || profile.id"
            @click="openAgent(profile)"
          />

          <span class="pointer-events-none relative flex size-10 shrink-0 items-center justify-center rounded-full bg-muted">
            <component
              :is="acpAgentIcon(profile.id, true)"
              class="size-5"
            />
            <span
              v-if="agentForm(profile).enabled"
              class="absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full bg-success ring-2 ring-card"
            />
          </span>

          <span class="pointer-events-none relative min-w-0 flex-1">
            <span class="block truncate text-sm font-medium text-foreground">
              {{ profile.display_name || profile.id }}
            </span>
            <span
              v-if="profile.description"
              class="mt-0.5 block truncate text-xs text-muted-foreground"
            >
              {{ profile.description }}
            </span>
          </span>

          <div class="relative flex shrink-0 items-center gap-3">
            <ChevronRight class="size-4 text-muted-foreground/60" />
            <Switch
              :model-value="agentForm(profile).enabled"
              :aria-label="profile.display_name || profile.id"
              @update:model-value="(val) => setAgentEnabled(profile, !!val)"
            />
          </div>
        </div>
      </div>
    </PageShell>

    <!-- Setup: configuration for the selected agent only. -->
    <section
      v-else
      class="mx-auto max-w-3xl pt-4 pb-8"
    >
      <Button
        variant="ghost"
        class="mb-2 text-foreground/85"
        @click="backToList()"
      >
        <ChevronLeft class="size-4" />
        {{ $t('bots.tabs.acp') }}
      </Button>

      <SettingsAcpDetail
        v-if="selectedProfile"
        :key="selectedProfile.id"
        :bot-id="botId"
        :profile="selectedProfile"
        :form="form"
        @commit="persistACPForm"
      />
    </section>
  </SwapTransition>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Button, Empty, EmptyDescription, EmptyTitle, Skeleton, Switch, toast } from '@memohai/ui'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getAcpProfiles, getBotsById, putBotsById } from '@memohai/sdk'
import type { AcpprofilePublicProfile, BotsUpdateBotRequest } from '@memohai/sdk'
import type { Ref } from 'vue'
import SettingsAcpDetail from './settings-acp-detail.vue'
import PageShell from '@/components/page-shell/index.vue'
import SwapTransition from '@/components/settings/swap-transition.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  acpAgentIcon,
  emptyACPAgentForm,
  ensureACPAgentForm,
  findMissingRequiredACPField,
  normalizeACPAgentID,
  normalizeACPForm,
  readACPConfig,
  withACPMetadata,
  type ACPAgentForm,
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

const { view, direction, openDetail, backToList } = useViewSwap()
const selectedId = ref('')

const { data: profileData, isLoading: profilesLoading } = useQuery({
  key: () => ['acp-profiles'],
  query: async () => {
    const { data } = await getAcpProfiles({ throwOnError: true })
    return data
  },
})

const profiles = computed<AcpprofilePublicProfile[]>(() => profileData.value?.items ?? [])

const selectedProfile = computed(() =>
  profiles.value.find(p => normalizeACPAgentID(p.id) === selectedId.value) ?? null,
)

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

// If the open agent vanishes after a profile refetch, fall back to the list.
watch(profiles, (list) => {
  if (view.value === 'detail' && selectedId.value && !list.some(p => normalizeACPAgentID(p.id) === selectedId.value)) {
    backToList()
  }
})

function agentForm(profile: AcpprofilePublicProfile): ACPAgentForm {
  return ensureACPAgentForm(form, profile)
}

function openAgent(profile: AcpprofilePublicProfile) {
  selectedId.value = normalizeACPAgentID(profile.id)
  openDetail()
}

function setAgentEnabled(profile: AcpprofilePublicProfile, enabled: boolean) {
  agentForm(profile).enabled = enabled
  void persistACPForm()
}

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
