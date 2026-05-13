<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground">
          {{ $t('bots.toolApproval.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.toolApproval.intro') }}
        </p>
      </div>

      <div class="flex items-center gap-3 shrink-0">
        <Transition name="fade">
          <div
            v-if="hasChanges"
            class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-muted/40 border border-border/50"
          >
            <div class="size-1 rounded-full bg-muted-foreground/40" />
            <span class="text-[10px] text-muted-foreground font-medium whitespace-nowrap">Unsaved</span>
          </div>
        </Transition>

        <Button
          size="sm"
          class="h-8 text-xs font-medium px-4 shadow-none min-w-24"
          :variant="hasChanges ? 'default' : 'secondary'"
          :disabled="!hasChanges || saveLoading"
          @click="handleSave"
        >
          <Spinner
            v-if="saveLoading"
            class="mr-2 size-4"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </header>

    <!-- Metrics Section (Overview Style) -->
    <div class="space-y-4 rounded-md border p-4">
      <!-- Section Header -->
      <div class="space-y-1">
        <h4 class="text-xs font-medium">
          {{ $t('bots.toolApproval.posture.title') }}
        </h4>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.toolApproval.posture.description') }}
        </p>
      </div>

      <!-- Metrics Grid -->
      <div class="grid gap-3 sm:grid-cols-3">
        <!-- Status Card -->
        <div
          class="rounded-md border bg-background/70 p-3 flex flex-col justify-between transition-all duration-200"
          :class="{ 'border-success/20 bg-success/5': form.tool_approval_config.enabled }"
        >
          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <p class="text-xs text-muted-foreground">
                {{ $t('common.status') }}
              </p>
              <Switch
                :model-value="form.tool_approval_config.enabled"
                class="scale-90"
                @update:model-value="(val) => form.tool_approval_config.enabled = !!val"
              />
            </div>
            <div class="flex items-center gap-2 mt-2">
              <ShieldCheck
                v-if="form.tool_approval_config.enabled"
                class="w-5 h-5 shrink-0 text-success"
              />
              <ShieldOff
                v-else
                class="w-5 h-5 shrink-0 text-muted-foreground"
              />
              <p class="text-2xl font-semibold">
                {{ form.tool_approval_config.enabled ? $t('bots.toolApproval.posture.hardened') : $t('common.inactive') }}
              </p>
            </div>
          </div>
          <div class="mt-3 min-h-[32px] flex items-center">
            <p class="text-[11px] leading-4 text-muted-foreground">
              {{ form.tool_approval_config.enabled ? $t('bots.toolApproval.posture.description') : $t('bots.toolApproval.warnings.disabled') }}
            </p>
          </div>
        </div>

        <!-- Active Tools Card -->
        <div class="rounded-md border bg-background/70 p-3 flex flex-col justify-between">
          <div class="space-y-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.toolApproval.metrics.activeRules') }}
            </p>
            <div class="mt-2">
              <span class="text-2xl font-semibold">{{ activeToolsCount }}</span>
              <span class="text-xs font-medium text-muted-foreground ml-1">/ {{ approvalTools.length }}</span>
            </div>
          </div>
          <div class="mt-3 min-h-[32px] flex items-center">
            <p class="text-[11px] leading-4 text-muted-foreground">
              {{ $t('bots.toolApproval.metrics.totalDefined') }}
            </p>
          </div>
        </div>

        <!-- Rules Count Card -->
        <div class="rounded-md border bg-background/70 p-3 flex flex-col justify-between">
          <div class="space-y-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.toolApproval.metrics.totalDefined') }}
            </p>
            <div class="mt-2">
              <span class="text-2xl font-semibold">{{ totalRulesCount }}</span>
            </div>
          </div>
          <div class="mt-3 min-h-[32px] flex items-center">
            <p class="text-[11px] leading-4 text-muted-foreground">
              {{ $t('bots.toolApproval.metrics.blockedCount') }} & Bypasses
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Tool Configurations -->
    <div
      class="space-y-4 transition-all duration-300"
      :class="{ 'opacity-50 grayscale pointer-events-none': !form.tool_approval_config.enabled }"
    >
      <div
        v-for="tool in approvalTools"
        :key="tool"
        class="rounded-md border border-border/60 bg-background overflow-hidden"
      >
        <div class="bg-muted/10 p-3 border-b border-border/40 flex items-center justify-between">
          <div class="flex items-center gap-3">
            <div
              class="size-8 rounded flex items-center justify-center bg-background border border-border/50"
              :class="{ 'text-success border-success/20 bg-success/5': toolApprovalPolicy(tool).require_approval }"
            >
              <component
                :is="TOOL_META[tool].icon"
                class="size-4"
              />
            </div>
            <div class="space-y-0.5">
              <div class="flex items-center gap-2">
                <span class="font-mono text-[11px] font-semibold text-foreground">
                  {{ tool }}
                </span>
                <Badge
                  v-if="toolApprovalPolicy(tool).require_approval"
                  variant="outline"
                  class="h-4 px-1.5 text-[9px] font-medium bg-success/10 text-success border-success/20 rounded-full"
                >
                  {{ $t('common.active') }}
                </Badge>
              </div>
              <p class="text-[11px] text-muted-foreground">
                {{ $t(TOOL_META[tool].descKey) }}
              </p>
            </div>
          </div>
          <Switch
            :model-value="toolApprovalPolicy(tool).require_approval"
            class="scale-90"
            @update:model-value="(val) => toolApprovalPolicy(tool).require_approval = !!val"
          />
        </div>

        <div class="p-3 space-y-4">
          <div
            v-if="!toolApprovalPolicy(tool).require_approval"
            class="flex items-center gap-2 rounded border border-warning/10 bg-warning/5 px-3 py-1.5 text-[11px] text-warning"
          >
            <ShieldAlert class="size-3.5" />
            {{ $t('bots.toolApproval.toolDisabledHint') }}
          </div>

          <div class="grid gap-4 md:grid-cols-2 items-stretch">
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <Label class="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                  <ShieldCheck class="size-3.5 text-success" />
                  {{ $t('bots.toolApproval.bypass') }}
                </Label>
                <span class="text-[10px] font-mono text-muted-foreground tabular-nums">
                  {{ bypassList(tool).length }}
                </span>
              </div>
              <Textarea
                :model-value="approvalBypassText(tool)"
                :placeholder="bypassPlaceholder(tool)"
                class="min-h-24 flex-1 resize-none font-mono text-[11px] bg-muted/20 border-border/50 rounded-md focus-visible:ring-success/30 shadow-none"
                @update:model-value="(val) => updateApprovalBypass(tool, String(val))"
              />
            </div>

            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <Label class="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                  <ShieldAlert class="size-3.5 text-warning" />
                  {{ $t('bots.toolApproval.mustReview') }}
                </Label>
                <span class="text-[10px] font-mono text-muted-foreground tabular-nums">
                  {{ forceReviewList(tool).length }}
                </span>
              </div>
              <Textarea
                :model-value="approvalForceReviewText(tool)"
                :placeholder="forceReviewPlaceholder(tool)"
                class="min-h-24 flex-1 resize-none font-mono text-[11px] bg-muted/20 border-border/50 rounded-md focus-visible:ring-warning/30 shadow-none"
                @update:model-value="(val) => updateApprovalForceReview(tool, String(val))"
              />
            </div>
          </div>
        </div>
      </div>
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
  Badge,
} from '@memohai/ui'
import {
  FilePlus2,
  FilePen,
  SquareTerminal,
  ShieldCheck,
  ShieldAlert,
  ShieldOff,
} from 'lucide-vue-next'
import { reactive, computed, watch } from 'vue'
import type { Component, Ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

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


