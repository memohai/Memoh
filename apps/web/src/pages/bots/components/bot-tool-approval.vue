<template>
  <PageShell
    variant="tab"
    :title="$t('bots.toolApproval.title')"
    :description="$t('bots.toolApproval.intro')"
  >
    <template #actions>
      <Button
        :disabled="!hasChanges"
        :loading="saveLoading"
        @click="handleSave"
      >
        {{ $t('common.saveChanges') }}
      </Button>
    </template>

    <div class="space-y-8">
      <!-- Status card: the page's center of gravity. The toggle controls whether any
           prompt ever fires; the body tells the user what that means right now. -->
      <SettingsSection>
        <SettingsRow :label="form.tool_approval_config.enabled ? $t('bots.toolApproval.status.on') : $t('bots.toolApproval.status.off')">
          <Switch
            :model-value="form.tool_approval_config.enabled"
            @update:model-value="(val) => form.tool_approval_config.enabled = !!val"
          />
        </SettingsRow>
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

        <ExpandableSettingsRow
          v-for="tool in approvalTools"
          :key="tool"
          :label="$t(`bots.toolApproval.toolNames.${tool}`)"
          :open="expandedTool === tool"
          @update:open="(val) => setExpanded(tool, val)"
        >
          <template #trailing>
            <span class="hidden text-xs text-muted-foreground sm:inline">
              {{ toolApprovalPolicy(tool).require_approval ? $t('bots.toolApproval.behavior.review') : $t('bots.toolApproval.behavior.auto') }}
            </span>
          </template>

          <template #expanded>
            <FormStack>
              <FieldStack :label="$t('bots.toolApproval.behavior.label')">
                <SegmentedControl
                  :model-value="toolApprovalPolicy(tool).require_approval ? 'review' : 'auto'"
                  :items="behaviorItems"
                  :aria-label="$t('bots.toolApproval.behavior.label')"
                  class="w-full sm:w-fit"
                  @update:model-value="(val) => toolApprovalPolicy(tool).require_approval = val === 'review'"
                />
              </FieldStack>

              <!-- Auto-approve exceptions only make sense when the default is to review.
                   An auto-by-default tool would treat them as redundant, so they're hidden. -->
              <FieldStack
                v-if="toolApprovalPolicy(tool).require_approval"
                :label="tool === 'exec' ? $t('bots.toolApproval.autoApprove.commandsTitle') : $t('bots.toolApproval.autoApprove.pathsTitle')"
              >
                <Textarea
                  :model-value="bypassText(tool)"
                  :placeholder="tool === 'exec' ? $t('bots.toolApproval.placeholders.execCommands') : $t('bots.toolApproval.placeholders.filePaths')"
                  class="min-h-24 resize-none font-mono text-xs"
                  @update:model-value="(val) => updateBypass(tool, String(val))"
                />
              </FieldStack>

              <FieldStack :label="tool === 'exec' ? $t('bots.toolApproval.review.commandsTitle') : $t('bots.toolApproval.review.pathsTitle')">
                <Textarea
                  :model-value="forceReviewText(tool)"
                  :placeholder="tool === 'exec' ? $t('bots.toolApproval.placeholders.execReview') : $t('bots.toolApproval.placeholders.fileReview')"
                  class="min-h-24 resize-none font-mono text-xs"
                  @update:model-value="(val) => updateForceReview(tool, String(val))"
                />
              </FieldStack>
            </FormStack>
          </template>
        </ExpandableSettingsRow>
      </SettingsSection>
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import {
  Textarea,
  Button,
  Switch,
  SegmentedControl,
} from '@memohai/ui'
import { RotateCcw } from 'lucide-vue-next'
import { reactive, computed, watch, ref } from 'vue'
import type { Ref } from 'vue'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import ExpandableSettingsRow from '@/components/settings/expandable-row.vue'
import FieldStack from '@/components/settings/field-stack.vue'
import FormStack from '@/components/settings/form-stack.vue'
import PageShell from '@/components/page-shell/index.vue'
import {
  defaultToolApprovalConfig,
  normalizeToolApprovalConfig,
  type ApprovalTool,
  type ToolApprovalConfig,
  type ToolApprovalExecPolicy,
  type ToolApprovalFilePolicy,
} from './tool-approval-config'

const props = defineProps<{
  botId: string
}>()

const approvalTools: ApprovalTool[] = ['read', 'write', 'exec']

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

// At most one editor open at a time: opening a row collapses any other. Each
// row is a controlled ExpandableSettingsRow driven off this single ref.
function setExpanded(tool: ApprovalTool, open: boolean) {
  expandedTool.value = open ? tool : (expandedTool.value === tool ? null : expandedTool.value)
}

const behaviorItems = computed(() => [
  { value: 'review', label: t('bots.toolApproval.behavior.review') },
  { value: 'auto', label: t('bots.toolApproval.behavior.auto') },
])

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

function resetToRecommended() {
  const defaults = defaultToolApprovalConfig()
  // Keep the enable state — it's the user's top-level decision, not a rule.
  form.tool_approval_config.read = defaults.read
  form.tool_approval_config.write = defaults.write
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
