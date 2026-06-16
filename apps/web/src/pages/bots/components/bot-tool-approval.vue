<template>
  <PageShell
    variant="tab"
    :title="$t('bots.toolApproval.title')"
    :description="$t('bots.toolApproval.intro')"
  >
    <template #actions>
      <Button
        :disabled="!hasChanges || saveLoading"
        @click="handleSave"
      >
        <Spinner
          v-if="saveLoading"
          class="size-3"
        />
        {{ $t('common.saveChanges') }}
      </Button>
    </template>

    <div class="space-y-8">
      <!-- Status card: the page's center of gravity. The toggle controls whether any
           prompt ever fires; the body and the summary line below it tell the user what
           that means right now, so the rules table doesn't have to re-explain it. -->
      <SettingsSection>
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
          <div class="min-w-0 text-sm font-medium text-foreground">
            {{ form.tool_approval_config.enabled ? $t('bots.toolApproval.status.on') : $t('bots.toolApproval.status.off') }}
          </div>
          <Switch
            class="shrink-0"
            :model-value="form.tool_approval_config.enabled"
            @update:model-value="(val) => form.tool_approval_config.enabled = !!val"
          />
        </div>
      </SettingsSection>

      <!-- Tool rules: a summary row per tool, with at most one inline editor open at a
           time. The summary shows default behavior + exception counts so the user reads
           the whole posture without expanding anything. -->
      <SettingsSection :title="$t('bots.toolApproval.rules.title')">
        <template #actions>
          <Button
            variant="outline"
            size="sm"
            @click="resetToRecommended"
          >
            <RotateCcw class="size-4" />
            {{ $t('bots.toolApproval.rules.reset') }}
          </Button>
        </template>

        <template
          v-for="tool in approvalTools"
          :key="tool"
        >
          <div class="mx-4 flex min-h-[3.75rem] items-center gap-4 border-b border-border py-3 last:border-b-0">
            <div class="min-w-0 flex-1">
              <div class="text-sm font-medium text-foreground">
                {{ $t(`bots.toolApproval.toolNames.${tool}`) }}
              </div>
            </div>

            <span class="hidden shrink-0 text-xs text-muted-foreground sm:inline">
              {{ toolApprovalPolicy(tool).require_approval ? $t('bots.toolApproval.behavior.review') : $t('bots.toolApproval.behavior.auto') }}
            </span>

            <Button
              variant="outline"
              size="sm"
              class="shrink-0"
              @click="toggleExpanded(tool)"
            >
              <ChevronRight
                class="size-4 transition-transform"
                :class="expandedTool === tool ? 'rotate-90' : ''"
              />
              {{ expandedTool === tool ? $t('common.collapse') : $t('common.edit') }}
            </Button>
          </div>

          <div
            v-if="expandedTool === tool"
            class="mx-4 space-y-4 border-b border-border py-4 last:border-b-0"
          >
            <div class="space-y-1.5">
              <Label class="text-xs font-medium text-muted-foreground">
                {{ $t('bots.toolApproval.behavior.label') }}
              </Label>
              <SegmentedControl
                :model-value="toolApprovalPolicy(tool).require_approval ? 'review' : 'auto'"
                :items="behaviorItems"
                :aria-label="$t('bots.toolApproval.behavior.label')"
                class="w-full sm:w-fit"
                @update:model-value="(val) => toolApprovalPolicy(tool).require_approval = val === 'review'"
              />
            </div>

            <!-- Auto-approve exceptions only make sense when the default is to review.
                 An auto-by-default tool would treat them as redundant, so they're hidden. -->
            <div
              v-if="toolApprovalPolicy(tool).require_approval"
              class="space-y-1.5"
            >
              <Label class="text-xs font-medium text-muted-foreground">
                {{ tool === 'exec' ? $t('bots.toolApproval.autoApprove.commandsTitle') : $t('bots.toolApproval.autoApprove.pathsTitle') }}
              </Label>
              <Textarea
                :model-value="bypassText(tool)"
                :placeholder="tool === 'exec' ? $t('bots.toolApproval.placeholders.execCommands') : $t('bots.toolApproval.placeholders.filePaths')"
                class="min-h-24 resize-none font-mono text-xs"
                @update:model-value="(val) => updateBypass(tool, String(val))"
              />
            </div>

            <div class="space-y-1.5">
              <Label class="text-xs font-medium text-muted-foreground">
                {{ tool === 'exec' ? $t('bots.toolApproval.review.commandsTitle') : $t('bots.toolApproval.review.pathsTitle') }}
              </Label>
              <Textarea
                :model-value="forceReviewText(tool)"
                :placeholder="tool === 'exec' ? $t('bots.toolApproval.placeholders.execReview') : $t('bots.toolApproval.placeholders.fileReview')"
                class="min-h-24 resize-none font-mono text-xs"
                @update:model-value="(val) => updateForceReview(tool, String(val))"
              />
            </div>
          </div>
        </template>
      </SettingsSection>
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import {
  Label,
  Textarea,
  Button,
  Spinner,
  Switch,
  SegmentedControl,
} from '@memohai/ui'
import { ChevronRight, RotateCcw } from 'lucide-vue-next'
import { reactive, computed, watch, ref } from 'vue'
import type { Ref } from 'vue'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import SettingsSection from '@/components/settings/section.vue'
import PageShell from '@/components/page-shell/index.vue'

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

const expandedTool = ref<ApprovalTool | null>(null)

const behaviorItems = computed(() => [
  { value: 'review', label: t('bots.toolApproval.behavior.review') },
  { value: 'auto', label: t('bots.toolApproval.behavior.auto') },
])

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

function bypassText(tool: ApprovalTool): string {
  return bypassList(tool).join('\n')
}

function forceReviewText(tool: ApprovalTool): string {
  return forceReviewList(tool).join('\n')
}

function parseList(raw: string): string[] {
  return raw.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean)
}

function updateBypass(tool: ApprovalTool, raw: string) {
  const values = parseList(raw)
  if (tool === 'exec') {
    form.tool_approval_config.exec.bypass_commands = values
  } else {
    form.tool_approval_config[tool].bypass_globs = values
  }
}

function updateForceReview(tool: ApprovalTool, raw: string) {
  const values = parseList(raw)
  if (tool === 'exec') {
    form.tool_approval_config.exec.force_review_commands = values
  } else {
    form.tool_approval_config[tool].force_review_globs = values
  }
}

function toggleExpanded(tool: ApprovalTool) {
  expandedTool.value = expandedTool.value === tool ? null : tool
}

function resetToRecommended() {
  const defaults = defaultToolApprovalConfig()
  // Keep the enable state — it's the user's top-level decision, not a rule.
  form.tool_approval_config.write = defaults.write
  form.tool_approval_config.edit = defaults.edit
  form.tool_approval_config.exec = defaults.exec
}

watch(settings, (val) => {
  if (val) {
    form.tool_approval_config = normalizeToolApprovalConfig(
      (val as SettingsSettings & { tool_approval_config?: unknown }).tool_approval_config,
    )
  }
}, { immediate: true })

function serverConfig(): ToolApprovalConfig {
  return normalizeToolApprovalConfig(
    (settings.value as SettingsSettings & { tool_approval_config?: unknown } | undefined)?.tool_approval_config,
  )
}

const hasChanges = computed(() => {
  if (!settings.value) return false
  return JSON.stringify(form.tool_approval_config) !== JSON.stringify(serverConfig())
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
