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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Switch } from '@memohai/ui'
import ModelSelect from './model-select.vue'
import { EFFORT_LABELS, REASONING_EFFORT_DISABLE, availableEffortsForMode, resolveEffortLevels, resolveThinkingMode } from './reasoning-effort'
import type { SettingsSettings, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'

const props = defineProps<{
  form: SettingsSettings
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
}>()

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
