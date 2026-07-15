<!-- eslint-disable vue/no-mutating-props -->
<template>
  <!-- id is a scroll anchor: the Overview "choose a model" reminder navigates
       here with ?section=interaction, and bot-settings.vue scrolls to it. -->
  <SettingsSection
    id="settings-section-interaction"
    :title="$t('bots.settings.blocks.interaction')"
  >
    <SettingsRow :label="$t('bots.settings.chatModel')">
      <div class="w-56">
        <ModelSelect
          v-model="form.chat_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('bots.settings.chatModel')"
        />
      </div>
    </SettingsRow>

    <SettingsRow
      :label="$t('bots.settings.externalAgentRuntime')"
      :description="externalAgentRuntimeDescription"
    >
      <Switch
        :model-value="externalAgentEnabled"
        :disabled="!externalAgentEnabled && selectableACPProfiles.length === 0"
        @update:model-value="(val) => setExternalAgentRuntime(!!val)"
      />
    </SettingsRow>

    <SettingsRow
      v-if="externalAgentEnabled"
      :label="$t('bots.settings.externalAgent')"
      :description="externalAgentDescription"
    >
      <Select
        :model-value="normalizedDefaultAgentID"
        :disabled="selectableACPProfiles.length === 0"
        @update:model-value="(value) => setDefaultACPAgent(String(value))"
      >
        <SelectTrigger class="w-56">
          <SelectValue :placeholder="$t('bots.settings.externalAgentPlaceholder')">
            <div
              v-if="selectedACPProfile"
              class="flex min-w-0 items-center gap-2"
            >
              <component :is="acpAgentIcon(selectedACPProfile.id, true)" />
              <span class="truncate text-xs">{{ selectedACPProfile.display_name || selectedACPProfile.id }}</span>
            </div>
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            v-for="profile in selectableACPProfiles"
            :key="profile.id"
            :value="normalizeACPAgentID(profile.id)"
          >
            <div class="flex min-w-0 items-center gap-2">
              <component :is="acpAgentIcon(profile.id, true)" />
              <span class="truncate text-xs">{{ profile.display_name || profile.id }}</span>
            </div>
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingsRow>

    <SettingsRow
      :label="$t('bots.settings.titleModel')"
      :description="$t('bots.settings.titleModelDescription')"
    >
      <div class="w-56">
        <ModelSelect
          v-model="form.title_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('bots.settings.titleModelPlaceholder')"
        />
      </div>
    </SettingsRow>

    <SettingsRow :label="$t('bots.settings.reasoningEffort')">
      <Select
        v-model="reasoningFormValue"
        :disabled="!chatModelSupportsReasoning"
      >
        <SelectTrigger class="w-44">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            v-for="effort in availableReasoningEfforts"
            :key="effort"
            :value="effort"
          >
            {{ $t(EFFORT_LABELS[effort] ?? effort) }}
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingsRow>

    <SettingsRow
      :label="$t('bots.settings.showToolCallsInIM')"
      :description="$t('bots.settings.showToolCallsInIMDescription')"
    >
      <Switch
        :model-value="form.show_tool_calls_in_im"
        @update:model-value="(val) => form.show_tool_calls_in_im = !!val"
      />
    </SettingsRow>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Switch } from '@felinic/ui'
