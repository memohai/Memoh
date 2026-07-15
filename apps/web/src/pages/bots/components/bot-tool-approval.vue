<template>
  <PageShell
    variant="tab"
    :title="t('bots.toolApproval.title')"
    :description="t('bots.toolApproval.intro')"
  >
    <SettingsSection v-if="initialLoading">
      <InlineLoadingRow surface="card-row">
        {{ t('bots.toolApproval.loading') }}
      </InlineLoadingRow>
    </SettingsSection>

    <SettingsSection v-else-if="loadFailed">
      <SettingsRow
        :label="t('bots.toolApproval.loadFailed')"
        :description="t('bots.toolApproval.loadFailedDescription')"
      >
        <Button
          variant="outline"
          size="sm"
          @click="refetchWorkspaceTargets()"
        >
          {{ t('runtimes.retry') }}
        </Button>
      </SettingsRow>
    </SettingsSection>

    <div
      v-else
      class="space-y-8"
    >
      <SettingsSection
        v-for="target in validTargets"
        :key="target.target_id"
        :title="targetName(target)"
      >
        <SettingsRow
          v-for="tool in approvalTools"
          :key="target.target_id + ':' + tool"
          stack="sm"
          :label="t('bots.toolApproval.toolNames.' + tool)"
          :description="t('bots.toolApproval.tools.' + tool)"
        >
          <SegmentedControl
            :model-value="modeFor(target, tool)"
            :items="modeItems"
            :aria-label="t('bots.toolApproval.toolNames.' + tool)"
            class="w-full sm:w-fit"
            @update:model-value="(value) => updateMode(target, tool, value)"
          />
        </SettingsRow>
      </SettingsSection>
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery } from '@pinia/colada'
import {
  getBotsByBotIdWorkspaceTargets,
  putBotsByBotIdWorkspaceTargetsByTargetIdToolApproval,
  type WorkspaceUpdateWorkspaceTargetToolApprovalRequest,
  type WorkspaceWorkspaceTarget,
} from '@memohai/sdk'
import {
  Button,
  SegmentedControl,
  toast,
} from '@felinic/ui'
import PageShell from '@/components/page-shell/index.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import InlineLoadingRow from '@/components/inline-loading-row/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import type {
  ApprovalTool,
  ToolApprovalMode,
} from './tool-approval-config'

const props = defineProps<{
  botId: string
}>()

type ValidWorkspaceTarget = WorkspaceWorkspaceTarget & {
  target_id: string
  kind: string
}

const approvalTools: ApprovalTool[] = ['read', 'write', 'exec']
const { t } = useI18n()

const {
  data: workspaceTargetsResponse,
  error: workspaceTargetsError,
  isLoading: workspaceTargetsLoading,
  refetch: refetchWorkspaceTargets,
} = useQuery({
  key: () => ['bot-workspace-targets', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdWorkspaceTargets({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
})

const targetItems = ref<WorkspaceWorkspaceTarget[]>([])
const savingTool = ref<ApprovalTool | null>(null)

watch(workspaceTargetsResponse, (response) => {
  if (!response) return
  targetItems.value = (response.targets ?? []).map(target => ({
    ...target,
    tool_approval: target.tool_approval ? { ...target.tool_approval } : undefined,
  }))
}, { immediate: true })

const validTargets = computed<ValidWorkspaceTarget[]>(() => (
  targetItems.value.filter((target): target is ValidWorkspaceTarget => (
    typeof target.target_id === 'string'
    && target.target_id.length > 0
    && typeof target.kind === 'string'
    && target.kind.length > 0
  ))
))
const initialLoading = computed(() => workspaceTargetsLoading.value && !workspaceTargetsResponse.value)
const loadFailed = computed(() => !!workspaceTargetsError.value && !workspaceTargetsResponse.value)
const modeItems = computed(() => [
  {
    value: 'allow' as const,
    label: t('bots.toolApproval.modes.allow'),
    disabled: !!savingTool.value,
  },
  {
    value: 'ask' as const,
    label: t('bots.toolApproval.modes.ask'),
    disabled: !!savingTool.value,
  },
  {
    value: 'deny' as const,
    label: t('bots.toolApproval.modes.deny'),
    disabled: !!savingTool.value,
  },
])

const { mutateAsync: updateToolApproval } = useMutation({
  mutation: async (input: {
    targetId: string
    body: WorkspaceUpdateWorkspaceTargetToolApprovalRequest
  }) => {
    await putBotsByBotIdWorkspaceTargetsByTargetIdToolApproval({
      path: {
        bot_id: props.botId,
        target_id: input.targetId,
      },
      body: input.body,
      throwOnError: true,
    })
  },
})

function targetName(target: WorkspaceWorkspaceTarget): string {
  if (target.kind === 'native') return t('bots.remoteRuntime.nativeWorkspace')
  return target.name || t('bots.remoteRuntime.unknownComputer')
}

function isMode(value: unknown): value is ToolApprovalMode {
  return value === 'allow' || value === 'ask' || value === 'deny'
}

function defaultMode(target: ValidWorkspaceTarget, tool: ApprovalTool): ToolApprovalMode {
  if (tool === 'read') return 'allow'
  if (tool === 'write') return 'ask'
  return target.kind === 'remote' ? 'ask' : 'allow'
}

function modeFor(target: ValidWorkspaceTarget, tool: ApprovalTool): ToolApprovalMode {
  const value = target.tool_approval?.[tool]
  return isMode(value) ? value : defaultMode(target, tool)
}

async function updateMode(
  target: ValidWorkspaceTarget,
  tool: ApprovalTool,
  value: string | number,
): Promise<void> {
  if (!isMode(value) || savingTool.value) return
  if (modeFor(target, tool) === value) return

  const previous = modeFor(target, tool)
  target.tool_approval = {
    ...target.tool_approval,
    [tool]: value,
  }
  savingTool.value = tool
  try {
    await updateToolApproval({
      targetId: target.target_id,
      body: {
        read: modeFor(target, 'read'),
        write: modeFor(target, 'write'),
        exec: modeFor(target, 'exec'),
      },
    })
    void refetchWorkspaceTargets()
  } catch (error) {
    target.tool_approval = {
      ...target.tool_approval,
      [tool]: previous,
    }
    toast.error(resolveApiErrorMessage(error, t('bots.toolApproval.saveFailed')))
  } finally {
    savingTool.value = null
  }
}

</script>
