<template>
  <div class="mx-auto max-w-3xl pt-6 pb-8">
    <div class="mb-6 flex items-start justify-between gap-4 px-2">
      <div class="min-w-0">
        <h1 class="text-lg font-semibold text-foreground">
          {{ $t('bots.toolApproval.title') }}
        </h1>
        <p class="mt-1 max-w-2xl text-xs text-muted-foreground">
          {{ $t('bots.toolApproval.intro') }}
        </p>
      </div>

      <div class="flex shrink-0 items-center gap-2">
        <Transition name="fade">
          <span
            v-if="hasChanges"
            class="text-xs text-muted-foreground"
          >
            {{ $t('common.unsaved') }}
          </span>
        </Transition>

        <Button
          size="sm"
          class="min-w-24"
          :disabled="!hasChanges || saveLoading"
          @click="handleSave"
        >
          <Spinner
            v-if="saveLoading"
            class="size-3"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </div>

    <div class="space-y-8">
      <SettingsSection :title="$t('bots.toolApproval.posture.title')">
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.settings.toolApproval') }}
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              {{ form.tool_approval_config.enabled ? $t('bots.toolApproval.posture.description') : $t('bots.toolApproval.warnings.disabled') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-3">
            <span class="hidden text-xs text-muted-foreground sm:inline">
              {{ form.tool_approval_config.enabled ? $t('common.enabled') : $t('common.disabled') }}
            </span>
            <Switch
              :model-value="form.tool_approval_config.enabled"
              @update:model-value="(val) => form.tool_approval_config.enabled = !!val"
            />
          </div>
        </div>
      </SettingsSection>

      <!-- The stats are telemetry, not nested settings rows, so the tile grid is the content. -->
      <section class="space-y-2.5">
        <h2 class="px-2 text-[13px] font-medium text-muted-foreground">
          {{ $t('bots.toolApproval.metrics.totalDefined') }}
        </h2>
        <div class="grid grid-cols-1 gap-px overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-border sm:grid-cols-3">
          <div
            v-for="stat in postureStats"
            :key="stat.key"
            class="bg-card px-4 py-3.5"
          >
            <p class="text-xs text-muted-foreground">
              {{ stat.label }}
            </p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-foreground">
              {{ stat.value }}
            </p>
            <p
              v-if="stat.detail"
              class="mt-0.5 text-xs text-muted-foreground"
            >
              {{ stat.detail }}
            </p>
          </div>
        </div>
      </section>

      <SettingsSection :title="$t('bots.toolApproval.rulesTitle')">
        <div
          v-if="!form.tool_approval_config.enabled"
          class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3 text-xs text-muted-foreground"
        >
          <ShieldAlert class="size-4 shrink-0" />
          {{ $t('bots.toolApproval.toolDisabledHint') }}
        </div>

        <div
          v-for="tool in approvalTools"
          :key="tool"
          class="mx-4 border-b border-border py-4 last:border-b-0"
        >
          <div class="flex min-h-[3.75rem] items-center justify-between gap-4">
            <div class="flex min-w-0 items-center gap-3">
              <span class="flex size-8 shrink-0 items-center justify-center">
                <component
                  :is="TOOL_META[tool].icon"
                  class="size-4 text-muted-foreground"
                />
              </span>
              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="truncate font-mono text-sm font-medium text-foreground">
                    {{ tool }}
                  </span>
                </div>
                <p class="mt-0.5 text-xs text-muted-foreground">
                  {{ $t(TOOL_META[tool].descKey) }}
                </p>
              </div>
            </div>

            <div class="flex shrink-0 items-center gap-3">
              <span class="hidden text-xs text-muted-foreground sm:inline">
                {{ toolApprovalPolicy(tool).require_approval ? $t('bots.settings.toolApprovalMustReview') : $t('bots.settings.toolApprovalBypass') }}
              </span>
              <Switch
                :model-value="toolApprovalPolicy(tool).require_approval"
                @update:model-value="(val) => toolApprovalPolicy(tool).require_approval = !!val"
              />
            </div>
          </div>

          <div class="mt-4 grid gap-4 md:grid-cols-2">
            <div class="flex min-w-0 flex-col gap-2">
              <div class="flex items-center justify-between gap-3">
                <Label class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <ShieldCheck class="size-3.5" />
                  {{ $t('bots.toolApproval.bypass') }}
                </Label>
                <span class="font-mono text-xs tabular-nums text-muted-foreground">
                  {{ bypassList(tool).length }}
                </span>
              </div>
              <Textarea
                :model-value="approvalBypassText(tool)"
                :placeholder="bypassPlaceholder(tool)"
                class="min-h-28 flex-1 resize-none font-mono text-xs"
                @update:model-value="(val) => updateApprovalBypass(tool, String(val))"
              />
              <p class="text-xs text-muted-foreground">
                {{ $t('bots.toolApproval.bypassHint') }}
              </p>
            </div>

            <div class="flex min-w-0 flex-col gap-2">
              <div class="flex items-center justify-between gap-3">
                <Label class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <ShieldAlert class="size-3.5" />
                  {{ $t('bots.toolApproval.mustReview') }}
                </Label>
                <span class="font-mono text-xs tabular-nums text-muted-foreground">
                  {{ forceReviewList(tool).length }}
                </span>
              </div>
              <Textarea
                :model-value="approvalForceReviewText(tool)"
                :placeholder="forceReviewPlaceholder(tool)"
                class="min-h-28 flex-1 resize-none font-mono text-xs"
                @update:model-value="(val) => updateApprovalForceReview(tool, String(val))"
              />
              <p class="text-xs text-muted-foreground">
                {{ $t('bots.toolApproval.mustReviewHint') }}
              </p>
            </div>
          </div>
        </div>
      </SettingsSection>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Textarea,
  Button,
  Spinner,
  Switch,
} from '@memohai/ui'
import {
  FilePlus2,
  FilePen,
  SquareTerminal,
  ShieldCheck,
  ShieldAlert,
} from 'lucide-vue-next'
import { reactive, computed, watch } from 'vue'
import type { Component, Ref } from 'vue'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import SettingsSection from '@/components/settings/section.vue'

const props = defineProps<{
  botId: string
}>()

type ApprovalTool = 'write' | 'edit' | 'exec'

interface ToolApprovalFilePolicy {
  require_approval: boolean
  bypass_globs: string[]
  force_review_globs: string[]
}

interface ToolApprovalExecPolicy {
  require_approval: boolean
  bypass_commands: string[]
  force_review_commands: string[]
}

interface ToolApprovalConfig {
  enabled: boolean
  write: ToolApprovalFilePolicy
  edit: ToolApprovalFilePolicy
  exec: ToolApprovalExecPolicy
}

const defaultToolApprovalConfig = (): ToolApprovalConfig => ({
  enabled: false,
  write: {
    require_approval: true,
    bypass_globs: ['/data/**', '/tmp/**'],
    force_review_globs: [],
  },
  edit: {
    require_approval: true,
    bypass_globs: ['/data/**', '/tmp/**'],
    force_review_globs: [],
  },
  exec: {
    require_approval: false,
    bypass_commands: [],
    force_review_commands: [],
  },
})

const approvalTools: ApprovalTool[] = ['write', 'edit', 'exec']

const TOOL_META: Record<ApprovalTool, { icon: Component; descKey: string }> = {
  write: { icon: FilePlus2, descKey: 'bots.toolApproval.tools.write' },
  edit: { icon: FilePen, descKey: 'bots.toolApproval.tools.edit' },
  exec: { icon: SquareTerminal, descKey: 'bots.toolApproval.tools.exec' },
}

const { t } = useI18n()

const botIdRef = computed(() => props.botId) as Ref<string>

const queryCache = useQueryCache()

const { data: settings } = useQuery({
  key: () => ['bot-settings', botIdRef.value],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({ path: { bot_id: botIdRef.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botIdRef.value,
})

const { mutateAsync: updateSettings, isLoading: saveLoading } = useMutation({
  mutation: async (body: Partial<SettingsSettings> & { tool_approval_config?: ToolApprovalConfig }) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: botIdRef.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-settings', botIdRef.value] }),
})

const form = reactive<{ tool_approval_config: ToolApprovalConfig }>({
  tool_approval_config: defaultToolApprovalConfig(),
})

function normalizeToolApprovalConfig(raw: unknown): ToolApprovalConfig {
  const defaults = defaultToolApprovalConfig()
  if (!raw || typeof raw !== 'object') return defaults
  const value = raw as Partial<ToolApprovalConfig>
  return {
    enabled: value.enabled ?? defaults.enabled,
    write: {
      require_approval: value.write?.require_approval ?? defaults.write.require_approval,
      bypass_globs: value.write?.bypass_globs ?? defaults.write.bypass_globs,
      force_review_globs: value.write?.force_review_globs ?? defaults.write.force_review_globs,
    },
    edit: {
      require_approval: value.edit?.require_approval ?? defaults.edit.require_approval,
      bypass_globs: value.edit?.bypass_globs ?? defaults.edit.bypass_globs,
      force_review_globs: value.edit?.force_review_globs ?? defaults.edit.force_review_globs,
    },
    exec: {
      require_approval: value.exec?.require_approval ?? defaults.exec.require_approval,
      bypass_commands: value.exec?.bypass_commands ?? defaults.exec.bypass_commands,
      force_review_commands: value.exec?.force_review_commands ?? defaults.exec.force_review_commands,
    },
  }
}

function toolApprovalPolicy(tool: ApprovalTool) {
  return form.tool_approval_config[tool]
}

function bypassList(tool: ApprovalTool): string[] {
  const policy = toolApprovalPolicy(tool)
  return tool === 'exec'
    ? (policy as ToolApprovalExecPolicy).bypass_commands
    : (policy as ToolApprovalFilePolicy).bypass_globs
}

function forceReviewList(tool: ApprovalTool): string[] {
  const policy = toolApprovalPolicy(tool)
  return tool === 'exec'
    ? (policy as ToolApprovalExecPolicy).force_review_commands
    : (policy as ToolApprovalFilePolicy).force_review_globs
}

function approvalBypassText(tool: ApprovalTool): string {
  return bypassList(tool).join('\n')
}

function approvalForceReviewText(tool: ApprovalTool): string {
  return forceReviewList(tool).join('\n')
}

function updateApprovalBypass(tool: ApprovalTool, raw: string) {
  const values = raw.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean)
  if (tool === 'exec') {
    form.tool_approval_config.exec.bypass_commands = values
  } else {
    form.tool_approval_config[tool].bypass_globs = values
  }
}

function updateApprovalForceReview(tool: ApprovalTool, raw: string) {
  const values = raw.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean)
  if (tool === 'exec') {
    form.tool_approval_config.exec.force_review_commands = values
  } else {
    form.tool_approval_config[tool].force_review_globs = values
  }
}

function bypassPlaceholder(tool: ApprovalTool): string {
  return tool === 'exec'
    ? t('bots.toolApproval.placeholders.execBypass')
    : t('bots.toolApproval.placeholders.fileBypass')
}

function forceReviewPlaceholder(tool: ApprovalTool): string {
  return tool === 'exec'
    ? t('bots.toolApproval.placeholders.execMustReview')
    : t('bots.toolApproval.placeholders.fileMustReview')
}

watch(settings, (val) => {
  if (val) {
    form.tool_approval_config = normalizeToolApprovalConfig(
      (val as SettingsSettings & { tool_approval_config?: unknown }).tool_approval_config,
    )
  }
}, { immediate: true })

const activeToolsCount = computed(() => {
  return approvalTools.filter(t => form.tool_approval_config[t].require_approval).length
})

const totalRulesCount = computed(() => {
  return approvalTools.reduce((acc, t) => {
    return acc + bypassList(t).length + forceReviewList(t).length
  }, 0)
})

const postureStats = computed(() => [
  {
    key: 'status',
    label: t('common.status'),
    value: form.tool_approval_config.enabled ? t('bots.toolApproval.posture.hardened') : t('common.inactive'),
    detail: form.tool_approval_config.enabled ? t('bots.toolApproval.status.active') : t('bots.toolApproval.warnings.disabled'),
  },
  {
    key: 'tools',
    label: t('bots.toolApproval.metrics.activeRules'),
    value: `${activeToolsCount.value} / ${approvalTools.length}`,
    detail: t('bots.toolApproval.activeToolsDetail'),
  },
  {
    key: 'rules',
    label: t('bots.toolApproval.metrics.totalDefined'),
    value: String(totalRulesCount.value),
    detail: t('bots.toolApproval.rulePatternsDetail'),
  },
])

const hasChanges = computed(() => {
  if (!settings.value) return false
  const current = normalizeToolApprovalConfig(
    (settings.value as SettingsSettings & { tool_approval_config?: unknown }).tool_approval_config,
  )
  return JSON.stringify(form.tool_approval_config) !== JSON.stringify(current)
})

async function handleSave() {
  try {
    await updateSettings({ tool_approval_config: form.tool_approval_config })
    toast.success(t('bots.settings.saveSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  }
}
</script>