import { useI18n } from 'vue-i18n'
import ModelSelect from './model-select.vue'
import { EFFORT_LABELS, REASONING_EFFORT_DISABLE, availableEffortsForMode, resolveEffortLevels, resolveThinkingMode } from './reasoning-effort'
import type { AcpprofilePublicProfile, SettingsSettings, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import { ACP_DEFAULT_PROJECT_MODE, ACP_DEFAULT_PROJECT_PATH, acpAgentIcon, findMissingRequiredManagedField, isACPAgentEnabled, normalizeACPAgentID, readACPAgentConfig } from '@/utils/acp'

type InteractionSettingsForm = SettingsSettings & {
  chat_runtime: string
  chat_acp_agent_id: string
  chat_acp_project_path: string
  chat_acp_project_mode: string
}

const props = defineProps<{
  form: InteractionSettingsForm
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  acpProfiles: AcpprofilePublicProfile[]
  botMetadata?: Record<string, unknown>
}>()

const { t } = useI18n()

const selectableACPProfiles = computed(() =>
  props.acpProfiles.filter((profile) => {
    if (!isACPAgentEnabled(props.botMetadata, profile.id)) return false
    const config = readACPAgentConfig(props.botMetadata, profile.id)
    return !config.setupModeSet || !findMissingRequiredManagedField(profile, config.managed, config.setupMode)
  }),
)

const externalAgentEnabled = computed(() => props.form.chat_runtime === 'acp_agent')
const normalizedDefaultAgentID = computed(() => normalizeACPAgentID(props.form.chat_acp_agent_id))
const selectedACPProfile = computed(() =>
  selectableACPProfiles.value.find(profile => normalizeACPAgentID(profile.id) === normalizedDefaultAgentID.value),
)
const selectedACPUnavailable = computed(() =>
  externalAgentEnabled.value && !!normalizedDefaultAgentID.value && !selectedACPProfile.value,
)
const externalAgentDescription = computed(() =>
  selectedACPUnavailable.value
    ? t('bots.settings.externalAgentUnavailableSaved')
    : t('bots.settings.externalAgentDescription'),
)
const externalAgentRuntimeDescription = computed(() => {
  return selectableACPProfiles.value.length > 0
    ? t('bots.settings.externalAgentRuntimeDescription')
    : t('bots.settings.externalAgentRuntimeUnavailable')
})

function ensureDefaultACPProject() {
  // eslint-disable-next-line vue/no-mutating-props
  props.form.chat_acp_project_path = props.form.chat_acp_project_path || ACP_DEFAULT_PROJECT_PATH
  // eslint-disable-next-line vue/no-mutating-props
  props.form.chat_acp_project_mode = props.form.chat_acp_project_mode || ACP_DEFAULT_PROJECT_MODE
}

function setDefaultACPAgent(agentID: string) {
  // eslint-disable-next-line vue/no-mutating-props
  props.form.chat_acp_agent_id = normalizeACPAgentID(agentID)
  ensureDefaultACPProject()
}

function setExternalAgentRuntime(enabled: boolean) {
  if (!enabled) {
    // eslint-disable-next-line vue/no-mutating-props
    props.form.chat_runtime = 'model'
    return
  }
  const first = selectableACPProfiles.value[0]
  if (!first) return
  // eslint-disable-next-line vue/no-mutating-props
  props.form.chat_runtime = 'acp_agent'
  if (!selectedACPProfile.value) {
    setDefaultACPAgent(first.id)
  } else {
    ensureDefaultACPProject()
  }
}

watch(selectableACPProfiles, (profiles) => {
  if (!externalAgentEnabled.value || profiles.length === 0 || normalizedDefaultAgentID.value || selectedACPProfile.value) return
  setDefaultACPAgent(profiles[0]!.id)
}, { immediate: true })

const chatModelConfig = computed(() => {
  if (!props.form.chat_model_id) return undefined
  return props.models.find((m) => m.id === props.form.chat_model_id)?.config
})

const chatModelClientType = computed(() => {
  if (!props.form.chat_model_id) return undefined
  const model = props.models.find((m) => m.id === props.form.chat_model_id)
  return props.providers.find((p) => p.id === model?.provider_id)?.client_type
})

const thinkingMode = computed(() => resolveThinkingMode(chatModelConfig.value))

const chatModelSupportsReasoning = computed(() => thinkingMode.value !== 'none')

const effortLevels = computed(() => resolveEffortLevels(chatModelConfig.value, chatModelClientType.value))

const availableReasoningEfforts = computed(() =>
  availableEffortsForMode(thinkingMode.value, effortLevels.value),
)

watch([effortLevels, thinkingMode], ([levels]) => {
  const current = props.form.reasoning_effort
  if (props.form.reasoning_enabled && (!current || !levels.includes(current))) {
    // eslint-disable-next-line vue/no-mutating-props
    props.form.reasoning_effort = levels.includes('medium') ? 'medium' : levels[0] ?? 'medium'
  }
}, { immediate: true })

const reasoningFormValue = computed({
  get: () => (props.form.reasoning_enabled ? (props.form.reasoning_effort ?? 'medium') : REASONING_EFFORT_DISABLE),
  set: (v: string) => {
    if (v === REASONING_EFFORT_DISABLE) {
      // eslint-disable-next-line vue/no-mutating-props
      props.form.reasoning_enabled = false
    } else {
      // eslint-disable-next-line vue/no-mutating-props
      props.form.reasoning_enabled = true
      // eslint-disable-next-line vue/no-mutating-props
      props.form.reasoning_effort = v
    }
  },
})
</script>
